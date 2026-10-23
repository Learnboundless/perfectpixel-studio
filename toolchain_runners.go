package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"perfectpixel/internal/config"
	"perfectpixel/internal/gen"
	"perfectpixel/internal/safemedia"
	"perfectpixel/internal/sprite"
	"perfectpixel/internal/uiassets"

	xdraw "golang.org/x/image/draw"
)

const (
	maxUIAnalysisRegions  = 256
	maxProviderPollErrors = 8
)

var provider5xxPattern = regexp.MustCompile(`(^|[^0-9])5[0-9]{2}([^0-9]|$)`)

type uiAnalysisDocument struct {
	Canvas           uiassets.Size     `json:"canvas"`
	Regions          []UIRegion        `json:"regions"`
	Tokens           map[string]any    `json:"tokens,omitempty"`
	Copy             string            `json:"copy,omitempty"`
	RecreationPrompt string            `json:"recreationPrompt,omitempty"`
	RegionAssetIDs   map[string]string `json:"regionAssetIds,omitempty"`
	Inferred         bool              `json:"inferred"`
}

type providerUIAnalysis struct {
	Regions          []providerUIRegion `json:"regions"`
	Tokens           map[string]any     `json:"tokens,omitempty"`
	Copy             string             `json:"copy,omitempty"`
	RecreationPrompt string             `json:"recreationPrompt,omitempty"`
}

type providerUIRegion struct {
	ID            string         `json:"id"`
	ParentID      string         `json:"parentId,omitempty"`
	Name          string         `json:"name"`
	Type          string         `json:"type"`
	Bounds        UIPixelBounds  `json:"bounds"`
	ZIndex        int            `json:"zIndex"`
	Confidence    float64        `json:"confidence"`
	Text          string         `json:"text,omitempty"`
	Style         map[string]any `json:"style,omitempty"`
	ExportEnabled *bool          `json:"exportEnabled,omitempty"`
}

func (a *App) runImageJob(ctx context.Context, job GenerationJob, control *GenerationJobControl) error {
	var req imageJobRequest
	if err := decodeStoredJobRequest(job.Request, &req); err != nil {
		return err
	}
	provider, err := toolchainImageProvider(job.Provider, job.Model)
	if err != nil {
		return err
	}
	refs, refAssets, err := a.loadImageAssets(req.ReferenceAssetIDs)
	if err != nil {
		return err
	}
	prompt := composeGenerationPrompt(req.Prompt, req.NegativePrompt)
	var providerMask []byte
	var compositeMask *image.Alpha
	var editBase *image.NRGBA
	if req.Mode == "masked_edit" {
		if len(refs) == 0 {
			return errors.New("局部重绘缺少源图")
		}
		editBase, _, err = uiassets.DecodeBytes(refs[0], uiassets.DefaultDecodeLimits())
		if err != nil {
			return err
		}
		rects := make([]uiassets.PixelRect, 0, len(req.MaskRegions))
		canvas := uiassets.Size{Width: editBase.Bounds().Dx(), Height: editBase.Bounds().Dy()}
		for _, bounds := range req.MaskRegions {
			rect := uiassets.PixelRect(bounds).Intersect(canvas)
			if rect.Empty() {
				return errors.New("遮罩区域未落在源图范围内")
			}
			rects = append(rects, rect)
		}
		compositeMask, err = uiassets.CreateMask(canvas, rects, uiassets.MaskOptions{Feather: 2})
		if err != nil {
			return err
		}
		maskForProvider, err := uiassets.CreateMask(canvas, rects, uiassets.MaskOptions{Feather: 2, Invert: true})
		if err != nil {
			return err
		}
		providerMask, err = encodeAlphaMaskPNG(maskForProvider)
		if err != nil {
			return err
		}
	}

	outputs := make([]string, 0, req.Variants*3)
	for variant := 0; variant < req.Variants; variant++ {
		if _, err := control.SetProgress("generating", 0.05+float64(variant)/float64(max(1, req.Variants))*0.65); err != nil {
			return err
		}
		var generated []byte
		_, err := control.Submit(func(submitCtx context.Context) (string, error) {
			submitCtx = gen.WithoutPaidRequestRetries(submitCtx)
			var generateErr error
			if req.Mode == "masked_edit" {
				maskProvider, ok := provider.(gen.MaskEditProvider)
				if !ok {
					return "", errors.New("当前图片 Provider 不支持遮罩重绘")
				}
				generated, generateErr = maskProvider.GenerateImageWithMask(submitCtx, prompt, refs, providerMask, req.AspectRatio)
			} else if singleAttempt, ok := provider.(gen.SingleAttemptImageProvider); ok {
				generated, generateErr = singleAttempt.GenerateImageOnce(submitCtx, prompt, refs, req.AspectRatio)
			} else {
				generated, generateErr = provider.GenerateImage(submitCtx, prompt, refs, req.AspectRatio)
			}
			return "", classifyPaidSubmissionError(generateErr)
		})
		if err != nil {
			return err
		}
		if req.Mode == "masked_edit" {
			generated, err = compositeMaskedImage(editBase, generated, compositeMask)
			if err != nil {
				return fmt.Errorf("合成局部重绘结果: %w", err)
			}
		}
		parentID := ""
		if len(refAssets) > 0 {
			parentID = refAssets[0].ID
		}
		rawAsset, rawImage, err := a.storeGeneratedImage(generated, ToolAsset{
			Name:          fmt.Sprintf("%s-%02d.png", job.ID, variant+1),
			Usage:         imageResultUsage(req),
			SourceModule:  job.SourceModule,
			ParentAssetID: parentID,
			Prompt:        req.Prompt,
			Provider:      job.Provider,
			Model:         job.Model,
			Tags:          []string{"generated", "original", req.Purpose, req.Mode},
		})
		if err != nil {
			return err
		}
		outputs = append(outputs, rawAsset.ID)
		if _, err := control.SetOutputAssetIDs(outputs); err != nil {
			return err
		}

		postBase := rawImage
		postParent := rawAsset.ID
		if req.PostProcess.RemoveBackground {
			remover, _ := appBackgroundRemover(req.PostProcess.BackgroundRemovalMode)
			clean, _ := remover.Remove(postBase)
			cleanAsset, _, err := a.storeGeneratedNRGBA(clean, ToolAsset{
				Name:          fmt.Sprintf("%s-%02d-no-bg.png", job.ID, variant+1),
				Usage:         "background_removed",
				SourceModule:  job.SourceModule,
				ParentAssetID: rawAsset.ID,
				Prompt:        req.Prompt,
				Provider:      job.Provider,
				Model:         job.Model,
				Tags:          []string{"derived", "background_removed"},
			})
			if err != nil {
				return err
			}
			outputs = append(outputs, cleanAsset.ID)
			postBase = clean
			postParent = cleanAsset.ID
			if _, err := control.SetOutputAssetIDs(outputs); err != nil {
				return err
			}
		}
		if req.PostProcess.Pixelize {
			target := max(32, req.PostProcess.PixelSize)
			scale := max(2, int(math.Ceil(float64(max(postBase.Bounds().Dx(), postBase.Bounds().Dy()))/float64(target))))
			pixelized := sprite.Pixelize(postBase, scale)
			pixelAsset, _, err := a.storeGeneratedNRGBA(pixelized, ToolAsset{
				Name:          fmt.Sprintf("%s-%02d-pixel.png", job.ID, variant+1),
				Usage:         "pixelized",
				SourceModule:  job.SourceModule,
				ParentAssetID: postParent,
				Prompt:        req.Prompt,
				Provider:      job.Provider,
				Model:         job.Model,
				Tags:          []string{"derived", "pixelized", fmt.Sprintf("logical_canvas_%d", target)},
			})
			if err != nil {
				return err
			}
			outputs = append(outputs, pixelAsset.ID)
			if _, err := control.SetOutputAssetIDs(outputs); err != nil {
				return err
			}
		}
	}
	summary, _ := json.Marshal(map[string]any{"assetIds": outputs, "variants": req.Variants, "mode": req.Mode})
	if _, err := control.SetProviderResponseSummary(summary); err != nil {
		return err
	}
	_, err = control.SetProgress("saving", 0.98)
	return err
}

