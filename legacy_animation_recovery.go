package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"perfectpixel/internal/config"
	"perfectpixel/internal/sprite"
)

type LegacyAnimationRecoveryItem struct {
	ID                string `json:"id"`
	ArtifactDir       string `json:"artifactDir"`
	StateHint         string `json:"stateHint,omitempty"`
	FrameCount        int    `json:"frameCount"`
	Thumbnail         string `json:"thumbnail,omitempty"`
	Reason            string `json:"reason"`
	BaseReferenceHash string `json:"baseReferenceHash,omitempty"`
	ContentHash       string `json:"contentHash"`
	SizeBytes         int64  `json:"sizeBytes"`
	DiscoveredAt      string `json:"discoveredAt"`
	IgnoredAt         string `json:"ignoredAt,omitempty"`
	ClaimedVersionID  string `json:"claimedVersionId,omitempty"`
}

type legacyAnimationRecoveryStore struct {
	SchemaVersion int                           `json:"schemaVersion"`
	Items         []LegacyAnimationRecoveryItem `json:"items"`
}

type legacyMigrationTarget struct {
	CharacterID      string
	StateID          string
	StateName        string
	FrameCount       int
	ArtifactDir      string
	RequestID        string
	RawVideoPath     string
	BaseHash         string
	CurrentVersionID string
	CurrentFrameHash string
	Character        map[string]any
	State            map[string]any
}

var legacyAnimationRecoveryMu sync.Mutex

func (s *AnimationVersionStore) recoveryPath() string {
	return filepath.Join(s.root, "legacy-recovery.json")
}

func (s *AnimationVersionStore) readRecoveryStore() (legacyAnimationRecoveryStore, error) {
	data, err := os.ReadFile(s.recoveryPath())
	if os.IsNotExist(err) {
		return legacyAnimationRecoveryStore{SchemaVersion: 1}, nil
	}
	if err != nil {
		return legacyAnimationRecoveryStore{}, err
	}
	var store legacyAnimationRecoveryStore
	if err := json.Unmarshal(data, &store); err != nil {
		return store, err
	}
	store.SchemaVersion = 1
	return store, nil
}

func (s *AnimationVersionStore) writeRecoveryStore(store legacyAnimationRecoveryStore) error {
	store.SchemaVersion = 1
	sort.SliceStable(store.Items, func(i, j int) bool { return store.Items[i].DiscoveredAt > store.Items[j].DiscoveredAt })
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return atomicWritePrivate(s.recoveryPath(), data)
}

