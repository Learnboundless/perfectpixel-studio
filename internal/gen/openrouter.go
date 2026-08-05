package gen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"
)

const openRouterEndpoint = "https://openrouter.ai/api/v1/chat/completions"

// OpenRouter는 OpenRouter 경유 이미지 생성 클라이언트입니다.
type OpenRouter struct {
	APIKey string
	Model  string
	HTTP   *http.Client
}

// NewOpenRouter는 새 OpenRouter 클라이언트를 생성합니다.
func NewOpenRouter(apiKey, model string) *OpenRouter {
	if model == "" {
		model = DefaultModelFor(ProviderOpenRouter)
	}
	return &OpenRouter{
		APIKey: apiKey,
		Model:  model,
		HTTP:   &http.Client{Timeout: 180 * time.Second},
	}
}

type orContentPart struct {
	Type     string      `json:"type"`
	Text     string      `json:"text,omitempty"`
	ImageURL *orImageURL `json:"image_url,omitempty"`
}

type orImageURL struct {
	URL string `json:"url"`
}

type orRequest struct {
	Model      string      `json:"model"`
	Messages   []orMessage `json:"messages"`
	Modalities []string    `json:"modalities"`
}

type orMessage struct {
	Role    string          `json:"role"`
	Content []orContentPart `json:"content"`
}

type orResponse struct {
	Choices []struct {
		Message struct {
			Images []struct {
				ImageURL orImageURL `json:"image_url"`
			} `json:"images"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// GenerateImage는 OpenRouter chat completions API로 이미지를 생성합니다.
func (c *OpenRouter) GenerateImage(ctx context.Context, prompt string, refImages [][]byte, aspectRatio string) ([]byte, error) {
	if c.APIKey == "" {
		return nil, errors.New("OpenRouter API 密钥未设置，请在设置中输入")
	}

	// OpenRouter는 종횡비 파라미터가 없어 프롬프트로 유도합니다.
	fullPrompt := prompt + "\n\n" + aspectHint(aspectRatio)

	parts := []orContentPart{{Type: "text", Text: fullPrompt}}
	for _, img := range refImages {
		parts = append(parts, orContentPart{
			Type: "image_url",
			ImageURL: &orImageURL{
				URL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(img),
			},
		})
	}

	body, err := json.Marshal(orRequest{
		Model:      c.Model,
		Messages:   []orMessage{{Role: "user", Content: parts}},
		Modalities: []string{"image", "text"},
	})
	if err != nil {
		return nil, fmt.Errorf("请求序列化失败: %w", err)
	}

	// 동시 배치 생성 시 일시적 429(rate limit)가 잦으므로 재시도를 넉넉히(6회) 두고
	// 백오프 상한을 둔 지수 증가 + 지터로 부하를 분산한다.
	var lastErr error
	backoff := 2 * time.Second
	for attempt := 0; attempt < 6; attempt++ {
		if attempt > 0 {
			jitter := time.Duration(rand.Int63n(int64(time.Second)))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff + jitter):
			}
			if backoff < 16*time.Second {
				backoff *= 2
			}
		}
		img, retryable, err := c.doRequest(ctx, body)
		if err == nil {
			return img, nil
		}
		lastErr = err
		if paidRequestRetriesDisabled(ctx) {
			return nil, err
		}
		if !retryable {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *OpenRouter) doRequest(ctx context.Context, body []byte) (img []byte, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("X-Title", "PerfectPixel")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("网络错误: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, true, fmt.Errorf("响应读取失败: %w", err)
	}

	var parsed orResponse
	_ = json.Unmarshal(respBytes, &parsed)

	if resp.StatusCode != http.StatusOK {
		retryable = resp.StatusCode == 429 || resp.StatusCode >= 500
		if parsed.Error != nil {
			return nil, retryable, fmt.Errorf("OpenRouter 错误 (%d): %s", resp.StatusCode, parsed.Error.Message)
		}
		return nil, retryable, fmt.Errorf("OpenRouter 错误 (HTTP %d)", resp.StatusCode)
	}
	if parsed.Error != nil {
		return nil, true, fmt.Errorf("OpenRouter 错误: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 || len(parsed.Choices[0].Message.Images) == 0 {
		return nil, true, errors.New("响应中没有图片")
	}
	data, err := decodeDataOrDownloadContext(ctx, c.HTTP, parsed.Choices[0].Message.Images[0].ImageURL.URL)
	if err != nil {
		return nil, false, err
	}
	return data, false, nil
}

// ValidateKey는 OpenRouter 키 유효성을 확인합니다.
func (c *OpenRouter) ValidateKey(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openrouter.ai/api/v1/key", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("网络错误: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return errors.New("API 密钥无效")
	}
	return fmt.Errorf("密钥校验失败 (HTTP %d)", resp.StatusCode)
}
