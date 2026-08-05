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
	"strconv"
	"strings"
	"time"
)

// bytePlusEndpoint은 BytePlus ModelArk 이미지 생성(Seedream) API 주소입니다.
const bytePlusEndpoint = "https://ark.ap-southeast.bytepluses.com/api/v3/images/generations"

// BytePlus는 BytePlus ModelArk(Seedream) 이미지 생성 클라이언트입니다.
type BytePlus struct {
	APIKey string
	Model  string // 예: seedream-4-0-250828
	HTTP   *http.Client

	endpoint string // 테스트용 오버라이드 (빈 값이면 bytePlusEndpoint)
}

// NewBytePlus는 새 BytePlus 클라이언트를 생성합니다.
func NewBytePlus(apiKey, model string) *BytePlus {
	if model == "" {
		model = DefaultModelFor(ProviderBytePlus)
	}
	return &BytePlus{
		APIKey: apiKey,
		Model:  model,
		HTTP:   &http.Client{Timeout: 300 * time.Second},
	}
}

type bpRequest struct {
	Model                     string   `json:"model"`
	Prompt                    string   `json:"prompt"`
	Image                     []string `json:"image,omitempty"` // 참조/편집 이미지 (data URL)
	Size                      string   `json:"size,omitempty"`
	ResponseFormat            string   `json:"response_format"`
	Watermark                 bool     `json:"watermark"`
	SequentialImageGeneration string   `json:"sequential_image_generation,omitempty"`
}

type bpResponse struct {
	Data []struct {
		URL     string `json:"url"`
		B64JSON string `json:"b64_json"`
	} `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// GenerateImage는 BytePlus Seedream으로 이미지를 생성합니다.
func (c *BytePlus) GenerateImage(ctx context.Context, prompt string, refImages [][]byte, aspectRatio string) ([]byte, error) {
	if c.APIKey == "" {
		return nil, errors.New("BytePlus API 密钥未设置，请在设置中输入")
	}

	reqData := bpRequest{
		Model: c.Model,
		// 종횡비 파라미터가 무시되는 경우를 대비해 프롬프트 힌트도 함께 전달
		Prompt:                    prompt + "\n\n" + aspectHint(aspectRatio),
		Size:                      bpSizeFor(aspectRatio),
		ResponseFormat:            "b64_json",
		Watermark:                 false,
		SequentialImageGeneration: "disabled",
	}
	for _, img := range refImages {
		reqData.Image = append(reqData.Image,
			"data:image/png;base64,"+base64.StdEncoding.EncodeToString(img))
	}

	body, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("请求序列化失败: %w", err)
	}

	var lastErr error
	backoff := 2 * time.Second
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
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

// PLACEHOLDER_DOREQUEST

func (c *BytePlus) doRequest(ctx context.Context, body []byte) (img []byte, retryable bool, err error) {
	ep := c.endpoint
	if ep == "" {
		ep = bytePlusEndpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep, bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("网络错误: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, true, fmt.Errorf("响应读取失败: %w", err)
	}

	var parsed bpResponse
	_ = json.Unmarshal(respBytes, &parsed)

	if resp.StatusCode != http.StatusOK {
		retryable = resp.StatusCode == 429 || resp.StatusCode >= 500
		if parsed.Error != nil && parsed.Error.Message != "" {
			return nil, retryable, fmt.Errorf("BytePlus 错误 (%d): %s", resp.StatusCode, parsed.Error.Message)
		}
		return nil, retryable, fmt.Errorf("BytePlus 错误 (HTTP %d)", resp.StatusCode)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, true, fmt.Errorf("BytePlus 错误: %s", parsed.Error.Message)
	}
	if len(parsed.Data) == 0 {
		return nil, true, errors.New("响应中没有图片")
	}
	d := parsed.Data[0]
	if d.B64JSON != "" {
		data, err := base64.StdEncoding.DecodeString(d.B64JSON)
		if err != nil {
			return nil, false, fmt.Errorf("图片解码失败: %w", err)
		}
		return data, false, nil
	}
	if d.URL != "" {
		data, err := decodeDataOrDownloadContext(ctx, c.HTTP, d.URL)
		if err != nil {
			return nil, false, err
		}
		return data, false, nil
	}
	return nil, true, errors.New("响应中没有图片")
}

// ValidateKey는 BytePlus 키 형식을 확인합니다 (경량 검증 엔드포인트가 없음).
func (c *BytePlus) ValidateKey(_ context.Context) error {
	key := strings.TrimSpace(c.APIKey)
	if len(key) < 10 {
		return errors.New("API 密钥太短")
	}
	return nil
}

// bpSizeFor는 종횡비를 Seedream이 허용하는 픽셀 크기(WxH)로 변환합니다.
// 각 변은 [1280, 4096] 범위로 제한됩니다.
func bpSizeFor(aspectRatio string) string {
	const (
		minSide = 1280
		maxSide = 4096
	)
	w, h := parseAspect(aspectRatio)
	if w <= 0 || h <= 0 {
		return "2048x2048"
	}
	// 긴 변을 maxSide로 맞춘 뒤 짧은 변을 비율대로 계산하고 범위로 클램프
	var pw, ph int
	if w >= h {
		pw = maxSide
		ph = int(float64(maxSide) * float64(h) / float64(w))
	} else {
		ph = maxSide
		pw = int(float64(maxSide) * float64(w) / float64(h))
	}
	clamp := func(v int) int {
		if v < minSide {
			return minSide
		}
		if v > maxSide {
			return maxSide
		}
		return v
	}
	return strconv.Itoa(clamp(pw)) + "x" + strconv.Itoa(clamp(ph))
}

// parseAspect는 "W:H" 문자열을 정수 비율로 파싱합니다 (실패 시 0,0).
func parseAspect(aspectRatio string) (int, int) {
	a, b, ok := strings.Cut(strings.TrimSpace(aspectRatio), ":")
	if !ok {
		return 0, 0
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(a))
	h, err2 := strconv.Atoi(strings.TrimSpace(b))
	if err1 != nil || err2 != nil {
		return 0, 0
	}
	return w, h
}
