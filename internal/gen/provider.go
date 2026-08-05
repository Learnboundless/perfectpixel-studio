package gen

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"perfectpixel/internal/safemedia"
)

// 지원 프로바이더 식별자
const (
	ProviderGemini     = "gemini"
	ProviderOpenAI     = "openai"
	ProviderOpenRouter = "openrouter"
	ProviderFal        = "fal"
	ProviderBytePlus   = "byteplus"
	ProviderZetaTechs  = "zetatechs"
	ProviderVolcengine = "volcengine"
)

// SupportedProviders는 지원 프로바이더 식별자 목록입니다 (UI 노출 순서).
var SupportedProviders = []string{ProviderGemini, ProviderOpenAI, ProviderOpenRouter, ProviderFal, ProviderBytePlus, ProviderZetaTechs, ProviderVolcengine}

type paidRequestRetryContextKey struct{}

// WithoutPaidRequestRetries marks a creation context as single-attempt. New
// persistent jobs use it so an ambiguous paid POST is never replayed locally.
func WithoutPaidRequestRetries(ctx context.Context) context.Context {
	return context.WithValue(ctx, paidRequestRetryContextKey{}, true)
}

func paidRequestRetriesDisabled(ctx context.Context) bool {
	disabled, _ := ctx.Value(paidRequestRetryContextKey{}).(bool)
	return disabled
}

// modelCatalog는 프로바이더별 선택 가능한 이미지 모델 목록입니다 (최신 모델이 맨 앞).
var modelCatalog = map[string][]string{
	ProviderGemini: {
		"gemini-3-pro-image", // Nano Banana Pro (최신)
		"gemini-3-pro-image-preview",
		"gemini-3.1-flash-image-preview", // Nano Banana 2
		"gemini-3.1-flash-image",
		"gemini-2.5-flash-image", // Nano Banana
	},
	ProviderOpenAI: {
		"gpt-image-2",
		"gpt-image-1.5",
		"gpt-image-1",
		"gpt-image-1-mini",
	},
	ProviderOpenRouter: {
		"google/gemini-3-pro-image-preview", // 최신
		"google/gemini-2.5-flash-image",
		"google/gemini-2.5-flash-image-preview",
	},
	ProviderFal: {
		"fal-ai/nano-banana-pro", // 최신
		"fal-ai/nano-banana",
		"fal-ai/flux-pro/v1.1-ultra",
		"fal-ai/flux/dev",
	},
	ProviderBytePlus: {
		"seedream-4-0-250828",     // Seedream 4.0 (최신)
		"seedream-3-0-t2i-250415", // Seedream 3.0
		"seededit-3-0-i2i-250628", // SeedEdit 3.0 (이미지 편집)
	},
	ProviderZetaTechs: {
		"gpt-image-2",
		"gpt-image-1.5",
		"gpt-image-1",                    // OpenAI Image 호환, 다중 이미지 편집 지원
		"gpt-image-1-mini",               // ZetaTechs 모델 광장에 노출되는 경량 이미지 모델
		"gemini-3.1-flash-image-preview", // Nano Banana 2
	},
}

// videoModelCatalog는 프로바이더별 선택 가능한 비디오 모델 목록입니다.
var videoModelCatalog = map[string][]string{
	ProviderZetaTechs: {
		"wan2.6-t2v",
		"wan2.6-i2v",
		"wan2.5-t2v-preview",
		"wan2.5-i2v-preview",
		"wan2.2-t2v-plus",
		"wan2.2-i2v-plus",
		"wan2.2-i2v-flash",
		"wanx2.1-t2v-plus",
		"wanx2.1-t2v-turbo",
	},
	ProviderVolcengine: {
		"doubao-seedance-2-0-mini-260615",
	},
}

// VideoModelShowcase describes a real smoke-tested sample shipped with the app.
type VideoModelShowcase struct {
	Model     string `json:"model"`
	SampleURL string `json:"sampleUrl"`
	Workflow  string `json:"workflow,omitempty"`
	TestedAt  string `json:"testedAt,omitempty"`
	Bytes     int64  `json:"bytes,omitempty"`
	Source    string `json:"source,omitempty"`
}