func (a *App) runVideoJob(ctx context.Context, job GenerationJob, control *GenerationJobControl) error {
	var req videoJobRequest
	if err := decodeStoredJobRequest(job.Request, &req); err != nil {
		return err
	}
	provider, err := toolchainVideoProvider(job.Provider, job.Model)
	if err != nil {
		return err
	}
	videoRequest, err := a.buildVideoRequest(req, job.Model)
	if err != nil {
		return err
	}
	var initial gen.VideoResult
	taskID, err := control.Submit(func(submitCtx context.Context) (string, error) {
		initial, err = provider.GenerateVideo(submitCtx, videoRequest)
		if err != nil {
			return "", classifyPaidSubmissionError(err)
		}
		return initial.ID, nil
	})
	if err != nil {
		return err
	}
	return a.finishVideoJob(ctx, job, req, provider, taskID, initial, control)
}

func (a *App) resumeVideoJob(ctx context.Context, job GenerationJob, control *GenerationJobControl) error {
	if strings.TrimSpace(job.ProviderTaskID) == "" {
		return errors.New("无法恢复没有远端任务 ID 的视频任务")
	}
	var req videoJobRequest
	if err := decodeStoredJobRequest(job.Request, &req); err != nil {
		return err
	}
	provider, err := toolchainVideoProvider(job.Provider, job.Model)
	if err != nil {
		return err
	}
	return a.finishVideoJob(ctx, job, req, provider, job.ProviderTaskID, gen.VideoResult{ID: job.ProviderTaskID}, control)
}

func (a *App) finishVideoJob(ctx context.Context, job GenerationJob, req videoJobRequest, provider gen.VideoProvider, taskID string, result gen.VideoResult, control *GenerationJobControl) error {
	if result.Failed() {
		return fmt.Errorf("视频生成失败: %s", strings.TrimSpace(result.Message))
	}
	if result.Status == "" && strings.TrimSpace(result.URL) != "" {
		result.Status = gen.VideoStatusCompleted
	}
	if !result.Done() {
		if strings.TrimSpace(taskID) == "" {
			return fmt.Errorf("%w: Provider 未返回可恢复的视频任务 ID", ErrSubmissionOutcomeUnknown)
		}
		var err error
		result, err = pollProviderVideo(ctx, provider, taskID, control)
		if err != nil {
			return err
		}
	}
	if _, err := control.SetProgress("downloading", 0.9); err != nil {
		return err
	}
	videoBytes, mimeType, err := downloadCompletedVideo(ctx, provider, taskID, result)
	if err != nil {
		return err
	}
	ext := extensionForMIME(mimeType)
	if ext == "" {
		ext = ".mp4"
	}
	width, height := videoDimensions(req.Resolution, req.AspectRatio)
	durationMs := int64(req.DurationSeconds) * 1000
	if probedWidth, probedHeight, probedDuration, probeErr := probeVideoMetadata(ctx, videoBytes, ext); probeErr == nil {
		width, height, durationMs = probedWidth, probedHeight, probedDuration
	}
	asset, err := a.toolchainStore.CreateAssetFromBytes(ToolAsset{
		Name:         job.ID + ext,
		MediaType:    AssetMediaVideo,
		MIMEType:     mimeType,
		Usage:        "generated_video",
		Width:        width,
		Height:       height,
		DurationMs:   durationMs,
		SourceModule: ToolchainModuleVideo,
		Prompt:       req.Prompt,
		Provider:     job.Provider,
		Model:        job.Model,
		Tags:         []string{"generated", req.Mode, req.ReferenceMode},
	}, videoBytes)
	if err != nil {
		return err
	}
	if _, err := control.SetOutputAssetIDs([]string{asset.ID}); err != nil {
		return err
	}
	summary, _ := json.Marshal(map[string]any{
		"providerTaskId": taskID,
		"status":         gen.VideoStatusCompleted,
		"assetId":        asset.ID,
		"sizeBytes":      len(videoBytes),
		"mimeType":       mimeType,
	})
	if _, err := control.SetProviderResponseSummary(summary); err != nil {
		return err
	}
	_, err = control.SetProgress("saving", 0.98)
	return err
}

