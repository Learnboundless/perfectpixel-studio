package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"perfectpixel/internal/config"
	"perfectpixel/internal/gen"
	"perfectpixel/internal/uiassets"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	maxToolchainPromptBytes = 32 << 10
	maxLocalImageBytes      = 32 << 20
	maxLocalVideoBytes      = 256 << 20
)

type imageJobRequest struct {
	SourceModule      string                  `json:"sourceModule"`
	Purpose           string                  `json:"purpose,omitempty"`
	Prompt            string                  `json:"prompt"`
	NegativePrompt    string                  `json:"negativePrompt,omitempty"`
	ReferenceAssetIDs []string                `json:"referenceAssetIds,omitempty"`
	Mode              string                  `json:"mode,omitempty"`
	MaskRegions       []UIPixelBounds         `json:"maskRegions,omitempty"`
	AspectRatio       string                  `json:"aspectRatio,omitempty"`
	Variants          int                     `json:"variants,omitempty"`
	Provider          string                  `json:"provider,omitempty"`
	Model             string                  `json:"model,omitempty"`
	PostProcess       imagePostProcessRequest `json:"postProcess,omitempty"`
}

type imagePostProcessRequest struct {
	RemoveBackground bool `json:"removeBackground,omitempty"`
	Pixelize         bool `json:"pixelize,omitempty"`
	PixelSize        int  `json:"pixelSize,omitempty"`
}

type videoJobRequest struct {
	SourceModule      string   `json:"sourceModule"`
	Prompt            string   `json:"prompt"`
	NegativePrompt    string   `json:"negativePrompt,omitempty"`
	Mode              string   `json:"mode,omitempty"`
	ReferenceMode     string   `json:"referenceMode,omitempty"`
	ReferenceAssetIDs []string `json:"referenceAssetIds,omitempty"`
	LastFrameAssetID  string   `json:"lastFrameAssetId,omitempty"`
	DurationSeconds   int      `json:"durationSeconds,omitempty"`
	FPS               int      `json:"fps,omitempty"`
	AspectRatio       string   `json:"aspectRatio,omitempty"`
	Resolution        string   `json:"resolution,omitempty"`
	Provider          string   `json:"provider,omitempty"`
	Model             string   `json:"model,omitempty"`
	Audio             bool     `json:"audio,omitempty"`
	Watermark         bool     `json:"watermark,omitempty"`
}

type promptOptimizationJobRequest struct {
	SourceModule string `json:"sourceModule"`
	Purpose      string `json:"purpose"`
	Prompt       string `json:"prompt"`
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
}

type uiAnalysisJobRequest struct {
	SourceModule  string `json:"sourceModule"`
	SourceAssetID string `json:"sourceAssetId"`
	Prompt        string `json:"prompt,omitempty"`
	Provider      string `json:"provider,omitempty"`
	Model         string `json:"model,omitempty"`
}

type uiSplitJobRequest struct {
	SourceModule    string     `json:"sourceModule"`
	SourceAssetID   string     `json:"sourceAssetId"`
	Mode            string     `json:"mode"`
	Rows            int        `json:"rows,omitempty"`
	Columns         int        `json:"columns,omitempty"`
	BackgroundColor string     `json:"backgroundColor,omitempty"`
	Regions         []UIRegion `json:"regions,omitempty"`
	TrimTransparent bool       `json:"trimTransparent,omitempty"`
	Padding         int        `json:"padding,omitempty"`
}

type importAssetsRequest struct {
	Module    string `json:"module"`
	MediaType string `json:"mediaType,omitempty"`
}

type exportUIProjectRequest struct {
	SourceAssetID string     `json:"sourceAssetId"`
	Regions       []UIRegion `json:"regions"`
	Prompt        string     `json:"prompt,omitempty"`
}

type sendAssetRequest struct {
	AssetID      string `json:"assetId"`
	TargetModule string `json:"targetModule"`
	CharacterID  string `json:"characterId,omitempty"`
	ActionName   string `json:"actionName,omitempty"`
	Frames       int    `json:"frames,omitempty"`
	FPS          int    `json:"fps,omitempty"`
	Loop         bool   `json:"loop,omitempty"`
}

type ToolchainModelCapability struct {
	Provider                   string   `json:"provider"`
	Model                      string   `json:"model"`
	Label                      string   `json:"label,omitempty"`
	SupportsText               bool     `json:"supportsText,omitempty"`
	SupportsImageReference     bool     `json:"supportsImageReference,omitempty"`
	SupportsMultipleReferences bool     `json:"supportsMultipleReferences,omitempty"`
	MaxReferenceImages         int      `json:"maxReferenceImages,omitempty"`
	SupportsEdit               bool     `json:"supportsEdit,omitempty"`
	SupportsMask               bool     `json:"supportsMask,omitempty"`
	SupportsFirstFrame         bool     `json:"supportsFirstFrame,omitempty"`
	SupportsLastFrame          bool     `json:"supportsLastFrame,omitempty"`
	SupportsIdentityReference  bool     `json:"supportsIdentityReference,omitempty"`
	SupportsAudio              bool     `json:"supportsAudio,omitempty"`
	SupportsWatermark          bool     `json:"supportsWatermark,omitempty"`
	AspectRatios               []string `json:"aspectRatios,omitempty"`
	Resolutions                []string `json:"resolutions,omitempty"`
	Durations                  []int    `json:"durations,omitempty"`
}