var videoModelShowcaseCatalog = map[string]map[string]VideoModelShowcase{
	ProviderZetaTechs: {
		"wan2.6-t2v": {
			Model:     "wan2.6-t2v",
			SampleURL: "/video-model-samples/wan2.6-t2v.mp4",
			Workflow:  VideoWorkflowTextToVideo,
			TestedAt:  "2026-07-25T21:08:08+08:00",
			Bytes:     1382521,
			Source:    "zeta-video-smoke-real-20260725-210653-ref-video-size",
		},
		"wan2.6-i2v": {
			Model:     "wan2.6-i2v",
			SampleURL: "/video-model-samples/wan2.6-i2v.mp4",
			Workflow:  VideoWorkflowImageToVideo,
			TestedAt:  "2026-07-25T20:55:17+08:00",
			Bytes:     3481337,
			Source:    "zeta-video-smoke-real-20260725-205354-quota-resume",
		},
		"wan2.5-t2v-preview": {
			Model:     "wan2.5-t2v-preview",
			SampleURL: "/video-model-samples/wan2.5-t2v-preview.mp4",
			Workflow:  VideoWorkflowTextToVideo,
			TestedAt:  "2026-07-25T20:46:54+08:00",
			Bytes:     943437,
			Source:    "zeta-video-smoke-real-20260725-203939-all-video-fixed3",
		},
		"wan2.5-i2v-preview": {
			Model:     "wan2.5-i2v-preview",
			SampleURL: "/video-model-samples/wan2.5-i2v-preview.mp4",
			Workflow:  VideoWorkflowImageToVideo,
			TestedAt:  "2026-07-25T20:45:06+08:00",
			Bytes:     2392215,
			Source:    "zeta-video-smoke-real-20260725-203939-all-video-fixed3",
		},
		"wan2.2-t2v-plus": {
			Model:     "wan2.2-t2v-plus",
			SampleURL: "/video-model-samples/wan2.2-t2v-plus.mp4",
			Workflow:  VideoWorkflowTextToVideo,
			TestedAt:  "2026-07-25T20:43:08+08:00",
			Bytes:     3514606,
			Source:    "zeta-video-smoke-real-20260725-203939-all-video-fixed3",
		},
		"wan2.2-i2v-plus": {
			Model:     "wan2.2-i2v-plus",
			SampleURL: "/video-model-samples/wan2.2-i2v-plus.mp4",
			Workflow:  VideoWorkflowImageToVideo,
			TestedAt:  "2026-07-25T20:41:52+08:00",
			Bytes:     637387,
			Source:    "zeta-video-smoke-real-20260725-203939-all-video-fixed3",
		},
		"wan2.2-i2v-flash": {
			Model:     "wan2.2-i2v-flash",
			SampleURL: "/video-model-samples/wan2.2-i2v-flash.mp4",
			Workflow:  VideoWorkflowImageToVideo,
			TestedAt:  "2026-07-25T20:40:37+08:00",
			Bytes:     3876164,
			Source:    "zeta-video-smoke-real-20260725-203939-all-video-fixed3",
		},
		"wanx2.1-t2v-plus": {
			Model:     "wanx2.1-t2v-plus",
			SampleURL: "/video-model-samples/wanx2.1-t2v-plus.mp4",
			Workflow:  VideoWorkflowTextToVideo,
			TestedAt:  "2026-07-25T21:12:19+08:00",
			Bytes:     4622820,
			Source:    "zeta-video-smoke-real-20260725-210653-ref-video-size",
		},
		"wanx2.1-t2v-turbo": {
			Model:     "wanx2.1-t2v-turbo",
			SampleURL: "/video-model-samples/wanx2.1-t2v-turbo.mp4",
			Workflow:  VideoWorkflowTextToVideo,
			TestedAt:  "2026-07-25T20:49:17+08:00",
			Bytes:     2952599,
			Source:    "zeta-video-smoke-real-20260725-203939-all-video-fixed3",
		},
	},
	ProviderVolcengine: {
		"doubao-seedance-2-0-mini-260615": {
			Model:     "doubao-seedance-2-0-mini-260615",
			SampleURL: "/video-model-samples/doubao-seedance-2-0-mini-260615.mp4",
			Workflow:  VideoWorkflowReferenceToVideo,
			TestedAt:  "2026-07-26T17:00:07+08:00",
			Bytes:     1214125,
			Source:    "ark-video-smoke-real-20260726-seedance2-mini-i2v-fixed",
		},
	},
}

