package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"perfectpixel/internal/config"
)

const animationVersionSchema = 1

const animationVersionResourcePrefix = "version-resource:"

type AnimationVersionFrame struct {
	ID          string                     `json:"id"`
	PNG         string                     `json:"png,omitempty"`
	Path        string                     `json:"path,omitempty"`
	Selected    bool                       `json:"selected"`
	Source      string                     `json:"source,omitempty"`
	SourceRef   string                     `json:"sourceRef,omitempty"`
	Replacement *AnimationFrameReplacement `json:"replacement,omitempty"`
}

type AnimationFrameReplacement struct {
	CandidateID      string  `json:"candidateId"`
	Source           string  `json:"source"`
	Label            string  `json:"label,omitempty"`
	TargetFrameIndex int     `json:"targetFrameIndex"`
	TimeMs           int     `json:"timeMs,omitempty"`
	Quality          float64 `json:"quality,omitempty"`
	Reason           string  `json:"reason,omitempty"`
	ReplacedAt       string  `json:"replacedAt"`
}

type AnimationVersionSummary struct {
	ID                    string  `json:"id"`
	CharacterID           string  `json:"characterId"`
	StateID               string  `json:"stateId"`
	StateName             string  `json:"stateName"`
	VersionNumber         int     `json:"versionNumber"`
	Status                string  `json:"status"`
	Source                string  `json:"source"`
	ParentVersionID       string  `json:"parentVersionId,omitempty"`
	FrameCount            int     `json:"frameCount"`
	OverallScore          float64 `json:"overallScore,omitempty"`
	BackgroundRemovalMode string  `json:"backgroundRemovalMode,omitempty"`
	Thumbnail             string  `json:"thumbnail,omitempty"`
	CreatedAt             string  `json:"createdAt"`
	DeletedAt             string  `json:"deletedAt,omitempty"`
	SizeBytes             int64   `json:"sizeBytes"`
	PartialRecovery       bool    `json:"partialRecovery,omitempty"`
	Current               bool    `json:"current"`
}

type AnimationVersion struct {
	AnimationVersionSummary
	BaselineFrames       []AnimationVersionFrame `json:"baselineFrames"`
	WorkingComposition   []AnimationVersionFrame `json:"workingComposition"`
	RawStrip             string                  `json:"rawStrip,omitempty"`
	RawStripPath         string                  `json:"rawStripPath,omitempty"`
	RawVideoPath         string                  `json:"rawVideoPath,omitempty"`
	RawExtractedFrames   []string                `json:"rawExtractedFrames,omitempty"`
	CandidateFrames      []CandidateFrame        `json:"candidateFrames,omitempty"`
	Warnings             []string                `json:"warnings,omitempty"`
	Feedback             string                  `json:"feedback,omitempty"`
	Trace                GenerationTrace         `json:"trace"`
	Scores               map[string]float64      `json:"scores,omitempty"`
	SequenceMetrics      map[string]float64      `json:"sequenceMetrics,omitempty"`
	GenerationParameters map[string]any          `json:"generationParameters,omitempty"`
	CharacterSnapshot    map[string]any          `json:"characterSnapshot,omitempty"`
	StateSnapshot        map[string]any          `json:"stateSnapshot,omitempty"`
	SourceKey            string                  `json:"sourceKey,omitempty"`
}

type CommitAnimationVersionArgs struct {
	CharacterID           string                  `json:"characterId"`
	StateID               string                  `json:"stateId"`
	StateName             string                  `json:"stateName"`
	Status                string                  `json:"status"`
	Source                string                  `json:"source"`
	SourceKey             string                  `json:"sourceKey,omitempty"`
	ParentVersionID       string                  `json:"parentVersionId,omitempty"`
	BackgroundRemovalMode string                  `json:"backgroundRemovalMode,omitempty"`
	Frames                []AnimationVersionFrame `json:"frames"`
	RawStrip              string                  `json:"rawStrip,omitempty"`
	RawVideoPath          string                  `json:"rawVideoPath,omitempty"`
	RawExtractedFrames    []string                `json:"rawExtractedFrames,omitempty"`
	CandidateFrames       []CandidateFrame        `json:"candidateFrames,omitempty"`
	Warnings              []string                `json:"warnings,omitempty"`
	Feedback              string                  `json:"feedback,omitempty"`
	Trace                 GenerationTrace         `json:"trace"`
	Scores                map[string]float64      `json:"scores,omitempty"`
	SequenceMetrics       map[string]float64      `json:"sequenceMetrics,omitempty"`
	GenerationParameters  map[string]any          `json:"generationParameters,omitempty"`
	CharacterSnapshot     map[string]any          `json:"characterSnapshot,omitempty"`
	StateSnapshot         map[string]any          `json:"stateSnapshot,omitempty"`
	PartialRecovery       bool                    `json:"partialRecovery,omitempty"`
	CreatedAt             string                  `json:"createdAt,omitempty"`
}