type ToolchainGenerationCapabilities struct {
	Image  []ToolchainModelCapability `json:"image"`
	Video  []ToolchainModelCapability `json:"video"`
	Vision []ToolchainModelCapability `json:"vision"`
	Text   []ToolchainModelCapability `json:"text"`
}

func (a *App) initToolchain(ctx context.Context) error {
	if a.toolchainErr != nil {
		return a.toolchainErr
	}
	if a.toolchainStore == nil {
		store, err := NewToolchainStore()
		if err != nil {
			a.toolchainErr = err
			return err
		}
		a.toolchainStore = store
	}
	if a.toolchainJobs != nil {
		return a.toolchainJobs.Start(ctx)
	}
	manager, err := NewGenerationJobManager(a.toolchainStore, GenerationJobManagerOptions{
		OnEvent: func(event GenerationJobEvent) {
			payload := map[string]any{
				"jobId": event.Job.ID,
				"type":  event.Type,
				"job":   event.Job,
			}
			a.emit("generation-job-updated", payload)
			a.emit("toolchain-job", payload)
		},
	})
	if err != nil {
		a.toolchainErr = err
		return err
	}
	a.toolchainJobs = manager
	if err := a.registerToolchainJobHandlers(); err != nil {
		a.toolchainErr = err
		return err
	}
	if err := manager.Start(ctx); err != nil {
		a.toolchainErr = err
		return err
	}
	if _, err := a.toolchainStore.IndexDefaultLegacyGallery(); err != nil {
		a.emit("toolchain-warning", map[string]any{"message": err.Error()})
	}
	return nil
}