func (a *App) ListLegacyAnimationRecovery(includeResolved bool) ([]LegacyAnimationRecoveryItem, error) {
	if a.animationVersions == nil {
		return nil, errors.New("animation version store is unavailable")
	}
	legacyAnimationRecoveryMu.Lock()
	defer legacyAnimationRecoveryMu.Unlock()
	store, err := a.animationVersions.readRecoveryStore()
	if err != nil {
		return nil, err
	}
	out := make([]LegacyAnimationRecoveryItem, 0, len(store.Items))
	for _, item := range store.Items {
		if !includeResolved && (item.IgnoredAt != "" || item.ClaimedVersionID != "") {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func (a *App) ClaimLegacyAnimationRecovery(id, characterID, stateID, stateName string) (AnimationVersion, error) {
	if a.animationVersions == nil {
		return AnimationVersion{}, errors.New("animation version store is unavailable")
	}
	legacyAnimationRecoveryMu.Lock()
	store, err := a.animationVersions.readRecoveryStore()
	if err != nil {
		legacyAnimationRecoveryMu.Unlock()
		return AnimationVersion{}, err
	}
	index := -1
	for i := range store.Items {
		if store.Items[i].ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		legacyAnimationRecoveryMu.Unlock()
		return AnimationVersion{}, errors.New("未找到待认领旧结果")
	}
	item := store.Items[index]
	legacyAnimationRecoveryMu.Unlock()
	if item.ClaimedVersionID != "" {
		return a.animationVersions.Get(item.ClaimedVersionID)
	}
	args, err := legacyArtifactCommitArgs(item.ArtifactDir, legacyMigrationTarget{
		CharacterID: characterID, StateID: stateID, StateName: stateName, FrameCount: item.FrameCount,
	})
	if err != nil {
		return AnimationVersion{}, err
	}
	version, err := a.animationVersions.Commit(args)
	if err != nil {
		return AnimationVersion{}, err
	}
	legacyAnimationRecoveryMu.Lock()
	defer legacyAnimationRecoveryMu.Unlock()
	store, err = a.animationVersions.readRecoveryStore()
	if err != nil {
		return version, err
	}
	for i := range store.Items {
		if store.Items[i].ID == id {
			store.Items[i].ClaimedVersionID = version.ID
			store.Items[i].IgnoredAt = ""
		}
	}
	return version, a.animationVersions.writeRecoveryStore(store)
}

func (a *App) IgnoreLegacyAnimationRecovery(id string) error {
	if a.animationVersions == nil {
		return errors.New("animation version store is unavailable")
	}
	legacyAnimationRecoveryMu.Lock()
	defer legacyAnimationRecoveryMu.Unlock()
	store, err := a.animationVersions.readRecoveryStore()
	if err != nil {
		return err
	}
	found := false
	for i := range store.Items {
		if store.Items[i].ID == id {
			store.Items[i].IgnoredAt = time.Now().UTC().Format(time.RFC3339Nano)
			found = true
		}
	}
	if !found {
		return errors.New("未找到待认领旧结果")
	}
	return a.animationVersions.writeRecoveryStore(store)
}

func (a *App) recoverLegacyAnimationArtifacts(targets []legacyMigrationTarget) error {
	root, err := config.GenerationArtifactsDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	byArtifact := map[string][]int{}
	byRequest := map[string][]int{}
	if pending, pendingErr := readPendingGenerationStore(); pendingErr == nil {
		requestsByVideo := map[string][]string{}
		for _, item := range pending.Items {
			if item.RawVideoPath != "" && item.RequestID != "" {
				key := canonicalPath(item.RawVideoPath)
				requestsByVideo[key] = append(requestsByVideo[key], item.RequestID)
			}
		}
		for i := range targets {
			if targets[i].RequestID != "" || targets[i].RawVideoPath == "" {
				continue
			}
			requests := requestsByVideo[canonicalPath(targets[i].RawVideoPath)]
			if len(requests) == 1 {
				targets[i].RequestID = requests[0]
			}
		}
	}
	for i := range targets {
		if targets[i].ArtifactDir != "" {
			key := canonicalPath(targets[i].ArtifactDir)
			byArtifact[key] = append(byArtifact[key], i)
		}
		if targets[i].RequestID != "" {
			byRequest[safeVersionPart(targets[i].RequestID)] = append(byRequest[safeVersionPart(targets[i].RequestID)], i)
		}
	}
	characterHashOwners := characterImageHashOwners()
	legacyAnimationRecoveryMu.Lock()
	store, err := a.animationVersions.readRecoveryStore()
	legacyAnimationRecoveryMu.Unlock()
	if err != nil {
		return err
	}
	existingInbox := map[string]bool{}
	for _, item := range store.Items {
		existingInbox[canonicalPath(item.ArtifactDir)+"\x00"+item.ContentHash] = true
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "session-evidence" {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		inspection, inspectErr := inspectLegacyArtifact(dir)
		if inspectErr != nil || inspection.FrameCount == 0 {
			continue
		}
		assigned := -1
		if matches := byArtifact[canonicalPath(dir)]; len(matches) == 1 {
			assigned = matches[0]
		}
		if assigned < 0 {
			for requestID, matches := range byRequest {
				if len(matches) == 1 && strings.Contains(safeVersionPart(entry.Name()), requestID) {
					if assigned >= 0 {
						assigned = -1
						break
					}
					assigned = matches[0]
				}
			}
		}
		if assigned < 0 && inspection.BaseReferenceHash != "" && len(characterHashOwners[inspection.BaseReferenceHash]) == 1 {
			owner := characterHashOwners[inspection.BaseReferenceHash][0]
			matches := make([]int, 0, 1)
			for i := range targets {
				if targets[i].CharacterID != owner || !artifactMatchesStateHint(entry.Name(), targets[i]) {
					continue
				}
				matches = append(matches, i)
			}
			if len(matches) == 1 {
				assigned = matches[0]
			}
		}
		if assigned >= 0 {
			target := targets[assigned]
			if target.CurrentFrameHash == inspection.FrameHash {
				continue
			}
			args, argsErr := legacyArtifactCommitArgs(dir, target)
			if argsErr != nil {
				continue
			}
			if _, commitErr := a.animationVersions.Commit(args); commitErr != nil {
				return commitErr
			}
			continue
		}
		key := canonicalPath(dir) + "\x00" + inspection.ContentHash
		if existingInbox[key] {
			continue
		}
		store.Items = append(store.Items, LegacyAnimationRecoveryItem{
			ID: "legacy-" + inspection.ContentHash[:24], ArtifactDir: dir, StateHint: legacyArtifactStateHint(entry.Name()),
			FrameCount: inspection.FrameCount, Thumbnail: inspection.Thumbnail, Reason: "无法通过 trace、requestId、原视频路径或唯一角色图片哈希确定角色与动作归属。",
			BaseReferenceHash: inspection.BaseReferenceHash, ContentHash: inspection.ContentHash, SizeBytes: directorySize(dir), DiscoveredAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
		existingInbox[key] = true
	}
	for _, target := range targets {
		if target.CurrentVersionID == "" {
			continue
		}
		_, _ = a.animationVersions.SetCurrent(target.CharacterID, target.StateID, target.CurrentVersionID)
	}
	legacyAnimationRecoveryMu.Lock()
	defer legacyAnimationRecoveryMu.Unlock()
	return a.animationVersions.writeRecoveryStore(store)
}

type legacyArtifactInspection struct {
	FrameCount        int
	FrameHash         string
	ContentHash       string
	BaseReferenceHash string
	Thumbnail         string
}

func inspectLegacyArtifact(dir string) (legacyArtifactInspection, error) {
	frames, _ := legacyArtifactFinalFramePaths(dir)
	frameCount := len(frames)
	frameHash := ""
	var err error
	thumbnailPath := ""
	if frameCount > 0 {
		frameHash, err = fileSetHash(frames)
		thumbnailPath = frames[0]
	} else {
		stripPath := filepath.Join(dir, "processed-strip.png")
		stripData, readErr := os.ReadFile(stripPath)
		if readErr != nil {
			return legacyArtifactInspection{}, errors.New("旧结果没有可恢复帧")
		}
		stripImage, decodeErr := decodeImage(stripData)
		if decodeErr != nil {
			return legacyArtifactInspection{}, decodeErr
		}
		frameCount = inferLegacyStripFrameCount(sprite.ToNRGBA(stripImage))
		if frameCount <= 0 {
			return legacyArtifactInspection{}, errors.New("旧条带无法确定帧数")
		}
		frameHash = sha256Hex(stripData)
		thumbnailPath = stripPath
	}
	if err != nil {
		return legacyArtifactInspection{}, err
	}
	all, _ := filepath.Glob(filepath.Join(dir, "*.png"))
	sort.Strings(all)
	contentHash, err := fileSetHash(all)
	if err != nil {
		return legacyArtifactInspection{}, err
	}
	baseHash, _ := imagePixelHashFile(filepath.Join(dir, "base-reference.png"))
	thumb := ""
	if data, err := os.ReadFile(thumbnailPath); err == nil {
		if img, decodeErr := decodeImage(data); decodeErr == nil {
			thumb, _ = thumbnailDataURL(img, 160)
		}
	}
	return legacyArtifactInspection{FrameCount: frameCount, FrameHash: frameHash, ContentHash: contentHash, BaseReferenceHash: baseHash, Thumbnail: thumb}, nil
}

func legacyArtifactCommitArgs(dir string, target legacyMigrationTarget) (CommitAnimationVersionArgs, error) {
	paths, _ := legacyArtifactFinalFramePaths(dir)
	frames := make([]AnimationVersionFrame, 0, max(len(paths), target.FrameCount))
	if len(paths) > 0 {
		for i, path := range paths {
			frames = append(frames, AnimationVersionFrame{ID: fmt.Sprintf("legacy-%03d", i+1), Path: path, Selected: true, Source: "legacy_artifact"})
		}
	} else {
		extracted, err := legacyStripFrames(dir, target.FrameCount)
		if err != nil {
			return CommitAnimationVersionArgs{}, err
		}
		for i, frame := range extracted {
			url, encodeErr := pngDataURL(frame)
			if encodeErr != nil {
				return CommitAnimationVersionArgs{}, encodeErr
			}
			frames = append(frames, AnimationVersionFrame{ID: fmt.Sprintf("legacy-%03d", i+1), PNG: url, Selected: true, Source: "legacy_artifact"})
		}
	}
	if len(frames) == 0 {
		return CommitAnimationVersionArgs{}, errors.New("旧结果没有可恢复帧")
	}
	rawPaths, _ := filepath.Glob(filepath.Join(dir, "raw-candidate-*.png"))
	sort.Strings(rawPaths)
	candidatePaths, _ := filepath.Glob(filepath.Join(dir, "timeline-candidate-*.png"))
	sort.Strings(candidatePaths)
	candidates := make([]CandidateFrame, 0, len(candidatePaths))
	for i, path := range candidatePaths {
		candidates = append(candidates, CandidateFrame{ID: fmt.Sprintf("legacy-candidate-%03d", i+1), Index: i, Source: candidateSourceVideoProcessed, Label: "历史恢复候选帧", SourcePath: path})
	}
	strip := ""
	if data, readErr := os.ReadFile(filepath.Join(dir, "processed-strip.png")); readErr == nil {
		strip = "data:image/png;base64," + encodeBase64(data)
	}
	contentHash := ""
	if len(paths) > 0 {
		contentHash, _ = fileSetHash(paths)
	} else if data, readErr := os.ReadFile(filepath.Join(dir, "processed-strip.png")); readErr == nil {
		contentHash = sha256Hex(data)
	}
	warnings := []string{"此版本从旧 generation-artifacts 恢复。"}
	partial := len(rawPaths) == 0 || len(candidatePaths) == 0 || target.RawVideoPath == ""
	if partial {
		warnings = append(warnings, "历史部分恢复：部分候选帧、源视频或生成参数不可用。")
	}
	createdAt := ""
	if info, statErr := os.Stat(dir); statErr == nil {
		createdAt = info.ModTime().UTC().Format(time.RFC3339Nano)
	}
	trace := GenerationTrace{Pipeline: pipelineVideoToFrames, BackgroundRemovalMode: "chroma", CalledAI: true, Parameters: map[string]string{"artifactDir": dir, "migration": "v4_generation_artifact"}}
	return CommitAnimationVersionArgs{
		CharacterID: target.CharacterID, StateID: target.StateID, StateName: firstSessionString(target.StateName, target.StateID), Status: "done", Source: "migration_artifact",
		SourceKey: "migration:v4:artifact:" + contentHash + ":" + sha256String(canonicalPath(dir))[:16], BackgroundRemovalMode: "chroma", Frames: frames,
		RawStrip: strip, RawVideoPath: target.RawVideoPath, RawExtractedFrames: rawPaths, CandidateFrames: candidates, Warnings: warnings, Trace: trace,
		GenerationParameters: map[string]any{"artifactDir": dir}, CharacterSnapshot: cloneAnyMap(target.Character), StateSnapshot: cloneAnyMap(target.State), PartialRecovery: partial, CreatedAt: createdAt,
	}, nil
}

func legacyArtifactFinalFramePaths(dir string) ([]string, error) {
	paths, _ := filepath.Glob(filepath.Join(dir, "final-frame-*.png"))
	sort.Strings(paths)
	return paths, nil
}

func legacyStripFrames(dir string, expected int) ([]*image.NRGBA, error) {
	stripPath := filepath.Join(dir, "processed-strip.png")
	data, err := os.ReadFile(stripPath)
	if err != nil {
		return nil, err
	}
	img, err := decodeImage(data)
	if err != nil {
		return nil, err
	}
	// Old image-strip artifacts have no metadata. Connected-component extraction
	// is deterministic and avoids guessing a frame count from the action name.
	clean := sprite.ToNRGBA(img)
	if expected <= 0 {
		expected = inferLegacyStripFrameCount(clean)
	}
	if expected <= 0 {
		return nil, errors.New("旧条带无法确定帧数")
	}
	extracted := sprite.ExtractFramesWithFitMode(clean, expected, clean.Bounds().Dy(), clean.Bounds().Dy(), 0, sprite.FrameFitContain)
	if len(extracted.Frames) == 0 {
		return nil, errors.New("旧条带无法提取帧")
	}
	return extracted.Frames, nil
}

func inferLegacyStripFrameCount(strip *image.NRGBA) int {
	if strip == nil || strip.Rect.Empty() {
		return 0
	}
	w, h := strip.Rect.Dx(), strip.Rect.Dy()
	if h > 0 && w >= h && w%h == 0 {
		count := w / h
		if count >= 1 && count <= maxAnimationFrames {
			return count
		}
	}
	runs := 0
	inRun := false
	for x := 0; x < w; x++ {
		hasAlpha := false
		for y := 0; y < h; y++ {
			if strip.NRGBAAt(strip.Rect.Min.X+x, strip.Rect.Min.Y+y).A > 8 {
				hasAlpha = true
				break
			}
		}
		if hasAlpha && !inRun {
			runs++
		}
		inRun = hasAlpha
	}
	if runs > maxAnimationFrames {
		return 0
	}
	return runs
}

func migrationFrameHash(frames []AnimationVersionFrame) string {
	h := sha256.New()
	for _, frame := range frames {
		data, err := decodeDataURL(frame.PNG)
		if err == nil {
			_, _ = h.Write(data)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func fileSetHash(paths []string) (string, error) {
	h := sha256.New()
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		_, _ = h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func imagePixelHashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	img, err := decodeImage(data)
	if err != nil {
		return "", err
	}
	nrgba := sprite.ToNRGBA(img)
	h := sha256.New()
	_, _ = h.Write(nrgba.Pix)
	_, _ = h.Write([]byte(fmt.Sprintf("%d:%d:%d", nrgba.Rect.Dx(), nrgba.Rect.Dy(), nrgba.Stride)))
	return hex.EncodeToString(h.Sum(nil)), nil
}

func characterImageHashOwners() map[string][]string {
	owners := map[string][]string{}
	store, err := readCharacterLibraryStore()
	if err != nil {
		return owners
	}
	for _, item := range store.Items {
		hash, hashErr := imagePixelHashFile(item.ImagePath)
		if hashErr == nil && hash != "" {
			owners[hash] = append(owners[hash], item.ID)
		}
	}
	return owners
}

func artifactMatchesStateHint(dirName string, target legacyMigrationTarget) bool {
	name := strings.ToLower(safeVersionPart(dirName))
	for _, value := range []string{target.StateID, target.StateName} {
		part := strings.ToLower(safeVersionPart(value))
		if part != "" && (name == part || strings.HasSuffix(name, "-"+part)) {
			return true
		}
	}
	return false
}

func legacyArtifactStateHint(dirName string) string {
	parts := strings.Split(strings.TrimSpace(dirName), "-")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func canonicalPath(path string) string {
	absolute, err := filepath.Abs(filepath.Clean(strings.TrimSpace(path)))
	if err != nil {
		return strings.ToLower(filepath.Clean(path))
	}
	return strings.ToLower(absolute)
}

func sha256String(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