// VideoModelCapability describes the request shape a video model family is
// expected to support through ZetaTechs/NewAPI.
type VideoModelCapability struct {
	Model                       string   `json:"model"`
	Family                      string   `json:"family"`
	Endpoint                    string   `json:"endpoint"`
	ProviderAdapter             string   `json:"providerAdapter,omitempty"`
	ReferenceSemantics          string   `json:"referenceSemantics"`
	Tags                        []string `json:"tags"`
	WorkflowModes               []string `json:"workflowModes,omitempty"`
	MediaSlots                  []string `json:"mediaSlots,omitempty"`
	SupportsTextPrompt          bool     `json:"supportsTextPrompt"`
	SupportsReferenceImage      bool     `json:"supportsReferenceImage"`
	SupportsIdentityReference   bool     `json:"supportsIdentityReference"`
	SupportsFirstFrameReference bool     `json:"supportsFirstFrameReference"`
	SupportsImageToVideo        bool     `json:"supportsImageToVideo"`
	SupportsStartEndFrames      bool     `json:"supportsStartEndFrames"`
	SupportsReferenceVideo      bool     `json:"supportsReferenceVideo"`
	SupportsReferenceAudio      bool     `json:"supportsReferenceAudio"`
	SupportsMultiImageReference bool     `json:"supportsMultiImageReference"`
	SupportsMotionTransfer      bool     `json:"supportsMotionTransfer"`
	SupportsMetadata            bool     `json:"supportsMetadata"`
	SupportsAudio               bool     `json:"supportsAudio"`
	SupportsWatermarkControl    bool     `json:"supportsWatermarkControl"`
	AllowedSeconds              []int    `json:"allowedSeconds,omitempty"`
	AllowedResolutions          []string `json:"allowedResolutions,omitempty"`
	AllowedAspectRatios         []string `json:"allowedAspectRatios,omitempty"`
}

// ModelsFor는 프로바이더가 제공하는 선택 가능한 모델 목록을 반환합니다 (최신 모델이 맨 앞).
func ModelsFor(provider string) []string {
	if list, ok := modelCatalog[provider]; ok {
		return append([]string(nil), list...)
	}
	return nil
}

// VideoModelsFor는 프로바이더가 제공하는 선택 가능한 비디오 모델 목록을 반환합니다.
func VideoModelsFor(provider string) []string {
	if list, ok := videoModelCatalog[provider]; ok {
		return append([]string(nil), list...)
	}
	return nil
}

// DefaultVideoModelFor는 프로바이더별 기본 비디오 모델을 반환합니다.
func DefaultVideoModelFor(provider string) string {
	if list := VideoModelsFor(provider); len(list) > 0 {
		return list[0]
	}
	return ""
}

// SupportsVideo는 프로바이더가 비디오 생성 파이프라인을 지원하는지 반환합니다.
func SupportsVideo(provider string) bool {
	return len(VideoModelsFor(provider)) > 0
}

// FilterVerifiedVideoModels keeps only models that have a real downloaded smoke
// sample in this app, preserving the product-facing catalog order.
func FilterVerifiedVideoModels(provider string, models []string) []string {
	if len(models) == 0 {
		return nil
	}
	available := make(map[string]bool, len(models))
	for _, model := range models {
		model = strings.ToLower(strings.TrimSpace(model))
		if model != "" {
			available[model] = true
		}
	}
	out := []string{}
	showcases := videoModelShowcaseCatalog[provider]
	for _, model := range VideoModelsFor(provider) {
		if _, verified := showcases[model]; verified && available[strings.ToLower(model)] {
			out = append(out, model)
		}
	}
	return out
}

// IsVerifiedVideoModel reports whether the model is part of the real-tested
// product catalog for a provider.
func IsVerifiedVideoModel(provider, model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return false
	}
	for candidate := range videoModelShowcaseCatalog[provider] {
		if strings.EqualFold(candidate, model) {
			return true
		}
	}
	return false
}

// VideoModelShowcasesFor returns all real-tested video model sample metadata.
func VideoModelShowcasesFor(provider string) map[string]VideoModelShowcase {
	return VideoModelShowcasesForModels(provider, VideoModelsFor(provider))
}