func (a *App) registerToolchainJobHandlers() error {
	if a.toolchainJobs == nil {
		return errors.New("toolchain job manager is unavailable")
	}
	clone := func(job GenerationJob) (GenerationJob, error) {
		job.ProviderTaskID = ""
		job.OutputAssetIDs = nil
		job.ProviderResponseSummary = nil
		return job, nil
	}
	handlers := map[GenerationJobKind]GenerationJobHandler{
		GenerationJobImage:              {Run: a.runImageJob, Clone: clone},
		GenerationJobVideo:              {Run: a.runVideoJob, Resume: a.resumeVideoJob, Clone: clone},
		GenerationJobUIAnalysis:         {Run: a.runUIAnalysisJob, Clone: clone},
		GenerationJobUISplit:            {Run: a.runUISplitJob, Clone: clone},
		GenerationJobPromptOptimization: {Run: a.runPromptOptimizationJob, Clone: clone},
	}
	for kind, handler := range handlers {
		if err := a.toolchainJobs.RegisterHandler(kind, handler); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) toolchainReady() error {
	if a == nil {
		return errors.New("toolchain is unavailable")
	}
	if a.toolchainErr != nil {
		return a.toolchainErr
	}
	if a.toolchainStore == nil || a.toolchainJobs == nil {
		return errors.New("toolchain has not finished starting")
	}
	return nil
}

func (a *App) CreateImageJob(request map[string]any) (GenerationJob, error) {
	if err := a.toolchainReady(); err != nil {
		return GenerationJob{}, err
	}
	var req imageJobRequest
	if err := decodeToolchainRequest(request, &req); err != nil {
		return GenerationJob{}, err
	}
	provider, model, err := resolveImageJobRequest(&req)
	if err != nil {
		return GenerationJob{}, err
	}
	if err := a.validateImageJobRequest(req, provider, model); err != nil {
		return GenerationJob{}, err
	}
	req.Provider, req.Model = provider, model
	snapshot, err := requestSnapshot(req)
	if err != nil {
		return GenerationJob{}, err
	}
	return a.toolchainJobs.CreateJob(GenerationJob{
		Kind:          GenerationJobImage,
		Provider:      provider,
		Model:         model,
		InputAssetIDs: req.ReferenceAssetIDs,
		Request:       snapshot,
		SourceModule:  normalizeToolchainModule(req.SourceModule, ToolchainModuleImage),
	})
}

func (a *App) CreateVideoJob(request map[string]any) (GenerationJob, error) {
	if err := a.toolchainReady(); err != nil {
		return GenerationJob{}, err
	}
	var req videoJobRequest
	if err := decodeToolchainRequest(request, &req); err != nil {
		return GenerationJob{}, err
	}
	provider, model, err := resolveVideoJobRequest(&req)
	if err != nil {
		return GenerationJob{}, err
	}
	if err := a.validateVideoJobRequest(req, provider, model); err != nil {
		return GenerationJob{}, err
	}
	req.Provider, req.Model = provider, model
	inputs := append([]string(nil), req.ReferenceAssetIDs...)
	if req.LastFrameAssetID != "" {
		inputs = append(inputs, req.LastFrameAssetID)
	}
	snapshot, err := requestSnapshot(req)
	if err != nil {
		return GenerationJob{}, err
	}
	return a.toolchainJobs.CreateJob(GenerationJob{
		Kind:          GenerationJobVideo,
		Provider:      provider,
		Model:         model,
		InputAssetIDs: inputs,
		Request:       snapshot,
		SourceModule:  normalizeToolchainModule(req.SourceModule, ToolchainModuleVideo),
	})
}

func (a *App) CreateUIAnalysisJob(request map[string]any) (GenerationJob, error) {
	if err := a.toolchainReady(); err != nil {
		return GenerationJob{}, err
	}
	var req uiAnalysisJobRequest
	if err := decodeToolchainRequest(request, &req); err != nil {
		return GenerationJob{}, err
	}
	provider, model, err := resolveVisionJobRequest(&req)
	if err != nil {
		return GenerationJob{}, err
	}
	if err := validatePrompt(req.Prompt, false); err != nil {
		return GenerationJob{}, err
	}
	if err := a.requireAsset(req.SourceAssetID, AssetMediaImage); err != nil {
		return GenerationJob{}, err
	}
	req.Provider, req.Model = provider, model
	snapshot, err := requestSnapshot(req)
	if err != nil {
		return GenerationJob{}, err
	}
	return a.toolchainJobs.CreateJob(GenerationJob{
		Kind:          GenerationJobUIAnalysis,
		Provider:      provider,
		Model:         model,
		InputAssetIDs: []string{req.SourceAssetID},
		Request:       snapshot,
		SourceModule:  ToolchainModuleUI,
	})
}

func (a *App) CreateUISplitJob(request map[string]any) (GenerationJob, error) {
	if err := a.toolchainReady(); err != nil {
		return GenerationJob{}, err
	}
	var req uiSplitJobRequest
	if err := decodeToolchainRequest(request, &req); err != nil {
		return GenerationJob{}, err
	}
	if err := a.validateUISplitJobRequest(req); err != nil {
		return GenerationJob{}, err
	}
	snapshot, err := requestSnapshot(req)
	if err != nil {
		return GenerationJob{}, err
	}
	return a.toolchainJobs.CreateJob(GenerationJob{
		Kind:          GenerationJobUISplit,
		InputAssetIDs: []string{req.SourceAssetID},
		Request:       snapshot,
		SourceModule:  ToolchainModuleUI,
	})
}

func (a *App) CreatePromptOptimizationJob(request map[string]any) (GenerationJob, error) {
	if err := a.toolchainReady(); err != nil {
		return GenerationJob{}, err
	}
	var req promptOptimizationJobRequest
	if err := decodeToolchainRequest(request, &req); err != nil {
		return GenerationJob{}, err
	}
	provider, model, err := resolveTextJobRequest(&req)
	if err != nil {
		return GenerationJob{}, err
	}
	if err := validatePrompt(req.Prompt, true); err != nil {
		return GenerationJob{}, err
	}
	switch req.Purpose {
	case "image", "video", "ui":
	default:
		return GenerationJob{}, errors.New("提示词用途必须是 image、video 或 ui")
	}
	req.Provider, req.Model = provider, model
	snapshot, err := requestSnapshot(req)
	if err != nil {
		return GenerationJob{}, err
	}
	return a.toolchainJobs.CreateJob(GenerationJob{
		Kind:         GenerationJobPromptOptimization,
		Provider:     provider,
		Model:        model,
		Request:      snapshot,
		SourceModule: normalizeToolchainModule(req.SourceModule, ToolchainModule(req.Purpose)),
	})
}

func (a *App) ListGenerationJobs() ([]GenerationJob, error) {
	if err := a.toolchainReady(); err != nil {
		return nil, err
	}
	return a.toolchainJobs.ListJobs(), nil
}

func (a *App) GetGenerationJob(id string) (GenerationJob, error) {
	if err := a.toolchainReady(); err != nil {
		return GenerationJob{}, err
	}
	return a.toolchainJobs.GetJob(strings.TrimSpace(id))
}

func (a *App) CancelGenerationJob(id string) (GenerationJob, error) {
	if err := a.toolchainReady(); err != nil {
		return GenerationJob{}, err
	}
	return a.toolchainJobs.CancelJob(strings.TrimSpace(id))
}

func (a *App) RetryGenerationJob(id string) (GenerationJob, error) {
	if err := a.toolchainReady(); err != nil {
		return GenerationJob{}, err
	}
	return a.toolchainJobs.RetryJob(strings.TrimSpace(id))
}

func (a *App) ListAssets() ([]ToolAsset, error) {
	if err := a.toolchainReady(); err != nil {
		return nil, err
	}
	return a.toolchainStore.ListAssets()
}

func (a *App) ImportAssets(request map[string]any) ([]ToolAsset, error) {
	if err := a.toolchainReady(); err != nil {
		return nil, err
	}
	var req importAssetsRequest
	if err := decodeToolchainRequest(request, &req); err != nil {
		return nil, err
	}
	module := normalizeToolchainModule(req.Module, ToolchainModuleImage)
	filters := []runtime.FileFilter{
		{DisplayName: "图片和视频 (*.png;*.jpg;*.jpeg;*.webp;*.bmp;*.mp4;*.webm;*.mov)", Pattern: "*.png;*.jpg;*.jpeg;*.webp;*.bmp;*.mp4;*.webm;*.mov"},
	}
	switch strings.ToLower(strings.TrimSpace(req.MediaType)) {
	case "image":
		filters = []runtime.FileFilter{{DisplayName: "图片 (*.png;*.jpg;*.jpeg;*.webp;*.bmp)", Pattern: "*.png;*.jpg;*.jpeg;*.webp;*.bmp"}}
	case "video":
		filters = []runtime.FileFilter{{DisplayName: "视频 (*.mp4;*.webm;*.mov)", Pattern: "*.mp4;*.webm;*.mov"}}
	case "", "all":
	default:
		return nil, errors.New("只支持导入图片或视频资产")
	}
	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{Title: "导入工具链资产", Filters: filters})
	if err != nil {
		return nil, err
	}
	assets := make([]ToolAsset, 0, len(paths))
	for _, path := range paths {
		mediaType, err := validateLocalAssetFile(path)
		if err != nil {
			return nil, err
		}
		if req.MediaType != "" && req.MediaType != "all" && string(mediaType) != req.MediaType {
			return nil, fmt.Errorf("文件 %s 与选择的媒体类型不一致", filepath.Base(path))
		}
		asset, err := a.toolchainStore.ImportAssetFromPath(path, ToolAsset{
			Name:         filepath.Base(path),
			MediaType:    mediaType,
			Usage:        "imported",
			SourceModule: module,
			Tags:         []string{"imported"},
		})
		if err != nil {
			return nil, err
		}
		if mediaType == AssetMediaImage {
			_ = a.ensureAssetThumbnail(asset.ID)
			asset, _ = a.toolchainStore.GetAsset(asset.ID)
		}
		assets = append(assets, asset)
	}
	return assets, nil
}