type AnimationVersionStorageStats struct {
	VersionCount int    `json:"versionCount"`
	TrashCount   int    `json:"trashCount"`
	VersionBytes int64  `json:"versionBytes"`
	TrashBytes   int64  `json:"trashBytes"`
	TotalBytes   int64  `json:"totalBytes"`
	FreeBytes    uint64 `json:"freeBytes"`
	WarningBytes int64  `json:"warningBytes"`
	Warning      string `json:"warning,omitempty"`
}

type animationVersionIndex struct {
	SchemaVersion int                                `json:"schemaVersion"`
	Versions      map[string]AnimationVersionSummary `json:"versions"`
	Current       map[string]string                  `json:"current"`
	Next          map[string]int                     `json:"next"`
	SourceKeys    map[string]string                  `json:"sourceKeys,omitempty"`
}

type AnimationVersionStore struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func NewAnimationVersionStore() (*AnimationVersionStore, error) {
	root, err := config.AnimationVersionsDir()
	if err != nil {
		return nil, err
	}
	return OpenAnimationVersionStore(root)
}

func OpenAnimationVersionStore(root string) (*AnimationVersionStore, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return nil, errors.New("animation version root is empty")
	}
	s := &AnimationVersionStore{root: root, now: time.Now}
	for _, dir := range []string{root, s.versionsRoot(), s.trashRoot(), s.stagingRoot()} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	if _, err := os.Stat(s.indexPath()); os.IsNotExist(err) {
		if err := s.writeIndex(animationVersionIndex{SchemaVersion: animationVersionSchema, Versions: map[string]AnimationVersionSummary{}, Current: map[string]string{}, Next: map[string]int{}, SourceKeys: map[string]string{}}); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *AnimationVersionStore) versionsRoot() string { return filepath.Join(s.root, "versions") }
func (s *AnimationVersionStore) trashRoot() string    { return filepath.Join(s.root, "trash") }
func (s *AnimationVersionStore) stagingRoot() string  { return filepath.Join(s.root, ".staging") }
func (s *AnimationVersionStore) indexPath() string    { return filepath.Join(s.root, "index.json") }

func versionStateKey(characterID, stateID string) string { return characterID + "\x00" + stateID }

func safeVersionPart(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

func newAnimationVersionID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return "av-" + hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("av-%d", time.Now().UnixNano())
}

func (s *AnimationVersionStore) readIndex() (animationVersionIndex, error) {
	data, err := os.ReadFile(s.indexPath())
	if err != nil {
		return animationVersionIndex{}, err
	}
	var idx animationVersionIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return idx, err
	}
	if idx.Versions == nil {
		idx.Versions = map[string]AnimationVersionSummary{}
	}
	if idx.Current == nil {
		idx.Current = map[string]string{}
	}
	if idx.Next == nil {
		idx.Next = map[string]int{}
	}
	if idx.SourceKeys == nil {
		idx.SourceKeys = map[string]string{}
	}
	return idx, nil
}

func (s *AnimationVersionStore) writeIndex(idx animationVersionIndex) error {
	idx.SchemaVersion = animationVersionSchema
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return atomicWritePrivate(s.indexPath(), data)
}

func (s *AnimationVersionStore) versionDir(v AnimationVersionSummary) string {
	root := s.versionsRoot()
	if v.DeletedAt != "" {
		root = s.trashRoot()
	}
	return filepath.Join(root, safeVersionPart(v.CharacterID), safeVersionPart(v.StateID), v.ID)
}

