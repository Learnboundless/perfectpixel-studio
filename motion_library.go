package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"perfectpixel/internal/config"
	"perfectpixel/internal/sprite"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	motionSourceModeNone            = "none"
	motionSourceModeDirect          = "direct"
	motionSourceModeGuideImageStrip = "guide_image_strip"
	motionSourceModeAIVideo         = "ai_video"

	candidateSourceMotionRaw       = "motion_library_raw"
	candidateSourceMotionProcessed = "motion_library_processed"
	candidateSourceVideoRaw        = "video_raw"
	candidateSourceVideoProcessed  = "video_processed"
	candidateSourceImageRaw        = "image_strip_raw"
	candidateSourceImageProcessed  = "image_strip_processed"
)

type GenerationReference struct {
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	Source  string `json:"source"`
	ID      string `json:"id,omitempty"`
	Path    string `json:"path,omitempty"`
	Index   int    `json:"index,omitempty"`
	Preview string `json:"preview,omitempty"`
}

type GenerationAttempt struct {
	Index      int                   `json:"index"`
	Pipeline   string                `json:"pipeline"`
	Provider   string                `json:"provider,omitempty"`
	Model      string                `json:"model,omitempty"`
	Prompt     string                `json:"prompt"`
	References []GenerationReference `json:"references,omitempty"`
	Parameters map[string]string     `json:"parameters,omitempty"`
}

type GenerationMediaSlotTrace struct {
	Slot  string `json:"slot"`
	Field string `json:"field"`
	Role  string `json:"role,omitempty"`
	Count int    `json:"count"`
}

type GenerationTrace struct {
	Pipeline                  string                     `json:"pipeline"`
	Provider                  string                     `json:"provider,omitempty"`
	ImageProvider             string                     `json:"imageProvider,omitempty"`
	VideoProvider             string                     `json:"videoProvider,omitempty"`
	Model                     string                     `json:"model,omitempty"`
	VideoModel                string                     `json:"videoModel,omitempty"`
	MotionSourceMode          string                     `json:"motionSourceMode"`
	MotionSourceID            string                     `json:"motionSourceId,omitempty"`
	MotionSourceName          string                     `json:"motionSourceName,omitempty"`
	VideoReferenceMode        string                     `json:"videoReferenceMode,omitempty"`
	VideoSubjectMode          string                     `json:"videoSubjectMode,omitempty"`
	ActualVideoReferenceMode  string                     `json:"actualVideoReferenceMode,omitempty"`
	VideoReferenceField       string                     `json:"videoReferenceField,omitempty"`
	VideoWorkflow             string                     `json:"videoWorkflow,omitempty"`
	VideoRequestRoute         string                     `json:"videoRequestRoute,omitempty"`
	VideoMediaSlots           []GenerationMediaSlotTrace `json:"videoMediaSlots,omitempty"`
	FrameSamplerMode          string                     `json:"frameSamplerMode,omitempty"`
	AlignmentMode             string                     `json:"alignmentMode,omitempty"`
	BackgroundRemovalMode     string                     `json:"backgroundRemovalMode,omitempty"`
	BackgroundRemovalModel    string                     `json:"backgroundRemovalModel,omitempty"`
	BackgroundRemovalSHA256   string                     `json:"backgroundRemovalSha256,omitempty"`
	BackgroundRemovalMs       int64                      `json:"backgroundRemovalMs,omitempty"`
	BackgroundRemovalFallback string                     `json:"backgroundRemovalFallback,omitempty"`
	Prompt                    string                     `json:"prompt,omitempty"`
	CalledAI                  bool                       `json:"calledAi"`
	Parameters                map[string]string          `json:"parameters,omitempty"`
	References                []GenerationReference      `json:"references,omitempty"`
	Attempts                  []GenerationAttempt        `json:"attempts,omitempty"`
	Selections                []FrameSelectionTrace      `json:"selections,omitempty"`
	StartedAt                 string                     `json:"startedAt,omitempty"`
	CompletedAt               string                     `json:"completedAt,omitempty"`
}

type FrameSelectionTrace struct {
	TargetFrameIndex int     `json:"targetFrameIndex"`
	CandidateID      string  `json:"candidateId"`
	Source           string  `json:"source"`
	TimeMs           int     `json:"timeMs,omitempty"`
	Quality          float64 `json:"quality,omitempty"`
	Reason           string  `json:"reason,omitempty"`
	SelectedAt       string  `json:"selectedAt,omitempty"`
}

type CandidateFrame struct {
	ID            string  `json:"id"`
	Index         int     `json:"index"`
	PNG           string  `json:"png,omitempty"`
	Source        string  `json:"source"`
	Label         string  `json:"label"`
	TimeMs        int     `json:"timeMs,omitempty"`
	SourcePath    string  `json:"sourcePath,omitempty"`
	Quality       float64 `json:"quality,omitempty"`
	Motion        float64 `json:"motion,omitempty"`
	Similarity    float64 `json:"similarity,omitempty"`
	Continuity    float64 `json:"continuity,omitempty"`
	NearDuplicate bool    `json:"nearDuplicate,omitempty"`
	Fingerprint   string  `json:"fingerprint,omitempty"`
	Reason        string  `json:"reason,omitempty"`
	Selected      bool    `json:"selected,omitempty"`
}

