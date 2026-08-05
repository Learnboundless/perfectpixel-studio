package gen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// SingleAttemptImageProvider is used by durable paid-job orchestration. Unlike
// GenerateImage, it never repeats a POST when the first response is ambiguous.
type SingleAttemptImageProvider interface {
	GenerateImageOnce(ctx context.Context, prompt string, refImages [][]byte, aspectRatio string) ([]byte, error)
}

func (c *OpenAI) GenerateImageOnce(ctx context.Context, prompt string, refImages [][]byte, aspectRatio string) ([]byte, error) {
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
		req := openAIImageRequest{Model: c.Model, Prompt: fullPrompt, N: 1, Size: size, Quality: "medium"}
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
	imageBytes, _, err := c.doImageRequest(ctx, endpoint, contentType, body)
	return imageBytes, err
}

func (c *BytePlus) GenerateImageOnce(ctx context.Context, prompt string, refImages [][]byte, aspectRatio string) ([]byte, error) {
	if c.APIKey == "" {
		return nil, errors.New("BytePlus API 密钥未设置，请在设置中输入")
	}
	reqData := bpRequest{
		Model:                     c.Model,
		Prompt:                    prompt + "\n\n" + aspectHint(aspectRatio),
		Size:                      bpSizeFor(aspectRatio),
		ResponseFormat:            "b64_json",
		Watermark:                 false,
		SequentialImageGeneration: "disabled",
	}
	for _, imageBytes := range refImages {
		reqData.Image = append(reqData.Image, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(imageBytes))
	}
	body, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("请求序列化失败: %w", err)
	}
	imageBytes, _, err := c.doRequest(ctx, body)
	return imageBytes, err
}

func (c *OpenRouter) GenerateImageOnce(ctx context.Context, prompt string, refImages [][]byte, aspectRatio string) ([]byte, error) {
	if c.APIKey == "" {
		return nil, errors.New("OpenRouter API 密钥未设置，请在设置中输入")
	}
	parts := []orContentPart{{Type: "text", Text: prompt + "\n\n" + aspectHint(aspectRatio)}}
	for _, imageBytes := range refImages {
		parts = append(parts, orContentPart{Type: "image_url", ImageURL: &orImageURL{URL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes)}})
	}
	body, err := json.Marshal(orRequest{
		Model:      c.Model,
		Messages:   []orMessage{{Role: "user", Content: parts}},
		Modalities: []string{"image", "text"},
	})
	if err != nil {
		return nil, fmt.Errorf("请求序列化失败: %w", err)
	}
	imageBytes, _, err := c.doRequest(ctx, body)
	return imageBytes, err
}

func (c *Fal) GenerateImageOnce(ctx context.Context, prompt string, refImages [][]byte, aspectRatio string) ([]byte, error) {
	if c.APIKey == "" {
		return nil, errors.New("fal.ai API 密钥未设置，请在设置中输入")
	}
	reqData := falRequest{
		Prompt:       prompt + "\n\n" + aspectHint(aspectRatio),
		NumImages:    1,
		OutputFormat: "png",
		AspectRatio:  aspectRatio,
		SyncMode:     false,
	}
	for _, imageBytes := range refImages {
		reqData.ImageURLs = append(reqData.ImageURLs, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(imageBytes))
	}
	body, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("请求序列化失败: %w", err)
	}
	imageBytes, _, err := c.doRequest(ctx, c.endpoint(len(refImages) > 0), body)
	return imageBytes, err
}

func (c *Client) GenerateImageOnce(ctx context.Context, prompt string, refImages [][]byte, aspectRatio string) ([]byte, error) {
	if c.APIKey == "" {
		return nil, errors.New("API 密钥未设置，请在设置中输入 Gemini API 密钥")
	}
	parts := make([]genPart, 0, len(refImages)+1)
	for _, imageBytes := range refImages {
		parts = append(parts, genPart{InlineData: &inlineData{MimeType: "image/png", Data: base64.StdEncoding.EncodeToString(imageBytes)}})
	}
	parts = append(parts, genPart{Text: prompt})
	config := &genConfig{ResponseModalities: []string{"TEXT", "IMAGE"}}
	if aspectRatio != "" {
		imageCfg := &imageConfig{AspectRatio: aspectRatio}
		if strings.HasPrefix(c.Model, "gemini-3-pro") && aspectRatio != "1:1" {
			imageCfg.ImageSize = "2K"
		}
		config.ImageConfig = imageCfg
	}
	body, err := json.Marshal(genRequest{
		Contents:         []genContent{{Parts: parts}},
		GenerationConfig: config,
	})
	if err != nil {
		return nil, fmt.Errorf("请求序列化失败: %w", err)
	}
	imageBytes, _, err := c.doRequest(ctx, c.Model, body)
	return imageBytes, err
}
