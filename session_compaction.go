package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"runtime/debug"
	"strings"

	"perfectpixel/internal/config"
)

func persistSessionImage(dataURL string) (string, error) {
	root, err := config.GenerationArtifactsDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "session-evidence")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(dataURL))
	path := filepath.Join(dir, hex.EncodeToString(hash[:12])+".png")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	raw, err := decodeDataURL(dataURL)
	if err != nil {
		return "", err
	}
	img, err := decodeImage(raw)
	if err != nil {
		return "", err
	}
	if err := writePNG(path, img); err != nil {
		return "", err
	}
	return path, nil
}

func compactGenerationReferences(refs []GenerationReference) (bool, error) {
	changed := false
	for i := range refs {
		preview := strings.TrimSpace(refs[i].Preview)
		if !strings.HasPrefix(preview, "data:image/") {
			continue
		}
		path, err := persistSessionImage(preview)
		if err != nil {
			return false, err
		}
		if refs[i].Path == "" {
			refs[i].Path = path
		}
		refs[i].Preview = ""
		changed = true
	}
	return changed, nil
}

func compactStateSession(raw json.RawMessage) (json.RawMessage, bool, error) {
	var state map[string]json.RawMessage
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, false, err
	}
	changed := false

	if value, ok := state["candidateFrames"]; ok {
		var candidates []CandidateFrame
		if err := json.Unmarshal(value, &candidates); err != nil {
			return nil, false, err
		}
		candidateChanged := false
		for i := range candidates {
			png := strings.TrimSpace(candidates[i].PNG)
			if png == "" {
				continue
			}
			if candidates[i].SourcePath == "" && strings.HasPrefix(png, "data:image/") {
				path, err := persistSessionImage(png)
				if err != nil {
					return nil, false, err
				}
				candidates[i].SourcePath = path
			}
			if candidates[i].SourcePath != "" {
				candidates[i].PNG = ""
				candidateChanged = true
			}
		}
		if candidateChanged {
			encoded, err := json.Marshal(candidates)
			if err != nil {
				return nil, false, err
			}
			state["candidateFrames"] = encoded
			changed = true
		}
	}

	if value, ok := state["rawExtractedFrames"]; ok {
		var frames []string
		if err := json.Unmarshal(value, &frames); err != nil {
			return nil, false, err
		}
		framesChanged := false
		for i, frame := range frames {
			if !strings.HasPrefix(strings.TrimSpace(frame), "data:image/") {
				continue
			}
			path, err := persistSessionImage(frame)
			if err != nil {
				return nil, false, err
			}
			frames[i] = path
			framesChanged = true
		}
		if framesChanged {
			encoded, err := json.Marshal(frames)
			if err != nil {
				return nil, false, err
			}
			state["rawExtractedFrames"] = encoded
			changed = true
		}
	}

	if value, ok := state["trace"]; ok && len(value) > 0 && string(value) != "null" {
		var trace GenerationTrace
		if err := json.Unmarshal(value, &trace); err != nil {
			return nil, false, err
		}
		traceChanged, err := compactGenerationReferences(trace.References)
		if err != nil {
			return nil, false, err
		}
		for i := range trace.Attempts {
			attemptChanged, err := compactGenerationReferences(trace.Attempts[i].References)
			if err != nil {
				return nil, false, err
			}
			traceChanged = traceChanged || attemptChanged
		}
		if traceChanged {
			encoded, err := json.Marshal(trace)
			if err != nil {
				return nil, false, err
			}
			state["trace"] = encoded
			changed = true
		}
	}

	if !changed {
		return raw, false, nil
	}
	encoded, err := json.Marshal(state)
	return encoded, true, err
}

func writeJSONFieldPrefix(buf *bytes.Buffer, first *bool, key string) error {
	if !*first {
		buf.WriteByte(',')
	}
	*first = false
	encodedKey, err := json.Marshal(key)
	if err != nil {
		return err
	}
	buf.Write(encodedKey)
	buf.WriteByte(':')
	return nil
}

