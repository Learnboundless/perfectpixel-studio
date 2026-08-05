// Command ppzeta-video-smoke runs guarded real ZetaTechs video smoke tests.
//
// The command exists because unit tests can verify request shape, but they
// cannot prove that the gateway accepts each long-running video task route.
// It records sanitized JSONL HTTP traces, creates real tasks when explicitly
// confirmed, polls them, and optionally downloads the resulting video.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"perfectpixel/internal/config"
	"perfectpixel/internal/gen"
)

type options struct {
	key           string
	baseURL       string
	models        string
	cases         string
	outDir        string
	modelFilter   string
	prompt        string
	startURL      string
	endURL        string
	driverURL     string
	audioURL      string
	seconds       int
	fps           int
	pollTimeout   time.Duration
	pollInterval  time.Duration
	download      bool
	listOnly      bool
	dryRun        bool
	confirmCost   bool
	allowFallback bool
	requireListed bool
	maxCases      int
}

const (
	defaultSmokeStartURL        = "https://wanx.alicdn.com/material/20250318/first_frame.png"
	defaultSmokeEndURL          = "https://wanx.alicdn.com/material/20250318/last_frame.png"
	defaultSmokeMoveImageURL    = "https://help-static-aliyun-doc.aliyuncs.com/file-manage-files/zh-CN/20250919/adsyrp/move_input_image.jpeg"
	defaultSmokeMoveVideoURL    = "https://help-static-aliyun-doc.aliyuncs.com/file-manage-files/zh-CN/20250919/kaakcn/move_input_video.mp4"
	defaultSmokeMixImageURL     = "https://help-static-aliyun-doc.aliyuncs.com/file-manage-files/zh-CN/20250919/bhkfor/mix_input_image.jpeg"
	defaultSmokeMixVideoURL     = "https://help-static-aliyun-doc.aliyuncs.com/file-manage-files/zh-CN/20250919/wqefue/mix_input_video.mp4"
	defaultSmokeGenericVideoURL = defaultSmokeMoveVideoURL
	defaultSmokeS2VImageURL     = "https://img.alicdn.com/imgextra/i3/O1CN011FObkp1T7Ttowoq4F_!!6000000002335-0-tps-1440-1797.jpg"
	defaultSmokeS2VAudioURL     = "https://help-static-aliyun-doc.aliyuncs.com/file-manage-files/zh-CN/20250825/iaqpio/input_audio.MP3"
)

type smokeCase struct {
	Name            string         `json:"name"`
	Model           string         `json:"model"`
	Endpoint        string         `json:"endpoint"`
	ProviderAdapter string         `json:"providerAdapter,omitempty"`
	Workflow        string         `json:"workflow"`
	ReferenceMode   string         `json:"referenceMode"`
	Description     string         `json:"description"`
	MediaSlots      map[string]int `json:"mediaSlots,omitempty"`
	request         gen.VideoRequest
}

type caseResult struct {
	Case          smokeCase `json:"case"`
	Skipped       bool      `json:"skipped,omitempty"`
	SkipReason    string    `json:"skipReason,omitempty"`
	OK            bool      `json:"ok"`
	CreatedTaskID string    `json:"createdTaskId,omitempty"`
	FinalStatus   string    `json:"finalStatus,omitempty"`
	Progress      int       `json:"progress,omitempty"`
	Message       string    `json:"message,omitempty"`
	URLSeen       bool      `json:"urlSeen,omitempty"`
	Downloaded    bool      `json:"downloaded,omitempty"`
	DownloadBytes int       `json:"downloadBytes,omitempty"`
	VideoPath     string    `json:"videoPath,omitempty"`
	Error         string    `json:"error,omitempty"`
	ElapsedMs     int64     `json:"elapsedMs"`
}

type report struct {
	GeneratedAt        string       `json:"generatedAt"`
	BaseURL            string       `json:"baseUrl"`
	DynamicModelCount  int          `json:"dynamicModelCount"`
	DynamicVideoModels []string     `json:"dynamicVideoModels,omitempty"`
	DynamicImageModels []string     `json:"dynamicImageModels,omitempty"`
	DynamicListError   string       `json:"dynamicListError,omitempty"`
	Cases              []caseResult `json:"cases"`
	TracePath          string       `json:"tracePath"`
}