func (a *App) runPromptOptimizationJob(ctx context.Context, job GenerationJob, control *GenerationJobControl) error {
	var req promptOptimizationJobRequest
	if err := decodeStoredJobRequest(job.Request, &req); err != nil {
		return err
	}
	client, err := toolchainTextClient(job.Provider, job.Model)
	if err != nil {
		return err
	}
	systemPrompt := promptOptimizationSystemPrompt(req.Purpose)
	var optimized string
	_, err = control.Submit(func(submitCtx context.Context) (string, error) {
		optimized, err = client.CompleteWithMaxTokens(submitCtx, systemPrompt, req.Prompt, 1600)
		return "", classifyPaidSubmissionError(err)
	})
	if err != nil {
		return err
	}
	optimized = strings.TrimSpace(optimized)
	if optimized == "" {
		return errors.New("提示词优化模型返回了空结果")
	}
	asset, err := a.toolchainStore.CreateAssetFromBytes(ToolAsset{
		Name:         job.ID + "-optimized.txt",
		MediaType:    AssetMediaText,
		MIMEType:     "text/plain; charset=utf-8",
		Usage:        "optimized_prompt",
		SourceModule: job.SourceModule,
		Prompt:       req.Prompt,
		Provider:     job.Provider,
		Model:        job.Model,
		Tags:         []string{"prompt", "optimized", req.Purpose},
	}, []byte(optimized))
	if err != nil {
		return err
	}
	if _, err := control.SetOutputAssetIDs([]string{asset.ID}); err != nil {
		return err
	}
	summary, _ := json.Marshal(map[string]any{"originalPrompt": req.Prompt, "optimizedPrompt": optimized, "assetId": asset.ID, "purpose": req.Purpose})
	if _, err := control.SetProviderResponseSummary(summary); err != nil {
		return err
	}
	_, err = control.SetProgress("saving", 0.98)
	return err
}

func (a *App) runUIAnalysisJob(ctx context.Context, job GenerationJob, control *GenerationJobControl) error {
	var req uiAnalysisJobRequest
	if err := decodeStoredJobRequest(job.Request, &req); err != nil {
		return err
	}
	client, err := toolchainVisionClient(job.Provider, job.Model)
	if err != nil {
		return err
	}
	raw, sourceAsset, err := a.readAssetBytes(req.SourceAssetID, maxLocalImageBytes)
	if err != nil {
		return err
	}
	source, info, err := uiassets.DecodeBytes(raw, uiassets.DefaultDecodeLimits())
	if err != nil {
		return err
	}
	var providerJSON json.RawMessage
	_, err = control.Submit(func(submitCtx context.Context) (string, error) {
		providerJSON, err = client.AnalyzeJSON(submitCtx, uiAnalysisSystemPrompt(), uiAnalysisUserPrompt(req.Prompt, info.Width, info.Height), [][]byte{raw}, 8000)
		return "", classifyPaidSubmissionError(err)
	})
	if err != nil {
		return err
	}
	document, err := validateUIAnalysis(providerJSON, info.Width, info.Height)
	if err != nil {
		return err
	}
	if _, err := control.SetProgress("cropping_regions", 0.72); err != nil {
		return err
	}
	outputIDs := make([]string, 0, len(document.Regions)+1)
	document.RegionAssetIDs = make(map[string]string)
	for i, region := range document.Regions {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !region.ExportEnabled {
			continue
		}
		crop, err := uiassets.CropImage(source, uiassets.PixelRect(region.Bounds))
		if err != nil {
			return err
		}
		cropAsset, _, err := a.storeGeneratedNRGBA(crop, ToolAsset{
			Name:          fmt.Sprintf("%03d-%s.png", i+1, safeAssetName(region.Name)),
			Usage:         "ui_component",
			SourceModule:  ToolchainModuleUI,
			ParentAssetID: sourceAsset.ID,
			Provider:      job.Provider,
			Model:         job.Model,
			Tags:          []string{"ui", "inferred", region.Type},
		})
		if err != nil {
			return err
		}
		document.RegionAssetIDs[region.ID] = cropAsset.ID
		outputIDs = append(outputIDs, cropAsset.ID)
		if _, err := control.SetOutputAssetIDs(outputIDs); err != nil {
			return err
		}
	}
	documentJSON, err := json.Marshal(document)
	if err != nil {
		return err
	}
	analysisAsset, err := a.toolchainStore.CreateAssetFromBytes(ToolAsset{
		Name:          job.ID + "-ui-analysis.json",
		MediaType:     AssetMediaJSON,
		MIMEType:      "application/json",
		Usage:         "ui_analysis",
		SourceModule:  ToolchainModuleUI,
		ParentAssetID: sourceAsset.ID,
		Prompt:        req.Prompt,
		Provider:      job.Provider,
		Model:         job.Model,
		Tags:          []string{"ui", "analysis", "inferred"},
	}, documentJSON)
	if err != nil {
		return err
	}
	outputIDs = append([]string{analysisAsset.ID}, outputIDs...)
	if _, err := control.SetOutputAssetIDs(outputIDs); err != nil {
		return err
	}
	if _, err := control.SetProviderResponseSummary(documentJSON); err != nil {
		return err
	}
	_, err = control.SetProgress("saving", 0.98)
	return err
}