func compactCandidateArrayStream(decoder *json.Decoder, buf *bytes.Buffer) (bool, error) {
	token, err := decoder.Token()
	if err != nil {
		return false, err
	}
	if token == nil {
		buf.WriteString("null")
		return false, nil
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '[' {
		return false, fmt.Errorf("candidateFrames 不是数组")
	}
	buf.WriteByte('[')
	changed := false
	first := true
	for decoder.More() {
		var candidate CandidateFrame
		if err := decoder.Decode(&candidate); err != nil {
			return false, err
		}
		png := strings.TrimSpace(candidate.PNG)
		if png != "" {
			if candidate.SourcePath == "" && strings.HasPrefix(png, "data:image/") {
				path, err := persistSessionImage(png)
				if err != nil {
					return false, err
				}
				candidate.SourcePath = path
			}
			if candidate.SourcePath != "" {
				candidate.PNG = ""
				changed = true
			}
		}
		encoded, err := json.Marshal(candidate)
		if err != nil {
			return false, err
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		buf.Write(encoded)
	}
	if _, err := decoder.Token(); err != nil {
		return false, err
	}
	buf.WriteByte(']')
	return changed, nil
}

func compactRawFrameArrayStream(decoder *json.Decoder, buf *bytes.Buffer) (bool, error) {
	token, err := decoder.Token()
	if err != nil {
		return false, err
	}
	if token == nil {
		buf.WriteString("null")
		return false, nil
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '[' {
		return false, fmt.Errorf("rawExtractedFrames 不是数组")
	}
	buf.WriteByte('[')
	changed := false
	first := true
	for decoder.More() {
		var frame string
		if err := decoder.Decode(&frame); err != nil {
			return false, err
		}
		if strings.HasPrefix(strings.TrimSpace(frame), "data:image/") {
			path, err := persistSessionImage(frame)
			if err != nil {
				return false, err
			}
			frame = path
			changed = true
		}
		encoded, err := json.Marshal(frame)
		if err != nil {
			return false, err
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		buf.Write(encoded)
	}
	if _, err := decoder.Token(); err != nil {
		return false, err
	}
	buf.WriteByte(']')
	return changed, nil
}

func compactTraceStream(decoder *json.Decoder, buf *bytes.Buffer) (bool, error) {
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return false, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		buf.Write(raw)
		return false, nil
	}
	var trace GenerationTrace
	if err := json.Unmarshal(raw, &trace); err != nil {
		return false, err
	}
	changed, err := compactGenerationReferences(trace.References)
	if err != nil {
		return false, err
	}
	for i := range trace.Attempts {
		attemptChanged, err := compactGenerationReferences(trace.Attempts[i].References)
		if err != nil {
			return false, err
		}
		changed = changed || attemptChanged
	}
	if !changed {
		buf.Write(raw)
		return false, nil
	}
	encoded, err := json.Marshal(trace)
	if err != nil {
		return false, err
	}
	buf.Write(encoded)
	return true, nil
}

func compactStateStream(decoder *json.Decoder, buf *bytes.Buffer) (bool, error) {
	token, err := decoder.Token()
	if err != nil {
		return false, err
	}
	if token == nil {
		buf.WriteString("null")
		return false, nil
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return false, fmt.Errorf("状态项不是对象")
	}
	buf.WriteByte('{')
	changed := false
	first := true
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return false, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return false, fmt.Errorf("状态字段名无效")
		}
		if err := writeJSONFieldPrefix(buf, &first, key); err != nil {
			return false, err
		}
		var fieldChanged bool
		switch key {
		case "candidateFrames":
			fieldChanged, err = compactCandidateArrayStream(decoder, buf)
		case "rawExtractedFrames":
			fieldChanged, err = compactRawFrameArrayStream(decoder, buf)
		case "trace":
			fieldChanged, err = compactTraceStream(decoder, buf)
		default:
			var raw json.RawMessage
			err = decoder.Decode(&raw)
			buf.Write(raw)
		}
		if err != nil {
			return false, err
		}
		changed = changed || fieldChanged
	}
	if _, err := decoder.Token(); err != nil {
		return false, err
	}
	buf.WriteByte('}')
	return changed, nil
}

func compactStatesStream(decoder *json.Decoder, buf *bytes.Buffer) (bool, error) {
	token, err := decoder.Token()
	if err != nil {
		return false, err
	}
	if token == nil {
		buf.WriteString("null")
		return false, nil
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '[' {
		return false, fmt.Errorf("states 不是数组")
	}
	buf.WriteByte('[')
	changed := false
	first := true
	for decoder.More() {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		stateChanged, err := compactStateStream(decoder, buf)
		if err != nil {
			return false, err
		}
		changed = changed || stateChanged
	}
	if _, err := decoder.Token(); err != nil {
		return false, err
	}
	buf.WriteByte(']')
	return changed, nil
}

func compactWorkspaceStream(decoder *json.Decoder, buf *bytes.Buffer) (bool, error) {
	token, err := decoder.Token()
	if err != nil {
		return false, err
	}
	if token == nil {
		buf.WriteString("null")
		return false, nil
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return false, fmt.Errorf("角色工作区不是对象")
	}
	buf.WriteByte('{')
	changed := false
	first := true
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return false, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return false, fmt.Errorf("角色工作区字段名无效")
		}
		if err := writeJSONFieldPrefix(buf, &first, key); err != nil {
			return false, err
		}
		if key == "states" {
			stateChanged, err := compactStatesStream(decoder, buf)
			if err != nil {
				return false, err
			}
			changed = changed || stateChanged
			continue
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return false, err
		}
		buf.Write(raw)
	}
	if _, err := decoder.Token(); err != nil {
		return false, err
	}
	buf.WriteByte('}')
	return changed, nil
}

