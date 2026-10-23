package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const sessionSchemaV5 = 5

// migrateSessionToV5 imports every unversioned v4 current result as an
// immutable migration version, then replaces the large session payload with
// lightweight version pointers. SourceKey makes the operation safe to repeat.
func (a *App) migrateSessionToV5(path string, data []byte) ([]byte, bool, error) {
	if a.animationVersions == nil || len(data) == 0 {
		return data, false, nil
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return data, false, err
	}
	version := intFromAny(root["v"])
	if version >= sessionSchemaV5 {
		return data, false, nil
	}
	if version != 4 {
		return data, false, nil
	}
	backup := filepath.Join(filepath.Dir(path), "session.v4.backup.json")
	if _, err := os.Stat(backup); os.IsNotExist(err) {
		if err := atomicWritePrivate(backup, data); err != nil {
			return data, false, fmt.Errorf("备份 v4 会话失败: %w", err)
		}
	}

	workspaces, _ := root["workspaces"].([]any)
	if len(workspaces) == 0 {
		// Very old single-character sessions are left to the existing frontend
		// compatibility loader; it will save them as v5 afterwards.
		root["v"] = sessionSchemaV5
		encoded, err := json.Marshal(root)
		return encoded, err == nil, err
	}
	changed := false
	targets := make([]legacyMigrationTarget, 0)
	for _, workspaceValue := range workspaces {
		workspace, ok := workspaceValue.(map[string]any)
		if !ok {
			continue
		}
		characterID := strings.TrimSpace(stringFromAny(workspace["characterId"]))
		if characterID == "" {
			if character, ok := workspace["character"].(map[string]any); ok {
				characterID = strings.TrimSpace(stringFromAny(character["libraryId"]))
			}
		}
		if characterID == "" {
			continue
		}
		states, _ := workspace["states"].([]any)
		for _, stateValue := range states {
			state, ok := stateValue.(map[string]any)
			if !ok {
				continue
			}
			stateID := strings.TrimSpace(stringFromAny(state["id"]))
			if stateID == "" {
				continue
			}
			characterSnapshot := map[string]any{}
			if character, ok := workspace["character"].(map[string]any); ok {
				characterSnapshot = cloneAnyMap(character)
			}
			trace := generationTraceFromAny(state["trace"])
			frames := migrationFrames(state["items"])
			target := legacyMigrationTarget{
				CharacterID: characterID, StateID: stateID, StateName: stringFromAny(state["name"]), FrameCount: intFromAny(state["frames"]),
				ArtifactDir: trace.Parameters["artifactDir"], RequestID: trace.Parameters["requestId"], RawVideoPath: stringFromAny(state["rawVideoPath"]),
				Character: characterSnapshot, State: state, CurrentFrameHash: migrationFrameHash(frames),
			}
			if strings.TrimSpace(stringFromAny(state["currentVersionId"])) == "" && len(frames) > 0 {
				sourceKey := migrationSourceKey(characterID, stateID, frames)
				args := CommitAnimationVersionArgs{
					CharacterID:           characterID,
					StateID:               stateID,
					StateName:             stringFromAny(state["name"]),
					Status:                migrationStatus(state),
					Source:                "migration",
					SourceKey:             sourceKey,
					BackgroundRemovalMode: firstSessionString(stringFromAny(state["backgroundRemovalMode"]), "inherit"),
					Frames:                frames,
					RawStrip:              stringFromAny(state["rawStrip"]),
					RawVideoPath:          stringFromAny(state["rawVideoPath"]),
					RawExtractedFrames:    stringSliceFromAny(state["rawExtractedFrames"]),
					CandidateFrames:       candidateFramesFromAny(state["candidateFrames"]),
					Warnings:              stringSliceFromAny(state["warnings"]),
					Feedback:              stringFromAny(state["feedback"]),
					Trace:                 trace,
					Scores:                floatMapFromAny(state["scores"]),
					SequenceMetrics:       floatMapFromAny(state["sequenceMetrics"]),
					StateSnapshot:         cloneAnyMap(state),
					CharacterSnapshot:     characterSnapshot,
				}
				committed, err := a.animationVersions.Commit(args)
				if err != nil {
					return data, false, fmt.Errorf("迁移动作 %s: %w", stateID, err)
				}
				target.CurrentVersionID = committed.ID
				state["currentVersionId"] = committed.ID
				state["currentVersionNumber"] = committed.VersionNumber
				stripLargeSessionState(state)
				changed = true
			}
			targets = append(targets, target)
		}
	}
	if err := a.recoverLegacyAnimationArtifacts(targets); err != nil {
		return data, false, fmt.Errorf("恢复旧动作产物: %w", err)
	}
	for i := range targets {
		versions, err := a.animationVersions.List(targets[i].CharacterID, targets[i].StateID, false)
		if err != nil {
			return data, false, err
		}
		if len(versions) == 0 {
			continue
		}
		current := versions[0]
		for _, version := range versions {
			if version.Current {
				current = version
				break
			}
		}
		targets[i].State["currentVersionId"] = current.ID
		targets[i].State["currentVersionNumber"] = current.VersionNumber
		targets[i].State["versionCount"] = len(versions)
		stripLargeSessionState(targets[i].State)
		changed = true
	}
	root["v"] = sessionSchemaV5
	encoded, err := json.Marshal(root)
	if err != nil {
		return data, false, err
	}
	return encoded, changed || version != sessionSchemaV5, nil
}