func (s *AnimationVersionStore) Commit(args CommitAnimationVersionArgs) (AnimationVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(args.CharacterID) == "" || strings.TrimSpace(args.StateID) == "" {
		return AnimationVersion{}, errors.New("characterId and stateId are required")
	}
	if len(args.Frames) == 0 {
		return AnimationVersion{}, errors.New("animation version requires at least one frame")
	}
	idx, err := s.readIndex()
	if err != nil {
		return AnimationVersion{}, err
	}
	if args.SourceKey != "" {
		if id := idx.SourceKeys[args.SourceKey]; id != "" {
			return s.getLocked(idx, id)
		}
	}
	key := versionStateKey(args.CharacterID, args.StateID)
	number := idx.Next[key]
	if number < 1 {
		number = 1
	}
	id := newAnimationVersionID()
	stage := filepath.Join(s.stagingRoot(), id)
	if err := os.MkdirAll(stage, 0o700); err != nil {
		return AnimationVersion{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(stage)
		}
	}()

	baseline := make([]AnimationVersionFrame, 0, len(args.Frames))
	working := make([]AnimationVersionFrame, 0, len(args.Frames))
	for i, frame := range args.Frames {
		name := fmt.Sprintf("frame-%03d.png", i+1)
		target := filepath.Join(stage, name)
		if strings.TrimSpace(frame.PNG) != "" {
			data, err := decodeDataURL(frame.PNG)
			if err != nil {
				return AnimationVersion{}, fmt.Errorf("decode frame %d: %w", i+1, err)
			}
			if err := os.WriteFile(target, data, 0o600); err != nil {
				return AnimationVersion{}, err
			}
		} else if strings.TrimSpace(frame.Path) != "" {
			if err := linkOrCopy(frame.Path, target); err != nil {
				return AnimationVersion{}, fmt.Errorf("copy frame %d: %w", i+1, err)
			}
		} else {
			return AnimationVersion{}, fmt.Errorf("frame %d is empty", i+1)
		}
		stored := frame
		stored.PNG = ""
		stored.Path = name
		baseline = append(baseline, stored)
		working = append(working, stored)
	}
	var rawStripPath string
	if strings.TrimSpace(args.RawStrip) != "" {
		data, err := decodeDataURL(args.RawStrip)
		if err == nil {
			rawStripPath = "processed-strip.png"
			if err := os.WriteFile(filepath.Join(stage, rawStripPath), data, 0o600); err != nil {
				return AnimationVersion{}, err
			}
		}
	}
	copyInto := func(source, name string) string {
		if strings.TrimSpace(source) == "" {
			return ""
		}
		target := filepath.Join(stage, name)
		if err := linkOrCopy(source, target); err != nil {
			return source
		}
		return name
	}
	rawVideoPath := copyInto(args.RawVideoPath, "source"+strings.ToLower(filepath.Ext(args.RawVideoPath)))
	rawExtracted := make([]string, 0, len(args.RawExtractedFrames))
	for i, source := range args.RawExtractedFrames {
		name := fmt.Sprintf("raw-frame-%03d.png", i+1)
		if strings.HasPrefix(strings.TrimSpace(source), "data:image/") {
			if data, decodeErr := decodeDataURL(source); decodeErr == nil {
				if writeErr := os.WriteFile(filepath.Join(stage, name), data, 0o600); writeErr == nil {
					rawExtracted = append(rawExtracted, name)
					continue
				}
			}
		}
		rawExtracted = append(rawExtracted, copyInto(source, name))
	}
	candidates := append([]CandidateFrame(nil), args.CandidateFrames...)
	for i := range candidates {
		name := fmt.Sprintf("candidate-%03d.png", i+1)
		if strings.TrimSpace(candidates[i].SourcePath) == "" && strings.HasPrefix(strings.TrimSpace(candidates[i].PNG), "data:image/") {
			if data, decodeErr := decodeDataURL(candidates[i].PNG); decodeErr == nil {
				if writeErr := os.WriteFile(filepath.Join(stage, name), data, 0o600); writeErr == nil {
					candidates[i].SourcePath = name
				}
			}
		}
		if copied := copyInto(candidates[i].SourcePath, name); copied != "" {
			candidates[i].SourcePath = copied
		}
		candidates[i].PNG = ""
	}
	created := strings.TrimSpace(args.CreatedAt)
	if created == "" {
		created = s.now().UTC().Format(time.RFC3339Nano)
	}
	summary := AnimationVersionSummary{
		ID: id, CharacterID: args.CharacterID, StateID: args.StateID, StateName: args.StateName,
		VersionNumber: number, Status: args.Status, Source: args.Source, ParentVersionID: args.ParentVersionID,
		FrameCount: len(working), OverallScore: args.Scores["overall"], BackgroundRemovalMode: args.BackgroundRemovalMode,
		CreatedAt: created, PartialRecovery: args.PartialRecovery, Current: true,
	}
	version := AnimationVersion{AnimationVersionSummary: summary, BaselineFrames: baseline, WorkingComposition: working,
		RawStripPath: rawStripPath, RawVideoPath: rawVideoPath, RawExtractedFrames: rawExtracted,
		CandidateFrames: candidates, Warnings: args.Warnings, Feedback: args.Feedback, Trace: args.Trace,
		Scores: args.Scores, SequenceMetrics: args.SequenceMetrics, GenerationParameters: args.GenerationParameters,
		CharacterSnapshot: args.CharacterSnapshot, StateSnapshot: args.StateSnapshot, SourceKey: args.SourceKey,
	}
	meta, err := json.MarshalIndent(version, "", "  ")
	if err != nil {
		return AnimationVersion{}, err
	}
	if err := os.WriteFile(filepath.Join(stage, "version.json"), meta, 0o600); err != nil {
		return AnimationVersion{}, err
	}
	finalDir := filepath.Join(s.versionsRoot(), safeVersionPart(args.CharacterID), safeVersionPart(args.StateID), id)
	if err := os.MkdirAll(filepath.Dir(finalDir), 0o700); err != nil {
		return AnimationVersion{}, err
	}
	if err := os.Rename(stage, finalDir); err != nil {
		return AnimationVersion{}, err
	}
	cleanup = false
	summary.SizeBytes = directorySize(finalDir)
	for existingID, existing := range idx.Versions {
		if existing.CharacterID == args.CharacterID && existing.StateID == args.StateID {
			existing.Current = false
			idx.Versions[existingID] = existing
		}
	}
	idx.Versions[id] = summary
	idx.Current[key] = id
	idx.Next[key] = number + 1
	if args.SourceKey != "" {
		idx.SourceKeys[args.SourceKey] = id
	}
	if err := s.writeIndex(idx); err != nil {
		_ = os.RemoveAll(finalDir)
		return AnimationVersion{}, err
	}
	version.AnimationVersionSummary = summary
	return s.hydrateVersion(finalDir, version)
}