type MotionLibraryItem struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	Action           string           `json:"action"`
	Facing           string           `json:"facing,omitempty"`
	Frames           int              `json:"frames"`
	FPS              int              `json:"fps"`
	Loop             bool             `json:"loop"`
	AssetUse         string           `json:"assetUse,omitempty"`
	SourceKind       string           `json:"sourceKind"`
	SourcePath       string           `json:"sourcePath,omitempty"`
	SourceFPS        int              `json:"sourceFps,omitempty"`
	SourceDurationMs int              `json:"sourceDurationMs,omitempty"`
	RawFrames        []string         `json:"rawFrames,omitempty"`
	ProcessedFrames  []string         `json:"processedFrames,omitempty"`
	CandidateFrames  []CandidateFrame `json:"candidateFrames,omitempty"`
	Strip            string           `json:"strip,omitempty"`
	Thumbnail        string           `json:"thumbnail,omitempty"`
	Tags             []string         `json:"tags,omitempty"`
	DirectUsable     bool             `json:"directUsable"`
	ReferenceOnly    bool             `json:"referenceOnly"`
	AlignmentMode    string           `json:"alignmentMode,omitempty"`
	CreatedAt        string           `json:"createdAt"`
	UpdatedAt        string           `json:"updatedAt"`
}

type SaveMotionLibraryItemArgs struct {
	ID               string           `json:"id,omitempty"`
	Name             string           `json:"name"`
	Action           string           `json:"action"`
	Facing           string           `json:"facing,omitempty"`
	Frames           int              `json:"frames"`
	FPS              int              `json:"fps"`
	Loop             bool             `json:"loop"`
	AssetUse         string           `json:"assetUse,omitempty"`
	SourceKind       string           `json:"sourceKind"`
	SourcePath       string           `json:"sourcePath,omitempty"`
	SourceFPS        int              `json:"sourceFps,omitempty"`
	SourceDurationMs int              `json:"sourceDurationMs,omitempty"`
	RawFrames        []string         `json:"rawFrames,omitempty"`
	ProcessedFrames  []string         `json:"processedFrames,omitempty"`
	CandidateFrames  []CandidateFrame `json:"candidateFrames,omitempty"`
	Strip            string           `json:"strip,omitempty"`
	Thumbnail        string           `json:"thumbnail,omitempty"`
	Tags             []string         `json:"tags,omitempty"`
	DirectUsable     bool             `json:"directUsable"`
	ReferenceOnly    bool             `json:"referenceOnly"`
	AlignmentMode    string           `json:"alignmentMode,omitempty"`
}

type ImportMotionLibraryArgs struct {
	Name       string   `json:"name"`
	Action     string   `json:"action"`
	Facing     string   `json:"facing,omitempty"`
	Frames     int      `json:"frames"`
	FPS        int      `json:"fps"`
	Loop       bool     `json:"loop"`
	CellSize   int      `json:"cellSize"`
	Seconds    int      `json:"seconds"`
	Tags       []string `json:"tags,omitempty"`
	SourcePath string   `json:"sourcePath,omitempty"`
}

type motionLibraryStore struct {
	Items []MotionLibraryItem `json:"items"`
}

type preparedMotionSource struct {
	Item                MotionLibraryItem
	RawFrameURLs        []string
	ProcessedFrameURLs  []string
	ProcessedFrames     []*image.NRGBA
	Strip               string
	Warnings            []string
	Candidates          []CandidateFrame
	ReferenceStripBytes []byte
}

func normalizeMotionSourceMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case motionSourceModeDirect, motionSourceModeGuideImageStrip, motionSourceModeAIVideo:
		return strings.TrimSpace(mode)
	default:
		return motionSourceModeNone
	}
}

func motionLibraryDir() (string, error) {
	dir, err := config.MotionLibraryDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func motionLibraryIndexPath() (string, error) {
	dir, err := motionLibraryDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "index.json"), nil
}

func readMotionLibraryStore() (motionLibraryStore, error) {
	path, err := motionLibraryIndexPath()
	if err != nil {
		return motionLibraryStore{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return motionLibraryStore{}, nil
		}
		return motionLibraryStore{}, err
	}
	var store motionLibraryStore
	if err := json.Unmarshal(data, &store); err != nil {
		return motionLibraryStore{}, fmt.Errorf("动作库索引损坏: %w", err)
	}
	return store, nil
}