func main() {
	var opt options
	flag.StringVar(&opt.key, "key", "", "ZetaTechs API key or NewAPI connection JSON. Defaults to config/.env.")
	flag.StringVar(&opt.baseURL, "base-url", "", "ZetaTechs gateway base URL. Defaults to config/.env or https://ent.zetatechs.com.")
	flag.StringVar(&opt.models, "models", "", "Comma-separated explicit video models. Use @dynamic-video to smoke every video model returned by /v1/models. Empty means representative models from /v1/models.")
	flag.StringVar(&opt.cases, "case", "representative", "Comma-separated cases: representative, all, auto, openai_t2v, openai_identity, video_t2v, video_i2v, video_start_end, model_t2v, model_i2v.")
	flag.StringVar(&opt.outDir, "out", filepath.Join(".codex", "zeta-video-smoke-"+time.Now().Format("20060102-150405")), "Output directory for report, trace, and downloaded videos.")
	flag.StringVar(&opt.modelFilter, "model-filter", "", "Optional case-insensitive substring filter used with @dynamic-video.")
	flag.StringVar(&opt.prompt, "prompt", "pixel art game character performs one clean idle bounce, fixed camera, centered full body, plain solid green background, no camera movement", "Smoke-test prompt.")
	flag.StringVar(&opt.startURL, "start-url", defaultSmokeStartURL, "Public image URL for first-frame/first-last URL-only smoke cases.")
	flag.StringVar(&opt.endURL, "end-url", defaultSmokeEndURL, "Public image URL for last-frame URL-only smoke cases.")
	flag.StringVar(&opt.driverURL, "driver-video-url", "", "Public MP4 URL for motion-transfer smoke cases. Empty uses model-specific official examples.")
	flag.StringVar(&opt.audioURL, "reference-audio-url", defaultSmokeS2VAudioURL, "Public audio URL for speech/audio-reference smoke cases.")
	flag.IntVar(&opt.seconds, "seconds", 4, "Requested video duration in seconds.")
	flag.IntVar(&opt.fps, "fps", 12, "Requested FPS for providers that accept it.")
	flag.DurationVar(&opt.pollTimeout, "poll-timeout", 15*time.Minute, "Per-case polling timeout.")
	flag.DurationVar(&opt.pollInterval, "poll-interval", 4*time.Second, "Polling interval.")
	flag.BoolVar(&opt.download, "download", true, "Download the completed video.")
	flag.BoolVar(&opt.listOnly, "list-models", false, "Only list/filter models; does not create paid video tasks.")
	flag.BoolVar(&opt.dryRun, "dry-run", false, "Build cases and sanitized request plans without calling create/poll/download.")
	flag.BoolVar(&opt.confirmCost, "confirm-cost", false, "Required for real video task creation unless PP_ZETA_REAL_VIDEO_SMOKE=1 is set.")
	flag.BoolVar(&opt.allowFallback, "allow-static-fallback", false, "Allow built-in fallback models when /v1/models does not list a representative model.")
	flag.BoolVar(&opt.requireListed, "require-listed", true, "Skip explicit/static models not returned by /v1/models when a dynamic list is available.")
	flag.IntVar(&opt.maxCases, "max-cases", 0, "Optional cap for created cases after model expansion; 0 means no cap.")
	flag.Parse()

	if err := run(opt); err != nil {
		fmt.Fprintf(os.Stderr, "ppzeta-video-smoke 失败: %v\n", err)
		os.Exit(1)
	}
}