func linkOrCopy(source, target string) error {
	source = filepath.Clean(strings.TrimSpace(source))
	if source == "" {
		return errors.New("empty source")
	}
	if err := os.Link(source, target); err == nil {
		return nil
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(target)
		return copyErr
	}
	return closeErr
}

func directorySize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(_ string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			if info, e := entry.Info(); e == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

func (s *AnimationVersionStore) getLocked(idx animationVersionIndex, id string) (AnimationVersion, error) {
	summary, ok := idx.Versions[id]
	if !ok {
		return AnimationVersion{}, errors.New("animation version not found")
	}
	dir := s.versionDir(summary)
	version, err := s.readVersionMetadata(dir, summary)
	if err != nil {
		return AnimationVersion{}, err
	}
	return s.hydrateVersion(dir, version)
}

func (s *AnimationVersionStore) readVersionMetadata(dir string, summary AnimationVersionSummary) (AnimationVersion, error) {
	data, err := os.ReadFile(filepath.Join(dir, "version.json"))
	if err != nil {
		return AnimationVersion{}, err
	}
	var version AnimationVersion
	if err := json.Unmarshal(data, &version); err != nil {
		return version, err
	}
	version.AnimationVersionSummary = summary
	return version, nil
}

func animationVersionResourceToken(versionID, kind string, index int) string {
	return fmt.Sprintf("%s%s:%s:%d", animationVersionResourcePrefix, versionID, kind, index)
}

func (s *AnimationVersionStore) hydrateVersion(dir string, version AnimationVersion) (AnimationVersion, error) {
	hydrateFrames := func(frames []AnimationVersionFrame) error {
		for i := range frames {
			data, err := os.ReadFile(filepath.Join(dir, frames[i].Path))
			if err != nil {
				return err
			}
			frames[i].PNG = "data:image/png;base64," + encodeBase64(data)
			frames[i].Path = ""
		}
		return nil
	}
	if err := hydrateFrames(version.BaselineFrames); err != nil {
		return version, err
	}
	if err := hydrateFrames(version.WorkingComposition); err != nil {
		return version, err
	}
	if version.RawStripPath != "" {
		if data, err := os.ReadFile(filepath.Join(dir, version.RawStripPath)); err == nil {
			version.RawStrip = "data:image/png;base64," + encodeBase64(data)
		}
	}
	for i := range version.CandidateFrames {
		if version.CandidateFrames[i].SourcePath != "" {
			version.CandidateFrames[i].SourcePath = animationVersionResourceToken(version.ID, "candidate", i)
		}
	}
	if version.RawVideoPath != "" {
		version.RawVideoPath = animationVersionResourceToken(version.ID, "video", 0)
	}
	for i, path := range version.RawExtractedFrames {
		if path != "" {
			version.RawExtractedFrames[i] = animationVersionResourceToken(version.ID, "raw-frame", i)
		}
	}
	return version, nil
}

func (s *AnimationVersionStore) ResolveResource(token string) (string, error) {
	parts := strings.Split(strings.TrimSpace(token), ":")
	if len(parts) != 4 || parts[0] != strings.TrimSuffix(animationVersionResourcePrefix, ":") {
		return "", errors.New("invalid animation version resource token")
	}
	index := 0
	if _, err := fmt.Sscanf(parts[3], "%d", &index); err != nil || index < 0 {
		return "", errors.New("invalid animation version resource index")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.readIndex()
	if err != nil {
		return "", err
	}
	summary, ok := idx.Versions[parts[1]]
	if !ok {
		return "", errors.New("animation version resource not found")
	}
	dir := s.versionDir(summary)
	version, err := s.readVersionMetadata(dir, summary)
	if err != nil {
		return "", err
	}
	var stored string
	switch parts[2] {
	case "video":
		if index != 0 {
			return "", errors.New("invalid video resource index")
		}
		stored = version.RawVideoPath
	case "raw-frame":
		if index >= len(version.RawExtractedFrames) {
			return "", errors.New("raw frame resource index out of range")
		}
		stored = version.RawExtractedFrames[index]
	case "candidate":
		if index >= len(version.CandidateFrames) {
			return "", errors.New("candidate resource index out of range")
		}
		stored = version.CandidateFrames[index].SourcePath
	default:
		return "", errors.New("unsupported animation version resource kind")
	}
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return "", errors.New("animation version resource is empty")
	}
	resolved := stored
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(dir, resolved)
	}
	resolved = filepath.Clean(resolved)
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("animation version resource is a directory")
	}
	return resolved, nil
}