func (a *App) DeleteAsset(id string) error {
	if err := a.toolchainReady(); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	for _, job := range a.toolchainJobs.ListJobs() {
		if !job.Status.Terminal() && (containsString(job.InputAssetIDs, id) || containsString(job.OutputAssetIDs, id)) {
			return errors.New("资产正在被后台任务使用，任务结束或取消后才能删除")
		}
	}
	return a.toolchainStore.DeleteAsset(id)
}

func (a *App) ExportAsset(id string) (string, error) {
	if err := a.toolchainReady(); err != nil {
		return "", err
	}
	asset, err := a.toolchainStore.GetAsset(strings.TrimSpace(id))
	if err != nil {
		return "", err
	}
	source, err := a.toolchainStore.ResolveAssetContentPath(asset.ID)
	if err != nil {
		return "", err
	}
	name := filepath.Base(asset.Name)
	if name == "." || name == "" {
		name = asset.ID + extensionForMIME(asset.MIMEType)
	}
	target, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "导出资产", DefaultFilename: name})
	if err != nil || target == "" {
		return target, err
	}
	if err := copyFileContents(source, target); err != nil {
		return "", err
	}
	return target, nil
}

func (a *App) ExportUIProject(request map[string]any) (string, error) {
	if err := a.toolchainReady(); err != nil {
		return "", err
	}
	var req exportUIProjectRequest
	if err := decodeToolchainRequest(request, &req); err != nil {
		return "", err
	}
	if len(req.Regions) == 0 || len(req.Regions) > 512 {
		return "", errors.New("UI 导出需要 1 到 512 个区域")
	}
	asset, err := a.requireImageAsset(req.SourceAssetID)
	if err != nil {
		return "", err
	}
	raw, _, err := a.readAssetBytes(asset.ID, maxLocalImageBytes)
	if err != nil {
		return "", err
	}
	source, _, err := uiassets.DecodeBytes(raw, uiassets.DefaultDecodeLimits())
	if err != nil {
		return "", err
	}
	layout, err := uiLayoutFromRegions(req.Regions, source.Bounds().Dx(), source.Bounds().Dy())
	if err != nil {
		return "", err
	}
	archive, err := uiassets.BuildProjectZIP(uiassets.ExportProject{
		ID:               "ui-project-" + time.Now().UTC().Format("20060102-150405"),
		Name:             strings.TrimSuffix(asset.Name, filepath.Ext(asset.Name)),
		Source:           source,
		Layout:           layout,
		Tokens:           collectUITokens(req.Regions),
		Copy:             collectUICopy(req.Regions),
		RecreationPrompt: firstNonBlank(req.Prompt, asset.Prompt, defaultUIRecreationPrompt(req.Regions)),
	})
	if err != nil {
		return "", err
	}
	base := sanitizeName(strings.TrimSuffix(asset.Name, filepath.Ext(asset.Name)))
	if base == "" {
		base = "perfectpixel-ui"
	}
	target, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "导出 UI 项目",
		DefaultFilename: base + "-ui.zip",
		Filters:         []runtime.FileFilter{{DisplayName: "ZIP 项目 (*.zip)", Pattern: "*.zip"}},
	})
	if err != nil || target == "" {
		return target, err
	}
	if err := os.WriteFile(target, archive, 0o600); err != nil {
		return "", err
	}
	return target, nil
}

