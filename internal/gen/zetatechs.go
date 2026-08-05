package gen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

const (
	zetaTechsDefaultBaseURL  = "https://ent.zetatechs.com"
	zetaTechsModelsPath      = "/v1/models"
	zetaTechsGenerationPath  = "/v1/images/generations"
	zetaTechsEditPath        = "/v1/images/edits"
	zetaTechsVideoPath       = "/v1/videos"
	zetaTechsVideoGenPath    = "/v1/video/generations"
	zetaTechsModelGenPath    = "/model/v1/generation"
	zetaTechsModelTaskPath   = "/model/v1/generation/task/"
	zetaTechsAPIModelsPath   = "/api/models"
	zetaTechsVideoTaskPrefix = "video-generation:"
	zetaTechsModelTaskPrefix = "model-generation:"
)

var errVideoMissingTaskResult = errors.New("视频响应中没有任务 ID 或下载地址")

// ZetaTechs는 ZetaTechs OpenAI Image 호환 API 클라이언트입니다.
type ZetaTechs struct {
	*OpenAI
	BaseURL string
}

// NewZetaTechs는 새 ZetaTechs 이미지 생성 클라이언트를 생성합니다.
func NewZetaTechs(apiKey, model string) *ZetaTechs {
	return NewZetaTechsWithBaseURL(apiKey, model, "")
}

// NewZetaTechsWithBaseURL은 NewAPI 호환 ZetaTechs 게이트웨이 주소를 지정해 클라이언트를 생성합니다.
func NewZetaTechsWithBaseURL(apiKey, model, baseURL string) *ZetaTechs {
	if model == "" {
		model = DefaultModelFor(ProviderZetaTechs)
	}
	baseURL = normalizeZetaTechsBaseURL(baseURL)
	return &ZetaTechs{
		BaseURL: baseURL,
		OpenAI: &OpenAI{
			APIKey:               apiKey,
			Model:                model,
			HTTP:                 &http.Client{Timeout: 300 * time.Second},
			Name:                 "ZetaTechs",
			generationEndpoint:   baseURL + zetaTechsGenerationPath,
			editEndpoint:         baseURL + zetaTechsEditPath,
			sizeFor:              zetaTechsSizeFor,
			omitGenerationFormat: true,
			omitEditFormat:       true,
		},
	}
}

// GenerateVideo creates a ZetaTechs video task. Models exposed through the
// NewAPI video generation adapter use the JSON endpoint; other models use the
// OpenAI-compatible multipart video endpoint.
func (c *ZetaTechs) GenerateVideo(ctx context.Context, req VideoRequest) (VideoResult, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return VideoResult{}, errors.New("ZetaTechs API 密钥未设置，请在设置中输入。")
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = DefaultVideoModelFor(ProviderZetaTechs)
	}
	if req.Seconds <= 0 {
		req.Seconds = 4
	}
	if req.FPS <= 0 {
		req.FPS = 12
	}
	if req.Size == "" {
		req.Size = "1024x1024"
	}
	if strings.TrimSpace(req.ReferenceMode) == "" {
		req.ReferenceMode = VideoReferenceModeAuto
	}
	if strings.TrimSpace(req.Workflow) == "" {
		req.Workflow = inferVideoWorkflow(req)
	}
	if usesModelGenerationEndpoint(req.Model) {
		return c.generateModelGeneration(ctx, req)
	}
	if usesVideoGenerationsEndpoint(req.Model) {
		return c.generateVideoGeneration(ctx, req)
	}
	return c.generateOpenAIVideo(ctx, req)
}

func (c *ZetaTechs) generateOpenAIVideo(ctx context.Context, req VideoRequest) (VideoResult, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fields := map[string]string{
		"model":   req.Model,
		"prompt":  req.Prompt,
		"seconds": fmt.Sprintf("%d", req.Seconds),
		"size":    req.Size,
	}
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return VideoResult{}, fmt.Errorf("请求序列化失败: %w", err)
		}
	}
	if ref := firstOpenAIVideoReference(req); ref.Data != nil && shouldSendOpenAIVideoReference(req) {
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", `form-data; name="input_reference"; filename="reference.png"`)
		h.Set("Content-Type", "image/png")
		part, err := w.CreatePart(h)
		if err != nil {
			return VideoResult{}, fmt.Errorf("请求序列化失败: %w", err)
		}
		if _, err := part.Write(ref.Data); err != nil {
			return VideoResult{}, fmt.Errorf("请求序列化失败: %w", err)
		}
	}
	if err := w.Close(); err != nil {
		return VideoResult{}, fmt.Errorf("请求序列化失败: %w", err)
	}
	return c.doVideoRequest(ctx, http.MethodPost, c.BaseURL+zetaTechsVideoPath, w.FormDataContentType(), buf.Bytes())
}