func compactWorkspacesStream(decoder *json.Decoder, buf *bytes.Buffer) (bool, error) {
	token, err := decoder.Token()
	if err != nil {
		return false, err
	}
	if token == nil {
		buf.WriteString("null")
		return false, nil
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '[' {
		return false, fmt.Errorf("角色工作区不是数组")
	}
	buf.WriteByte('[')
	changed := false
	first := true
	for decoder.More() {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		workspaceChanged, err := compactWorkspaceStream(decoder, buf)
		if err != nil {
			return false, err
		}
		changed = changed || workspaceChanged
	}
	if _, err := decoder.Token(); err != nil {
		return false, err
	}
	buf.WriteByte(']')
	return changed, nil
}

// compactSessionJSON moves high-resolution evidence out of session.json while
// preserving the active/old final frames. This bounds both WebView bridge
// payloads and autosave memory during regeneration.
func compactSessionJSON(data []byte) ([]byte, bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return nil, false, err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return nil, false, fmt.Errorf("会话根节点不是对象")
	}
	var buf bytes.Buffer
	buf.Grow(min(len(data), 8<<20))
	buf.WriteByte('{')
	changed := false
	first := true
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, false, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, false, fmt.Errorf("会话字段名无效")
		}
		if err := writeJSONFieldPrefix(&buf, &first, key); err != nil {
			return nil, false, err
		}
		if key == "states" {
			statesChanged, err := compactStatesStream(decoder, &buf)
			if err != nil {
				return nil, false, err
			}
			changed = changed || statesChanged
			continue
		}
		if key == "workspaces" {
			workspacesChanged, err := compactWorkspacesStream(decoder, &buf)
			if err != nil {
				return nil, false, err
			}
			changed = changed || workspacesChanged
			continue
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, false, err
		}
		buf.Write(raw)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, false, err
	}
	buf.WriteByte('}')
	if !changed {
		return data, false, nil
	}
	return buf.Bytes(), true, nil
}

func releaseLargeWorkingSet() {
	stdruntime.GC()
	debug.FreeOSMemory()
}