func (a *App) runUISplitJob(ctx context.Context, job GenerationJob, control *GenerationJobControl) error {
	var req uiSplitJobRequest
	if err := decodeStoredJobRequest(job.Request, &req); err != nil {
		return err
	}
	raw, sourceAsset, err := a.readAssetBytes(req.SourceAssetID, maxLocalImageBytes)
	if err != nil {
		return err
	}
	source, info, err := uiassets.DecodeBytes(raw, uiassets.DefaultDecodeLimits())
	if err != nil {
		return err
	}
	options := uiassets.SplitOptions{
		Mode:      uiassets.SplitMode(req.Mode),
		Trim:      req.TrimTransparent,
		Padding:   req.Padding,
		Detection: uiassets.DefaultDetectionOptions(),
	}
	switch options.Mode {
	case uiassets.SplitGrid:
		options.Grid = uiassets.GridOptions{Rows: req.Rows, Columns: req.Columns}
	case uiassets.SplitManual:
		options.Manual = make([]uiassets.Region, 0, len(req.Regions))
		for i, region := range req.Regions {
			options.Manual = append(options.Manual, uiassets.Region{
				ID:     firstNonBlank(region.ID, fmt.Sprintf("manual-%03d", i+1)),
				Name:   firstNonBlank(region.Name, fmt.Sprintf("Region %03d", i+1)),
				Bounds: uiassets.PixelRect(region.Bounds),
			})
		}
	case uiassets.SplitAuto:
		if imageHasTransparency(source) {
			options.Detection.BackgroundMode = uiassets.BackgroundAlpha
		} else if req.BackgroundColor != "" {
			background, err := parseHexRGBA(req.BackgroundColor)
			if err != nil {
				return err
			}
			options.Detection.BackgroundMode = uiassets.BackgroundSolid
			options.Detection.BackgroundColor = &background
		}
	}
	if _, err := control.SetProgress("detecting_regions", 0.18); err != nil {
		return err
	}
	crops, err := uiassets.Split(source, options)
	if err != nil {
		return err
	}
	if len(crops) == 0 {
		return errors.New("没有检测到可导出的 UI 切片")
	}
	regions := make([]UIRegion, 0, len(crops))
	assetIDs := make([]string, 0, len(crops))
	regionAssetIDs := make(map[string]string, len(crops))
	for i, crop := range crops {
		if err := ctx.Err(); err != nil {
			return err
		}
		asset, _, err := a.storeGeneratedNRGBA(crop.Image, ToolAsset{
			Name:          fmt.Sprintf("%03d-%s.png", i+1, safeAssetName(crop.Region.Name)),
			Usage:         "ui_slice",
			SourceModule:  ToolchainModuleUI,
			ParentAssetID: sourceAsset.ID,
			Tags:          []string{"ui", "slice", req.Mode},
		})
		if err != nil {
			return err
		}
		assetIDs = append(assetIDs, asset.ID)
		regionAssetIDs[crop.Region.ID] = asset.ID
		regions = append(regions, UIRegion{
			ID:            crop.Region.ID,
			Name:          crop.Region.Name,
			Type:          "slice",
			Bounds:        UIPixelBounds(crop.Region.Bounds),
			ZIndex:        i,
			Confidence:    1,
			ExportEnabled: true,
		})
		if _, err := control.SetOutputAssetIDs(assetIDs); err != nil {
			return err
		}
		_, _ = control.SetProgress("saving_slices", 0.3+0.65*float64(i+1)/float64(len(crops)))
	}
	summary, _ := json.Marshal(map[string]any{
		"canvas":         uiassets.Size{Width: info.Width, Height: info.Height},
		"regions":        regions,
		"regionAssetIds": regionAssetIDs,
		"mode":           req.Mode,
	})
	if _, err := control.SetProviderResponseSummary(summary); err != nil {
		return err
	}
	_, err = control.SetProgress("saving", 0.98)
	return err
}

func toolchainImageProvider(provider, model string) (gen.Provider, error) {
	settings := config.Load()
	cfg := settings.Cfg(provider)
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("%s API 密钥未设置，请在设置中输入。", gen.ProviderLabel(provider))
	}
	if provider == gen.ProviderZetaTechs {
		return gen.NewZetaTechsWithBaseURL(cfg.APIKey, model, cfg.BaseURL), nil
	}
	return gen.New(provider, cfg.APIKey, model)
}

func toolchainVideoProvider(provider, model string) (gen.VideoProvider, error) {
	settings := config.Load()
	cfg := settings.Cfg(provider)
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("%s API 密钥未设置，请在设置中输入。", gen.ProviderLabel(provider))
	}
	if provider == gen.ProviderVolcengine {
		return gen.NewVolcengineWithBaseURL(cfg.APIKey, model, cfg.BaseURL), nil
	}
	if provider == gen.ProviderZetaTechs {
		return gen.NewZetaTechsWithBaseURL(cfg.APIKey, model, cfg.BaseURL), nil
	}
	return nil, fmt.Errorf("%s 不支持视频生成", gen.ProviderLabel(provider))
}

func toolchainTextClient(provider, model string) (*gen.TextClient, error) {
	settings := config.Load()
	cfg := settings.Cfg(provider)
	key := firstNonBlank(cfg.TextAPIKey, cfg.APIKey)
	if key == "" {
		return nil, fmt.Errorf("%s 文本 API 密钥未设置", gen.ProviderLabel(provider))
	}
	return gen.NewTextClient(provider, key, model, cfg.BaseURL)
}