func encodeBase64(data []byte) string {
	const table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var out bytes.Buffer
	enc := make([]byte, ((len(data)+2)/3)*4)
	for i, j := 0, 0; i < len(data); i, j = i+3, j+4 {
		var v uint32 = uint32(data[i]) << 16
		if i+1 < len(data) {
			v |= uint32(data[i+1]) << 8
		}
		if i+2 < len(data) {
			v |= uint32(data[i+2])
		}
		enc[j] = table[(v>>18)&63]
		enc[j+1] = table[(v>>12)&63]
		if i+1 < len(data) {
			enc[j+2] = table[(v>>6)&63]
		} else {
			enc[j+2] = '='
		}
		if i+2 < len(data) {
			enc[j+3] = table[v&63]
		} else {
			enc[j+3] = '='
		}
	}
	out.Write(enc)
	return out.String()
}

func (s *AnimationVersionStore) Get(id string) (AnimationVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.readIndex()
	if err != nil {
		return AnimationVersion{}, err
	}
	return s.getLocked(idx, id)
}

func (s *AnimationVersionStore) List(characterID, stateID string, includeDeleted bool) ([]AnimationVersionSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.readIndex()
	if err != nil {
		return nil, err
	}
	out := []AnimationVersionSummary{}
	for _, item := range idx.Versions {
		if item.CharacterID != characterID || item.StateID != stateID || (!includeDeleted && item.DeletedAt != "") {
			continue
		}
		if item.Thumbnail == "" {
			if version, e := s.getLocked(idx, item.ID); e == nil && len(version.WorkingComposition) > 0 {
				item.Thumbnail = version.WorkingComposition[0].PNG
			}
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].VersionNumber > out[j].VersionNumber })
	return out, nil
}