func writeMotionLibraryStore(store motionLibraryStore) error {
	path, err := motionLibraryIndexPath()
	if err != nil {
		return err
	}
	sort.SliceStable(store.Items, func(i, j int) bool {
		return store.Items[i].UpdatedAt > store.Items[j].UpdatedAt
	})
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func newMotionLibraryID() string {
	return "motion-" + galleryStamp()
}

func (a *App) ListMotionLibrary() ([]MotionLibraryItem, error) {
	store, err := readMotionLibraryStore()
	if err != nil {
		return nil, err
	}
	sort.SliceStable(store.Items, func(i, j int) bool {
		return store.Items[i].UpdatedAt > store.Items[j].UpdatedAt
	})
	return store.Items, nil
}

func (a *App) GetMotionLibraryItem(id string) (MotionLibraryItem, error) {
	return getMotionLibraryItem(id)
}

func getMotionLibraryItem(id string) (MotionLibraryItem, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return MotionLibraryItem{}, errors.New("请选择一个动作库条目")
	}
	store, err := readMotionLibraryStore()
	if err != nil {
		return MotionLibraryItem{}, err
	}
	for _, item := range store.Items {
		if item.ID == id {
			return item, nil
		}
	}
	return MotionLibraryItem{}, errors.New("未找到动作库条目")
}

func (a *App) DeleteMotionLibraryItem(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("请选择一个动作库条目")
	}
	store, err := readMotionLibraryStore()
	if err != nil {
		return err
	}
	next := store.Items[:0]
	found := false
	for _, item := range store.Items {
		if item.ID == id {
			found = true
			continue
		}
		next = append(next, item)
	}
	if !found {
		return errors.New("未找到动作库条目")
	}
	store.Items = next
	return writeMotionLibraryStore(store)
}