func toolchainVisionClient(provider, model string) (*gen.VisionClient, error) {
	settings := config.Load()
	cfg := settings.Cfg(provider)
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("%s 视觉 API 密钥未设置", gen.ProviderLabel(provider))
	}
	return gen.NewVisionClient(provider, cfg.APIKey, model, cfg.BaseURL)
}

func (a *App) buildVideoRequest(req videoJobRequest, model string) (gen.VideoRequest, error) {
	references, _, err := a.loadImageAssets(req.ReferenceAssetIDs)
	if err != nil {
		return gen.VideoRequest{}, err
	}
	media := make([]gen.VideoMediaInput, 0, len(references))
	for i, raw := range references {
		media = append(media, gen.VideoMediaInput{Data: raw, MIME: "image/png", Label: fmt.Sprintf("reference-%d", i+1)})
	}
	referenceMode := strings.TrimSpace(req.ReferenceMode)
	if req.Mode == "image" && len(media) > 0 && (referenceMode == "" || referenceMode == gen.VideoReferenceModeNone || referenceMode == gen.VideoReferenceModeAuto) {
		capability := gen.VideoModelCapabilityFor(model)
		if capability.ReferenceSemantics == "first_frame" || (!capability.SupportsIdentityReference && capability.SupportsFirstFrameReference) {
			referenceMode = gen.VideoReferenceModeFirstFrame
		} else {
			referenceMode = gen.VideoReferenceModeIdentityReference
		}
	}
	request := gen.VideoRequest{
		Model:           model,
		Prompt:          req.Prompt,
		NegativePrompt:  req.NegativePrompt,
		ReferenceMode:   referenceMode,
		MediaSlots:      make(map[string][]gen.VideoMediaInput),
		Controls:        map[string]any{"generate_audio": req.Audio, "watermark": req.Watermark},
		Seconds:         req.DurationSeconds,
		FPS:             req.FPS,
		AspectRatio:     req.AspectRatio,
		Resolution:      req.Resolution,
		ProviderAdapter: gen.VideoModelCapabilityFor(model).ProviderAdapter,
	}
	if len(media) > 0 {
		request.ReferenceImage = media[0].Data
	}
	switch referenceMode {
	case "first_frame":
		request.Workflow = gen.VideoWorkflowImageToVideo
		request.MediaSlots[gen.VideoSlotStartImage] = media[:min(1, len(media))]
	case "start_end_frame":
		request.Workflow = gen.VideoWorkflowFirstLastToVideo
		request.MediaSlots[gen.VideoSlotStartImage] = media[:min(1, len(media))]
		last, _, err := a.loadImageAssets([]string{req.LastFrameAssetID})
		if err != nil {
			return gen.VideoRequest{}, err
		}
		request.MediaSlots[gen.VideoSlotEndImage] = []gen.VideoMediaInput{{Data: last[0], MIME: "image/png", Label: "last-frame"}}
	case "identity_reference", "multi_reference":
		request.Workflow = gen.VideoWorkflowReferenceToVideo
		request.MediaSlots[gen.VideoSlotReferenceImages] = media
	case "none", "":
		request.Workflow = gen.VideoWorkflowTextToVideo
		request.ReferenceMode = gen.VideoReferenceModeNone
	}
	return request, nil
}

func pollProviderVideo(ctx context.Context, provider gen.VideoProvider, taskID string, control *GenerationJobControl) (gen.VideoResult, error) {
	backoff := 2 * time.Second
	errorsInRow := 0
	deadline := time.NewTimer(45 * time.Minute)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			return gen.VideoResult{}, ctx.Err()
		case <-deadline.C:
			_, _ = control.Update(terminalJobUpdate(GenerationJobStopped, "stopped", "本地轮询超过 45 分钟，已停止等待；上游任务可能仍在继续", true))
			return gen.VideoResult{}, errors.New("视频任务轮询超过 45 分钟")
		case <-time.After(backoff):
		}
		result, err := provider.PollVideo(ctx, taskID)
		if err != nil {
			errorsInRow++
			if errorsInRow >= maxProviderPollErrors {
				_, _ = control.Update(terminalJobUpdate(GenerationJobStopped, "stopped", "本地轮询连续失败，已停止等待；上游任务可能仍在继续", true))
				return gen.VideoResult{}, fmt.Errorf("视频任务轮询连续失败: %w", err)
			}
			backoff = minDuration(20*time.Second, backoff*2)
			continue
		}
		errorsInRow = 0
		backoff = minDuration(10*time.Second, backoff+time.Second)
		progress := 0.18
		if result.Progress > 0 {
			progress += 0.68 * float64(min(100, result.Progress)) / 100
		}
		if _, err := control.SetPolling("polling", progress); err != nil {
			return gen.VideoResult{}, err
		}
		if result.Failed() {
			return result, fmt.Errorf("视频生成失败: %s", strings.TrimSpace(result.Message))
		}
		if result.Done() {
			return result, nil
		}
	}
}

func downloadCompletedVideo(ctx context.Context, provider gen.VideoProvider, taskID string, result gen.VideoResult) ([]byte, string, error) {
	if strings.TrimSpace(result.URL) != "" {
		downloader := safemedia.NewDownloader(safemedia.DownloaderOptions{})
		download, err := downloader.Download(ctx, result.URL, safemedia.VideoDownloadLimits())
		if err != nil {
			return nil, "", err
		}
		return download.Data, download.MIMEType, nil
	}
	data, err := provider.DownloadVideo(ctx, taskID)
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 || len(data) > maxLocalVideoBytes {
		return nil, "", errors.New("视频产物为空或超过 256 MB 限制")
	}
	mimeType := strings.ToLower(strings.TrimSpace(http.DetectContentType(data[:min(len(data), 512)])))
	if mimeType != "video/mp4" && mimeType != "video/webm" && mimeType != "video/quicktime" {
		return nil, "", fmt.Errorf("Provider 返回了不支持的视频 MIME：%s", mimeType)
	}
	return data, mimeType, nil
}