func run(opt options) error {
	if opt.seconds <= 0 {
		opt.seconds = 4
	}
	if opt.fps <= 0 {
		opt.fps = 12
	}
	if opt.pollInterval <= 0 {
		opt.pollInterval = 4 * time.Second
	}
	if opt.pollTimeout <= 0 {
		opt.pollTimeout = 15 * time.Minute
	}
	if err := os.MkdirAll(opt.outDir, 0o755); err != nil {
		return err
	}
	videoDir := filepath.Join(opt.outDir, "videos")
	if opt.download {
		if err := os.MkdirAll(videoDir, 0o755); err != nil {
			return err
		}
	}
	key, baseURL, err := resolveZetaConnection(opt)
	if err != nil {
		return err
	}
	tracePath := filepath.Join(opt.outDir, "events.jsonl")
	trace, err := newTraceWriter(tracePath)
	if err != nil {
		return err
	}
	defer trace.Close()

	modelClient := gen.NewZetaTechsWithBaseURL(key, "", baseURL)
	modelClient.HTTP = &http.Client{Timeout: 90 * time.Second, Transport: &traceTransport{label: "list_models", out: trace, next: http.DefaultTransport}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	models, listErr := modelClient.ListModels(ctx)
	if listErr != nil {
		fmt.Printf("模型列表刷新失败，将只使用内置兜底/显式模型：%v\n", listErr)
	} else {
		fmt.Printf("模型列表刷新成功：%d 个模型\n", len(models))
	}
	dynamicVideo := gen.ZetaTechsVideoModelsFrom(models)
	dynamicImage := gen.ZetaTechsImageModelsFrom(models)
	sort.Strings(dynamicVideo)
	sort.Strings(dynamicImage)
	if opt.listOnly {
		printModelSummary(models)
		return writeReport(opt.outDir, report{
			GeneratedAt:        time.Now().Format(time.RFC3339),
			BaseURL:            baseURL,
			DynamicModelCount:  len(models),
			DynamicVideoModels: dynamicVideo,
			DynamicImageModels: dynamicImage,
			DynamicListError:   errString(listErr),
			TracePath:          tracePath,
		})
	}

	startPNG := fixturePNG(false)
	endPNG := fixturePNG(true)
	cases, skipped := buildCases(opt, models, startPNG, endPNG)
	if len(cases) == 0 && len(skipped) == 0 {
		return fmt.Errorf("没有可运行的视频 smoke case；可用 -models 指定模型，或加 -allow-static-fallback 使用内置兜底模型")
	}
	printCasePlan(cases, opt.dryRun)
	if opt.dryRun {
		results := append([]caseResult{}, skipped...)
		results = append(results, dryRunResults(cases)...)
		return writeReport(opt.outDir, report{
			GeneratedAt:        time.Now().Format(time.RFC3339),
			BaseURL:            baseURL,
			DynamicModelCount:  len(models),
			DynamicVideoModels: dynamicVideo,
			DynamicImageModels: dynamicImage,
			DynamicListError:   errString(listErr),
			Cases:              results,
			TracePath:          tracePath,
		})
	}
	if !opt.confirmCost && os.Getenv("PP_ZETA_REAL_VIDEO_SMOKE") != "1" {
		return fmt.Errorf("真实视频 smoke 会创建付费长任务；请加 -confirm-cost，或设置 PP_ZETA_REAL_VIDEO_SMOKE=1")
	}

	rep := report{
		GeneratedAt:        time.Now().Format(time.RFC3339),
		BaseURL:            baseURL,
		DynamicModelCount:  len(models),
		DynamicVideoModels: dynamicVideo,
		DynamicImageModels: dynamicImage,
		DynamicListError:   errString(listErr),
		Cases:              append([]caseResult{}, skipped...),
		TracePath:          tracePath,
	}
	for i, sc := range cases {
		fmt.Printf("\n[%d/%d] %s · %s · %s\n", i+1, len(cases), sc.Name, sc.Model, sc.Endpoint)
		result := runSmokeCase(opt, key, baseURL, sc, videoDir, trace)
		rep.Cases = append(rep.Cases, result)
		if result.OK {
			fmt.Printf("  OK status=%s task=%s downloaded=%t bytes=%d elapsed=%s\n",
				result.FinalStatus, result.CreatedTaskID, result.Downloaded, result.DownloadBytes, time.Duration(result.ElapsedMs)*time.Millisecond)
		} else if result.Skipped {
			fmt.Printf("  SKIP %s\n", result.SkipReason)
		} else {
			fmt.Printf("  FAIL %s\n", result.Error)
		}
		_ = writeReport(opt.outDir, rep)
	}
	if err := writeReport(opt.outDir, rep); err != nil {
		return err
	}
	fmt.Printf("\n报告: %s\n事件: %s\n", filepath.Join(opt.outDir, "report.json"), tracePath)
	return nil
}

func resolveZetaConnection(opt options) (key, baseURL string, err error) {
	settings := config.Load()
	cfg := settings.ZetaTechs
	key = strings.TrimSpace(opt.key)
	baseURL = strings.TrimSpace(opt.baseURL)
	if parsedKey, parsedURL, ok, parseErr := parseConnectionJSON(key); parseErr != nil {
		return "", "", parseErr
	} else if ok {
		key = parsedKey
		if baseURL == "" {
			baseURL = parsedURL
		}
	}
	if key == "" {
		key = cfg.APIKey
	}
	if baseURL == "" {
		baseURL = cfg.BaseURL
	}
	if baseURL == "" {
		baseURL = "https://ent.zetatechs.com"
	}
	if key == "" {
		return "", "", fmt.Errorf("ZetaTechs API key 未配置；请在设置页保存，或传 -key / ZETATECHS_API_KEY")
	}
	return key, strings.TrimRight(baseURL, "/"), nil
}

func parseConnectionJSON(input string) (key, baseURL string, ok bool, err error) {
	input = strings.TrimSpace(input)
	if input == "" || !strings.HasPrefix(input, "{") {
		return "", "", false, nil
	}
	var raw struct {
		Key string `json:"key"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		return "", "", false, fmt.Errorf("ZetaTechs 连接 JSON 格式不正确: %w", err)
	}
	if strings.TrimSpace(raw.Key) == "" {
		return "", "", false, fmt.Errorf("ZetaTechs 连接 JSON 中缺少 key")
	}
	return strings.TrimSpace(raw.Key), strings.TrimSpace(raw.URL), true, nil
}

func buildCases(opt options, dynamicModels []string, startPNG, endPNG []byte) ([]smokeCase, []caseResult) {
	if strings.TrimSpace(opt.models) != "" {
		models, skipped := requestedModels(opt, dynamicModels)
		cases := explicitModelCases(models, selectedCases(opt.cases), opt, startPNG, endPNG)
		return limitCases(cases, opt.maxCases), skipped
	}
	want := selectedCases(opt.cases)
	if hasCase(want, "representative") || hasCase(want, "all") || hasCase(want, "auto") {
		want = map[string]bool{
			"openai_t2v":      true,
			"openai_identity": true,
			"video_t2v":       true,
			"video_i2v":       true,
			"video_start_end": true,
			"model_t2v":       true,
			"model_i2v":       true,
		}
	}
	dyn := modelSet(dynamicModels)
	pick := func(candidates ...string) string {
		for _, model := range candidates {
			if dyn[strings.ToLower(model)] {
				return model
			}
		}
		if opt.allowFallback && len(candidates) > 0 {
			return candidates[0]
		}
		return ""
	}
	var out []smokeCase
	add := func(sc smokeCase) {
		if sc.Model != "" {
			out = append(out, sc)
		}
	}
	if want["openai_t2v"] {
		add(makeCase("openai_t2v", pick("sora-2", "sora-2-pro"), gen.VideoWorkflowTextToVideo, gen.VideoReferenceModeNone, opt, nil, nil))
	}
	if want["openai_identity"] {
		add(makeCase("openai_identity", pick("sora-2", "sora-2-pro"), gen.VideoWorkflowReferenceToVideo, gen.VideoReferenceModeIdentityReference, opt, startPNG, nil))
	}
	if want["video_t2v"] {
		add(makeCase("video_t2v", pick(
			"wan2.6-t2v",
			"wan2.5-t2v-preview",
			"wan2.2-t2v-plus",
			"wanx2.1-t2v-turbo",
			"doubao-seedance-1-0-lite-t2v-250428",
			"doubao-seedance-2-0-fast-260128",
			"doubao-seedance-2-0-260128",
		), gen.VideoWorkflowTextToVideo, gen.VideoReferenceModeNone, opt, nil, nil))
	}
	if want["video_i2v"] {
		add(makeCase("video_i2v", pick(
			"wan2.6-i2v",
			"doubao-seedance-1-0-lite-i2v-250428",
			"wan2.5-i2v-preview",
			"wan2.2-i2v-flash",
			"kling-v2-master",
			"jimeng_vgfm_i2v_l20",
			"viduq1",
		), gen.VideoWorkflowImageToVideo, gen.VideoReferenceModeFirstFrame, opt, startPNG, nil))
	}
	if want["video_start_end"] {
		add(makeCase("video_start_end", pick("wan2.2-kf2v-flash", "kling-v2-master"), gen.VideoWorkflowFirstLastToVideo, gen.VideoReferenceModeStartEndFrame, opt, startPNG, endPNG))
	}
	if want["model_t2v"] {
		add(makeCase("model_t2v", pick("spark_dance_v2_0_fast", "spark_dance_v2_0", "vtrix:spark_dance_v2_0_fast"), gen.VideoWorkflowTextToVideo, gen.VideoReferenceModeNone, opt, nil, nil))
	}
	if want["model_i2v"] {
		add(makeCase("model_i2v", pick("spark_dance_v2_0_fast", "spark_dance_v2_0", "vtrix:spark_dance_v2_0_fast"), gen.VideoWorkflowImageToVideo, gen.VideoReferenceModeFirstFrame, opt, startPNG, nil))
	}
	return limitCases(dedupeCases(out), opt.maxCases), nil
}

func requestedModels(opt options, dynamicModels []string) ([]string, []caseResult) {
	raw := splitCSV(opt.models)
	if len(raw) == 0 {
		return nil, nil
	}
	dynamicVideo := gen.ZetaTechsVideoModelsFrom(dynamicModels)
	sort.Strings(dynamicVideo)
	dynSet := modelSet(dynamicModels)
	var models []string
	var skipped []caseResult
	addModel := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" {
			return
		}
		models = append(models, model)
	}
	for _, item := range raw {
		switch strings.ToLower(strings.TrimSpace(item)) {
		case "@dynamic-video", "dynamic-video", "available-video", "@available-video":
			for _, model := range dynamicVideo {
				if opt.modelFilter != "" && !strings.Contains(strings.ToLower(model), strings.ToLower(opt.modelFilter)) {
					continue
				}
				addModel(model)
			}
		default:
			if opt.requireListed && len(dynamicModels) > 0 && !dynSet[strings.ToLower(item)] {
				skipped = append(skipped, caseResult{
					Case:       smokeCase{Name: "unavailable_" + safeName(item), Model: item},
					Skipped:    true,
					SkipReason: "上层 /v1/models 未返回该视频模型，按 require-listed 跳过。",
				})
				continue
			}
			addModel(item)
		}
	}
	return uniqueRequestedModels(models), skipped
}

func uniqueRequestedModels(models []string) []string {
	out := make([]string, 0, len(models))
	seen := map[string]bool{}
	for _, model := range models {
		model = strings.TrimSpace(model)
		key := strings.ToLower(model)
		if model == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, model)
	}
	return out
}

func explicitModelCases(models []string, want map[string]bool, opt options, startPNG, endPNG []byte) []smokeCase {
	if len(want) == 0 || want["representative"] || want["auto"] {
		want = map[string]bool{"auto": true}
	}
	var out []smokeCase
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		cap := gen.VideoModelCapabilityFor(model)
		if want["all"] {
			if cap.SupportsTextPrompt {
				out = append(out, makeCase("auto_t2v_"+safeName(model), model, gen.VideoWorkflowTextToVideo, gen.VideoReferenceModeNone, opt, nil, nil))
			}
			if cap.SupportsMotionTransfer {
				out = append(out, makeCase("auto_motion_"+safeName(model), model, gen.VideoWorkflowMotionTransfer, gen.VideoReferenceModeFirstFrame, opt, startPNG, nil))
			}
			if cap.SupportsReferenceVideo {
				out = append(out, makeCase("auto_reference_"+safeName(model), model, gen.VideoWorkflowReferenceToVideo, gen.VideoReferenceModeFirstFrame, opt, startPNG, nil))
			}
			if cap.SupportsImageToVideo {
				out = append(out, makeCase("auto_i2v_"+safeName(model), model, gen.VideoWorkflowImageToVideo, gen.VideoReferenceModeFirstFrame, opt, startPNG, nil))
			}
			if cap.SupportsStartEndFrames {
				out = append(out, makeCase("auto_start_end_"+safeName(model), model, gen.VideoWorkflowFirstLastToVideo, gen.VideoReferenceModeStartEndFrame, opt, startPNG, endPNG))
			}
			continue
		}
		if want["openai_t2v"] || want["video_t2v"] || want["model_t2v"] {
			out = append(out, makeCase("explicit_t2v_"+safeName(model), model, gen.VideoWorkflowTextToVideo, gen.VideoReferenceModeNone, opt, nil, nil))
		}
		if want["openai_identity"] {
			out = append(out, makeCase("explicit_identity_"+safeName(model), model, gen.VideoWorkflowReferenceToVideo, gen.VideoReferenceModeIdentityReference, opt, startPNG, nil))
		}
		if want["video_i2v"] || want["model_i2v"] {
			out = append(out, makeCase("explicit_i2v_"+safeName(model), model, gen.VideoWorkflowImageToVideo, gen.VideoReferenceModeFirstFrame, opt, startPNG, nil))
		}
		if want["video_start_end"] {
			out = append(out, makeCase("explicit_start_end_"+safeName(model), model, gen.VideoWorkflowFirstLastToVideo, gen.VideoReferenceModeStartEndFrame, opt, startPNG, endPNG))
		}
		if want["auto"] || len(out) == 0 {
			switch {
			case cap.SupportsMotionTransfer:
				out = append(out, makeCase("auto_motion_"+safeName(model), model, gen.VideoWorkflowMotionTransfer, gen.VideoReferenceModeFirstFrame, opt, startPNG, nil))
			case cap.SupportsStartEndFrames:
				out = append(out, makeCase("auto_start_end_"+safeName(model), model, gen.VideoWorkflowFirstLastToVideo, gen.VideoReferenceModeStartEndFrame, opt, startPNG, endPNG))
			case cap.SupportsReferenceVideo:
				out = append(out, makeCase("auto_reference_"+safeName(model), model, gen.VideoWorkflowReferenceToVideo, gen.VideoReferenceModeFirstFrame, opt, startPNG, nil))
			case cap.SupportsImageToVideo:
				out = append(out, makeCase("auto_i2v_"+safeName(model), model, gen.VideoWorkflowImageToVideo, gen.VideoReferenceModeFirstFrame, opt, startPNG, nil))
			default:
				out = append(out, makeCase("auto_t2v_"+safeName(model), model, gen.VideoWorkflowTextToVideo, gen.VideoReferenceModeNone, opt, nil, nil))
			}
		}
	}
	return dedupeCases(out)
}

func makeCase(name, model, workflow, refMode string, opt options, startPNG, endPNG []byte) smokeCase {
	cap := gen.VideoModelCapabilityFor(model)
	req := gen.VideoRequest{
		Model:           model,
		Prompt:          opt.prompt,
		Workflow:        workflow,
		ReferenceMode:   refMode,
		Seconds:         opt.seconds,
		FPS:             opt.fps,
		Size:            "1024x1024",
		AspectRatio:     "1:1",
		Resolution:      "720p",
		ProviderAdapter: cap.ProviderAdapter,
	}
	mediaSlots := map[string]int{}
	if len(startPNG) > 0 {
		req.MediaSlots = ensureSlots(req.MediaSlots)
		start := gen.VideoMediaInput{Data: startPNG, MIME: "image/png", Label: "smoke_start"}
		if shouldUseStartURLForSmoke(model, workflow) {
			if u := smokeStartURLForModel(model, opt); u != "" {
				start = gen.VideoMediaInput{URL: u, MIME: smokeImageMIME(u), Label: "smoke_start_url"}
			}
		}
		req.MediaSlots[gen.VideoSlotStartImage] = []gen.VideoMediaInput{start}
		mediaSlots[gen.VideoSlotStartImage] = 1
	}
	if len(endPNG) > 0 {
		req.MediaSlots = ensureSlots(req.MediaSlots)
		end := gen.VideoMediaInput{Data: endPNG, MIME: "image/png", Label: "smoke_end"}
		if workflow == gen.VideoWorkflowFirstLastToVideo && strings.TrimSpace(opt.endURL) != "" {
			end = gen.VideoMediaInput{URL: strings.TrimSpace(opt.endURL), MIME: "image/png", Label: "smoke_end_url"}
		}
		req.MediaSlots[gen.VideoSlotEndImage] = []gen.VideoMediaInput{end}
		mediaSlots[gen.VideoSlotEndImage] = 1
	}
	if workflow == gen.VideoWorkflowMotionTransfer {
		req.MediaSlots = ensureSlots(req.MediaSlots)
		if u := smokeDriverURLForModel(model, opt); u != "" {
			req.MediaSlots[gen.VideoSlotDriverVideo] = []gen.VideoMediaInput{{
				URL:   u,
				MIME:  "video/mp4",
				Label: "smoke_driver_video_url",
			}}
			mediaSlots[gen.VideoSlotDriverVideo] = 1
		}
	}
	if workflow == gen.VideoWorkflowReferenceToVideo {
		req.MediaSlots = ensureSlots(req.MediaSlots)
		if u := smokeDriverURLForModel(model, opt); u != "" {
			req.MediaSlots[gen.VideoSlotReferenceVideos] = []gen.VideoMediaInput{{
				URL:   u,
				MIME:  "video/mp4",
				Label: "smoke_reference_video_url",
			}}
			mediaSlots[gen.VideoSlotReferenceVideos] = 1
		}
	}
	if strings.Contains(strings.ToLower(model), "s2v") {
		req.MediaSlots = ensureSlots(req.MediaSlots)
		if u := strings.TrimSpace(opt.audioURL); u != "" {
			req.MediaSlots[gen.VideoSlotReferenceAudios] = []gen.VideoMediaInput{{
				URL:   u,
				MIME:  "audio/mpeg",
				Label: "smoke_reference_audio_url",
			}}
			mediaSlots[gen.VideoSlotReferenceAudios] = 1
		}
	}
	if refMode == gen.VideoReferenceModeIdentityReference && len(startPNG) > 0 {
		req.MediaSlots = map[string][]gen.VideoMediaInput{
			gen.VideoSlotReferenceImages: {{Data: startPNG, MIME: "image/png", Label: "smoke_identity_reference"}},
		}
		mediaSlots = map[string]int{gen.VideoSlotReferenceImages: 1}
	}
	return smokeCase{
		Name:            name,
		Model:           model,
		Endpoint:        cap.Endpoint,
		ProviderAdapter: cap.ProviderAdapter,
		Workflow:        workflow,
		ReferenceMode:   refMode,
		Description:     cap.Family + " " + strings.Join(cap.Tags, ","),
		MediaSlots:      mediaSlots,
		request:         req,
	}
}

func shouldUseStartURLForSmoke(model, workflow string) bool {
	return workflow == gen.VideoWorkflowFirstLastToVideo ||
		workflow == gen.VideoWorkflowMotionTransfer ||
		strings.Contains(strings.ToLower(model), "s2v")
}

func smokeStartURLForModel(model string, opt options) string {
	m := strings.ToLower(strings.TrimSpace(model))
	if strings.Contains(m, "animate-move") {
		return defaultSmokeMoveImageURL
	}
	if strings.Contains(m, "animate-mix") {
		return defaultSmokeMixImageURL
	}
	if strings.Contains(m, "s2v") {
		return defaultSmokeS2VImageURL
	}
	return strings.TrimSpace(opt.startURL)
}

func smokeDriverURLForModel(model string, opt options) string {
	if u := strings.TrimSpace(opt.driverURL); u != "" {
		return u
	}
	m := strings.ToLower(strings.TrimSpace(model))
	if strings.Contains(m, "animate-move") {
		return defaultSmokeMoveVideoURL
	}
	if strings.Contains(m, "animate-mix") {
		return defaultSmokeMixVideoURL
	}
	return defaultSmokeGenericVideoURL
}

func smokeImageMIME(url string) string {
	lower := strings.ToLower(strings.TrimSpace(url))
	if strings.Contains(lower, ".jpg") || strings.Contains(lower, ".jpeg") {
		return "image/jpeg"
	}
	if strings.Contains(lower, ".webp") {
		return "image/webp"
	}
	return "image/png"
}

func ensureSlots(slots map[string][]gen.VideoMediaInput) map[string][]gen.VideoMediaInput {
	if slots == nil {
		return map[string][]gen.VideoMediaInput{}
	}
	return slots
}

func runSmokeCase(opt options, key, baseURL string, sc smokeCase, videoDir string, trace *traceWriter) caseResult {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), opt.pollTimeout+2*time.Minute)
	defer cancel()
	client := gen.NewZetaTechsWithBaseURL(key, "", baseURL)
	client.HTTP = &http.Client{Timeout: 300 * time.Second, Transport: &traceTransport{label: sc.Name, out: trace, next: http.DefaultTransport}}
	result := caseResult{Case: sc}

	task, err := client.GenerateVideo(ctx, sc.request)
	if err != nil {
		result.Error = err.Error()
		result.ElapsedMs = time.Since(start).Milliseconds()
		return result
	}
	result.CreatedTaskID = task.ID
	result.FinalStatus = task.Status
	result.Progress = task.Progress
	result.URLSeen = task.URL != ""
	deadline := time.Now().Add(opt.pollTimeout)
	pollCount := 0
	for !task.Done() {
		if task.Failed() {
			result.FinalStatus = task.Status
			result.Progress = task.Progress
			result.Message = task.Message
			if task.Message != "" {
				result.Error = task.Message
			} else {
				result.Error = "provider task failed"
			}
			result.ElapsedMs = time.Since(start).Milliseconds()
			return result
		}
		if task.ID == "" || task.URL != "" {
			break
		}
		if time.Now().After(deadline) {
			result.FinalStatus = task.Status
			result.Progress = task.Progress
			result.Error = "poll timeout after " + opt.pollTimeout.String()
			result.ElapsedMs = time.Since(start).Milliseconds()
			return result
		}
		pollCount++
		percent := progressPercent(task.Progress, time.Since(start), opt.pollTimeout)
		fmt.Printf("  等待视频生成 poll=%d status=%s providerProgress=%d estimated=%d%%\n", pollCount, task.Status, task.Progress, percent)
		select {
		case <-ctx.Done():
			result.Error = ctx.Err().Error()
			result.ElapsedMs = time.Since(start).Milliseconds()
			return result
		case <-time.After(opt.pollInterval):
		}
		next, err := client.PollVideo(ctx, task.ID)
		if err != nil {
			if isTransientPollError(err) && time.Now().Before(deadline) {
				result.Message = "轮询暂时失败，继续重试: " + err.Error()
				continue
			}
			result.Error = err.Error()
			result.ElapsedMs = time.Since(start).Milliseconds()
			return result
		}
		if next.ID == "" {
			next.ID = task.ID
		}
		task = next
		result.FinalStatus = task.Status
		result.Progress = task.Progress
		result.URLSeen = result.URLSeen || task.URL != ""
		result.Message = task.Message
	}
	target := task.URL
	if target == "" {
		target = task.ID
	}
	if opt.download && target != "" {
		videoBytes, err := client.DownloadVideo(ctx, target)
		if err != nil {
			result.Error = err.Error()
			result.ElapsedMs = time.Since(start).Milliseconds()
			return result
		}
		result.Downloaded = true
		result.DownloadBytes = len(videoBytes)
		result.VideoPath = filepath.Join(videoDir, safeName(sc.Name)+"-"+safeName(sc.Model)+".mp4")
		if err := os.WriteFile(result.VideoPath, videoBytes, 0o644); err != nil {
			result.Error = err.Error()
			result.ElapsedMs = time.Since(start).Milliseconds()
			return result
		}
	}
	result.FinalStatus = task.Status
	result.Progress = maxInt(result.Progress, task.Progress)
	result.URLSeen = result.URLSeen || task.URL != ""
	result.OK = task.Done() || result.Downloaded || task.URL != ""
	result.ElapsedMs = time.Since(start).Milliseconds()
	return result
}

func isTransientPollError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unexpected eof") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "temporarily unavailable")
}

func progressPercent(providerProgress int, elapsed, timeout time.Duration) int {
	if providerProgress > 0 {
		return 10 + minInt(80, providerProgress*80/100)
	}
	if timeout <= 0 {
		return 10
	}
	percent := 10 + int(elapsed.Seconds()*70/timeout.Seconds())
	if percent < 10 {
		return 10
	}
	if percent > 88 {
		return 88
	}
	return percent
}

func selectedCases(raw string) map[string]bool {
	out := map[string]bool{}
	for _, item := range splitCSV(raw) {
		out[strings.ToLower(item)] = true
	}
	return out
}

func hasCase(cases map[string]bool, name string) bool {
	return cases[strings.ToLower(name)]
}

func dedupeCases(cases []smokeCase) []smokeCase {
	seen := map[string]bool{}
	out := make([]smokeCase, 0, len(cases))
	for _, sc := range cases {
		key := sc.Name + "\x00" + sc.Model + "\x00" + sc.Workflow + "\x00" + sc.ReferenceMode
		if strings.TrimSpace(sc.Model) == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, sc)
	}
	return out
}

func limitCases(cases []smokeCase, maxCases int) []smokeCase {
	if maxCases <= 0 || len(cases) <= maxCases {
		return cases
	}
	return cases[:maxCases]
}

func splitCSV(raw string) []string {
	var out []string
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func modelSet(models []string) map[string]bool {
	out := map[string]bool{}
	for _, model := range models {
		out[strings.ToLower(strings.TrimSpace(model))] = true
	}
	return out
}

func printModelSummary(models []string) {
	video := gen.ZetaTechsVideoModelsFrom(models)
	imageModels := gen.ZetaTechsImageModelsFrom(models)
	sort.Strings(video)
	sort.Strings(imageModels)
	fmt.Printf("视频模型 %d 个:\n", len(video))
	for _, model := range video {
		cap := gen.VideoModelCapabilityFor(model)
		fmt.Printf("  - %s [%s] endpoint=%s tags=%s\n", model, cap.Family, cap.Endpoint, strings.Join(cap.Tags, ","))
	}
	fmt.Printf("图片模型 %d 个\n", len(imageModels))
}

func printCasePlan(cases []smokeCase, dryRun bool) {
	if dryRun {
		fmt.Println("Dry run case plan:")
	} else {
		fmt.Println("Real video smoke case plan:")
	}
	for i, sc := range cases {
		fmt.Printf("  %d. %s model=%s endpoint=%s workflow=%s ref=%s slots=%v\n",
			i+1, sc.Name, sc.Model, sc.Endpoint, sc.Workflow, sc.ReferenceMode, sc.MediaSlots)
	}
}

func dryRunResults(cases []smokeCase) []caseResult {
	out := make([]caseResult, 0, len(cases))
	for _, sc := range cases {
		out = append(out, caseResult{Case: sc, Skipped: true, SkipReason: "dry-run"})
	}
	return out
}

func writeReport(outDir string, rep report) error {
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "report.json"), data, 0o644)
}

func fixturePNG(end bool) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.NRGBA{R: 0, G: 255, B: 0, A: 255}}, image.Point{}, draw.Src)
	body := color.NRGBA{R: 40, G: 70, B: 210, A: 255}
	if end {
		body = color.NRGBA{R: 210, G: 70, B: 40, A: 255}
	}
	drawRect(img, 222, 170, 290, 320, body)
	drawRect(img, 238, 105, 274, 170, color.NRGBA{R: 245, G: 210, B: 170, A: 255})
	drawRect(img, 230, 315, 250, 405, color.NRGBA{R: 30, G: 30, B: 60, A: 255})
	drawRect(img, 264, 315, 284, 405, color.NRGBA{R: 30, G: 30, B: 60, A: 255})
	if end {
		drawRect(img, 292, 180, 380, 205, body)
	} else {
		drawRect(img, 132, 185, 220, 210, body)
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func drawRect(img *image.NRGBA, x0, y0, x1, y1 int, c color.NRGBA) {
	draw.Draw(img, image.Rect(x0, y0, x1, y1), &image.Uniform{C: c}, image.Point{}, draw.Src)
}

func safeName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "unnamed"
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "unnamed"
	}
	return out
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type traceWriter struct {
	mu   sync.Mutex
	file *os.File
	enc  *json.Encoder
}

func newTraceWriter(path string) (*traceWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &traceWriter{file: f, enc: json.NewEncoder(f)}, nil
}

func (w *traceWriter) Close() error {
	if w == nil || w.file == nil {
		return nil
	}
	return w.file.Close()
}

func (w *traceWriter) Write(v any) {
	if w == nil || w.enc == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.enc.Encode(v)
}

type traceTransport struct {
	label string
	out   *traceWriter
	next  http.RoundTripper
}

func (t *traceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	next := t.next
	if next == nil {
		next = http.DefaultTransport
	}
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	start := time.Now()
	entry := map[string]any{
		"time":   start.Format(time.RFC3339),
		"case":   t.label,
		"method": req.Method,
		"url":    sanitizeURL(req.URL.String()),
		"path":   req.URL.Path,
		"request": map[string]any{
			"contentType": req.Header.Get("Content-Type"),
			"accept":      req.Header.Get("Accept"),
			"body":        summarizeBody(req.Header.Get("Content-Type"), body),
		},
	}
	resp, err := next.RoundTrip(req)
	if err != nil {
		entry["error"] = err.Error()
		entry["elapsedMs"] = time.Since(start).Milliseconds()
		t.out.Write(entry)
		return nil, err
	}
	respBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(respBody))
	entry["elapsedMs"] = time.Since(start).Milliseconds()
	entry["response"] = map[string]any{
		"status":      resp.StatusCode,
		"contentType": resp.Header.Get("Content-Type"),
		"body":        summarizeResponse(resp.Header.Get("Content-Type"), respBody),
	}
	t.out.Write(entry)
	return resp, nil
}

func summarizeBody(contentType string, body []byte) any {
	if len(body) == 0 {
		return nil
	}
	mediaType, params, _ := mime.ParseMediaType(contentType)
	switch mediaType {
	case "application/json":
		var root any
		if err := json.Unmarshal(body, &root); err == nil {
			return summarizeJSONValue(root)
		}
	case "multipart/form-data":
		boundary := params["boundary"]
		if boundary == "" {
			return map[string]any{"bytes": len(body), "multipart": "missing boundary"}
		}
		reader := multipart.NewReader(bytes.NewReader(body), boundary)
		var parts []map[string]any
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				parts = append(parts, map[string]any{"error": err.Error()})
				break
			}
			data, _ := io.ReadAll(part)
			item := map[string]any{
				"name":        part.FormName(),
				"filename":    part.FileName(),
				"contentType": part.Header.Get("Content-Type"),
				"bytes":       len(data),
			}
			if part.FileName() == "" {
				item["value"] = summarizeString(string(data), part.FormName())
			}
			parts = append(parts, item)
		}
		return map[string]any{"parts": parts}
	}
	return map[string]any{"bytes": len(body), "preview": truncate(string(body), 240)}
}

func summarizeResponse(contentType string, body []byte) any {
	if len(body) == 0 {
		return nil
	}
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if strings.HasPrefix(mediaType, "video/") || strings.HasPrefix(mediaType, "application/octet-stream") {
		return map[string]any{"bytes": len(body), "sha256": shortHash(body)}
	}
	var root any
	if err := json.Unmarshal(body, &root); err == nil {
		return summarizeJSONValue(root)
	}
	return map[string]any{"bytes": len(body), "preview": truncate(string(body), 500)}
}

func summarizeJSONValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		keys := make([]string, 0, len(x))
		for key := range x {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if isSecretKey(key) {
				out[key] = "<redacted>"
				continue
			}
			out[key] = summarizeJSONValueWithKey(x[key], key)
		}
		return out
	case []any:
		limit := len(x)
		if limit > 12 {
			limit = 12
		}
		out := make([]any, 0, limit+1)
		for i := 0; i < limit; i++ {
			out = append(out, summarizeJSONValue(x[i]))
		}
		if len(x) > limit {
			out = append(out, map[string]any{"truncatedItems": len(x) - limit})
		}
		return out
	case string:
		return summarizeString(x, "")
	default:
		return x
	}
}

func summarizeJSONValueWithKey(v any, key string) any {
	if s, ok := v.(string); ok {
		return summarizeString(s, key)
	}
	return summarizeJSONValue(v)
}

func summarizeString(value, key string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lowerKey := strings.ToLower(key)
	if isSecretKey(lowerKey) {
		return "<redacted>"
	}
	if strings.HasPrefix(value, "data:") {
		return summarizeDataURL(value)
	}
	if u := sanitizeURLIfHTTP(value); u != "" {
		return u
	}
	if len(value) > 300 && looksBase64ish(value) {
		return map[string]any{"kind": "base64", "chars": len(value)}
	}
	return truncate(value, 700)
}

func summarizeDataURL(value string) map[string]any {
	header, payload, ok := strings.Cut(value, ",")
	out := map[string]any{"kind": "data_url", "header": truncate(header, 80), "chars": len(value)}
	if ok && strings.Contains(header, "base64") {
		if data, err := base64.StdEncoding.DecodeString(payload); err == nil {
			out["bytes"] = len(data)
		}
	}
	return out
}

func isSecretKey(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "authorization") ||
		strings.Contains(key, "api_key") ||
		strings.Contains(key, "apikey") ||
		key == "key" ||
		strings.Contains(key, "token") ||
		strings.Contains(key, "secret")
}

func sanitizeURLIfHTTP(value string) string {
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return sanitizeURL(value)
	}
	return ""
}

func sanitizeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return raw
	}
	u.RawQuery = ""
	u.Fragment = ""
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String()
}

func looksBase64ish(value string) bool {
	if len(value) < 256 {
		return false
	}
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '+' || r == '/' || r == '=' || r == '\n' || r == '\r' {
			continue
		}
		return false
	}
	return true
}

func truncate(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "...(truncated " + strconv.Itoa(len(value)-limit) + " chars)"
}

func shortHash(data []byte) string {
	// FNV would be enough, but a tiny SHA-256 prefix is easier to compare across tools.
	const hex = "0123456789abcdef"
	h := uint64(1469598103934665603)
	for _, b := range data {
		h ^= uint64(b)
		h *= 1099511628211
	}
	var out [16]byte
	for i := 0; i < 8; i++ {
		v := byte(h >> ((7 - i) * 8))
		out[i*2] = hex[v>>4]
		out[i*2+1] = hex[v&0xf]
	}
	return string(out[:])
}