// VideoModelShowcasesForModels returns sample metadata for the given visible
// model list, so dynamic availability can hide models that the upstream no
// longer exposes.
func VideoModelShowcasesForModels(provider string, models []string) map[string]VideoModelShowcase {
	catalog := videoModelShowcaseCatalog[provider]
	if len(catalog) == 0 || len(models) == 0 {
		return nil
	}
	out := make(map[string]VideoModelShowcase, len(models))
	for _, model := range models {
		if sample, ok := catalog[model]; ok {
			out[model] = sample
		}
	}
	return out
}

// VideoModelCapabilitiesFor returns capability labels for a concrete model list.
func VideoModelCapabilitiesFor(models []string) map[string]VideoModelCapability {
	out := make(map[string]VideoModelCapability, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		out[model] = VideoModelCapabilityFor(model)
	}
	return out
}

// VideoModelCapabilityFor infers the ZetaTechs request capabilities from a
// model name. ZetaTechs exposes several upstream families through one gateway,
// so the model name is currently the stable contract available to the app.
func VideoModelCapabilityFor(model string) VideoModelCapability {
	m := strings.ToLower(strings.TrimSpace(model))
	cap := VideoModelCapability{
		Model:                     strings.TrimSpace(model),
		Family:                    "video",
		Endpoint:                  "/v1/videos",
		ProviderAdapter:           "openai_video",
		ReferenceSemantics:        "identity_reference",
		SupportsTextPrompt:        true,
		SupportsIdentityReference: true,
		WorkflowModes:             []string{VideoWorkflowTextToVideo, VideoWorkflowReferenceToVideo},
		MediaSlots:                []string{VideoSlotReferenceImages},
		Tags:                      []string{"text_prompt", "identity_reference", "openai_video"},
		AllowedSeconds:            []int{4, 8, 12},
		AllowedResolutions:        []string{"720p"},
		AllowedAspectRatios:       []string{"1:1", "16:9", "9:16"},
	}
	if m == "" {
		return cap
	}
	if usesModelGenerationEndpoint(m) {
		cap.Endpoint = "/model/v1/generation"
		cap.ProviderAdapter = "vtrix.kirin_video"
		cap.ReferenceSemantics = "media_slots"
		cap.SupportsReferenceImage = true
		cap.SupportsFirstFrameReference = true
		cap.SupportsImageToVideo = true
		cap.SupportsMetadata = true
		cap.WorkflowModes = []string{VideoWorkflowTextToVideo, VideoWorkflowImageToVideo}
		cap.MediaSlots = []string{VideoSlotStartImage}
		cap.Tags = []string{"text_prompt", "image_to_video", "first_frame_reference", "media_slots", "model_generation"}
		if strings.Contains(m, "spark_dance") {
			cap.Family = "Spark Dance"
			cap.ProviderAdapter = "vtrix.content_multimodal"
			cap.SupportsStartEndFrames = true
			cap.SupportsReferenceVideo = true
			cap.SupportsReferenceAudio = true
			cap.SupportsMultiImageReference = true
			cap.WorkflowModes = appendMissingTag(cap.WorkflowModes, VideoWorkflowFirstLastToVideo, VideoWorkflowReferenceToVideo, VideoWorkflowMotionTransfer, VideoWorkflowOmniMultimodal)
			cap.MediaSlots = appendMissingTag(cap.MediaSlots, VideoSlotReferenceImages, VideoSlotEndImage, VideoSlotReferenceVideos, VideoSlotReferenceAudios, VideoSlotDriverVideo, VideoSlotDriverAudio)
			cap.Tags = appendMissingTag(cap.Tags, "start_end_frames", "multi_image_reference", "reference_video", "reference_audio", "animation_transfer")
		} else if strings.Contains(m, "kirin") {
			cap.Family = "Kirin"
			cap.SupportsStartEndFrames = true
			cap.WorkflowModes = appendMissingTag(cap.WorkflowModes, VideoWorkflowFirstLastToVideo)
			cap.MediaSlots = appendMissingTag(cap.MediaSlots, VideoSlotEndImage)
			cap.Tags = appendMissingTag(cap.Tags, "start_end_frames")
		}
		return cap
	}
	if usesVideoGenerationsEndpoint(m) {
		cap.Endpoint = "/v1/video/generations"
		cap.ProviderAdapter = "newapi_video_generations"
		cap.ReferenceSemantics = "first_frame"
		cap.SupportsIdentityReference = false
		cap.WorkflowModes = []string{VideoWorkflowTextToVideo}
		cap.MediaSlots = nil
		cap.Tags = []string{"text_prompt", "video_generations"}
		cap.SupportsMetadata = true
	}
	if strings.HasPrefix(m, "sora") {
		cap.Family = "Sora"
		cap.SupportsReferenceImage = true
		cap.SupportsIdentityReference = true
		cap.SupportsImageToVideo = true
		cap.WorkflowModes = []string{VideoWorkflowTextToVideo, VideoWorkflowReferenceToVideo}
		cap.MediaSlots = []string{VideoSlotReferenceImages}
		cap.Tags = []string{"text_prompt", "identity_reference", "openai_video"}
		return cap
	}
	if strings.HasPrefix(m, "kling") {
		cap.Family = "Kling"
		cap.SupportsReferenceImage = true
		cap.SupportsFirstFrameReference = true
		cap.SupportsImageToVideo = true
		cap.SupportsStartEndFrames = true
		cap.WorkflowModes = []string{VideoWorkflowTextToVideo, VideoWorkflowImageToVideo, VideoWorkflowFirstLastToVideo}
		cap.MediaSlots = []string{VideoSlotStartImage, VideoSlotEndImage}
		cap.Tags = []string{"text_prompt", "image_to_video", "first_frame_reference", "start_end_frames", "metadata", "video_generations"}
		return cap
	}
	if strings.HasPrefix(m, "jimeng") {
		cap.Family = "Jimeng"
		cap.SupportsReferenceImage = true
		cap.SupportsFirstFrameReference = true
		cap.SupportsImageToVideo = true
		cap.SupportsMultiImageReference = true
		cap.SupportsMetadata = true
		cap.WorkflowModes = []string{VideoWorkflowTextToVideo, VideoWorkflowImageToVideo, VideoWorkflowReferenceToVideo}
		cap.MediaSlots = []string{VideoSlotStartImage, VideoSlotReferenceImages}
		cap.Tags = []string{"text_prompt", "image_to_video", "first_frame_reference", "multi_image_reference", "metadata", "video_generations"}
		return cap
	}
	if strings.HasPrefix(m, "vidu") {
		cap.Family = "Vidu"
		cap.SupportsReferenceImage = true
		cap.SupportsFirstFrameReference = true
		cap.SupportsImageToVideo = true
		cap.SupportsMetadata = true
		cap.WorkflowModes = []string{VideoWorkflowTextToVideo, VideoWorkflowImageToVideo}
		cap.MediaSlots = []string{VideoSlotStartImage}
		cap.Tags = []string{"text_prompt", "image_to_video", "first_frame_reference", "metadata", "video_generations"}
		return cap
	}
	if strings.Contains(m, "seedance") {
		cap.Family = "Seedance"
		cap.ProviderAdapter = "seedance_multimodal"
		cap.WorkflowModes = []string{VideoWorkflowTextToVideo}
		if strings.Contains(m, "seedance-2-0") || strings.Contains(m, "seedance-2.0") {
			cap.Endpoint = "/api/v3/contents/generations/tasks"
			cap.ReferenceSemantics = "identity_reference"
			cap.SupportsReferenceImage = true
			cap.SupportsIdentityReference = true
			cap.SupportsFirstFrameReference = true
			cap.SupportsImageToVideo = true
			cap.SupportsStartEndFrames = true
			cap.SupportsReferenceVideo = true
			cap.SupportsReferenceAudio = true
			cap.SupportsMultiImageReference = true
			cap.WorkflowModes = appendMissingTag(cap.WorkflowModes, VideoWorkflowImageToVideo, VideoWorkflowFirstLastToVideo, VideoWorkflowReferenceToVideo, VideoWorkflowOmniMultimodal)
			cap.MediaSlots = []string{VideoSlotReferenceImages, VideoSlotStartImage, VideoSlotEndImage, VideoSlotReferenceVideos, VideoSlotReferenceAudios}
			cap.Tags = []string{"text_to_video", "identity_reference", "image_to_video", "start_end_frames", "multi_image_reference", "reference_video", "reference_audio"}
			cap.SupportsAudio = true
			cap.SupportsWatermarkControl = true
			cap.AllowedSeconds = []int{4, 5, 6, 8, 10, 12, 15}
			cap.AllowedResolutions = []string{"480p", "720p"}
			cap.AllowedAspectRatios = []string{"1:1", "16:9", "9:16", "4:3", "3:4"}
			return cap
		}
		if strings.Contains(m, "i2v") || !strings.Contains(m, "t2v") {
			cap.SupportsReferenceImage = true
			cap.SupportsFirstFrameReference = true
			cap.SupportsImageToVideo = true
			cap.WorkflowModes = appendMissingTag(cap.WorkflowModes, VideoWorkflowImageToVideo)
			cap.MediaSlots = appendMissingTag(cap.MediaSlots, VideoSlotStartImage)
			cap.Tags = appendMissingTag(cap.Tags, "image_to_video", "first_frame_reference")
		}
		if strings.Contains(m, "t2v") || !strings.Contains(m, "i2v") {
			cap.Tags = appendMissingTag(cap.Tags, "text_to_video")
		}
		return cap
	}
	if strings.HasPrefix(m, "wan") || strings.HasPrefix(m, "wanx") {
		cap.Family = "Wan"
		cap.ProviderAdapter = "wan_video_generations"
		cap.WorkflowModes = []string{VideoWorkflowTextToVideo}
		switch {
		case strings.Contains(m, "kf2v"):
			cap.SupportsReferenceImage = true
			cap.SupportsFirstFrameReference = true
			cap.SupportsImageToVideo = true
			cap.SupportsStartEndFrames = true
			cap.WorkflowModes = appendMissingTag(cap.WorkflowModes, VideoWorkflowImageToVideo, VideoWorkflowFirstLastToVideo)
			cap.MediaSlots = appendMissingTag(cap.MediaSlots, VideoSlotStartImage, VideoSlotEndImage)
			cap.Tags = appendMissingTag(cap.Tags, "keyframe_to_video", "start_end_frames", "first_frame_reference")
		case strings.Contains(m, "i2v") || strings.Contains(m, "r2v") || strings.Contains(m, "s2v") || strings.Contains(m, "animate") || strings.Contains(m, "vace"):
			cap.SupportsReferenceImage = true
			cap.SupportsFirstFrameReference = true
			cap.SupportsImageToVideo = true
			cap.WorkflowModes = appendMissingTag(cap.WorkflowModes, VideoWorkflowImageToVideo)
			cap.MediaSlots = appendMissingTag(cap.MediaSlots, VideoSlotStartImage)
			cap.Tags = appendMissingTag(cap.Tags, "image_to_video", "first_frame_reference")
		case strings.Contains(m, "t2v"):
			cap.Tags = appendMissingTag(cap.Tags, "text_to_video")
		}
		if strings.Contains(m, "r2v") || strings.Contains(m, "vace") {
			cap.SupportsReferenceVideo = true
			cap.WorkflowModes = appendMissingTag(cap.WorkflowModes, VideoWorkflowReferenceToVideo)
			cap.MediaSlots = appendMissingTag(cap.MediaSlots, VideoSlotReferenceVideos)
			cap.Tags = appendMissingTag(cap.Tags, "reference_video")
		}
		if strings.Contains(m, "s2v") {
			cap.SupportsReferenceAudio = true
			cap.MediaSlots = appendMissingTag(cap.MediaSlots, VideoSlotReferenceAudios)
			cap.Tags = appendMissingTag(cap.Tags, "reference_audio")
		}
		if strings.Contains(m, "animate") {
			cap.SupportsMotionTransfer = true
			cap.WorkflowModes = appendMissingTag(cap.WorkflowModes, VideoWorkflowMotionTransfer)
			cap.MediaSlots = appendMissingTag(cap.MediaSlots, VideoSlotDriverVideo)
			cap.Tags = appendMissingTag(cap.Tags, "animation_transfer")
		}
		if strings.Contains(m, "vace") {
			cap.Tags = appendMissingTag(cap.Tags, "advanced_control")
		}
		return cap
	}
	if strings.Contains(m, "image-to-video") || strings.Contains(m, "i2v") {
		cap.SupportsReferenceImage = true
		cap.SupportsFirstFrameReference = true
		cap.SupportsImageToVideo = true
		cap.WorkflowModes = appendMissingTag(cap.WorkflowModes, VideoWorkflowImageToVideo)
		cap.MediaSlots = appendMissingTag(cap.MediaSlots, VideoSlotStartImage)
		cap.Tags = appendMissingTag(cap.Tags, "image_to_video", "first_frame_reference")
	}
	if strings.Contains(m, "text-to-video") || strings.Contains(m, "t2v") {
		cap.Tags = appendMissingTag(cap.Tags, "text_to_video")
	}
	return cap
}