func (a *App) SendAssetToModule(request map[string]any) (map[string]any, error) {
	if err := a.toolchainReady(); err != nil {
		return nil, err
	}
	var req sendAssetRequest
	if err := decodeToolchainRequest(request, &req); err != nil {
		return nil, err
	}
	asset, err := a.toolchainStore.GetAsset(strings.TrimSpace(req.AssetID))
	if err != nil {
		return nil, err
	}
	target := normalizeToolchainModule(req.TargetModule, "")
	if target == "" {
		return nil, errors.New("目标模块无效")
	}
	result := map[string]any{"asset": asset, "targetModule": target}
	if target != ToolchainModuleAnimation {
		result["prefill"] = map[string]any{"referenceAssetIds": []string{asset.ID}, "sourceAssetId": asset.ID}
		a.emit("toolchain-asset-sent", map[string]any{"assetId": asset.ID, "targetModule": target})
		return result, nil
	}
	path, err := a.toolchainStore.ResolveAssetContentPath(asset.ID)
	if err != nil {
		return nil, err
	}
	switch asset.MediaType {
	case AssetMediaVideo:
		if strings.TrimSpace(req.ActionName) == "" {
			return nil, errors.New("发送视频到动画工作台需要动作名")
		}
		if req.Frames < 2 || req.Frames > 64 || req.FPS < 1 || req.FPS > 60 {
			return nil, errors.New("动画帧数必须为 2–64，FPS 必须为 1–60")
		}
		seconds := max(1, int((asset.DurationMs+999)/1000))
		if seconds <= 1 {
			seconds = max(1, (req.Frames+req.FPS-1)/req.FPS)
		}
		item, err := a.ImportMotionLibraryFile(ImportMotionLibraryArgs{
			Name:       req.ActionName,
			Action:     req.ActionName,
			Frames:     req.Frames,
			FPS:        req.FPS,
			Loop:       req.Loop,
			Seconds:    seconds,
			SourcePath: path,
			Tags:       []string{"toolchain", asset.ID},
		})
		if err != nil {
			return nil, err
		}
		result["characterId"] = strings.TrimSpace(req.CharacterID)
		result["motionLibraryItem"] = item
		a.emit("toolchain-asset-sent", map[string]any{"assetId": asset.ID, "targetModule": target, "kind": "motion"})
		return result, nil
	case AssetMediaImage:
		raw, _, err := a.readAssetBytes(asset.ID, maxLocalImageBytes)
		if err != nil {
			return nil, err
		}
		img, err := decodeImage(raw)
		if err != nil {
			return nil, err
		}
		var encoded strings.Builder
		encoded.WriteString("data:image/png;base64,")
		var pngBuffer bytes.Buffer
		if err := png.Encode(&pngBuffer, img); err != nil {
			return nil, err
		}
		encoded.WriteString(base64.StdEncoding.EncodeToString(pngBuffer.Bytes()))
		item, err := a.SaveCharacterLibraryItem(SaveCharacterLibraryItemArgs{
			ID:          strings.TrimSpace(req.CharacterID),
			Name:        strings.TrimSuffix(asset.Name, filepath.Ext(asset.Name)),
			Description: "Shared toolchain asset " + asset.ID,
			StyleKey:    "pixel",
			Image:       encoded.String(),
		})
		if err != nil {
			return nil, err
		}
		item.Image = ""
		result["characterLibraryItem"] = item
		a.emit("toolchain-asset-sent", map[string]any{"assetId": asset.ID, "targetModule": target, "kind": "character"})
		return result, nil
	default:
		return nil, errors.New("只有图片或视频可以发送到动画工作台")
	}
}

func (a *App) GetGenerationCapabilities() ToolchainGenerationCapabilities {
	settings := config.Load()
	result := ToolchainGenerationCapabilities{}
	for _, provider := range gen.SupportedProviders {
		if provider == gen.ProviderVolcengine {
			continue
		}
		models := append([]string(nil), gen.ModelsFor(provider)...)
		configured := strings.TrimSpace(settings.Cfg(provider).Model)
		if configured != "" && !containsString(models, configured) {
			models = append([]string{configured}, models...)
		}
		for _, model := range models {
			capability := gen.ImageModelCapabilityFor(provider, model)
			result.Image = append(result.Image, ToolchainModelCapability{
				Provider:                   provider,
				Model:                      model,
				Label:                      gen.ProviderLabel(provider) + " · " + model,
				SupportsText:               capability.SupportsTextToImage,
				SupportsImageReference:     capability.SupportsReferenceImages,
				SupportsMultipleReferences: capability.MaxReferenceImages > 1,
				MaxReferenceImages:         capability.MaxReferenceImages,
				SupportsEdit:               capability.SupportsReferenceImages,
				SupportsMask:               capability.SupportsMaskEdit,
				AspectRatios:               append([]string(nil), capability.AllowedAspectRatios...),
			})
		}
	}
	for _, provider := range []string{gen.ProviderVolcengine, gen.ProviderZetaTechs} {
		models := append([]string(nil), gen.VideoModelsFor(provider)...)
		configured := strings.TrimSpace(settings.Cfg(provider).VideoModel)
		if configured != "" && !containsString(models, configured) {
			models = append([]string{configured}, models...)
		}
		for _, model := range models {
			capability := gen.VideoModelCapabilityFor(model)
			result.Video = append(result.Video, ToolchainModelCapability{
				Provider:                   provider,
				Model:                      model,
				Label:                      gen.ProviderLabel(provider) + " · " + model,
				SupportsText:               capability.SupportsTextPrompt,
				SupportsImageReference:     capability.SupportsReferenceImage,
				SupportsMultipleReferences: capability.SupportsMultiImageReference,
				MaxReferenceImages:         videoMaxReferenceImages(capability),
				SupportsFirstFrame:         capability.SupportsFirstFrameReference,
				SupportsLastFrame:          capability.SupportsStartEndFrames,
				SupportsIdentityReference:  capability.SupportsIdentityReference,
				SupportsAudio:              capability.SupportsAudio,
				SupportsWatermark:          capability.SupportsWatermarkControl,
				AspectRatios:               append([]string(nil), capability.AllowedAspectRatios...),
				Resolutions:                append([]string(nil), capability.AllowedResolutions...),
				Durations:                  append([]int(nil), capability.AllowedSeconds...),
			})
		}
	}
	for _, provider := range gen.SupportedVisionProviders {
		models := append([]string(nil), gen.VisionModelsFor(provider)...)
		configured := strings.TrimSpace(settings.Cfg(provider).VisionModel)
		if configured != "" && !containsString(models, configured) {
			models = append([]string{configured}, models...)
		}
		for _, model := range models {
			result.Vision = append(result.Vision, ToolchainModelCapability{Provider: provider, Model: model, Label: gen.ProviderLabel(provider) + " · " + model, SupportsImageReference: true, MaxReferenceImages: 4})
		}
	}
	for _, provider := range gen.SupportedTextProviders {
		models := append([]string(nil), gen.TextModelsFor(provider)...)
		configured := strings.TrimSpace(settings.Cfg(provider).TextModel)
		if configured != "" && !containsString(models, configured) {
			models = append([]string{configured}, models...)
		}
		for _, model := range models {
			result.Text = append(result.Text, ToolchainModelCapability{Provider: provider, Model: model, Label: gen.ProviderLabel(provider) + " · " + model, SupportsText: true})
		}
	}
	return result
}

