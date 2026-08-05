package gen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	volcengineDefaultBaseURL = "https://ark.cn-beijing.volces.com/api/v3"
	volcengineTasksPath      = "/contents/generations/tasks"
	volcengineModelsPath     = "/models"
)

// Volcengine implements the China-region Ark asynchronous video API.
type Volcengine struct {
	APIKey  string
	Model   string
	BaseURL string
	HTTP    *http.Client
}

func NewVolcengine(apiKey, model string) *Volcengine {
	return NewVolcengineWithBaseURL(apiKey, model, "")
}

func NewVolcengineWithBaseURL(apiKey, model, baseURL string) *Volcengine {
	if strings.TrimSpace(model) == "" {
		model = DefaultVideoModelFor(ProviderVolcengine)
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = volcengineDefaultBaseURL
	}
	return &Volcengine{
		APIKey:  strings.TrimSpace(apiKey),
		Model:   strings.TrimSpace(model),
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 5 * time.Minute},
	}
}

func (c *Volcengine) GenerateImage(context.Context, string, [][]byte, string) ([]byte, error) {
	return nil, errors.New("火山方舟当前仅用于视频生成，请为图片管线选择其他图片服务商。")
}

func (c *Volcengine) ValidateKey(ctx context.Context) error {
	_, err := c.ListVideoModels(ctx)
	return err
}

