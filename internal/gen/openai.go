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
	openAIImageGenerationEndpoint = "https://api.openai.com/v1/images/generations"
	openAIImageEditEndpoint       = "https://api.openai.com/v1/images/edits"
	openAIModelsEndpoint          = "https://api.openai.com/v1/models"
)

// OpenAI는 OpenAI Image API 클라이언트입니다.
type OpenAI struct {
	APIKey string
	Model  string
	HTTP   *http.Client
	Name   string

	generationEndpoint   string // 테스트용 오버라이드
	editEndpoint         string // 테스트용 오버라이드
	modelsEndpoint       string // 테스트용 오버라이드
	sizeFor              func(string) string
	responseFormat       string
	omitGenerationFormat bool
	omitEditFormat       bool
}

// NewOpenAI는 새 OpenAI 이미지 생성 클라이언트를 생성합니다.
func NewOpenAI(apiKey, model string) *OpenAI {
	if model == "" {
		model = DefaultModelFor(ProviderOpenAI)
	}
	return &OpenAI{
		APIKey: apiKey,
		Model:  model,
		HTTP:   &http.Client{Timeout: 300 * time.Second},
		Name:   "OpenAI",
	}
}

func (c *OpenAI) providerName() string {
	if c.Name != "" {
		return c.Name
	}
	return "OpenAI"
}

func (c *OpenAI) imageSizeFor(aspectRatio string) string {
	if c.sizeFor != nil {
		return c.sizeFor(aspectRatio)
	}
	return openAISizeFor(aspectRatio)
}

type openAIImageRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	OutputFormat   string `json:"output_format,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

type openAIImageResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
		URL     string `json:"url"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error"`
}

// GenerateImage는 OpenAI GPT Image 모델로 이미지를 생성하거나 참조 이미지를 편집합니다.
func (c *OpenAI) GenerateImage(ctx context.Context, prompt string, refImages [][]byte, aspectRatio string) ([]byte, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("%s API 密钥未设置，请在设置中输入。", c.providerName())
	}

	fullPrompt := prompt + "\n\n" + aspectHint(aspectRatio)
	size := c.imageSizeFor(aspectRatio)

	var body []byte
	var contentType string
	var endpoint string
	var err error
	if len(refImages) == 0 {
		endpoint = c.generationEndpoint
		if endpoint == "" {
			endpoint = openAIImageGenerationEndpoint
		}
		req := openAIImageRequest{
			Model:   c.Model,
			Prompt:  fullPrompt,
			N:       1,
			Size:    size,
			Quality: "medium",
		}
		if !c.omitGenerationFormat {
			if c.responseFormat != "" {
				req.ResponseFormat = c.responseFormat
			} else {
				req.OutputFormat = "png"
			}
		}
		body, err = json.Marshal(req)
		contentType = "application/json"
	} else {
		endpoint = c.editEndpoint
		if endpoint == "" {
			endpoint = openAIImageEditEndpoint
		}
		body, contentType, err = c.buildEditBody(fullPrompt, refImages, size)
	}
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
		img, retryable, err := c.doImageRequest(ctx, endpoint, contentType, body)
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

// GenerateImageWithMask edits the first reference image inside the transparent
// area of mask. Additional references remain available as visual guidance.
func (c *OpenAI) GenerateImageWithMask(ctx context.Context, prompt string, refImages [][]byte, mask []byte, aspectRatio string) ([]byte, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("%s API 密钥未设置，请在设置中输入。", c.providerName())
	}
	if len(refImages) == 0 || len(mask) == 0 {
		return nil, errors.New("局部重绘需要原图和遮罩。")
	}
	endpoint := c.editEndpoint
	if endpoint == "" {
		endpoint = openAIImageEditEndpoint
	}
	fullPrompt := prompt + "\n\n" + aspectHint(aspectRatio)
	body, contentType, err := c.buildEditBodyWithMask(fullPrompt, refImages, mask, c.imageSizeFor(aspectRatio))
	if err != nil {
		return nil, fmt.Errorf("请求序列化失败: %w", err)
	}
	img, _, err := c.doImageRequest(ctx, endpoint, contentType, body)
	return img, err
}

