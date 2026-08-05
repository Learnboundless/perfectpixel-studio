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

// Fal은 fal.ai 동기 실행(run) API 클라이언트입니다.
type Fal struct {
	APIKey string
	Model  string // 예: fal-ai/nano-banana-pro (참조 이미지가 있으면 자동으로 /edit 사용)
	HTTP   *http.Client
}

// NewFal은 새 fal.ai 클라이언트를 생성합니다.
func NewFal(apiKey, model string) *Fal {
	if model == "" {
		model = DefaultModelFor(ProviderFal)
	}
	return &Fal{
		APIKey: apiKey,
		Model:  model,
		HTTP:   &http.Client{Timeout: 300 * time.Second},
	}
}

type falRequest struct {
	Prompt       string   `json:"prompt"`
	ImageURLs    []string `json:"image_urls,omitempty"`
	NumImages    int      `json:"num_images"`
	OutputFormat string   `json:"output_format"`
	AspectRatio  string   `json:"aspect_ratio,omitempty"`
	SyncMode     bool     `json:"sync_mode"`
}

type falResponse struct {
	Images []struct {
		URL string `json:"url"`
	} `json:"images"`
	Detail any `json:"detail"`
}

// endpoint는 참조 이미지 유무에 따라 edit 엔드포인트를 선택합니다.
func (c *Fal) endpoint(hasRefs bool) string {
	model := strings.TrimSuffix(strings.TrimSpace(c.Model), "/")
	if hasRefs && !strings.HasSuffix(model, "/edit") {
		model += "/edit"
	}
	return "https://fal.run/" + model
}

// GenerateImage는 fal.ai로 이미지를 생성합니다.
func (c *Fal) GenerateImage(ctx context.Context, prompt string, refImages [][]byte, aspectRatio string) ([]byte, error) {
	if c.APIKey == "" {
		return nil, errors.New("fal.ai API 密钥未设置，请在设置中输入")
	}

	reqData := falRequest{
		// 종횡비 파라미터가 무시되는 경우를 대비해 프롬프트 힌트도 함께 전달
		Prompt:       prompt + "\n\n" + aspectHint(aspectRatio),
		NumImages:    1,
		OutputFormat: "png",
		AspectRatio:  aspectRatio,
		SyncMode:     false,
	}
	for _, img := range refImages {
		reqData.ImageURLs = append(reqData.ImageURLs,
			"data:image/png;base64,"+base64.StdEncoding.EncodeToString(img))
	}

	url := c.endpoint(len(refImages) > 0)

	var lastErr error
	backoff := 2 * time.Second
	withAspect := true
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}

		send := reqData
		if !withAspect {
			send.AspectRatio = ""
		}
		body, err := json.Marshal(send)
		if err != nil {
			return nil, fmt.Errorf("请求序列化失败: %w", err)
		}

		img, status, err := c.doRequest(ctx, url, body)
		if err == nil {
			return img, nil
		}
		lastErr = err

		// 스키마 거부(422)면 aspect_ratio 없이 1회 재시도
		if status == 422 && withAspect {
			withAspect = false
			continue
		}
		if paidRequestRetriesDisabled(ctx) {
			return nil, err
		}
		if status != 429 && status < 500 && status != 0 {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *Fal) doRequest(ctx context.Context, url string, body []byte) (img []byte, status int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Key "+c.APIKey)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("网络错误: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("响应读取失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var parsed falResponse
		_ = json.Unmarshal(respBytes, &parsed)
		detail := ""
		if parsed.Detail != nil {
			if d, jerr := json.Marshal(parsed.Detail); jerr == nil {
				detail = ": " + string(d)
			}
		}
		return nil, resp.StatusCode, fmt.Errorf("fal.ai 错误 (HTTP %d)%s", resp.StatusCode, detail)
	}

	var parsed falResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("响应解析失败: %w", err)
	}
	if len(parsed.Images) == 0 || parsed.Images[0].URL == "" {
		return nil, 500, errors.New("响应中没有图片")
	}
	data, err := decodeDataOrDownloadContext(ctx, c.HTTP, parsed.Images[0].URL)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

// ValidateKey는 fal 키 형식을 확인합니다 (fal은 경량 검증 엔드포인트가 없음).
func (c *Fal) ValidateKey(_ context.Context) error {
	key := strings.TrimSpace(c.APIKey)
	if len(key) < 10 {
		return errors.New("API 密钥太短")
	}
	return nil
}