func (s *AnimationVersionStore) SetCurrent(characterID, stateID, id string) (AnimationVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.readIndex()
	if err != nil {
		return AnimationVersion{}, err
	}
	selected, ok := idx.Versions[id]
	if !ok || selected.CharacterID != characterID || selected.StateID != stateID || selected.DeletedAt != "" {
		return AnimationVersion{}, errors.New("animation version cannot be selected")
	}
	for existingID, item := range idx.Versions {
		if item.CharacterID == characterID && item.StateID == stateID {
			item.Current = existingID == id
			idx.Versions[existingID] = item
		}
	}
	idx.Current[versionStateKey(characterID, stateID)] = id
	if err := s.writeIndex(idx); err != nil {
		return AnimationVersion{}, err
	}
	return s.getLocked(idx, id)
}

func (s *AnimationVersionStore) UpdateComposition(id string, frames []AnimationVersionFrame) (AnimationVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.readIndex()
	if err != nil {
		return AnimationVersion{}, err
	}
	summary, ok := idx.Versions[id]
	if !ok || summary.DeletedAt != "" {
		return AnimationVersion{}, errors.New("animation version not found")
	}
	if !summary.Current {
		return AnimationVersion{}, errors.New("only the current animation version can be edited")
	}
	dir := s.versionDir(summary)
	data, err := os.ReadFile(filepath.Join(dir, "version.json"))
	if err != nil {
		return AnimationVersion{}, err
	}
	var version AnimationVersion
	if err := json.Unmarshal(data, &version); err != nil {
		return version, err
	}
	for i, frame := range frames {
		if frame.PNG == "" {
			return version, fmt.Errorf("composition frame %d is empty", i+1)
		}
		pngBytes, err := decodeDataURL(frame.PNG)
		if err != nil {
			return version, err
		}
		name := fmt.Sprintf("working-%03d.png", i+1)
		if err := os.WriteFile(filepath.Join(dir, name), pngBytes, 0o600); err != nil {
			return version, err
		}
		frame.PNG = ""
		frame.Path = name
		frames[i] = frame
	}
	version.WorkingComposition = frames
	meta, _ := json.MarshalIndent(version, "", "  ")
	if err := atomicWritePrivate(filepath.Join(dir, "version.json"), meta); err != nil {
		return version, err
	}
	summary.SizeBytes = directorySize(dir)
	idx.Versions[id] = summary
	if err := s.writeIndex(idx); err != nil {
		return version, err
	}
	version.AnimationVersionSummary = summary
	return s.hydrateVersion(dir, version)
}

func (s *AnimationVersionStore) move(id string, restore bool) (AnimationVersionSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.readIndex()
	if err != nil {
		return AnimationVersionSummary{}, err
	}
	item, ok := idx.Versions[id]
	if !ok {
		return item, errors.New("animation version not found")
	}
	if restore == (item.DeletedAt == "") {
		return item, nil
	}
	oldDir := s.versionDir(item)
	if restore {
		item.DeletedAt = ""
	} else {
		item.DeletedAt = s.now().UTC().Format(time.RFC3339Nano)
		item.Current = false
	}
	newDir := s.versionDir(item)
	if err := os.MkdirAll(filepath.Dir(newDir), 0o700); err != nil {
		return item, err
	}
	if err := os.Rename(oldDir, newDir); err != nil {
		return item, err
	}
	idx.Versions[id] = item
	key := versionStateKey(item.CharacterID, item.StateID)
	if !restore && idx.Current[key] == id {
		delete(idx.Current, key)
		var newest AnimationVersionSummary
		for existingID, candidate := range idx.Versions {
			if candidate.CharacterID == item.CharacterID && candidate.StateID == item.StateID && candidate.DeletedAt == "" && candidate.VersionNumber > newest.VersionNumber {
				newest = candidate
				newest.ID = existingID
			}
		}
		if newest.ID != "" {
			newest.Current = true
			idx.Versions[newest.ID] = newest
			idx.Current[key] = newest.ID
		}
	}
	if err := s.writeIndex(idx); err != nil {
		return item, err
	}
	return item, nil
}