func (a *App) SaveMotionLibraryItem(args SaveMotionLibraryItemArgs) (MotionLibraryItem, error) {
	if strings.HasPrefix(strings.TrimSpace(args.SourcePath), animationVersionResourcePrefix) {
		path, err := a.animationVersions.ResolveResource(args.SourcePath)
		if err != nil {
			return MotionLibraryItem{}, fmt.Errorf("无法读取版本源素材: %w", err)
		}
		args.SourcePath = copyMotionLibrarySource(path)
	}
	for i, value := range args.RawFrames {
		if !strings.HasPrefix(strings.TrimSpace(value), animationVersionResourcePrefix) {
			continue
		}
		loaded, err := a.LoadAnimationVersionResource(value, 0)
		if err != nil {
			return MotionLibraryItem{}, fmt.Errorf("无法读取版本原始帧 %d: %w", i+1, err)
		}
		args.RawFrames[i] = loaded
	}
	for i := range args.CandidateFrames {
		if !strings.HasPrefix(strings.TrimSpace(args.CandidateFrames[i].SourcePath), animationVersionResourcePrefix) {
			continue
		}
		loaded, err := a.LoadAnimationVersionResource(args.CandidateFrames[i].SourcePath, 0)
		if err != nil {
			return MotionLibraryItem{}, fmt.Errorf("无法读取版本候选帧 %d: %w", i+1, err)
		}
		args.CandidateFrames[i].PNG = loaded
		args.CandidateFrames[i].SourcePath = ""
	}
	now := time.Now().Format(time.RFC3339)
	id := strings.TrimSpace(args.ID)
	if id == "" {
		id = newMotionLibraryID()
	}
	name := strings.TrimSpace(args.Name)
	if name == "" {
		name = strings.TrimSpace(args.Action)
	}
	if name == "" {
		name = "未命名动作"
	}
	item := MotionLibraryItem{
		ID:               id,
		Name:             name,
		Action:           strings.TrimSpace(args.Action),
		Facing:           strings.TrimSpace(args.Facing),
		Frames:           args.Frames,
		FPS:              args.FPS,
		Loop:             args.Loop,
		AssetUse:         strings.TrimSpace(args.AssetUse),
		SourceKind:       strings.TrimSpace(args.SourceKind),
		SourcePath:       strings.TrimSpace(args.SourcePath),
		SourceFPS:        args.SourceFPS,
		SourceDurationMs: args.SourceDurationMs,
		RawFrames:        compactStrings(args.RawFrames),
		ProcessedFrames:  compactStrings(args.ProcessedFrames),
		CandidateFrames:  compactCandidateFrames(args.CandidateFrames),
		Strip:            strings.TrimSpace(args.Strip),
		Thumbnail:        strings.TrimSpace(args.Thumbnail),
		Tags:             compactStrings(args.Tags),
		DirectUsable:     args.DirectUsable,
		ReferenceOnly:    args.ReferenceOnly,
		AlignmentMode:    sprite.NormalizeAlignmentMode(args.AlignmentMode),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if item.SourceKind == "" {
		item.SourceKind = "result"
	}
	if item.AssetUse == "" {
		if item.ReferenceOnly {
			item.AssetUse = "reference"
		} else {
			item.AssetUse = "direct"
		}
	}
	if item.Frames <= 0 {
		item.Frames = len(item.ProcessedFrames)
	}
	if item.Frames <= 0 {
		item.Frames = len(item.RawFrames)
	}
	if item.FPS <= 0 {
		item.FPS = 8
	}
	if item.SourceFPS <= 0 {
		item.SourceFPS = item.FPS
	}
	if item.SourceDurationMs <= 0 && item.Frames > 0 && item.FPS > 0 {
		item.SourceDurationMs = int(float64(item.Frames) / float64(item.FPS) * 1000)
	}
	if item.Strip == "" && len(item.ProcessedFrames) > 0 {
		if frames, err := decodeFrameURLs(item.ProcessedFrames); err == nil {
			if strip := composeStrip(frames); strip != nil {
				item.Strip, _ = pngDataURL(strip)
			}
		}
	}
	if item.Thumbnail == "" {
		if len(item.ProcessedFrames) > 0 {
			item.Thumbnail = item.ProcessedFrames[0]
		} else if len(item.RawFrames) > 0 {
			item.Thumbnail = item.RawFrames[0]
		} else if len(item.CandidateFrames) > 0 {
			item.Thumbnail = item.CandidateFrames[0].PNG
		}
	}
	if len(item.ProcessedFrames) == 0 && item.Strip == "" && item.SourcePath == "" {
		return MotionLibraryItem{}, errors.New("动作库条目至少需要最终帧、条带或源文件路径")
	}

	store, err := readMotionLibraryStore()
	if err != nil {
		return MotionLibraryItem{}, err
	}
	replaced := false
	for i, old := range store.Items {
		if old.ID == item.ID {
			if old.CreatedAt != "" {
				item.CreatedAt = old.CreatedAt
			}
			store.Items[i] = item
			replaced = true
			break
		}
	}
	if !replaced {
		store.Items = append(store.Items, item)
	}
	if err := writeMotionLibraryStore(store); err != nil {
		return MotionLibraryItem{}, err
	}
	return item, nil
}

func (a *App) ImportMotionLibraryFile(args ImportMotionLibraryArgs) (MotionLibraryItem, error) {
	path := strings.TrimSpace(args.SourcePath)
	if path == "" {
		picked, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
			Title: "导入动作库素材",
			Filters: []wailsruntime.FileFilter{
				{DisplayName: "动作素材 (*.mp4;*.webm;*.mov;*.gif;*.png;*.jpg;*.jpeg;*.webp)", Pattern: "*.mp4;*.webm;*.mov;*.gif;*.png;*.jpg;*.jpeg;*.webp"},
			},
		})
		if err != nil {
			return MotionLibraryItem{}, err
		}
		if picked == "" {
			return MotionLibraryItem{}, nil
		}
		path = picked
	}
	defer a.emitProgress("idle", "", "", -1, 0, 0)
	a.emitProgress("motion_import_prepare", "", "正在准备导入动作库素材...", 8, 1, 5)
	expected := args.Frames
	if expected <= 0 {
		expected = 6
	}
	cellSize := args.CellSize
	if cellSize <= 0 {
		cellSize = 256
	}
	margin := max(8, cellSize/12)
	fps := args.FPS
	if fps <= 0 {
		fps = 8
	}

	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ext := strings.ToLower(filepath.Ext(path))
	var sourceKind string
	var sourcePath string
	var rawFrames []*image.NRGBA
	var candidateFrames []CandidateFrame
	var sourceDurationMs int
	switch ext {
	case ".mp4", ".webm", ".mov":
		a.emitProgress("motion_import_read", "", "正在读取动作视频...", 20, 2, 5)
		data, err := os.ReadFile(path)
		if err != nil {
			return MotionLibraryItem{}, fmt.Errorf("无法读取视频: %w", err)
		}
		seconds := args.Seconds
		if seconds <= 0 {
			seconds = max(1, expected/max(1, fps))
		}
		a.emitProgress("motion_import_extract", "", "正在从动作视频抽取候选帧...", 38, 3, 5)
		extracted, err := sprite.ExtractVideoFramesWithSamplerForLoop(ctx, data, expected, seconds, sprite.FrameSamplerCandidatePool, args.Loop)
		if err != nil {
			return MotionLibraryItem{}, err
		}
		rawFrames = extracted.Frames
		sourceDurationMs = seconds * 1000
		candidateFrames = candidateFramesFromImages(extracted.Candidates, extracted.CandidateMeta, candidateSourceMotionRaw, "动作库视频候选帧")
		sourceKind = "video"
		sourcePath = copyMotionLibrarySource(path)
	case ".gif":
		a.emitProgress("motion_import_extract", "", "正在解码 GIF 动作帧...", 38, 3, 5)
		frames, err := decodeGIFFrames(path)
		if err != nil {
			return MotionLibraryItem{}, err
		}
		candidateFrames = candidateFramesFromImages(frames, nil, candidateSourceMotionRaw, "动作库 GIF 候选帧")
		rawFrames = sampleNRGBAFrames(frames, expected)
		sourceKind = "gif"
		sourcePath = copyMotionLibrarySource(path)
	default:
		a.emitProgress("motion_import_extract", "", "正在解析图片条带...", 38, 3, 5)
		data, err := os.ReadFile(path)
		if err != nil {
			return MotionLibraryItem{}, fmt.Errorf("无法读取图片: %w", err)
		}
		img, err := decodeImage(data)
		if err != nil {
			return MotionLibraryItem{}, err
		}
		nimg := sprite.ToNRGBA(img)
		remover, _ := appBackgroundRemover("inherit")
		clean, _ := remover.Remove(nimg)
		extracted := sprite.ExtractFrames(clean, expected, cellSize, cellSize, margin)
		rawFrames = extracted.Frames
		if len(rawFrames) == 0 {
			rawFrames = []*image.NRGBA{clean}
		}
		candidateFrames = candidateFramesFromImages(rawFrames, nil, candidateSourceMotionRaw, "动作库图片候选帧")
		sourceKind = "image_strip"
		sourcePath = copyMotionLibrarySource(path)
	}
	if len(rawFrames) == 0 {
		return MotionLibraryItem{}, errors.New("没有从素材中提取到可用帧")
	}
	rawFrames = sampleNRGBAFrames(rawFrames, expected)
	rawURLs := frameURLs(rawFrames)
	a.emitProgress("motion_import_process", "", "正在像素化并统一动作帧...", 70, 4, 5)
	processed := sprite.PrepareVideoFramesWithAlignment(rawFrames, expected, cellSize, cellSize, margin, sprite.AlignmentPreserveSourceCanvas)
	if len(processed.Frames) == 0 {
		return MotionLibraryItem{}, errors.New("素材帧处理后为空，请检查背景或角色位置")
	}
	if args.Loop {
		sprite.AlignLoopFrameAnchors(processed.Frames)
	}
	sprite.PixelPostProcess(processed.Frames, sprite.PaletteSizeForStyle("pixel"))
	processedURLs := frameURLs(processed.Frames)
	strip := ""
	if composed := composeStrip(processed.Frames); composed != nil {
		strip, _ = pngDataURL(composed)
	}
	name := strings.TrimSpace(args.Name)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	a.emitProgress("motion_import_save", "", "正在保存动作库条目...", 92, 5, 5)
	item, err := a.SaveMotionLibraryItem(SaveMotionLibraryItemArgs{
		Name:             name,
		Action:           args.Action,
		Facing:           args.Facing,
		Frames:           expected,
		FPS:              fps,
		Loop:             args.Loop,
		AssetUse:         "direct",
		SourceKind:       sourceKind,
		SourcePath:       sourcePath,
		SourceFPS:        fps,
		SourceDurationMs: sourceDurationMs,
		RawFrames:        rawURLs,
		ProcessedFrames:  processedURLs,
		CandidateFrames:  candidateFrames,
		Strip:            strip,
		Thumbnail:        firstNonEmpty(processedURLs, rawURLs),
		Tags:             args.Tags,
		DirectUsable:     true,
		ReferenceOnly:    false,
		AlignmentMode:    sprite.AlignmentPreserveSourceCanvas,
	})
	if err != nil {
		return MotionLibraryItem{}, err
	}
	a.emitProgress("motion_import_done", "", "动作库导入完成。", 100, 5, 5)
	return item, nil
}