func (c *ZetaTechs) generateVideoGeneration(ctx context.Context, req VideoRequest) (VideoResult, error) {
	body := buildVideoGenerationBody(req)
	data, err := json.Marshal(body)
	if err != nil {
		return VideoResult{}, fmt.Errorf("请求序列化失败: %w", err)
	}
	result, err := c.doVideoRequest(ctx, http.MethodPost, c.BaseURL+zetaTechsVideoGenPath, "application/json", data)
	if err != nil {
		return VideoResult{}, err
	}
	if result.ID != "" && !strings.HasPrefix(result.ID, zetaTechsVideoTaskPrefix) {
		result.ID = zetaTechsVideoTaskPrefix + result.ID
	}
	return result, nil
}

func (c *ZetaTechs) generateModelGeneration(ctx context.Context, req VideoRequest) (VideoResult, error) {
	body := map[string]any{
		"model": modelGenerationProviderModel(req.Model),
		"input": []map[string]any{
			{"params": buildModelGenerationParams(req)},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return VideoResult{}, fmt.Errorf("请求序列化失败: %w", err)
	}
	result, err := c.doVideoRequest(ctx, http.MethodPost, c.BaseURL+zetaTechsModelGenPath, "application/json", data)
	if err != nil {
		return VideoResult{}, err
	}
	if result.ID != "" && !strings.HasPrefix(result.ID, zetaTechsModelTaskPrefix) {
		result.ID = zetaTechsModelTaskPrefix + result.ID
	}
	return result, nil
}

func buildVideoGenerationBody(req VideoRequest) map[string]any {
	body := map[string]any{
		"model":  req.Model,
		"prompt": req.Prompt,
		"n":      1,
	}
	if shouldSendVideoGenerationTiming(req.Model) {
		body["duration"] = req.Seconds
		body["fps"] = req.FPS
	}
	if isWanVideoGenerationModel(req.Model) {
		if req.Workflow == VideoWorkflowTextToVideo || strings.Contains(strings.ToLower(req.Model), "t2v") {
			body["size"] = wanVideoSizeFor(req)
		} else {
			body["resolution"] = providerResolution(req.Resolution, "720P")
		}
	} else {
		if strings.TrimSpace(req.Size) != "" {
			body["size"] = req.Size
		}
		if strings.TrimSpace(req.Resolution) != "" {
			body["resolution"] = req.Resolution
		}
		if strings.TrimSpace(req.AspectRatio) != "" {
			body["ratio"] = req.AspectRatio
		}
	}
	if strings.TrimSpace(req.NegativePrompt) != "" {
		body["negative_prompt"] = req.NegativePrompt
	}
	if start := firstVideoGenerationStartImage(req); !start.empty() {
		imageValue := videoMediaDataURL(start)
		body["image"] = imageValue
		if isWanVideoGenerationModel(req.Model) && strings.TrimSpace(start.URL) != "" {
			body["image_url"] = strings.TrimSpace(start.URL)
			body["first_frame_url"] = strings.TrimSpace(start.URL)
		}
	}
	metadata := map[string]any{}
	if end := firstSlotMedia(req, VideoSlotEndImage); !end.empty() {
		tail := videoMediaDataURL(end)
		if isWanVideoGenerationModel(req.Model) {
			body["image_tail"] = tail
			body["end_image"] = tail
			if strings.TrimSpace(end.URL) != "" {
				body["last_frame_url"] = strings.TrimSpace(end.URL)
			}
		} else {
			metadata["image_tail"] = tail
			metadata["end_image"] = tail
		}
	}
	if refs := slotMediaValues(req, VideoSlotReferenceImages, true); len(refs) > 0 && supportsReferenceImageSlots(req.Model) {
		metadata["reference_images"] = refs
	}
	if videos := videoReferenceURLs(req, VideoSlotReferenceVideos); len(videos) > 0 {
		if isWanVideoGenerationModel(req.Model) {
			body["reference_video_urls"] = videos
			body["reference_urls"] = videos
			body["reference_videos"] = videos
			metadata["reference_video_urls"] = videos
			metadata["reference_urls"] = videos
			metadata["reference_videos"] = videos
			if len(videos) == 1 {
				body["reference_video_url"] = videos[0]
				body["video_url"] = videos[0]
				metadata["reference_video_url"] = videos[0]
				metadata["video_url"] = videos[0]
			}
		} else {
			metadata["reference_videos"] = videos
		}
	}
	if videos := videoReferenceURLs(req, VideoSlotDriverVideo); len(videos) > 0 {
		if isWanVideoGenerationModel(req.Model) {
			body["driver_video"] = videos[0]
			body["driver_video_url"] = videos[0]
			body["driving_video"] = videos[0]
			body["driving_video_url"] = videos[0]
			body["video"] = videos[0]
			body["video_url"] = videos[0]
			metadata["driver_video"] = videos[0]
			metadata["driver_video_url"] = videos[0]
			metadata["driving_video"] = videos[0]
			metadata["driving_video_url"] = videos[0]
			metadata["video"] = videos[0]
			metadata["video_url"] = videos[0]
		} else {
			metadata["driver_video"] = videos[0]
		}
	}
	if audios := audioReferenceURLs(req, VideoSlotReferenceAudios); len(audios) > 0 {
		metadata["reference_audios"] = audios
	}
	if audios := audioReferenceURLs(req, VideoSlotDriverAudio); len(audios) > 0 {
		metadata["driver_audio"] = audios[0]
	}
	if isWanVideoGenerationModel(req.Model) {
		addWanOfficialPayload(req, body, metadata)
	}
	for key, value := range req.Controls {
		if strings.TrimSpace(key) != "" && value != nil {
			metadata[key] = value
		}
	}
	if strings.TrimSpace(req.AspectRatio) != "" && !isWanVideoGenerationModel(req.Model) {
		metadata["aspect_ratio"] = req.AspectRatio
	}
	if strings.TrimSpace(req.Resolution) != "" && !isWanVideoGenerationModel(req.Model) {
		metadata["resolution"] = req.Resolution
	}
	if len(metadata) > 0 {
		body["metadata"] = metadata
	}
	return body
}

func addWanOfficialPayload(req VideoRequest, body, metadata map[string]any) {
	m := strings.ToLower(strings.TrimSpace(req.Model))
	input := map[string]any{}
	parameters := map[string]any{}
	start := firstVideoGenerationStartImage(req)
	end := firstSlotMedia(req, VideoSlotEndImage)
	driverVideos := videoReferenceURLs(req, VideoSlotDriverVideo)
	referenceVideos := videoReferenceURLs(req, VideoSlotReferenceVideos)
	referenceAudios := audioReferenceURLs(req, VideoSlotReferenceAudios)
	if len(referenceAudios) == 0 {
		referenceAudios = audioReferenceURLs(req, VideoSlotDriverAudio)
	}
	putInput := func(key string, value string) {
		if strings.TrimSpace(value) != "" {
			input[key] = value
		}
	}
	putParam := func(key string, value any) {
		if value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" {
			parameters[key] = value
		}
	}
	switch {
	case strings.Contains(m, "animate"):
		putInput("image_url", mediaURLOrData(start))
		if len(driverVideos) > 0 {
			putInput("video_url", driverVideos[0])
		}
		input["watermark"] = false
		putParam("mode", firstControlString(req.Controls, "mode", "wan-std"))
		putParam("check_image", true)
	case strings.Contains(m, "kf2v"):
		putInput("prompt", req.Prompt)
		putInput("first_frame_url", mediaURLOrData(start))
		putInput("last_frame_url", mediaURLOrData(end))
		if strings.TrimSpace(req.NegativePrompt) != "" {
			putInput("negative_prompt", req.NegativePrompt)
		}
		putParam("resolution", providerResolution(req.Resolution, "720P"))
		putParam("prompt_extend", false)
	case strings.Contains(m, "s2v"):
		putInput("prompt", req.Prompt)
		putInput("image_url", mediaURLOrData(start))
		if len(referenceAudios) > 0 {
			putInput("audio_url", referenceAudios[0])
		}
		putParam("resolution", providerResolution(req.Resolution, "480P"))
		putParam("style", firstControlString(req.Controls, "style", "speech"))
	case strings.Contains(m, "r2v") || strings.Contains(m, "vace"):
		putInput("prompt", req.Prompt)
		putInput("image_url", mediaURLOrData(start))
		if len(referenceVideos) > 0 {
			input["reference_video_urls"] = referenceVideos
			input["reference_urls"] = referenceVideos
			putInput("video_url", referenceVideos[0])
		}
		putParam("resolution", providerResolution(req.Resolution, "720P"))
	default:
		return
	}
	if len(input) > 0 {
		body["input"] = input
		metadata["input"] = input
	}
	if len(parameters) > 0 {
		body["parameters"] = parameters
		metadata["parameters"] = parameters
	}
}

