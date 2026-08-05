package gen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var SupportedTextProviders = []string{
	ProviderVolcengine,
	ProviderZetaTechs,
	ProviderOpenAI,
	ProviderOpenRouter,
}

var textModelCatalog = map[string][]string{
	ProviderVolcengine: {
		"glm-5-2-260617",
		"glm-4-7-251222",
		"glm-4-5-air-20250728",
	},
	ProviderZetaTechs:  {"glm-5.2"},
	ProviderOpenAI:     {"gpt-5-mini"},
	ProviderOpenRouter: {"z-ai/glm-5.2"},
}

type TextClient struct {
	APIKey     string
	Model      string
	Provider   string
	BaseURL    string
	HTTP       *http.Client
	chatPath   string
	modelsPath string
}

type textMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type textCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []textMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type textCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error"`
}

func TextModelsFor(provider string) []string {
	return append([]string(nil), textModelCatalog[provider]...)
}

func DefaultTextModelFor(provider string) string {
	if models := textModelCatalog[provider]; len(models) > 0 {
		return models[0]
	}
	return ""
}

func SupportsText(provider string) bool {
	for _, candidate := range SupportedTextProviders {
		if candidate == provider {
			return true
		}
	}
	return false
}

func NewTextClient(provider, apiKey, model, baseURL string) (*TextClient, error) {
	if !SupportsText(provider) {
		return nil, fmt.Errorf("%s 暂不支持文本改写。", ProviderLabel(provider))
	}
	if strings.TrimSpace(model) == "" {
		model = DefaultTextModelFor(provider)
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	chatPath := "/chat/completions"
	modelsPath := "/models"
	switch provider {
	case ProviderVolcengine:
		if baseURL == "" {
			baseURL = volcengineDefaultBaseURL
		}
	case ProviderZetaTechs:
		baseURL = normalizeZetaTechsBaseURL(baseURL)
		chatPath = "/v1/chat/completions"
		modelsPath = "/v1/models"
	case ProviderOpenRouter:
		if baseURL == "" {
			baseURL = "https://openrouter.ai/api/v1"
		}
	case ProviderOpenAI:
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
	}
	return &TextClient{
		APIKey:     strings.TrimSpace(apiKey),
		Model:      strings.TrimSpace(model),
		Provider:   provider,
		BaseURL:    baseURL,
		HTTP:       &http.Client{Timeout: 90 * time.Second},
		chatPath:   chatPath,
		modelsPath: modelsPath,
	}, nil
}

func (c *TextClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	return c.CompleteWithMaxTokens(ctx, systemPrompt, userPrompt, 900)
}

func (c *TextClient) CompleteWithMaxTokens(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error) {
	if c == nil || c.APIKey == "" {
		return "", errors.New("文本 LLM API 密钥未设置，请在设置中输入。")
	}
	if c.Model == "" {
		return "", errors.New("文本模型未设置，请在设置中填写模型 ID。")
	}
	if maxTokens < 100 {
		maxTokens = 100
	}
	if maxTokens > 8000 {
		maxTokens = 8000
	}
	body, err := json.Marshal(textCompletionRequest{
		Model: c.Model,
		Messages: []textMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.2,
		MaxTokens:   maxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("文本请求序列化失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+c.chatPath, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s 文本模型网络错误: %w", ProviderLabel(c.Provider), err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", fmt.Errorf("读取文本模型响应失败: %w", err)
	}
	var payload textCompletionResponse
	_ = json.Unmarshal(data, &payload)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if payload.Error != nil && strings.TrimSpace(payload.Error.Message) != "" {
			return "", fmt.Errorf("%s 文本模型错误（%d）：%s", ProviderLabel(c.Provider), resp.StatusCode, payload.Error.Message)
		}
		return "", fmt.Errorf("%s 文本模型错误（HTTP %d）", ProviderLabel(c.Provider), resp.StatusCode)
	}
	if payload.Error != nil && strings.TrimSpace(payload.Error.Message) != "" {
		return "", fmt.Errorf("%s 文本模型错误：%s", ProviderLabel(c.Provider), payload.Error.Message)
	}
	if len(payload.Choices) == 0 {
		return "", errors.New("文本模型响应中没有改写结果。")
	}
	content := decodeTextContent(payload.Choices[0].Message.Content)
	if content == "" {
		return "", errors.New("文本模型返回了空的改写结果。")
	}
	return content, nil
}

func (c *TextClient) ListModels(ctx context.Context) ([]string, error) {
	if c == nil || c.APIKey == "" {
		return nil, errors.New("文本 LLM API 密钥未设置，请在设置中输入。")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+c.modelsPath, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s 模型列表网络错误: %w", ProviderLabel(c.Provider), err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("读取模型列表失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s 模型列表错误（HTTP %d）", ProviderLabel(c.Provider), resp.StatusCode)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("模型列表格式错误: %w", err)
	}
	out := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		if id := strings.TrimSpace(item.ID); id != "" {
			out = append(out, id)
		}
	}
	return out, nil
}

func decodeTextContent(raw json.RawMessage) string {
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		return strings.TrimSpace(plain)
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var out []string
		for _, part := range parts {
			if strings.TrimSpace(part.Text) != "" {
				out = append(out, strings.TrimSpace(part.Text))
			}
		}
		return strings.TrimSpace(strings.Join(out, "\n"))
	}
	return ""
}