func VideoModelSupportsReferenceImage(model string) bool {
	return VideoModelCapabilityFor(model).SupportsReferenceImage
}

func appendMissingTag(tags []string, extra ...string) []string {
	seen := map[string]bool{}
	for _, tag := range tags {
		seen[tag] = true
	}
	for _, tag := range extra {
		if tag != "" && !seen[tag] {
			tags = append(tags, tag)
			seen[tag] = true
		}
	}
	return tags
}

// MergeModelLists appends models from extra that do not already exist in base.
func MergeModelLists(base, extra []string) []string {
	out := append([]string(nil), base...)
	seen := make(map[string]bool, len(out)+len(extra))
	for _, model := range out {
		seen[model] = true
	}
	for _, model := range extra {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		out = append(out, model)
	}
	return out
}

// ZetaTechsImageModelsFrom filters a raw gateway model list down to image-capable models.
func ZetaTechsImageModelsFrom(models []string) []string {
	out := make([]string, 0, len(models))
	for _, model := range models {
		if isLikelyZetaTechsImageModel(model) {
			out = append(out, model)
		}
	}
	return out
}

// ZetaTechsVideoModelsFrom filters a raw gateway model list down to video-capable models.
func ZetaTechsVideoModelsFrom(models []string) []string {
	out := make([]string, 0, len(models))
	for _, model := range models {
		if isLikelyZetaTechsVideoModel(model) {
			out = append(out, model)
		}
	}
	return out
}

func isLikelyZetaTechsImageModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" || isLikelyZetaTechsVideoModel(m) {
		return false
	}
	return strings.Contains(m, "gpt-image") ||
		strings.Contains(m, "image") ||
		strings.Contains(m, "imagen") ||
		strings.Contains(m, "dall-e") ||
		strings.Contains(m, "seedream") ||
		strings.Contains(m, "seededit") ||
		strings.Contains(m, "nano-banana") ||
		strings.Contains(m, "flux") ||
		strings.Contains(m, "t2i") ||
		strings.Contains(m, "i2i")
}

func isLikelyZetaTechsVideoModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}
	if strings.HasPrefix(m, "sora") ||
		strings.HasPrefix(m, "kling") ||
		strings.HasPrefix(m, "jimeng") ||
		strings.HasPrefix(m, "vidu") ||
		strings.Contains(m, "seedance") {
		return true
	}
	if strings.HasPrefix(m, "wan") || strings.HasPrefix(m, "wanx") {
		return strings.Contains(m, "t2v") ||
			strings.Contains(m, "i2v") ||
			strings.Contains(m, "s2v") ||
			strings.Contains(m, "r2v") ||
			strings.Contains(m, "kf2v") ||
			strings.Contains(m, "vace") ||
			strings.Contains(m, "animate")
	}
	return strings.Contains(m, "text-to-video") ||
		strings.Contains(m, "image-to-video") ||
		strings.Contains(m, "video")
}