func mediaURLOrData(media VideoMediaInput) string {
	if media.empty() {
		return ""
	}
	return videoMediaDataURL(media)
}

func firstControlString(controls map[string]any, key, fallback string) string {
	if controls != nil {
		if value := strings.TrimSpace(fmt.Sprint(controls[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	return fallback
}

func shouldSendVideoGenerationTiming(model string) bool {
	// Real Wan tasks reject custom duration/fps with:
	// "duration customization is not supported". Let the upstream choose its
	// native length and use our requested frame count only during extraction.
	return !isWanVideoGenerationModel(model)
}

func isWanVideoGenerationModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(m, "wan") || strings.HasPrefix(m, "wanx")
}

func providerResolution(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	if strings.EqualFold(value, "720p") {
		return "720P"
	}
	if strings.EqualFold(value, "1080p") {
		return "1080P"
	}
	return value
}

func wanVideoSizeFor(req VideoRequest) string {
	resolution := strings.ToLower(providerResolution(req.Resolution, "720P"))
	aspect := strings.TrimSpace(req.AspectRatio)
	portrait := aspect == "9:16" || aspect == "3:4"
	square := aspect == "" || aspect == "1:1"
	if strings.Contains(resolution, "1080") {
		if portrait {
			return "1080*1920"
		}
		if square {
			return "960*960"
		}
		return "1920*1080"
	}
	if square {
		return "960*960"
	}
	if portrait {
		return "720*1280"
	}
	return "1280*720"
}

func buildModelGenerationParams(req VideoRequest) map[string]any {
	if usesContentMultimodalAdapter(req) {
		return buildContentMultimodalParams(req)
	}
	input := map[string]any{"prompt": req.Prompt}
	if start := firstVideoGenerationStartImage(req); !start.empty() {
		input["image"] = videoMediaPureBase64OrURL(start)
	}
	if refs := slotMediaValues(req, VideoSlotReferenceImages, false); len(refs) > 0 {
		input["images"] = refs
	}
	if end := firstSlotMedia(req, VideoSlotEndImage); !end.empty() {
		input["image_tail"] = videoMediaPureBase64OrURL(end)
	}
	if req.Seconds > 0 {
		input["duration"] = req.Seconds
	}
	if strings.TrimSpace(req.Resolution) != "" {
		input["resolution"] = req.Resolution
	}
	if strings.TrimSpace(req.AspectRatio) != "" {
		input["aspect_ratio"] = req.AspectRatio
	}
	if strings.TrimSpace(req.NegativePrompt) != "" {
		input["negative_prompt"] = req.NegativePrompt
	}
	for key, value := range req.Controls {
		if strings.TrimSpace(key) != "" && value != nil {
			input[key] = value
		}
	}
	return input
}

func buildContentMultimodalParams(req VideoRequest) map[string]any {
	content := []map[string]any{{"type": "text", "text": req.Prompt}}
	appendImage := func(slot, role string) {
		for _, media := range req.MediaSlots[slot] {
			if media.empty() {
				continue
			}
			content = append(content, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": videoMediaDataURL(media)},
				"role":      role,
			})
		}
	}
	appendImage(VideoSlotStartImage, "first_frame")
	appendImage(VideoSlotReferenceImages, "reference_image")
	appendImage(VideoSlotEndImage, "last_frame")
	for _, url := range videoReferenceURLs(req, VideoSlotReferenceVideos) {
		content = append(content, map[string]any{
			"type":      "video_url",
			"video_url": map[string]any{"url": url},
			"role":      "reference_video",
		})
	}
	for _, url := range videoReferenceURLs(req, VideoSlotDriverVideo) {
		content = append(content, map[string]any{
			"type":      "video_url",
			"video_url": map[string]any{"url": url},
			"role":      "driver_video",
		})
	}
	for _, url := range audioReferenceURLs(req, VideoSlotReferenceAudios) {
		content = append(content, map[string]any{
			"type":      "audio_url",
			"audio_url": map[string]any{"url": url},
			"role":      "reference_audio",
		})
	}
	for _, url := range audioReferenceURLs(req, VideoSlotDriverAudio) {
		content = append(content, map[string]any{
			"type":      "audio_url",
			"audio_url": map[string]any{"url": url},
			"role":      "driver_audio",
		})
	}
	input := map[string]any{"content": content}
	if req.Seconds > 0 {
		input["duration"] = req.Seconds
	}
	if strings.TrimSpace(req.Resolution) != "" {
		input["resolution"] = req.Resolution
	}
	if strings.TrimSpace(req.AspectRatio) != "" {
		input["ratio"] = req.AspectRatio
	}
	if strings.TrimSpace(req.NegativePrompt) != "" {
		input["negative_prompt"] = req.NegativePrompt
	}
	for key, value := range req.Controls {
		if strings.TrimSpace(key) != "" && value != nil {
			input[key] = value
		}
	}
	return input
}

// PollVideo returns the current state of a ZetaTechs video task.
func (c *ZetaTechs) PollVideo(ctx context.Context, taskID string) (VideoResult, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return VideoResult{}, errors.New("视频任务 ID 为空。")
	}
	path := zetaTechsVideoPath + "/" + taskID
	if usesVideoGenerationTaskID(taskID) {
		path = zetaTechsVideoGenPath + "/" + strings.TrimPrefix(taskID, zetaTechsVideoTaskPrefix)
	} else if usesModelGenerationTaskID(taskID) {
		path = zetaTechsModelTaskPath + strings.TrimPrefix(taskID, zetaTechsModelTaskPrefix)
	}
	result, err := c.doVideoRequest(ctx, http.MethodGet, c.BaseURL+path, "", nil)
	if err != nil {
		if errors.Is(err, errVideoMissingTaskResult) {
			return VideoResult{ID: taskID, Status: VideoStatusQueued}, nil
		}
		return VideoResult{}, err
	}
	if usesVideoGenerationTaskID(taskID) && result.ID != "" && !strings.HasPrefix(result.ID, zetaTechsVideoTaskPrefix) {
		result.ID = zetaTechsVideoTaskPrefix + result.ID
	}
	if usesModelGenerationTaskID(taskID) && result.ID != "" && !strings.HasPrefix(result.ID, zetaTechsModelTaskPrefix) {
		result.ID = zetaTechsModelTaskPrefix + result.ID
	}
	return result, nil
}

// DownloadVideo downloads a completed video by URL or OpenAI-compatible task ID.
func (c *ZetaTechs) DownloadVideo(ctx context.Context, idOrURL string) ([]byte, error) {
	idOrURL = strings.TrimSpace(idOrURL)
	if idOrURL == "" {
		return nil, errors.New("视频下载地址为空。")
	}
	if strings.HasPrefix(idOrURL, "data:") {
		return decodeDataOrDownload(c.HTTP, idOrURL)
	}
	if strings.HasPrefix(idOrURL, "http://") || strings.HasPrefix(idOrURL, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, idOrURL, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, fmt.Errorf("视频下载网络错误: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("视频下载失败（HTTP %d）", resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 256<<20))
	}
	if usesModelGenerationTaskID(idOrURL) {
		task, err := c.PollVideo(ctx, idOrURL)
		if err != nil {
			return nil, err
		}
		if task.URL == "" {
			return nil, errors.New("模型任务尚未返回视频下载地址。")
		}
		return c.DownloadVideo(ctx, task.URL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+zetaTechsVideoPath+"/"+idOrURL+"/content", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("视频下载网络错误: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg := zetaTechsErrorMessage(resp)
		if msg != "" {
			return nil, fmt.Errorf("视频下载失败（%d）：%s", resp.StatusCode, msg)
		}
		return nil, fmt.Errorf("视频下载失败（HTTP %d）", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 256<<20))
}

// ListModels returns the models currently exposed by the configured gateway.
func (c *ZetaTechs) ListModels(ctx context.Context) ([]string, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, errors.New("ZetaTechs API 密钥未设置，请在设置中输入。")
	}
	var lastErr error
	for _, path := range []string{zetaTechsModelsPath, zetaTechsAPIModelsPath} {
		models, err := c.fetchModelList(ctx, c.BaseURL+path)
		if err == nil && len(models) > 0 {
			return models, nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("ZetaTechs 模型列表为空：%s", path)
		}
	}
	return nil, lastErr
}

func (c *ZetaTechs) fetchModelList(ctx context.Context, endpoint string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ZetaTechs 模型列表网络错误: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("ZetaTechs 模型列表响应读取失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := extractVideoMessage(data)
		if msg != "" {
			return nil, fmt.Errorf("ZetaTechs 模型列表错误（%d）：%s", resp.StatusCode, msg)
		}
		return nil, fmt.Errorf("ZetaTechs 模型列表错误（HTTP %d）", resp.StatusCode)
	}
	return parseModelList(data), nil
}

func (c *ZetaTechs) doVideoRequest(ctx context.Context, method, endpoint, contentType string, body []byte) (VideoResult, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, r)
	if err != nil {
		return VideoResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return VideoResult{}, fmt.Errorf("视频网络错误: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return VideoResult{}, fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := extractVideoMessage(data)
		if msg != "" {
			return VideoResult{}, fmt.Errorf("ZetaTechs 视频错误（%d）：%s", resp.StatusCode, msg)
		}
		return VideoResult{}, fmt.Errorf("ZetaTechs 视频错误（HTTP %d）", resp.StatusCode)
	}
	result := parseVideoResult(data)
	if result.ID == "" && result.URL == "" {
		return VideoResult{}, errVideoMissingTaskResult
	}
	if result.Status == "" {
		result.Status = VideoStatusQueued
	}
	return result, nil
}

func usesVideoGenerationsEndpoint(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(m, "kling") ||
		strings.HasPrefix(m, "jimeng") ||
		strings.HasPrefix(m, "vidu") ||
		strings.Contains(m, "seedance") ||
		((strings.HasPrefix(m, "wan") || strings.HasPrefix(m, "wanx")) && isLikelyZetaTechsVideoModel(m))
}

func usesModelGenerationEndpoint(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(m, "vtrix:") ||
		strings.HasPrefix(m, "vtrix/") ||
		strings.HasPrefix(m, "kirin") ||
		strings.HasPrefix(m, "spark_")
}

func modelGenerationProviderModel(model string) string {
	model = strings.TrimSpace(model)
	lower := strings.ToLower(model)
	for _, prefix := range []string{"vtrix:", "vtrix/"} {
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(model[len(prefix):])
		}
	}
	return model
}

func shouldSendOpenAIVideoReference(req VideoRequest) bool {
	if !VideoModelSupportsReferenceImage(req.Model) {
		return false
	}
	switch strings.TrimSpace(req.ReferenceMode) {
	case VideoReferenceModeNone:
		return false
	case VideoReferenceModeFirstFrame, VideoReferenceModeStartEndFrame:
		return false
	default:
		return true
	}
}

func shouldSendVideoGenerationImage(req VideoRequest) bool {
	if !VideoModelSupportsReferenceImage(req.Model) {
		return false
	}
	switch strings.TrimSpace(req.ReferenceMode) {
	case VideoReferenceModeFirstFrame, VideoReferenceModeStartEndFrame:
		return true
	default:
		return false
	}
}

func inferVideoWorkflow(req VideoRequest) string {
	if firstSlotMedia(req, VideoSlotEndImage).empty() && strings.TrimSpace(req.ReferenceMode) == VideoReferenceModeStartEndFrame {
		return VideoWorkflowFirstLastToVideo
	}
	if !firstSlotMedia(req, VideoSlotStartImage).empty() || strings.TrimSpace(req.ReferenceMode) == VideoReferenceModeFirstFrame {
		return VideoWorkflowImageToVideo
	}
	if len(videoReferenceURLs(req, VideoSlotDriverVideo)) > 0 {
		return VideoWorkflowMotionTransfer
	}
	if len(videoReferenceURLs(req, VideoSlotReferenceVideos)) > 0 {
		return VideoWorkflowReferenceToVideo
	}
	if !firstSlotMedia(req, VideoSlotReferenceImages).empty() || strings.TrimSpace(req.ReferenceMode) == VideoReferenceModeIdentityReference {
		return VideoWorkflowReferenceToVideo
	}
	return VideoWorkflowTextToVideo
}

func firstOpenAIVideoReference(req VideoRequest) VideoMediaInput {
	if media := firstSlotMedia(req, VideoSlotReferenceImages); !media.empty() {
		return media
	}
	if len(req.ReferenceImage) > 0 {
		return VideoMediaInput{Data: req.ReferenceImage, MIME: "image/png", Label: "reference"}
	}
	return VideoMediaInput{}
}

func firstVideoGenerationStartImage(req VideoRequest) VideoMediaInput {
	if media := firstSlotMedia(req, VideoSlotStartImage); !media.empty() {
		return media
	}
	if len(req.ReferenceImage) > 0 && shouldSendVideoGenerationImage(req) {
		return VideoMediaInput{Data: req.ReferenceImage, MIME: "image/png", Label: "start_image"}
	}
	return VideoMediaInput{}
}

func firstSlotMedia(req VideoRequest, slot string) VideoMediaInput {
	for _, media := range req.MediaSlots[slot] {
		if !media.empty() {
			return media
		}
	}
	return VideoMediaInput{}
}

func slotMediaValues(req VideoRequest, slot string, dataURL bool) []string {
	out := []string{}
	for _, media := range req.MediaSlots[slot] {
		if media.empty() {
			continue
		}
		if dataURL {
			out = append(out, videoMediaDataURL(media))
		} else {
			out = append(out, videoMediaPureBase64OrURL(media))
		}
	}
	return out
}

func videoReferenceURLs(req VideoRequest, slot string) []string {
	out := []string{}
	for _, media := range req.MediaSlots[slot] {
		if url := strings.TrimSpace(media.URL); url != "" {
			out = append(out, url)
		}
	}
	if slot == VideoSlotReferenceVideos {
		out = append(out, compactNonEmptyStrings(req.ReferenceVideos)...)
	}
	return uniqueNonEmptyStrings(out)
}

func audioReferenceURLs(req VideoRequest, slot string) []string {
	out := []string{}
	for _, media := range req.MediaSlots[slot] {
		if url := strings.TrimSpace(media.URL); url != "" {
			out = append(out, url)
		}
	}
	if slot == VideoSlotReferenceAudios {
		out = append(out, compactNonEmptyStrings(req.ReferenceAudios)...)
	}
	return uniqueNonEmptyStrings(out)
}

func (m VideoMediaInput) empty() bool {
	return len(m.Data) == 0 && strings.TrimSpace(m.URL) == ""
}

func videoMediaDataURL(media VideoMediaInput) string {
	if url := strings.TrimSpace(media.URL); url != "" {
		return url
	}
	mime := strings.TrimSpace(media.MIME)
	if mime == "" {
		mime = "image/png"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(media.Data)
}

func videoMediaPureBase64OrURL(media VideoMediaInput) string {
	if url := strings.TrimSpace(media.URL); url != "" {
		return url
	}
	return base64.StdEncoding.EncodeToString(media.Data)
}

func supportsReferenceImageSlots(model string) bool {
	cap := VideoModelCapabilityFor(model)
	return cap.SupportsMultiImageReference || strings.EqualFold(cap.ProviderAdapter, "seedance_multimodal")
}

func usesContentMultimodalAdapter(req VideoRequest) bool {
	adapter := strings.ToLower(strings.TrimSpace(req.ProviderAdapter))
	model := strings.ToLower(strings.TrimSpace(req.Model))
	return strings.Contains(adapter, "content_multimodal") ||
		strings.Contains(adapter, "seedance_multimodal") ||
		strings.HasPrefix(model, "spark_dance")
}

func compactNonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func uniqueNonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func usesVideoGenerationTaskID(taskID string) bool {
	id := strings.ToLower(strings.TrimSpace(taskID))
	return strings.HasPrefix(id, zetaTechsVideoTaskPrefix)
}

func usesModelGenerationTaskID(taskID string) bool {
	id := strings.ToLower(strings.TrimSpace(taskID))
	return strings.HasPrefix(id, zetaTechsModelTaskPrefix)
}

func parseVideoResult(data []byte) VideoResult {
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return VideoResult{Message: strings.TrimSpace(string(data))}
	}
	var out VideoResult
	walkVideoJSON(root, &out)
	out.Status = normalizeVideoStatus(out.Status)
	return out
}

func parseModelList(data []byte) []string {
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil
	}
	var out []string
	walkModelJSON(root, &out)
	return uniqueModels(out)
}

func walkModelJSON(v any, out *[]string) {
	switch x := v.(type) {
	case map[string]any:
		for _, key := range []string{"id", "model", "name"} {
			if model := asString(x[key]); looksLikeModelName(model) {
				*out = append(*out, strings.TrimSpace(model))
			}
		}
		if data, ok := x["data"]; ok {
			walkModelJSON(data, out)
		}
		for _, val := range x {
			switch val.(type) {
			case []any, map[string]any:
				walkModelJSON(val, out)
			}
		}
	case []any:
		for _, item := range x {
			if model := asString(item); looksLikeModelName(model) {
				*out = append(*out, strings.TrimSpace(model))
				continue
			}
			walkModelJSON(item, out)
		}
	}
}

func uniqueModels(models []string) []string {
	out := make([]string, 0, len(models))
	seen := make(map[string]bool, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		out = append(out, model)
	}
	return out
}

func looksLikeModelName(model string) bool {
	model = strings.TrimSpace(model)
	if model == "" || len(model) > 200 {
		return false
	}
	switch strings.ToLower(model) {
	case "model", "models", "list", "data", "success", "true", "false":
		return false
	}
	return !strings.ContainsAny(model, " \t\r\n{}[]")
}

func walkVideoJSON(v any, out *VideoResult) {
	state := videoJSONParseState{}
	walkVideoJSONValue(v, out, &state)
}

type videoJSONParseState struct {
	idRank     int
	statusRank int
}

func walkVideoJSONValue(v any, out *VideoResult, state *videoJSONParseState) {
	switch x := v.(type) {
	case map[string]any:
		if id := firstStringForKeys(x, "task_id", "taskId", "taskid"); id != "" {
			setVideoID(out, state, id, 2)
		} else if id := firstStringForKeys(x, "id"); id != "" {
			setVideoID(out, state, id, 1)
		}
		if status := firstStringForKeys(x, "status", "state", "task_status", "taskStatus"); status != "" {
			setVideoStatus(out, state, status)
		}
		if out.URL == "" {
			if url := firstStringForKeys(x, "url", "video_url", "videoUrl", "download_url", "downloadUrl", "output_url", "outputUrl", "file_url", "fileUrl"); url != "" {
				out.URL = url
			}
		}
		if out.Progress == 0 {
			if progress := asInt(x["progress"]); progress != 0 {
				out.Progress = progress
			}
		}
		if out.Message == "" {
			if msg := firstStringForKeys(x, "message", "msg", "error_message", "errorMessage", "reason", "fail_reason", "failReason", "status_message", "statusMessage"); msg != "" {
				out.Message = msg
			} else if errVal, ok := x["error"]; ok {
				out.Message = extractAnyMessage(errVal)
			}
		}
		for _, val := range x {
			walkVideoJSONValue(val, out, state)
		}
	case []any:
		for _, item := range x {
			walkVideoJSONValue(item, out, state)
		}
	case string:
		if out.URL == "" {
			out.URL = findHTTPURL(x)
		}
	}
}

func setVideoID(out *VideoResult, state *videoJSONParseState, id string, rank int) {
	id = strings.TrimSpace(id)
	if id == "" || rank < state.idRank {
		return
	}
	if rank == state.idRank && out.ID != "" {
		return
	}
	out.ID = id
	state.idRank = rank
}

func setVideoStatus(out *VideoResult, state *videoJSONParseState, status string) {
	normalized := normalizeVideoStatus(status)
	rank := videoStatusRank(normalized)
	if normalized == "" || rank < state.statusRank {
		return
	}
	if rank == state.statusRank && out.Status != "" {
		return
	}
	out.Status = normalized
	state.statusRank = rank
}

func videoStatusRank(status string) int {
	switch status {
	case VideoStatusFailed:
		return 5
	case VideoStatusCompleted:
		return 4
	case VideoStatusProcessing:
		return 3
	case VideoStatusQueued:
		return 2
	case "":
		return 0
	default:
		return 1
	}
}

func firstStringForKeys(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := asString(values[key]); value != "" {
			return value
		}
		want := normalizeJSONKey(key)
		for actual, value := range values {
			if normalizeJSONKey(actual) == want {
				if text := asString(value); text != "" {
					return text
				}
			}
		}
	}
	return ""
}