func (a *App) loadImageAssets(ids []string) ([][]byte, []ToolAsset, error) {
	images := make([][]byte, 0, len(ids))
	assets := make([]ToolAsset, 0, len(ids))
	for _, id := range ids {
		raw, asset, err := a.readAssetBytes(id, maxLocalImageBytes)
		if err != nil {
			return nil, nil, err
		}
		if asset.MediaType != AssetMediaImage {
			return nil, nil, fmt.Errorf("资产 %s 不是图片", id)
		}
		img, _, err := uiassets.DecodeBytes(raw, uiassets.DefaultDecodeLimits())
		if err != nil {
			return nil, nil, fmt.Errorf("解码参考图 %s: %w", asset.Name, err)
		}
		pngBytes, err := encodeRasterPNG(img)
		if err != nil {
			return nil, nil, err
		}
		images = append(images, pngBytes)
		assets = append(assets, asset)
	}
	return images, assets, nil
}

func (a *App) readAssetBytes(id string, maxBytes int64) ([]byte, ToolAsset, error) {
	asset, err := a.toolchainStore.GetAsset(strings.TrimSpace(id))
	if err != nil {
		return nil, ToolAsset{}, err
	}
	path, err := a.toolchainStore.ResolveAssetContentPath(asset.ID)
	if err != nil {
		return nil, ToolAsset{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, ToolAsset{}, err
	}
	if info.Size() <= 0 || info.Size() > maxBytes {
		return nil, ToolAsset{}, fmt.Errorf("资产 %s 超过大小限制", asset.Name)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, ToolAsset{}, err
	}
	return data, asset, nil
}

func (a *App) storeGeneratedImage(raw []byte, metadata ToolAsset) (ToolAsset, *image.NRGBA, error) {
	if len(raw) == 0 || len(raw) > maxLocalImageBytes {
		return ToolAsset{}, nil, errors.New("图片产物为空或超过 32 MB 限制")
	}
	img, _, err := uiassets.DecodeBytes(raw, uiassets.DefaultDecodeLimits())
	if err != nil {
		return ToolAsset{}, nil, fmt.Errorf("Provider 图片产物无法解码: %w", err)
	}
	return a.storeGeneratedNRGBA(img, metadata)
}

func (a *App) storeGeneratedNRGBA(img *image.NRGBA, metadata ToolAsset) (ToolAsset, *image.NRGBA, error) {
	if img == nil || img.Bounds().Empty() {
		return ToolAsset{}, nil, errors.New("图片产物为空")
	}
	raw, err := encodeRasterPNG(img)
	if err != nil {
		return ToolAsset{}, nil, err
	}
	metadata.MediaType = AssetMediaImage
	metadata.MIMEType = "image/png"
	if filepath.Ext(metadata.Name) == "" {
		metadata.Name += ".png"
	}
	asset, err := a.toolchainStore.CreateAssetFromBytes(metadata, raw)
	if err != nil {
		return ToolAsset{}, nil, err
	}
	_ = a.setImageThumbnail(asset.ID, img)
	asset, _ = a.toolchainStore.GetAsset(asset.ID)
	return asset, img, nil
}

func (a *App) ensureAssetThumbnail(id string) error {
	raw, asset, err := a.readAssetBytes(id, maxLocalImageBytes)
	if err != nil {
		return err
	}
	if asset.ThumbnailAvailable {
		return nil
	}
	img, _, err := uiassets.DecodeBytes(raw, uiassets.DefaultDecodeLimits())
	if err != nil {
		return err
	}
	return a.setImageThumbnail(id, img)
}

func (a *App) setImageThumbnail(id string, img *image.NRGBA) error {
	if img == nil || img.Bounds().Empty() {
		return errors.New("thumbnail source is empty")
	}
	const maxSide = 384
	width, height := img.Bounds().Dx(), img.Bounds().Dy()
	scale := min(1.0, float64(maxSide)/float64(max(width, height)))
	targetW := max(1, int(math.Round(float64(width)*scale)))
	targetH := max(1, int(math.Round(float64(height)*scale)))
	thumb := image.NewNRGBA(image.Rect(0, 0, targetW, targetH))
	xdraw.CatmullRom.Scale(thumb, thumb.Bounds(), img, img.Bounds(), xdraw.Src, nil)
	raw, err := encodeRasterPNG(thumb)
	if err != nil {
		return err
	}
	_, err = a.toolchainStore.SetAssetThumbnail(id, "image/png", raw)
	return err
}

func compositeMaskedImage(base *image.NRGBA, generated []byte, mask *image.Alpha) ([]byte, error) {
	if base == nil || mask == nil {
		return nil, errors.New("局部重绘合成缺少源图或遮罩")
	}
	edited, _, err := uiassets.DecodeBytes(generated, uiassets.DefaultDecodeLimits())
	if err != nil {
		return nil, err
	}
	if edited.Bounds().Size() != base.Bounds().Size() {
		resized := image.NewNRGBA(base.Bounds())
		xdraw.CatmullRom.Scale(resized, resized.Bounds(), edited, edited.Bounds(), xdraw.Src, nil)
		edited = resized
	}
	combined, err := uiassets.Composite(base, edited, mask)
	if err != nil {
		return nil, err
	}
	return encodeRasterPNG(combined)
}

func validateUIAnalysis(raw json.RawMessage, width, height int) (uiAnalysisDocument, error) {
	if !json.Valid(raw) {
		return uiAnalysisDocument{}, errors.New("视觉模型没有返回合法 JSON")
	}
	var provider providerUIAnalysis
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&provider); err != nil {
		return uiAnalysisDocument{}, fmt.Errorf("视觉 JSON 不符合 UI 区域结构: %w", err)
	}
	if len(provider.Regions) == 0 || len(provider.Regions) > maxUIAnalysisRegions {
		return uiAnalysisDocument{}, fmt.Errorf("视觉模型必须返回 1 到 %d 个区域", maxUIAnalysisRegions)
	}
	canvas := uiassets.Size{Width: width, Height: height}
	regions := make([]UIRegion, 0, len(provider.Regions))
	idMap := make(map[string]string, len(provider.Regions))
	used := make(map[string]bool, len(provider.Regions))
	for i, candidate := range provider.Regions {
		originalID := strings.TrimSpace(candidate.ID)
		id := uniqueUIRegionID(firstNonBlank(originalID, candidate.Name, fmt.Sprintf("region-%03d", i+1)), used)
		if originalID != "" {
			if _, exists := idMap[originalID]; !exists {
				idMap[originalID] = id
			}
		}
		bounds := uiassets.PixelRect(candidate.Bounds).Intersect(canvas)
		if bounds.Empty() {
			return uiAnalysisDocument{}, fmt.Errorf("视觉区域 %d 不在源图范围内", i+1)
		}
		confidence := candidate.Confidence
		if math.IsNaN(confidence) || math.IsInf(confidence, 0) {
			return uiAnalysisDocument{}, errors.New("视觉区域置信度必须是有限数字")
		}
		confidence = max(0, min(1, confidence))
		exportEnabled := true
		if candidate.ExportEnabled != nil {
			exportEnabled = *candidate.ExportEnabled
		}
		regions = append(regions, UIRegion{
			ID:            id,
			Name:          firstNonBlank(candidate.Name, fmt.Sprintf("Region %03d", i+1)),
			Type:          firstNonBlank(candidate.Type, "component"),
			Bounds:        UIPixelBounds(bounds),
			ZIndex:        candidate.ZIndex,
			Confidence:    confidence,
			Text:          strings.TrimSpace(candidate.Text),
			Style:         candidate.Style,
			ExportEnabled: exportEnabled,
		})
	}
	for i, candidate := range provider.Regions {
		parentOriginalID := strings.TrimSpace(candidate.ParentID)
		if parentOriginalID == "" {
			continue
		}
		parentID := idMap[parentOriginalID]
		if parentID != "" && parentID != regions[i].ID {
			regions[i].ParentID = parentID
		}
	}
	breakUIRegionParentCycles(regions)
	return uiAnalysisDocument{
		Canvas:           canvas,
		Regions:          regions,
		Tokens:           provider.Tokens,
		Copy:             strings.TrimSpace(provider.Copy),
		RecreationPrompt: strings.TrimSpace(provider.RecreationPrompt),
		Inferred:         true,
	}, nil
}