func (c *OpenAI) buildEditBody(prompt string, refImages [][]byte, size string) ([]byte, string, error) {
	return c.buildEditBodyWithMask(prompt, refImages, nil, size)
}

func (c *OpenAI) buildEditBodyWithMask(prompt string, refImages [][]byte, mask []byte, size string) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fields := map[string]string{
		"model":   c.Model,
		"prompt":  prompt,
		"n":       "1",
		"size":    size,
		"quality": "medium",
	}
	if !c.omitEditFormat {
		if c.responseFormat != "" {
			fields["response_format"] = c.responseFormat
		} else {
			fields["output_format"] = "png"
		}
	}
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return nil, "", err
		}
	}
	for i, img := range refImages {
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image[]"; filename="reference-%d.png"`, i+1))
		h.Set("Content-Type", "image/png")
		part, err := w.CreatePart(h)
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(img); err != nil {
			return nil, "", err
		}
	}
	if len(mask) > 0 {
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", `form-data; name="mask"; filename="mask.png"`)
		h.Set("Content-Type", "image/png")
		part, err := w.CreatePart(h)
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(mask); err != nil {
			return nil, "", err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

func (c *OpenAI) doImageRequest(ctx context.Context, endpoint, contentType string, body []byte) (img []byte, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", contentType)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("网络错误: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, true, fmt.Errorf("读取响应失败: %w", err)
	}

	var parsed openAIImageResponse
	_ = json.Unmarshal(respBytes, &parsed)

	if resp.StatusCode != http.StatusOK {
		retryable = resp.StatusCode == 429 || resp.StatusCode >= 500
		if parsed.Error != nil && parsed.Error.Message != "" {
			return nil, retryable, fmt.Errorf("%s 错误（%d）：%s", c.providerName(), resp.StatusCode, parsed.Error.Message)
		}
		return nil, retryable, fmt.Errorf("%s 错误（HTTP %d）", c.providerName(), resp.StatusCode)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, true, fmt.Errorf("%s 错误：%s", c.providerName(), parsed.Error.Message)
	}
	if len(parsed.Data) == 0 {
		return nil, true, errors.New("响应中没有图片。")
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
	return nil, true, errors.New("响应中没有图片。")
}

// ValidateKey는 OpenAI 키 유효성을 확인합니다.
func (c *OpenAI) ValidateKey(ctx context.Context) error {
	key := strings.TrimSpace(c.APIKey)
	if !strings.HasPrefix(key, "sk-") || len(key) < 20 {
		return errors.New("OpenAI API 密钥格式不正确。")
	}
	ep := c.modelsEndpoint
	if ep == "" {
		ep = openAIModelsEndpoint + "/" + c.Model
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ep, nil)
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
		return errors.New("API 密钥无效，或当前账号没有此模型权限。")
	}
	if resp.StatusCode == 404 {
		return errors.New("当前 OpenAI 账号中找不到 GPT Image 模型。")
	}
	return fmt.Errorf("密钥验证失败（HTTP %d）", resp.StatusCode)
}

// openAISizeFor는 GPT Image 2가 허용하는 WxH 해상도로 종횡비를 변환합니다.
func openAISizeFor(aspectRatio string) string {
	w, h := parseAspect(aspectRatio)
	if w <= 0 || h <= 0 {
		return "1024x1024"
	}
	if w == h {
		return "1024x1024"
	}
	const maxEdge = 1792
	var pw, ph int
	if w > h {
		pw = maxEdge
		ph = int(float64(maxEdge) * float64(h) / float64(w))
	} else {
		ph = maxEdge
		pw = int(float64(maxEdge) * float64(w) / float64(h))
	}
	round16 := func(v int) int {
		if v < 640 {
			v = 640
		}
		return (v / 16) * 16
	}
	return strconv.Itoa(round16(pw)) + "x" + strconv.Itoa(round16(ph))
}