func (a *App) ImportMotionLibraryFolder(args ImportMotionLibraryArgs) (MotionLibraryItem, error) {
	path := strings.TrimSpace(args.SourcePath)
	if path == "" {
		picked, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "选择图片序列文件夹"})
		if err != nil {
			return MotionLibraryItem{}, err
		}
		if picked == "" {
			return MotionLibraryItem{}, nil
		}
		path = picked
	}
	defer a.emitProgress("idle", "", "", -1, 0, 0)
	a.emitProgress("motion_import_prepare", "", "正在读取图片序列文件夹...", 10, 1, 5)
	entries, err := os.ReadDir(path)
	if err != nil {
		return MotionLibraryItem{}, fmt.Errorf("无法读取图片序列文件夹: %w", err)
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	a.emitProgress("motion_import_extract", "", "正在解码图片序列帧...", 32, 2, 5)
	var frames []*image.NRGBA
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if imageExtMime[ext] == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(path, entry.Name()))
		if err != nil {
			continue
		}
		img, err := decodeImage(data)
		if err != nil {
			continue
		}
		frames = append(frames, sprite.ToNRGBA(img))
		if len(frames) >= 100 {
			break
		}
	}
	if len(frames) == 0 {
		return MotionLibraryItem{}, errors.New("文件夹中没有可导入的图片帧")
	}
	expected := args.Frames
	if expected <= 0 {
		expected = len(frames)
	}
	args.SourcePath = ""
	rawFrames := sampleNRGBAFrames(frames, expected)
	cellSize := args.CellSize
	if cellSize <= 0 {
		cellSize = 256
	}
	margin := max(8, cellSize/12)
	a.emitProgress("motion_import_process", "", "正在对齐并像素化图片序列...", 70, 4, 5)
	processed := sprite.PrepareVideoFramesWithAlignment(rawFrames, expected, cellSize, cellSize, margin, sprite.AlignmentPreserveSourceCanvas)
	if args.Loop {
		sprite.AlignLoopFrameAnchors(processed.Frames)
	}
	sprite.PixelPostProcess(processed.Frames, sprite.PaletteSizeForStyle("pixel"))
	name := strings.TrimSpace(args.Name)
	if name == "" {
		name = filepath.Base(path)
	}
	strip := ""
	if composed := composeStrip(processed.Frames); composed != nil {
		strip, _ = pngDataURL(composed)
	}
	a.emitProgress("motion_import_save", "", "正在保存动作库条目...", 92, 5, 5)
	item, err := a.SaveMotionLibraryItem(SaveMotionLibraryItemArgs{
		Name:             name,
		Action:           args.Action,
		Facing:           args.Facing,
		Frames:           expected,
		FPS:              max(1, args.FPS),
		Loop:             args.Loop,
		AssetUse:         "direct",
		SourceKind:       "image_sequence",
		SourcePath:       path,
		SourceFPS:        max(1, args.FPS),
		SourceDurationMs: int(float64(expected) / float64(max(1, args.FPS)) * 1000),
		RawFrames:        frameURLs(rawFrames),
		ProcessedFrames:  frameURLs(processed.Frames),
		CandidateFrames:  candidateFramesFromImages(frames, nil, candidateSourceMotionRaw, "动作库序列候选帧"),
		Strip:            strip,
		Thumbnail:        firstNonEmpty(frameURLs(processed.Frames), frameURLs(rawFrames)),
		Tags:             args.Tags,
		DirectUsable:     true,
		AlignmentMode:    sprite.AlignmentPreserveSourceCanvas,
	})
	if err != nil {
		return MotionLibraryItem{}, err
	}
	a.emitProgress("motion_import_done", "", "动作库导入完成。", 100, 5, 5)
	return item, nil
}