func breakUIRegionParentCycles(regions []UIRegion) {
	parent := make(map[string]string, len(regions))
	for _, region := range regions {
		parent[region.ID] = region.ParentID
	}
	for i := range regions {
		seen := map[string]bool{regions[i].ID: true}
		current := regions[i].ParentID
		for current != "" {
			if seen[current] {
				regions[i].ParentID = ""
				break
			}
			seen[current] = true
			current = parent[current]
		}
	}
}

func uniqueUIRegionID(value string, used map[string]bool) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune(r)
		case builder.Len() > 0:
			builder.WriteByte('-')
		}
	}
	base := strings.Trim(builder.String(), "-_")
	if base == "" {
		base = "region"
	}
	id := base
	for suffix := 2; used[id]; suffix++ {
		id = base + "-" + strconv.Itoa(suffix)
	}
	used[id] = true
	return id
}

func promptOptimizationSystemPrompt(purpose string) string {
	common := "You rewrite production prompts. Return only the improved prompt, never commentary, headings, quotes, or markdown. Preserve the user's intent and do not silently add copyrighted characters, brands, or living artists."
	switch purpose {
	case "video":
		return common + " Optimize for game-asset video generation: describe subject consistency, camera behavior, temporal action, motion cadence, loopability when implied, lighting, background, and artifacts to avoid. Keep controls such as duration and resolution out of prose unless the user explicitly supplied them."
	case "ui":
		return common + " Optimize for a complete game UI concept or UI element generation: state screen purpose, information hierarchy, component inventory, layout, spacing, typography character, color tokens, icon treatment, interaction states, readability, and platform constraints. Do not request HTML, React, or CSS."
	default:
		return common + " Optimize for game image generation: make subject, composition, camera, silhouette, materials, lighting, palette, background, asset usability, and unwanted artifacts concrete. Preserve requested art direction without naming living artists."
	}
}

func uiAnalysisSystemPrompt() string {
	return `You are a game UI screenshot analyzer. Return one strict JSON object and nothing else. Analyze only visible pixels; never claim hidden layers, original font files, or source design objects. Coordinates must be integer source-image pixels. The exact schema is:
{"regions":[{"id":"stable-id","parentId":"optional-parent-id","name":"human name","type":"screen|panel|button|icon|image|text|input|list|slot|badge|divider|component","bounds":{"x":0,"y":0,"width":1,"height":1},"zIndex":0,"confidence":0.0,"text":"visible copy or empty","style":{"colors":["#RRGGBB"],"spacing":0,"radius":0,"notes":"inferred visible styling"},"exportEnabled":true}],"tokens":{"colors":{},"spacing":{},"typography":{}},"copy":"all visible copy in reading order","recreationPrompt":"a concise prompt for recreating the visible screen without code"}
Every region must be a useful visible component, stay inside the canvas, and use parentId only when that parent id exists. Confidence is 0 to 1.`
}