func stripLargeSessionState(state map[string]any) {
	state["items"] = []any{}
	delete(state, "rawStrip")
	state["rawExtractedFrames"] = []any{}
	state["candidateFrames"] = []any{}
	delete(state, "trace")
}

func migrationFrames(value any) []AnimationVersionFrame {
	items, _ := value.([]any)
	out := make([]AnimationVersionFrame, 0, len(items))
	for _, itemValue := range items {
		item, ok := itemValue.(map[string]any)
		if !ok || !strings.HasPrefix(strings.TrimSpace(stringFromAny(item["png"])), "data:image/") {
			continue
		}
		frame := AnimationVersionFrame{
			ID:        stringFromAny(item["id"]),
			PNG:       stringFromAny(item["png"]),
			Selected:  boolFromAny(item["selected"], true),
			Source:    stringFromAny(item["source"]),
			SourceRef: stringFromAny(item["sourceRef"]),
		}
		if replacement, ok := item["replacement"].(map[string]any); ok {
			frame.Replacement = &AnimationFrameReplacement{
				CandidateID: stringFromAny(replacement["candidateId"]), Source: stringFromAny(replacement["source"]),
				Label: stringFromAny(replacement["label"]), TargetFrameIndex: intFromAny(replacement["targetFrameIndex"]),
				TimeMs: intFromAny(replacement["timeMs"]), Quality: floatFromAny(replacement["quality"]),
				Reason: stringFromAny(replacement["reason"]), ReplacedAt: stringFromAny(replacement["replacedAt"]),
			}
		}
		out = append(out, frame)
	}
	return out
}

func migrationSourceKey(characterID, stateID string, frames []AnimationVersionFrame) string {
	h := sha256.New()
	for _, frame := range frames {
		_, _ = h.Write([]byte(frame.PNG))
	}
	return "migration:v4:" + characterID + ":" + stateID + ":" + hex.EncodeToString(h.Sum(nil)[:12])
}

func migrationStatus(state map[string]any) string {
	if stringFromAny(state["status"]) == "review" || state["qualityReview"] != nil {
		return "review"
	}
	return "done"
}

func candidateFramesFromAny(value any) []CandidateFrame {
	data, _ := json.Marshal(value)
	var out []CandidateFrame
	_ = json.Unmarshal(data, &out)
	return out
}

func generationTraceFromAny(value any) GenerationTrace {
	data, _ := json.Marshal(value)
	var out GenerationTrace
	_ = json.Unmarshal(data, &out)
	return out
}

func floatMapFromAny(value any) map[string]float64 {
	data, _ := json.Marshal(value)
	out := map[string]float64{}
	_ = json.Unmarshal(data, &out)
	return out
}

func cloneAnyMap(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func stringSliceFromAny(value any) []string {
	items, _ := value.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text := stringFromAny(item); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func intFromAny(value any) int {
	if number, ok := value.(float64); ok {
		return int(number)
	}
	return 0
}

func floatFromAny(value any) float64 {
	if number, ok := value.(float64); ok {
		return number
	}
	return 0
}

func boolFromAny(value any, fallback bool) bool {
	if result, ok := value.(bool); ok {
		return result
	}
	return fallback
}

func firstSessionString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