// Provider는 이미지 생성 백엔드 공통 인터페이스입니다.
type Provider interface {
	// GenerateImage는 프롬프트와 참조 이미지(PNG)로 이미지를 생성합니다.
	GenerateImage(ctx context.Context, prompt string, refImages [][]byte, aspectRatio string) ([]byte, error)
	// ValidateKey는 API 키 유효성을 확인합니다.
	ValidateKey(ctx context.Context) error
}

// MaskEditProvider is implemented by image providers that accept an explicit
// transparent mask for local, non-destructive image editing.
type MaskEditProvider interface {
	GenerateImageWithMask(ctx context.Context, prompt string, refImages [][]byte, mask []byte, aspectRatio string) ([]byte, error)
}

// DefaultModelFor는 프로바이더별 기본 모델을 반환합니다.
func DefaultModelFor(provider string) string {
	switch provider {
	case ProviderOpenAI:
		return "gpt-image-2"
	case ProviderOpenRouter:
		return "google/gemini-3-pro-image-preview"
	case ProviderFal:
		return "fal-ai/nano-banana-pro"
	case ProviderBytePlus:
		return "seedream-4-0-250828"
	case ProviderZetaTechs:
		return "gpt-image-2"
	case ProviderVolcengine:
		return ""
	default:
		return DefaultModel // gemini-3-pro-image (Nano Banana Pro)
	}
}