func (s *AnimationVersionStore) Purge(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.readIndex()
	if err != nil {
		return err
	}
	item, ok := idx.Versions[id]
	if !ok {
		return errors.New("animation version not found")
	}
	if item.DeletedAt == "" {
		return errors.New("only trashed versions can be permanently deleted")
	}
	if err := os.RemoveAll(s.versionDir(item)); err != nil {
		return err
	}
	delete(idx.Versions, id)
	for key, value := range idx.SourceKeys {
		if value == id {
			delete(idx.SourceKeys, key)
		}
	}
	return s.writeIndex(idx)
}

func (s *AnimationVersionStore) Stats() (AnimationVersionStorageStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.readIndex()
	if err != nil {
		return AnimationVersionStorageStats{}, err
	}
	var out AnimationVersionStorageStats
	for _, item := range idx.Versions {
		if item.DeletedAt == "" {
			out.VersionCount++
			out.VersionBytes += item.SizeBytes
		} else {
			out.TrashCount++
			out.TrashBytes += item.SizeBytes
		}
	}
	out.TotalBytes = out.VersionBytes + out.TrashBytes
	settings := config.Load()
	warningGB := settings.AnimationVersionWarningGB
	if warningGB <= 0 {
		warningGB = 5
	}
	out.WarningBytes = int64(warningGB) << 30
	if free, err := diskFreeBytes(s.root); err == nil {
		out.FreeBytes = free
	}
	warnings := make([]string, 0, 2)
	if out.TotalBytes >= out.WarningBytes {
		warnings = append(warnings, fmt.Sprintf("版本库已达到 %dGB 容量提醒阈值。", warningGB))
	}
	if out.FreeBytes > 0 && out.FreeBytes < 10<<30 {
		warnings = append(warnings, "磁盘剩余空间不足 10GB。")
	}
	out.Warning = strings.Join(warnings, " ")
	return out, nil
}

func (a *App) CommitAnimationVersion(args CommitAnimationVersionArgs) (AnimationVersion, error) {
	if a.animationVersions == nil {
		return AnimationVersion{}, errors.New("animation version store is unavailable")
	}
	return a.animationVersions.Commit(args)
}
func (a *App) ListAnimationVersions(characterID, stateID string, includeDeleted bool) ([]AnimationVersionSummary, error) {
	return a.animationVersions.List(characterID, stateID, includeDeleted)
}
func (a *App) GetAnimationVersion(id string) (AnimationVersion, error) {
	return a.animationVersions.Get(id)
}
func (a *App) SetCurrentAnimationVersion(characterID, stateID, id string) (AnimationVersion, error) {
	return a.animationVersions.SetCurrent(characterID, stateID, id)
}
func (a *App) UpdateAnimationVersionComposition(id string, frames []AnimationVersionFrame) (AnimationVersion, error) {
	return a.animationVersions.UpdateComposition(id, frames)
}
func (a *App) SoftDeleteAnimationVersion(id string) (AnimationVersionSummary, error) {
	return a.animationVersions.move(id, false)
}
func (a *App) RestoreAnimationVersion(id string) (AnimationVersionSummary, error) {
	return a.animationVersions.move(id, true)
}
func (a *App) PurgeAnimationVersion(id string) error { return a.animationVersions.Purge(id) }
func (a *App) GetAnimationVersionStorageStats() (AnimationVersionStorageStats, error) {
	return a.animationVersions.Stats()
}