func (c *Volcengine) ListVideoModels(ctx context.Context) ([]string, error) {
	if c.APIKey == "" {
		return nil, errors.New("火山方舟 API 密钥未设置，请在设置中输入。")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+volcengineModelsPath, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("火山方舟模型列表网络错误: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("火山方舟模型列表读取失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, volcengineHTTPError("模型列表", resp.StatusCode, data)
	}
	var payload struct {
		Data []struct {
			ID         string   `json:"id"`
			Name       string   `json:"name"`
			Domain     string   `json:"domain"`
			TaskType   []string `json:"task_type"`
			Modalities struct {
				Output []string `json:"output_modalities"`
			} `json:"modalities"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("火山方舟模型列表格式错误: %w", err)
	}
	var out []string
	for _, item := range payload.Data {
		if !isVolcengineVideoModel(item.ID, item.Domain, item.Modalities.Output) {
			continue
		}
		out = append(out, strings.TrimSpace(item.ID))
	}
	if len(out) == 0 {
		return nil, errors.New("火山方舟没有返回可用的视频模型。")
	}
	return out, nil
}

func isVolcengineVideoModel(id, domain string, outputs []string) bool {
	if strings.EqualFold(strings.TrimSpace(domain), "VideoGeneration") {
		return true
	}
	for _, output := range outputs {
		if strings.EqualFold(strings.TrimSpace(output), "video") {
			return true
		}
	}
	return strings.Contains(strings.ToLower(id), "seedance")
}

func (c *Volcengine) GenerateVideo(ctx context.Context, req VideoRequest) (VideoResult, error) {
	if c.APIKey == "" {
		return VideoResult{}, errors.New("火山方舟 API 密钥未设置，请在设置中输入。")
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = c.Model
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = DefaultVideoModelFor(ProviderVolcengine)
	}
	body, err := buildVolcengineVideoBody(req)
	if err != nil {
		return VideoResult{}, err
	}
	return c.doTaskRequest(ctx, http.MethodPost, c.BaseURL+volcengineTasksPath, body)
}

func buildVolcengineVideoBody(req VideoRequest) ([]byte, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return nil, errors.New("火山方舟视频模型为空。")
	}
	content := make([]map[string]any, 0, 8)
	referenceAdded := false
	if prompt := strings.TrimSpace(req.Prompt); prompt != "" {
		content = append(content, map[string]any{"type": "text", "text": prompt})
	}
	appendImage := func(media VideoMediaInput, role string) {
		if media.Data == nil && strings.TrimSpace(media.URL) == "" {
			return
		}
		content = append(content, map[string]any{
			"type":      "image_url",
			"image_url": map[string]any{"url": volcengineMediaURL(media, "image/png")},
			"role":      role,
		})
		referenceAdded = true
	}
	appendURLMedia := func(media VideoMediaInput, mediaType, role string) {
		url := strings.TrimSpace(media.URL)
		if url == "" {
			return
		}
		content = append(content, map[string]any{
			"type":             mediaType + "_url",
			mediaType + "_url": map[string]any{"url": url},
			"role":             role,
		})
		referenceAdded = true
	}

	switch req.ReferenceMode {
	case VideoReferenceModeIdentityReference:
		for _, media := range req.MediaSlots[VideoSlotReferenceImages] {
			appendImage(media, "reference_image")
		}
	case VideoReferenceModeFirstFrame:
		for _, media := range req.MediaSlots[VideoSlotStartImage] {
			appendImage(media, "first_frame")
		}
	case VideoReferenceModeStartEndFrame:
		for _, media := range req.MediaSlots[VideoSlotStartImage] {
			appendImage(media, "first_frame")
		}
		for _, media := range req.MediaSlots[VideoSlotEndImage] {
			appendImage(media, "last_frame")
		}
	}
	if !referenceAdded && len(req.ReferenceImage) > 0 && req.ReferenceMode != VideoReferenceModeNone {
		role := "reference_image"
		if req.ReferenceMode == VideoReferenceModeFirstFrame || req.ReferenceMode == VideoReferenceModeStartEndFrame {
			role = "first_frame"
		}
		appendImage(VideoMediaInput{Data: req.ReferenceImage, MIME: "image/png"}, role)
	}
	for _, media := range req.MediaSlots[VideoSlotReferenceVideos] {
		appendURLMedia(media, "video", "reference_video")
	}
	for _, media := range req.MediaSlots[VideoSlotReferenceAudios] {
		appendURLMedia(media, "audio", "reference_audio")
	}
	if len(content) == 0 {
		return nil, errors.New("火山方舟视频请求至少需要提示词或参考素材。")
	}

	seconds := req.Seconds
	if strings.Contains(strings.ToLower(model), "seedance-2-0") {
		if seconds < 4 {
			seconds = 4
		}
		if seconds > 15 {
			seconds = 15
		}
	} else if seconds <= 0 {
		seconds = 4
	}
	resolution := strings.ToLower(strings.TrimSpace(req.Resolution))
	if resolution == "" {
		resolution = "720p"
	}
	if strings.Contains(strings.ToLower(model), "mini") && resolution == "1080p" {
		resolution = "720p"
	}
	ratio := strings.TrimSpace(req.AspectRatio)
	if ratio == "" {
		ratio = "1:1"
	}
	body := map[string]any{
		"model":             model,
		"content":           content,
		"duration":          seconds,
		"resolution":        resolution,
		"ratio":             ratio,
		"generate_audio":    false,
		"watermark":         false,
		"return_last_frame": true,
	}
	for key, value := range req.Controls {
		if strings.TrimSpace(key) != "" && value != nil {
			body[key] = value
		}
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("火山方舟请求序列化失败: %w", err)
	}
	return data, nil
}

func volcengineMediaURL(media VideoMediaInput, fallbackMIME string) string {
	if url := strings.TrimSpace(media.URL); url != "" {
		return url
	}
	mime := strings.TrimSpace(media.MIME)
	if mime == "" {
		mime = fallbackMIME
	}
	data := media.Data
	if mime == "image/png" {
		data = normalizeVolcenginePNG(data)
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func normalizeVolcenginePNG(data []byte) []byte {
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width >= 300 && height >= 300 {
		return data
	}
	scale := 1
	for width*scale < 300 || height*scale < 300 {
		scale++
	}
	dst := image.NewNRGBA(image.Rect(0, 0, width*scale, height*scale))
	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			dst.Set(x, y, src.At(bounds.Min.X+x/scale, bounds.Min.Y+y/scale))
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return data
	}
	return out.Bytes()
}

func (c *Volcengine) PollVideo(ctx context.Context, taskID string) (VideoResult, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return VideoResult{}, errors.New("火山方舟视频任务 ID 为空。")
	}
	return c.doTaskRequest(ctx, http.MethodGet, c.BaseURL+volcengineTasksPath+"/"+taskID, nil)
}

func (c *Volcengine) DownloadVideo(ctx context.Context, idOrURL string) ([]byte, error) {
	target := strings.TrimSpace(idOrURL)
	if target == "" {
		return nil, errors.New("火山方舟视频下载地址为空。")
	}
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		task, err := c.PollVideo(ctx, target)
		if err != nil {
			return nil, err
		}
		if !task.Done() || task.URL == "" {
			return nil, errors.New("火山方舟任务尚未返回视频下载地址。")
		}
		target = task.URL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("火山方舟视频下载网络错误: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, volcengineHTTPError("视频下载", resp.StatusCode, data)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 256<<20))
}

func (c *Volcengine) doTaskRequest(ctx context.Context, method, endpoint string, body []byte) (VideoResult, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return VideoResult{}, err
	}
	c.setHeaders(req)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return VideoResult{}, fmt.Errorf("火山方舟视频网络错误: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return VideoResult{}, fmt.Errorf("火山方舟视频响应读取失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return VideoResult{}, volcengineHTTPError("视频任务", resp.StatusCode, data)
	}
	return parseVolcengineTask(data)
}

func (c *Volcengine) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Accept", "application/json")
	if req.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
}

func parseVolcengineTask(data []byte) (VideoResult, error) {
	var payload struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Progress int    `json:"progress"`
		Error    any    `json:"error"`
		Content  struct {
			VideoURL string `json:"video_url"`
			FileURL  string `json:"file_url"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return VideoResult{}, fmt.Errorf("火山方舟视频响应格式错误: %w", err)
	}
	result := VideoResult{
		ID:       strings.TrimSpace(payload.ID),
		Status:   normalizeVolcengineStatus(payload.Status),
		URL:      strings.TrimSpace(payload.Content.VideoURL),
		Progress: payload.Progress,
	}
	if result.URL == "" {
		result.URL = strings.TrimSpace(payload.Content.FileURL)
	}
	if payload.Error != nil {
		if raw, err := json.Marshal(payload.Error); err == nil && string(raw) != "null" {
			result.Message = extractVolcengineMessage(raw)
		}
	}
	if result.ID == "" && result.URL == "" {
		return VideoResult{}, errors.New("火山方舟响应中没有任务 ID 或视频地址。")
	}
	return result, nil
}

func normalizeVolcengineStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded", "success", "completed":
		return VideoStatusCompleted
	case "failed", "failure", "expired", "cancelled", "canceled":
		return VideoStatusFailed
	case "running", "processing":
		return VideoStatusProcessing
	default:
		return VideoStatusQueued
	}
}

