package gen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxVisionImageBytes = 25 << 20

var SupportedVisionProviders = []string{
	ProviderZetaTechs,
	ProviderOpenAI,
	ProviderOpenRouter,
}

var visionModelCatalog = map[string][]string{
	ProviderZetaTechs:  {"gpt-4o-mini", "gemini-2.5-flash"},
	ProviderOpenAI:     {"gpt-4o-mini", "gpt-4.1-mini"},
	ProviderOpenRouter: {"openai/gpt-4o-mini", "google/gemini-2.5-flash"},
}

type VisionClient struct {
	APIKey   string
	Model    string
	Provider string
	BaseURL  string
	HTTP     *http.Client
	chatPath string
}

type visionContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *visionImageURL `json:"image_url,omitempty"`
}

type visionImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type visionMessage struct {
	Role    string              `json:"role"`
	Content []visionContentPart `json:"content"`
}

type visionRequest struct {
	Model          string          `json:"model"`
	Messages       []visionMessage `json:"messages"`
	Temperature    float64         `json:"temperature"`
	MaxTokens      int             `json:"max_tokens"`
	ResponseFormat map[string]any  `json:"response_format,omitempty"`
}

func VisionModelsFor(provider string) []string {
	return append([]string(nil), visionModelCatalog[provider]...)
}

func DefaultVisionModelFor(provider string) string {
	if models := visionModelCatalog[provider]; len(models) > 0 {
		return models[0]
	}
	return ""
}

func SupportsVision(provider string) bool {
	for _, candidate := range SupportedVisionProviders {
		if candidate == provider {
			return true
		}
	}
	return false
}

func NewVisionClient(provider, apiKey, model, baseURL string) (*VisionClient, error) {
	if !SupportsVision(provider) {
		return nil, fmt.Errorf("%s 暂不支持图片理解。", ProviderLabel(provider))
	}
	if strings.TrimSpace(model) == "" {
		model = DefaultVisionModelFor(provider)
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	chatPath := "/chat/completions"
	switch provider {
	case ProviderZetaTechs:
		baseURL = normalizeZetaTechsBaseURL(baseURL)
		chatPath = "/v1/chat/completions"
	case ProviderOpenRouter:
		if baseURL == "" {
			baseURL = "https://openrouter.ai/api/v1"
		}
	case ProviderOpenAI:
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
	}
	return &VisionClient{
		APIKey:   strings.TrimSpace(apiKey),
		Model:    strings.TrimSpace(model),
		Provider: provider,
		BaseURL:  baseURL,
		HTTP:     &http.Client{Timeout: 120 * time.Second},
		chatPath: chatPath,
	}, nil
}

// AnalyzeJSON asks a multimodal model for a single JSON object. The caller is
// responsible for validating the object against its domain schema.
func (c *VisionClient) AnalyzeJSON(ctx context.Context, systemPrompt, userPrompt string, images [][]byte, maxTokens int) (json.RawMessage, error) {
	if c == nil || c.APIKey == "" {
		return nil, errors.New("视觉模型 API 密钥未设置，请在设置中输入。")
	}
	if c.Model == "" {
		return nil, errors.New("视觉模型未设置。")
	}
	if len(images) == 0 {
		return nil, errors.New("请提供需要分析的图片。")
	}
	if len(images) > 4 {
		return nil, errors.New("视觉分析最多支持 4 张参考图片。")
	}
	if maxTokens < 500 {
		maxTokens = 500
	}
	if maxTokens > 12000 {
		maxTokens = 12000
	}

	parts := []visionContentPart{{Type: "text", Text: strings.TrimSpace(userPrompt)}}
	for _, imageBytes := range images {
		if len(imageBytes) == 0 || len(imageBytes) > maxVisionImageBytes {
			return nil, fmt.Errorf("视觉分析图片大小必须在 1 字节到 %d MB 之间。", maxVisionImageBytes>>20)
		}
		mime, err := supportedVisionMIME(imageBytes)
		if err != nil {
			return nil, err
		}
		parts = append(parts, visionContentPart{
			Type: "image_url",
			ImageURL: &visionImageURL{
				URL:    "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(imageBytes),
				Detail: "high",
			},
		})
	}

	body, err := json.Marshal(visionRequest{
		Model: c.Model,
		Messages: []visionMessage{
			{Role: "system", Content: []visionContentPart{{Type: "text", Text: strings.TrimSpace(systemPrompt)}}},
			{Role: "user", Content: parts},
		},
		Temperature:    0.1,
		MaxTokens:      maxTokens,
		ResponseFormat: map[string]any{"type": "json_object"},
	})
	if err != nil {
		return nil, fmt.Errorf("视觉请求序列化失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+c.chatPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s 视觉模型网络错误: %w", ProviderLabel(c.Provider), err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("读取视觉模型响应失败: %w", err)
	}
	var payload textCompletionResponse
	_ = json.Unmarshal(data, &payload)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if payload.Error != nil && strings.TrimSpace(payload.Error.Message) != "" {
			return nil, fmt.Errorf("%s 视觉模型错误（%d）：%s", ProviderLabel(c.Provider), resp.StatusCode, payload.Error.Message)
		}
		return nil, fmt.Errorf("%s 视觉模型错误（HTTP %d）", ProviderLabel(c.Provider), resp.StatusCode)
	}
	if len(payload.Choices) == 0 {
		return nil, errors.New("视觉模型响应中没有分析结果。")
	}
	content := cleanJSONObject(decodeTextContent(payload.Choices[0].Message.Content))
	if content == "" || !json.Valid([]byte(content)) {
		return nil, errors.New("视觉模型没有返回有效 JSON。")
	}
	return json.RawMessage(content), nil
}

func supportedVisionMIME(data []byte) (string, error) {
	mime := http.DetectContentType(data)
	switch mime {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		return mime, nil
	default:
		return "", fmt.Errorf("视觉模型不支持该图片格式：%s", mime)
	}
}

func cleanJSONObject(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "```") {
		if idx := strings.IndexByte(value, '\n'); idx >= 0 {
			value = value[idx+1:]
		}
		value = strings.TrimSuffix(strings.TrimSpace(value), "```")
	}
	start := strings.IndexByte(value, '{')
	end := strings.LastIndexByte(value, '}')
	if start >= 0 && end >= start {
		return strings.TrimSpace(value[start : end+1])
	}
	return ""
}