func (a *App) prepareMotionSource(ctx context.Context, id string, expected, cellSize, margin int, styleKey, frameFitMode string, loop bool) (preparedMotionSource, error) {
	item, err := getMotionLibraryItem(id)
	if err != nil {
		return preparedMotionSource{}, err
	}
	var rawFrames []*image.NRGBA
	var rawURLs []string
	var warnings []string
	if len(item.ProcessedFrames) > 0 {
		frames, err := decodeFrameURLs(item.ProcessedFrames)
		if err == nil {
			rawFrames = frames
			rawURLs = item.ProcessedFrames
		}
	}
	if len(rawFrames) == 0 && len(item.RawFrames) > 0 {
		frames, err := decodeFrameURLs(item.RawFrames)
		if err == nil {
			rawFrames = frames
			rawURLs = item.RawFrames
		}
	}
	if len(rawFrames) == 0 && strings.TrimSpace(item.Strip) != "" {
		raw, err := decodeDataURL(item.Strip)
		if err != nil {
			return preparedMotionSource{}, err
		}
		img, err := decodeImage(raw)
		if err != nil {
			return preparedMotionSource{}, err
		}
		remover, _ := appBackgroundRemover("inherit")
		clean, _ := remover.Remove(sprite.ToNRGBA(img))
		extracted := sprite.ExtractFramesWithFitMode(clean, expected, cellSize, cellSize, margin, frameFitMode)
		rawFrames = extracted.Frames
		rawURLs = frameURLs(rawFrames)
		warnings = append(warnings, extracted.Warnings...)
	}
	if len(rawFrames) == 0 && item.SourcePath != "" {
		frames, urls, err := loadMotionFramesFromSource(ctx, item, expected)
		if err != nil {
			return preparedMotionSource{}, err
		}
		rawFrames, rawURLs = frames, urls
	}
	if len(rawFrames) == 0 {
		return preparedMotionSource{}, errors.New("动作库条目没有可用帧")
	}
	rawFrames = sampleNRGBAFrames(rawFrames, expected)
	if len(rawURLs) != len(rawFrames) {
		rawURLs = frameURLs(rawFrames)
	}
	remover, _ := appBackgroundRemover("inherit")
	processed, _, err := sprite.PrepareVideoFramesWithBackgroundRemoverContext(ctx, rawFrames, expected, cellSize, cellSize, margin, sprite.AlignmentPreserveSourceCanvas, frameFitMode, remover, 2, nil)
	if err != nil {
		return preparedMotionSource{}, friendlyErr(err)
	}
	if len(processed.Frames) == 0 {
		return preparedMotionSource{}, errors.New("动作库帧处理后为空")
	}
	warnings = append(warnings, processed.Warnings...)
	if loop {
		sprite.AlignLoopFrameAnchors(processed.Frames)
	}
	sprite.PixelPostProcess(processed.Frames, sprite.PaletteSizeForStyle(styleKey))
	processedURLs := frameURLs(processed.Frames)
	strip := item.Strip
	if strings.TrimSpace(strip) == "" {
		if composed := composeStrip(processed.Frames); composed != nil {
			strip, _ = pngDataURL(composed)
		}
	}
	var stripBytes []byte
	if strip != "" {
		stripBytes, _ = decodeDataURL(strip)
	}
	candidates := candidateFramesFromURLs(rawURLs, candidateSourceMotionRaw, "动作库源帧")
	candidates = append(candidates, item.CandidateFrames...)
	candidates = append(candidates, candidateFramesFromURLs(processedURLs, candidateSourceMotionProcessed, "动作库处理帧")...)
	return preparedMotionSource{
		Item:                item,
		RawFrameURLs:        rawURLs,
		ProcessedFrameURLs:  processedURLs,
		ProcessedFrames:     processed.Frames,
		Strip:               strip,
		Warnings:            warnings,
		Candidates:          candidates,
		ReferenceStripBytes: stripBytes,
	}, nil
}