func (a *App) RefreshGenerationCapabilities() ToolchainGenerationCapabilities {
	return a.GetGenerationCapabilities()
}

func decodeToolchainRequest(input map[string]any, target any) error {
	if input == nil {
		return errors.New("请求不能为空")
	}
	data, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("请求格式无效: %w", err)
	}
	if len(data) > maxJobRequestSnapshotBytes {
		return errors.New("请求过大")
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("请求格式无效: %w", err)
	}
	return nil
}

func requestSnapshot(value any) (map[string]any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func normalizeToolchainModule(value string, fallback ToolchainModule) ToolchainModule {
	switch ToolchainModule(strings.ToLower(strings.TrimSpace(value))) {
	case ToolchainModuleAnimation:
		return ToolchainModuleAnimation
	case ToolchainModuleImage:
		return ToolchainModuleImage
	case ToolchainModuleVideo:
		return ToolchainModuleVideo
	case ToolchainModuleUI:
		return ToolchainModuleUI
	default:
		return fallback
	}
}

func validatePrompt(prompt string, required bool) error {
	prompt = strings.TrimSpace(prompt)
	if required && prompt == "" {
		return errors.New("请输入提示词")
	}
	if len(prompt) > maxToolchainPromptBytes {
		return fmt.Errorf("提示词不能超过 %d KB", maxToolchainPromptBytes>>10)
	}
	return nil
}

func (a *App) requireAsset(id string, mediaType AssetMediaType) error {
	asset, err := a.toolchainStore.GetAsset(strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if asset.MediaType != mediaType {
		return fmt.Errorf("资产 %s 不是%s", id, mediaType)
	}
	return nil
}

func (a *App) requireImageAsset(id string) (ToolAsset, error) {
	asset, err := a.toolchainStore.GetAsset(strings.TrimSpace(id))
	if err != nil {
		return ToolAsset{}, err
	}
	if asset.MediaType != AssetMediaImage {
		return ToolAsset{}, errors.New("所选资产不是图片")
	}
	return asset, nil
}

func (a *App) validateImageJobRequest(req imageJobRequest, provider, model string) error {
	if err := validatePrompt(req.Prompt, true); err != nil {
		return err
	}
	if err := validatePrompt(req.NegativePrompt, false); err != nil {
		return err
	}
	if req.Variants < 1 || req.Variants > 4 {
		return errors.New("图片变体数量必须为 1 到 4")
	}
	capability := gen.ImageModelCapabilityFor(provider, model)
	if !containsString(capability.AllowedAspectRatios, req.AspectRatio) {
		return fmt.Errorf("模型 %s 不支持比例 %s", model, req.AspectRatio)
	}
	if len(req.ReferenceAssetIDs) > capability.MaxReferenceImages {
		return fmt.Errorf("模型 %s 最多支持 %d 张参考图", model, capability.MaxReferenceImages)
	}
	if len(req.ReferenceAssetIDs) > 0 && !capability.SupportsReferenceImages {
		return fmt.Errorf("模型 %s 不支持参考图", model)
	}
	for _, id := range req.ReferenceAssetIDs {
		if err := a.requireAsset(id, AssetMediaImage); err != nil {
			return err
		}
	}
	switch req.Mode {
	case "", "text":
		if len(req.ReferenceAssetIDs) != 0 {
			return errors.New("文生图模式不能携带参考图")
		}
	case "reference", "edit":
		if len(req.ReferenceAssetIDs) == 0 {
			return errors.New("参考图模式至少需要一张图片")
		}
	case "masked_edit":
		if len(req.ReferenceAssetIDs) == 0 || len(req.MaskRegions) == 0 {
			return errors.New("局部重绘需要原图和遮罩区域")
		}
		if !capability.SupportsMaskEdit {
			return fmt.Errorf("模型 %s 不支持遮罩重绘", model)
		}
	default:
		return errors.New("不支持的图片生成模式")
	}
	if req.PostProcess.Pixelize && (req.PostProcess.PixelSize < 32 || req.PostProcess.PixelSize > 512) {
		return errors.New("像素画布尺寸必须为 32 到 512")
	}
	return nil
}

func (a *App) validateVideoJobRequest(req videoJobRequest, provider, model string) error {
	if err := validatePrompt(req.Prompt, true); err != nil {
		return err
	}
	if err := validatePrompt(req.NegativePrompt, false); err != nil {
		return err
	}
	capability := gen.VideoModelCapabilityFor(model)
	if !containsInt(capability.AllowedSeconds, req.DurationSeconds) {
		return fmt.Errorf("模型 %s 不支持 %d 秒时长", model, req.DurationSeconds)
	}
	if !containsString(capability.AllowedAspectRatios, req.AspectRatio) {
		return fmt.Errorf("模型 %s 不支持比例 %s", model, req.AspectRatio)
	}
	if !containsString(capability.AllowedResolutions, req.Resolution) {
		return fmt.Errorf("模型 %s 不支持分辨率 %s", model, req.Resolution)
	}
	if req.Audio && !capability.SupportsAudio {
		return fmt.Errorf("模型 %s 不支持生成音频", model)
	}
	if req.Watermark && !capability.SupportsWatermarkControl {
		return fmt.Errorf("模型 %s 不支持水印控制", model)
	}
	if len(req.ReferenceAssetIDs) > videoMaxReferenceImages(capability) {
		return errors.New("参考图数量超过当前视频模型限制")
	}
	for _, id := range req.ReferenceAssetIDs {
		if err := a.requireAsset(id, AssetMediaImage); err != nil {
			return err
		}
	}
	if req.LastFrameAssetID != "" {
		if err := a.requireAsset(req.LastFrameAssetID, AssetMediaImage); err != nil {
			return err
		}
	}
	switch req.Mode {
	case "", "text":
		if len(req.ReferenceAssetIDs) != 0 || req.LastFrameAssetID != "" {
			return errors.New("文生视频模式不能携带参考帧")
		}
	case "image":
		if len(req.ReferenceAssetIDs) == 0 || !capability.SupportsReferenceImage {
			return fmt.Errorf("模型 %s 不支持当前图生视频请求", model)
		}
	default:
		return errors.New("不支持的视频输入模式")
	}
	switch req.ReferenceMode {
	case "", "none":
	case "first_frame":
		if !capability.SupportsFirstFrameReference {
			return errors.New("当前模型不支持首帧参考")
		}
	case "start_end_frame":
		if !capability.SupportsStartEndFrames || req.LastFrameAssetID == "" {
			return errors.New("首尾帧模式需要模型支持和一张尾帧图片")
		}
	case "identity_reference", "multi_reference":
		if !capability.SupportsIdentityReference && !capability.SupportsMultiImageReference {
			return errors.New("当前模型不支持身份或多图参考")
		}
	default:
		return errors.New("不支持的视频参考模式")
	}
	_ = provider
	return nil
}

func (a *App) validateUISplitJobRequest(req uiSplitJobRequest) error {
	if err := a.requireAsset(req.SourceAssetID, AssetMediaImage); err != nil {
		return err
	}
	if req.Padding < 0 || req.Padding > 128 {
		return errors.New("切片留白必须为 0 到 128 像素")
	}
	switch req.Mode {
	case "auto":
		if req.BackgroundColor != "" {
			if _, err := parseHexRGBA(req.BackgroundColor); err != nil {
				return err
			}
		}
	case "grid":
		if req.Rows < 1 || req.Rows > 32 || req.Columns < 1 || req.Columns > 32 {
			return errors.New("网格行列数必须为 1 到 32")
		}
	case "manual":
		if len(req.Regions) == 0 || len(req.Regions) > 512 {
			return errors.New("手动拆图需要 1 到 512 个区域")
		}
	default:
		return errors.New("拆图模式必须是 auto、grid 或 manual")
	}
	return nil
}

func resolveImageJobRequest(req *imageJobRequest) (string, string, error) {
	settings := config.Load()
	provider := firstNonBlank(req.Provider, settings.ImageProvider)
	if !containsString(gen.SupportedProviders, provider) || provider == gen.ProviderVolcengine {
		return "", "", fmt.Errorf("不支持的图片提供方：%s", provider)
	}
	model := firstNonBlank(req.Model, settings.Cfg(provider).Model, gen.DefaultModelFor(provider))
	if model == "" {
		return "", "", errors.New("图片模型未设置")
	}
	if strings.TrimSpace(settings.Cfg(provider).APIKey) == "" {
		return "", "", fmt.Errorf("%s API 密钥未设置，请先在设置中配置", gen.ProviderLabel(provider))
	}
	if req.AspectRatio == "" {
		req.AspectRatio = "1:1"
	}
	if req.Variants == 0 {
		req.Variants = 1
	}
	if req.Mode == "" {
		if len(req.ReferenceAssetIDs) > 0 {
			req.Mode = "reference"
		} else {
			req.Mode = "text"
		}
	}
	return provider, model, nil
}

func resolveVideoJobRequest(req *videoJobRequest) (string, string, error) {
	settings := config.Load()
	provider := firstNonBlank(req.Provider, settings.VideoProvider)
	if provider != gen.ProviderVolcengine && provider != gen.ProviderZetaTechs {
		return "", "", fmt.Errorf("不支持的视频提供方：%s", provider)
	}
	model := firstNonBlank(req.Model, settings.Cfg(provider).VideoModel, gen.DefaultVideoModelFor(provider))
	if model == "" {
		return "", "", errors.New("视频模型未设置")
	}
	if strings.TrimSpace(settings.Cfg(provider).APIKey) == "" {
		return "", "", fmt.Errorf("%s API 密钥未设置，请先在设置中配置", gen.ProviderLabel(provider))
	}
	capability := gen.VideoModelCapabilityFor(model)
	if req.DurationSeconds == 0 {
		req.DurationSeconds = firstInt(capability.AllowedSeconds, 4)
	}
	if req.AspectRatio == "" {
		req.AspectRatio = firstString(capability.AllowedAspectRatios, "16:9")
	}
	if req.Resolution == "" {
		req.Resolution = firstString(capability.AllowedResolutions, "720p")
	}
	if req.FPS <= 0 {
		req.FPS = settings.VideoFPS
		if req.FPS <= 0 {
			req.FPS = 12
		}
	}
	if req.Mode == "" {
		if len(req.ReferenceAssetIDs) > 0 {
			req.Mode = "image"
		} else {
			req.Mode = "text"
		}
	}
	if req.ReferenceMode == "" {
		req.ReferenceMode = "none"
	}
	return provider, model, nil
}

func resolveVisionJobRequest(req *uiAnalysisJobRequest) (string, string, error) {
	settings := config.Load()
	provider := firstNonBlank(req.Provider, settings.VisionProvider)
	if !gen.SupportsVision(provider) {
		return "", "", fmt.Errorf("不支持的视觉提供方：%s", provider)
	}
	model := firstNonBlank(req.Model, settings.Cfg(provider).VisionModel, gen.DefaultVisionModelFor(provider))
	if model == "" {
		return "", "", errors.New("视觉模型未设置")
	}
	if strings.TrimSpace(settings.Cfg(provider).APIKey) == "" {
		return "", "", fmt.Errorf("%s 视觉 API 密钥未设置，请先在设置中配置", gen.ProviderLabel(provider))
	}
	return provider, model, nil
}

func resolveTextJobRequest(req *promptOptimizationJobRequest) (string, string, error) {
	settings := config.Load()
	provider := firstNonBlank(req.Provider, settings.TextProvider)
	if !gen.SupportsText(provider) {
		return "", "", fmt.Errorf("不支持的文本提供方：%s", provider)
	}
	model := firstNonBlank(req.Model, settings.Cfg(provider).TextModel, gen.DefaultTextModelFor(provider))
	if model == "" {
		return "", "", errors.New("文本模型未设置")
	}
	cfg := settings.Cfg(provider)
	if firstNonBlank(cfg.TextAPIKey, cfg.APIKey) == "" {
		return "", "", fmt.Errorf("%s 文本 API 密钥未设置，请先在设置中配置", gen.ProviderLabel(provider))
	}
	return provider, model, nil
}

func videoMaxReferenceImages(capability gen.VideoModelCapability) int {
	if capability.SupportsMultiImageReference {
		return 4
	}
	if capability.SupportsReferenceImage {
		return 1
	}
	return 0
}

func validateLocalAssetFile(path string) (AssetMediaType, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("只能导入普通文件")
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".bmp":
		if info.Size() <= 0 || info.Size() > maxLocalImageBytes {
			return "", fmt.Errorf("图片 %s 必须小于 %d MB", filepath.Base(path), maxLocalImageBytes>>20)
		}
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		_, _, decodeErr := uiassets.Decode(file, uiassets.DefaultDecodeLimits())
		_ = file.Close()
		if decodeErr != nil {
			return "", decodeErr
		}
		return AssetMediaImage, nil
	case ".mp4", ".webm", ".mov":
		if info.Size() <= 0 || info.Size() > maxLocalVideoBytes {
			return "", fmt.Errorf("视频 %s 必须小于 %d MB", filepath.Base(path), maxLocalVideoBytes>>20)
		}
		return AssetMediaVideo, nil
	default:
		return "", fmt.Errorf("不支持的资产格式：%s", ext)
	}
}

func copyFileContents(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func uiLayoutFromRegions(regions []UIRegion, width, height int) (uiassets.LayoutDocument, error) {
	layout := uiassets.LayoutDocument{Canvas: uiassets.Size{Width: width, Height: height}, Regions: make([]uiassets.LayoutRegion, 0, len(regions))}
	for _, region := range regions {
		bounds := uiassets.PixelRect(region.Bounds)
		if !bounds.Within(layout.Canvas) {
			return uiassets.LayoutDocument{}, fmt.Errorf("区域 %s 超出源图边界", region.Name)
		}
		layout.Regions = append(layout.Regions, uiassets.LayoutRegion{
			ID:            strings.TrimSpace(region.ID),
			ParentID:      strings.TrimSpace(region.ParentID),
			Name:          strings.TrimSpace(region.Name),
			Type:          strings.TrimSpace(region.Type),
			Bounds:        bounds,
			ZIndex:        region.ZIndex,
			Confidence:    region.Confidence,
			Text:          region.Text,
			Style:         region.Style,
			ExportEnabled: region.ExportEnabled,
		})
	}
	return layout, nil
}

func collectUITokens(regions []UIRegion) map[string]any {
	styles := make(map[string]any)
	for _, region := range regions {
		if len(region.Style) > 0 {
			styles[region.ID] = region.Style
		}
	}
	return map[string]any{"inferred": true, "styles": styles}
}

func collectUICopy(regions []UIRegion) string {
	var lines []string
	for _, region := range regions {
		if text := strings.TrimSpace(region.Text); text != "" {
			lines = append(lines, region.Name+": "+text)
		}
	}
	return strings.Join(lines, "\n")
}

func defaultUIRecreationPrompt(regions []UIRegion) string {
	types := make([]string, 0, len(regions))
	for _, region := range regions {
		if value := strings.TrimSpace(region.Type); value != "" && !containsString(types, value) {
			types = append(types, value)
		}
	}
	return "Recreate the visible game UI screen with the same pixel-coordinate hierarchy and these inferred component types: " + strings.Join(types, ", ") + ". Preserve readable copy, spacing, colors, and states without generating implementation code."
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func firstString(values []string, fallback string) string {
	if len(values) > 0 && strings.TrimSpace(values[0]) != "" {
		return values[0]
	}
	return fallback
}

func firstInt(values []int, fallback int) int {
	if len(values) > 0 && values[0] > 0 {
		return values[0]
	}
	return fallback
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