// ProviderLabel은 UI 표시용 이름입니다.
func ProviderLabel(provider string) string {
	switch provider {
	case ProviderOpenAI:
		return "OpenAI"
	case ProviderOpenRouter:
		return "OpenRouter"
	case ProviderFal:
		return "fal.ai"
	case ProviderBytePlus:
		return "BytePlus"
	case ProviderZetaTechs:
		return "ZetaTechs"
	case ProviderVolcengine:
		return "火山方舟"
	default:
		return "Gemini"
	}
}

// New는 프로바이더 구현체를 생성합니다.
func New(provider, apiKey, model string) (Provider, error) {
	if model == "" {
		model = DefaultModelFor(provider)
	}
	switch provider {
	case ProviderGemini, "":
		return NewClient(apiKey, model), nil
	case ProviderOpenAI:
		return NewOpenAI(apiKey, model), nil
	case ProviderOpenRouter:
		return NewOpenRouter(apiKey, model), nil
	case ProviderFal:
		return NewFal(apiKey, model), nil
	case ProviderBytePlus:
		return NewBytePlus(apiKey, model), nil
	case ProviderZetaTechs:
		return NewZetaTechs(apiKey, model), nil
	case ProviderVolcengine:
		return NewVolcengine(apiKey, model), nil
	default:
		return nil, fmt.Errorf("不支持的 provider: %s", provider)
	}
}

// aspectHint는 종횡비 파라미터를 지원하지 않는 API용 프롬프트 보조 문구입니다.
func aspectHint(aspectRatio string) string {
	switch aspectRatio {
	case "", "1:1":
		return "Render on a square 1:1 canvas."
	case "16:9":
		return "Render on a wide 16:9 landscape canvas, much wider than tall."
	case "21:9":
		return "Render on an extra-wide 21:9 panorama canvas, with plenty of horizontal room for a long row of poses."
	default:
		return fmt.Sprintf("Render on a wide %s landscape canvas.", aspectRatio)
	}
}

// decodeDataOrDownload preserves the legacy helper for callers without a
// request context. Provider generation paths should use the context-aware form.
func decodeDataOrDownload(httpClient *http.Client, rawURL string) ([]byte, error) {
	return decodeDataOrDownloadContext(context.Background(), httpClient, rawURL)
}

// decodeDataOrDownloadContext decodes inline image data or downloads a remote
// image through the public-HTTPS-only media transport. The provider client's
// custom transport is deliberately not reused because it may contain a proxy
// or dialer that bypasses destination validation.
func decodeDataOrDownloadContext(ctx context.Context, httpClient *http.Client, rawURL string) ([]byte, error) {
	if strings.HasPrefix(rawURL, "data:") {
		idx := strings.Index(rawURL, "base64,")
		if idx < 0 {
			return nil, errors.New("无法解析图片数据格式")
		}
		return base64.StdEncoding.DecodeString(rawURL[idx+7:])
	}
	options := safemedia.DownloaderOptions{}
	if httpClient != nil {
		options.Timeout = httpClient.Timeout
	}
	downloader := safemedia.NewDownloader(options)
	defer downloader.CloseIdleConnections()
	result, err := downloader.Download(ctx, rawURL, safemedia.ImageDownloadLimits())
	if err != nil {
		return nil, fmt.Errorf("结果图片下载失败: %w", err)
	}
	return result.Data, nil
}