func (a *App) generateMotionLibraryDirectState(args GenerateStateArgs, baseN *image.NRGBA, styleKey string, cellSize, margin int, trace GenerationTrace) (StateResult, error) {
	res := StateResult{RequestID: args.RequestID, Name: args.State.Name, Pipeline: pipelineImageStrip, Expected: args.State.Frames, Trace: trace}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	a.emitProgress("motion_library_prepare", args.State.Name, "正在读取动作库素材...", 12, 1, 4)
	prepared, err := a.prepareMotionSource(ctx, args.MotionSourceID, args.State.Frames, cellSize, margin, styleKey, args.FrameFitMode, args.State.Loop)
	if err != nil {
		return res, err
	}
	a.emitProgress("motion_library_process", args.State.Name, "正在处理动作库帧...", 58, 2, 4)
	res.Trace.MotionSourceID = prepared.Item.ID
	res.Trace.MotionSourceName = prepared.Item.Name
	res.Trace.CalledAI = false
	res.Trace.AlignmentMode = sprite.AlignmentPreserveSourceCanvas
	res.Trace.FrameSamplerMode = sprite.FrameSamplerCandidatePool
	if args.State.Loop {
		res.Trace.Parameters["loopClosure"] = "foot_anchor_distributed"
	}
	res.Trace.References = append(res.Trace.References, GenerationReference{
		Kind:   "motion_library",
		Label:  prepared.Item.Name,
		Source: prepared.Item.SourceKind,
		ID:     prepared.Item.ID,
		Path:   prepared.Item.SourcePath,
	})
	res.Trace.Prompt = ""
	res.CandidateFrames = prepared.Candidates
	res.RawExtractedFrames = prepared.RawFrameURLs
	res.RawStrip = prepared.Strip
	res.Found = len(prepared.ProcessedFrames)
	if len(prepared.ProcessedFrames) > 0 {
		res.CellSize = prepared.ProcessedFrames[0].Rect.Dx()
	}
	res.Warnings = append(res.Warnings, prepared.Warnings...)
	bgKey := [3]uint8{255, 0, 255}
	a.emitProgress("motion_library_inspect", args.State.Name, "正在质检动作库帧...", 82, 3, 4)
	insp := sprite.InspectFrames(prepared.ProcessedFrames, bgKey, baseN)
	res.Warnings = append(res.Warnings, insp.Errors...)
	res.Warnings = append(res.Warnings, insp.Warnings...)
	res.Scores = sprite.ScoreFrames(prepared.ProcessedFrames)
	if res.Found >= 2 && sprite.MotionPresence(prepared.ProcessedFrames) < 0.01 {
		res.Warnings = append(res.Warnings, "动作库帧之间运动量很低，请确认源素材是否就是静态动作。")
	}
	res.Frames = prepared.ProcessedFrameURLs
	res.Trace.CompletedAt = time.Now().Format(time.RFC3339)
	a.emitProgress("motion_library_save", args.State.Name, "正在保存动作库处理结果...", 96, 4, 4)
	saveGalleryFrames(args.State.Name, prepared.ProcessedFrames, args.State.FPS, args.State.Loop)
	return res, nil
}

func loadMotionFramesFromSource(ctx context.Context, item MotionLibraryItem, expected int) ([]*image.NRGBA, []string, error) {
	path := strings.TrimSpace(item.SourcePath)
	if path == "" {
		return nil, nil, errors.New("动作库源路径为空")
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp4", ".webm", ".mov":
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, err
		}
		seconds := max(1, item.Frames/max(1, item.FPS))
		extracted, err := sprite.ExtractVideoFramesWithSamplerForLoop(ctx, data, expected, seconds, sprite.FrameSamplerCandidatePool, item.Loop)
		if err != nil {
			return nil, nil, err
		}
		return extracted.Frames, frameURLs(extracted.Frames), nil
	case ".gif":
		frames, err := decodeGIFFrames(path)
		if err != nil {
			return nil, nil, err
		}
		frames = sampleNRGBAFrames(frames, expected)
		return frames, frameURLs(frames), nil
	default:
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, err
		}
		img, err := decodeImage(data)
		if err != nil {
			return nil, nil, err
		}
		remover, _ := appBackgroundRemover("inherit")
		clean, _ := remover.Remove(sprite.ToNRGBA(img))
		extracted := sprite.ExtractFrames(clean, expected, clean.Bounds().Dy(), clean.Bounds().Dy(), max(8, clean.Bounds().Dy()/12))
		return extracted.Frames, frameURLs(extracted.Frames), nil
	}
}