func normalizeJSONKey(key string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(key)), "_", "")
}

func extractVideoMessage(data []byte) string {
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return strings.TrimSpace(string(data))
	}
	return extractAnyMessage(root)
}

func extractAnyMessage(v any) string {
	switch x := v.(type) {
	case string:
		trimmed := strings.TrimSpace(x)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			var nested any
			if err := json.Unmarshal([]byte(trimmed), &nested); err == nil {
				if msg := extractAnyMessage(nested); msg != "" {
					return msg
				}
			}
		}
		return trimmed
	case map[string]any:
		for _, key := range []string{"message", "msg", "error_message", "detail", "reason", "fail_reason", "status_message"} {
			if msg := asString(x[key]); msg != "" {
				return msg
			}
		}
		if errVal, ok := x["error"]; ok {
			return extractAnyMessage(errVal)
		}
	}
	return ""
}

func normalizeVideoStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "created", "pending", "queued", "queueing", "queuing", "submitted", "not_start", "notstart":
		return VideoStatusQueued
	case "running", "processing", "in_progress", "generating":
		return VideoStatusProcessing
	case "completed", "complete", "succeeded", "success", "done":
		return VideoStatusCompleted
	case "failed", "fail", "failure", "error", "cancelled", "canceled", "rejected":
		return VideoStatusFailed
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func findHTTPURL(value string) string {
	for _, prefix := range []string{"https://", "http://"} {
		idx := strings.Index(value, prefix)
		if idx < 0 {
			continue
		}
		end := len(value)
		for i := idx; i < len(value); i++ {
			switch value[i] {
			case ' ', '\n', '\r', '\t', '"', '\'', '<', '>', ')', ']':
				end = i
				return strings.TrimRight(value[idx:end], ".,;")
			}
		}
		return strings.TrimRight(value[idx:end], ".,;")
	}
	return ""
}

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		return ""
	}
}

func asInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case string:
		s := strings.TrimSpace(strings.TrimSuffix(x, "%"))
		if s == "" {
			return 0
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int(f)
		}
		return 0
	default:
		return 0
	}
}

// ValidateKey는 ZetaTechs 토큰이 현재 게이트웨이에서 동작하는지 확인합니다.
func (c *ZetaTechs) ValidateKey(ctx context.Context) error {
	key := strings.TrimSpace(c.APIKey)
	if len(key) < 8 {
		return errors.New("ZetaTechs API 密钥太短。")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+zetaTechsModelsPath, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("ZetaTechs 密钥验证网络错误: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	msg := zetaTechsErrorMessage(resp)
	if msg != "" {
		return fmt.Errorf("ZetaTechs 密钥验证失败（%d）：%s", resp.StatusCode, msg)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errors.New("ZetaTechs API 密钥无效，或与当前 Base URL 不匹配。")
	}
	return fmt.Errorf("ZetaTechs 密钥验证失败（HTTP %d）", resp.StatusCode)
}

func normalizeZetaTechsBaseURL(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = zetaTechsDefaultBaseURL
	}
	return strings.TrimRight(baseURL, "/")
}

func zetaTechsErrorMessage(resp *http.Response) string {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var parsed struct {
		Message string `json:"message"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &parsed); err == nil {
		if parsed.Message != "" {
			return parsed.Message
		}
		if parsed.Error != nil && parsed.Error.Message != "" {
			return parsed.Error.Message
		}
	}
	return strings.TrimSpace(string(data))
}

// zetaTechsSizeFor는 ZetaTechs OpenAI Image 호환 모델에 맞는 해상도로 종횡비를 변환합니다.
func zetaTechsSizeFor(aspectRatio string) string {
	w, h := parseAspect(aspectRatio)
	if w <= 0 || h <= 0 || w == h {
		return "1024x1024"
	}
	if w > h {
		return "1536x1024"
	}
	return "1024x1536"
}