func uiAnalysisUserPrompt(userPrompt string, width, height int) string {
	contextText := strings.TrimSpace(userPrompt)
	if contextText == "" {
		contextText = "No additional product context was supplied."
	}
	return fmt.Sprintf("Analyze this %dx%d game UI screenshot. Product context: %s", width, height, contextText)
}

func composeGenerationPrompt(prompt, negative string) string {
	prompt = strings.TrimSpace(prompt)
	negative = strings.TrimSpace(negative)
	if negative == "" {
		return prompt
	}
	return prompt + "\n\nAvoid: " + negative
}

func imageResultUsage(req imageJobRequest) string {
	if req.Purpose == "ui_concept" {
		return "ui_concept"
	}
	if req.Mode == "masked_edit" || req.Mode == "edit" {
		return "edited_image"
	}
	return "generated_image"
}

func classifyPaidSubmissionError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(message, "network") || strings.Contains(message, "网络") ||
		strings.Contains(message, "timeout") || strings.Contains(message, "eof") ||
		strings.Contains(message, "connection reset") || strings.Contains(message, "broken pipe") ||
		provider5xxPattern.MatchString(message) {
		return fmt.Errorf("%w: Provider 创建请求结果未知，请先核对远端任务再重试", ErrSubmissionOutcomeUnknown)
	}
	return err
}

func decodeStoredJobRequest(request map[string]any, target any) error {
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("读取任务请求失败: %w", err)
	}
	return nil
}

func encodeRasterPNG(img image.Image) ([]byte, error) {
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func encodeAlphaMaskPNG(mask *image.Alpha) ([]byte, error) {
	if mask == nil || mask.Bounds().Empty() {
		return nil, errors.New("mask is empty")
	}
	rgba := image.NewNRGBA(mask.Bounds())
	for y := mask.Bounds().Min.Y; y < mask.Bounds().Max.Y; y++ {
		for x := mask.Bounds().Min.X; x < mask.Bounds().Max.X; x++ {
			alpha := mask.AlphaAt(x, y).A
			rgba.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: alpha})
		}
	}
	return encodeRasterPNG(rgba)
}

func imageHasTransparency(img *image.NRGBA) bool {
	if img == nil {
		return false
	}
	for index := 3; index < len(img.Pix); index += 4 {
		if img.Pix[index] < 250 {
			return true
		}
	}
	return false
}

func parseHexRGBA(value string) (uiassets.RGBA, error) {
	value = strings.TrimSpace(strings.TrimPrefix(value, "#"))
	if len(value) != 6 && len(value) != 8 {
		return uiassets.RGBA{}, errors.New("背景色必须是 #RRGGBB 或 #RRGGBBAA")
	}
	decoded, err := strconv.ParseUint(value, 16, 32)
	if err != nil {
		return uiassets.RGBA{}, errors.New("背景色格式无效")
	}
	if len(value) == 6 {
		return uiassets.RGBA{R: uint8(decoded >> 16), G: uint8(decoded >> 8), B: uint8(decoded), A: 255}, nil
	}
	return uiassets.RGBA{R: uint8(decoded >> 24), G: uint8(decoded >> 16), B: uint8(decoded >> 8), A: uint8(decoded)}, nil
}

func safeAssetName(value string) string {
	name := sanitizeName(strings.TrimSpace(value))
	if name == "" {
		return "asset"
	}
	return name
}

func videoDimensions(resolution, aspectRatio string) (int, int) {
	short := 720
	switch strings.ToLower(strings.TrimSpace(resolution)) {
	case "480p":
		short = 480
	case "1080p":
		short = 1080
	}
	parts := strings.Split(aspectRatio, ":")
	if len(parts) != 2 {
		return short, short
	}
	wRatio, errW := strconv.Atoi(parts[0])
	hRatio, errH := strconv.Atoi(parts[1])
	if errW != nil || errH != nil || wRatio <= 0 || hRatio <= 0 {
		return short, short
	}
	if wRatio >= hRatio {
		return int(math.Round(float64(short) * float64(wRatio) / float64(hRatio))), short
	}
	return short, int(math.Round(float64(short) * float64(hRatio) / float64(wRatio)))
}

func probeVideoMetadata(ctx context.Context, data []byte, extension string) (int, int, int64, error) {
	probe, err := exec.LookPath("ffprobe")
	if err != nil {
		probe, err = exec.LookPath("ffprobe.exe")
		if err != nil {
			return 0, 0, 0, err
		}
	}
	dir, err := os.MkdirTemp("", "perfectpixel-video-probe-")
	if err != nil {
		return 0, 0, 0, err
	}
	defer os.RemoveAll(dir)
	if extension == "" {
		extension = ".mp4"
	}
	path := filepath.Join(dir, "output"+extension)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return 0, 0, 0, err
	}
	output, err := exec.CommandContext(ctx, probe,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height:format=duration",
		"-of", "json",
		path,
	).Output()
	if err != nil {
		return 0, 0, 0, err
	}
	var parsed struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(output, &parsed); err != nil || len(parsed.Streams) == 0 {
		return 0, 0, 0, errors.New("ffprobe did not return a video stream")
	}
	duration, err := strconv.ParseFloat(parsed.Format.Duration, 64)
	if err != nil || duration <= 0 || duration > 60*60 {
		return 0, 0, 0, errors.New("ffprobe returned an invalid duration")
	}
	width, height := parsed.Streams[0].Width, parsed.Streams[0].Height
	if width <= 0 || height <= 0 {
		return 0, 0, 0, errors.New("ffprobe returned invalid dimensions")
	}
	return width, height, int64(math.Round(duration * 1000)), nil
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