func volcengineHTTPError(operation string, status int, data []byte) error {
	message := extractVolcengineMessage(data)
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("火山方舟%s失败（401）：API 密钥无效或已过期。", operation)
	case http.StatusForbidden:
		return fmt.Errorf("火山方舟%s失败（403）：当前账号没有模型权限或余额条件未满足。", operation)
	case http.StatusTooManyRequests:
		return fmt.Errorf("火山方舟%s失败（429）：请求过于频繁或并发额度不足，请稍后重试。", operation)
	}
	if message != "" {
		return fmt.Errorf("火山方舟%s失败（%d）：%s", operation, status, message)
	}
	return fmt.Errorf("火山方舟%s失败（HTTP %d）。", operation, status)
}

func extractVolcengineMessage(data []byte) string {
	var payload struct {
		Message string `json:"message"`
		Error   struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &payload) == nil {
		if strings.TrimSpace(payload.Error.Message) != "" {
			if strings.TrimSpace(payload.Error.Code) != "" {
				return payload.Error.Code + ": " + payload.Error.Message
			}
			return payload.Error.Message
		}
		if strings.TrimSpace(payload.Message) != "" {
			return payload.Message
		}
	}
	var message string
	if json.Unmarshal(data, &message) == nil {
		return strings.TrimSpace(message)
	}
	return strings.TrimSpace(string(data))
}