func decodeGIFFrames(path string) ([]*image.NRGBA, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("无法读取 GIF: %w", err)
	}
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("GIF 解码失败: %w", err)
	}
	var out []*image.NRGBA
	for _, src := range g.Image {
		rect := src.Bounds()
		dst := image.NewNRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
		draw.Draw(dst, dst.Bounds(), src, rect.Min, draw.Over)
		out = append(out, dst)
	}
	return out, nil
}

func decodeFrameURLs(urls []string) ([]*image.NRGBA, error) {
	out := make([]*image.NRGBA, 0, len(urls))
	for i, url := range urls {
		raw, err := decodeDataURL(url)
		if err != nil {
			return nil, fmt.Errorf("帧 %d 解码失败: %w", i+1, err)
		}
		img, err := decodeImage(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, sprite.ToNRGBA(img))
	}
	return out, nil
}

func frameURLs(frames []*image.NRGBA) []string {
	out := make([]string, 0, len(frames))
	for _, frame := range frames {
		if url, err := pngDataURL(frame); err == nil {
			out = append(out, url)
		}
	}
	return out
}

func candidateFramesFromURLs(urls []string, source, label string) []CandidateFrame {
	return candidateFramesFromURLsAndMeta(urls, nil, source, label)
}

func candidateFramesFromURLsAndMeta(urls []string, metas []sprite.FrameMeta, source, label string) []CandidateFrame {
	out := make([]CandidateFrame, 0, len(urls))
	for i, url := range urls {
		if strings.TrimSpace(url) == "" {
			continue
		}
		meta := sprite.FrameMeta{Index: i}
		if i < len(metas) {
			meta = metas[i]
		}
		out = append(out, CandidateFrame{
			ID:         fmt.Sprintf("%s-%d-%s", source, i+1, galleryStamp()),
			Index:      meta.Index,
			PNG:        url,
			Source:     source,
			Label:      label,
			TimeMs:     meta.TimeMs,
			SourcePath: meta.SourcePath,
			Quality:    meta.Quality,
			Reason:     meta.Reason,
			Selected:   meta.Selected,
		})
	}
	return out
}

func candidateFramesFromImages(frames []*image.NRGBA, metas []sprite.FrameMeta, source, label string) []CandidateFrame {
	urls := frameURLs(frames)
	out := make([]CandidateFrame, 0, len(urls))
	for i, url := range urls {
		if strings.TrimSpace(url) == "" {
			continue
		}
		meta := sprite.FrameMeta{Index: i}
		if i < len(metas) {
			meta = metas[i]
		}
		out = append(out, CandidateFrame{
			ID:         fmt.Sprintf("%s-%d-%s", source, i+1, galleryStamp()),
			Index:      meta.Index,
			PNG:        url,
			Source:     source,
			Label:      label,
			TimeMs:     meta.TimeMs,
			SourcePath: meta.SourcePath,
			Quality:    meta.Quality,
			Reason:     meta.Reason,
			Selected:   meta.Selected,
		})
	}
	return out
}

func compactCandidateFrames(values []CandidateFrame) []CandidateFrame {
	out := make([]CandidateFrame, 0, len(values))
	seen := map[string]bool{}
	for _, v := range values {
		v.PNG = strings.TrimSpace(v.PNG)
		if v.PNG == "" || seen[v.PNG] {
			continue
		}
		if v.ID == "" {
			v.ID = fmt.Sprintf("%s-%d-%s", v.Source, len(out)+1, galleryStamp())
		}
		if v.Source == "" {
			v.Source = "motion_library"
		}
		if v.Label == "" {
			v.Label = "动作库候选帧"
		}
		seen[v.PNG] = true
		out = append(out, v)
	}
	return out
}

func firstNonEmpty(groups ...[]string) string {
	for _, group := range groups {
		for _, v := range group {
			if strings.TrimSpace(v) != "" {
				return v
			}
		}
	}
	return ""
}

func sampleNRGBAFrames(frames []*image.NRGBA, expected int) []*image.NRGBA {
	if expected <= 0 || len(frames) == 0 {
		return append([]*image.NRGBA(nil), frames...)
	}
	if len(frames) == expected {
		return append([]*image.NRGBA(nil), frames...)
	}
	out := make([]*image.NRGBA, 0, expected)
	if expected == 1 {
		return []*image.NRGBA{frames[0]}
	}
	last := len(frames) - 1
	for i := 0; i < expected; i++ {
		idx := int(float64(i)*float64(last)/float64(expected-1) + 0.5)
		if idx < 0 {
			idx = 0
		}
		if idx > last {
			idx = last
		}
		out = append(out, frames[idx])
	}
	return out
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func copyMotionLibrarySource(path string) string {
	dir, err := motionLibraryDir()
	if err != nil {
		return path
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return path
	}
	ext := strings.ToLower(filepath.Ext(path))
	target := filepath.Join(dir, "sources", sanitizeName(strings.TrimSuffix(filepath.Base(path), ext))+"-"+galleryStamp()+ext)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return path
	}
	if err := os.WriteFile(target, src, 0o600); err != nil {
		return path
	}
	return target
}
