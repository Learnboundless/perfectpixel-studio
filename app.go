package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"perfectpixel/internal/config"
	"perfectpixel/internal/gen"
	"perfectpixel/internal/sprite"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	xdraw "golang.org/x/image/draw"
)

const (
	maxAnimationFrames = 120

	pipelineImageStrip    = "image_strip"
	pipelineVideoToFrames = "video_to_frames"

	videoReferenceModeAuto              = "auto"
	videoReferenceModeNone              = "none"
	videoReferenceModeIdentityReference = "identity_reference"
	videoReferenceModeFirstFrame        = "first_frame"
	videoReferenceModeStartEndFrame     = "start_end_frame"

	videoPromptLanguageAuto    = "auto"
	videoPromptLanguageChinese = "zh"
	videoPromptLanguageEnglish = "en"

	videoShadowModeAvoid        = "avoid"
	videoShadowModeAllowContact = "allow_contact"
)

// App은 Wails 애플리케이션 본체입니다.
type App struct {
	ctx context.Context

	toolchainStore      *ToolchainStore
	toolchainJobs       *GenerationJobManager
	toolchainErr        error
	animationVersions   *AnimationVersionStore
	animationVersionErr error

	genMu      sync.Mutex
	genCancels map[int]context.CancelFunc // 진행 중인 생성 작업들의 취소 함수 (병렬 배치 지원)
	genSeq     int
	pendingMu  sync.Mutex
}

// NewApp은 새 App 인스턴스를 생성합니다.
func NewApp() *App {
	store, err := NewToolchainStore()
	versions, versionErr := NewAnimationVersionStore()
	return &App{toolchainStore: store, toolchainErr: err, animationVersions: versions, animationVersionErr: versionErr}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if err := a.initToolchain(ctx); err != nil {
		a.emit("toolchain-error", map[string]any{"message": err.Error()})
	}
	if a.animationVersionErr != nil {
		a.emit("animation-version-error", map[string]any{"message": a.animationVersionErr.Error()})
	}
}

func (a *App) shutdown(context.Context) {
	if a.toolchainJobs != nil {
		a.toolchainJobs.Close()
	}
}

// imageProvider returns the independently selected image generation client.
func (a *App) imageProvider() (gen.Provider, error) {
	s := config.Load()
	provider := s.ImageProvider
	cfg := s.Cfg(provider)
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("%s API 密钥未设置，请在设置中输入。", gen.ProviderLabel(provider))
	}
	if provider == gen.ProviderZetaTechs {
		return gen.NewZetaTechsWithBaseURL(cfg.APIKey, cfg.Model, cfg.BaseURL), nil
	}
	return gen.New(provider, cfg.APIKey, cfg.Model)
}

// videoProvider returns the independently selected asynchronous video client.
func (a *App) videoProvider() (gen.VideoProvider, error) {
	s := config.Load()
	provider := s.VideoProvider
	cfg := s.Cfg(provider)
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("%s API 密钥未设置，请在设置中输入。", gen.ProviderLabel(provider))
	}
	var p gen.Provider
	var err error
	if provider == gen.ProviderZetaTechs {
		p = gen.NewZetaTechsWithBaseURL(cfg.APIKey, cfg.Model, cfg.BaseURL)
	} else {
		p, err = gen.New(provider, cfg.APIKey, cfg.VideoModel)
		if err != nil {
			return nil, err
		}
	}
	vp, ok := p.(gen.VideoProvider)
	if !ok {
		return nil, fmt.Errorf("%s 不支持视频生成。", gen.ProviderLabel(provider))
	}
	return vp, nil
}

// provider remains as a compatibility alias for image-only call sites.
func (a *App) provider() (gen.Provider, error) {
	return a.imageProvider()
}

func (a *App) emit(event string, data any) {
	if a == nil || a.ctx == nil || a.ctx.Value("events") == nil {
		return
	}
	runtime.EventsEmit(a.ctx, event, data)
}

func (a *App) emitProgress(phase, state, message string, percent, current, total int) {
	data := map[string]any{
		"phase":   phase,
		"message": message,
	}
	if state != "" {
		data["state"] = state
	}
	if percent >= 0 {
		if percent > 100 {
			percent = 100
		}
		data["percent"] = percent
	}
	if current > 0 {
		data["current"] = current
	}
	if total > 0 {
		data["total"] = total
	}
	a.emit("progress", data)
}

// genContext는 취소 가능한 생성용 컨텍스트를 만듭니다.
// 병렬 배치 생성에서 여러 컨텍스트가 동시에 살아 있을 수 있으므로 각각을 추적하고,
// 호출자가 완료 시 해제할 수 있도록 release 함수를 함께 반환합니다.
func (a *App) genContext() (context.Context, func()) {
	a.genMu.Lock()
	defer a.genMu.Unlock()
	base := a.ctx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithCancel(base)
	if a.genCancels == nil {
		a.genCancels = make(map[int]context.CancelFunc)
	}
	id := a.genSeq
	a.genSeq++
	a.genCancels[id] = cancel
	release := func() {
		a.genMu.Lock()
		defer a.genMu.Unlock()
		if c, ok := a.genCancels[id]; ok {
			c()
			delete(a.genCancels, id)
		}
	}
	return ctx, release
}

// CancelGeneration은 진행 중인 모든 생성 작업을 취소합니다.
func (a *App) CancelGeneration() {
	a.genMu.Lock()
	defer a.genMu.Unlock()
	for id, cancel := range a.genCancels {
		cancel()
		delete(a.genCancels, id)
	}
}

// friendlyErr는 취소 오류를 사용자 친화적 메시지로 바꿉니다.
func friendlyErr(err error) error {
	if errors.Is(err, context.Canceled) {
		return errors.New("生成已取消。")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("图片生成响应超时，请稍后再试。")
	}
	return err
}

// ---------- 설정 ----------

// ProviderInfo는 프로바이더별 설정 상태입니다.
type ProviderInfo struct {
	HasKey                 bool                                `json:"hasKey"`
	KeyPreview             string                              `json:"keyPreview"`
	HasTextKey             bool                                `json:"hasTextKey"`
	TextKeyPreview         string                              `json:"textKeyPreview"`
	TextUsesProviderKey    bool                                `json:"textUsesProviderKey"`
	Model                  string                              `json:"model"`
	Models                 []string                            `json:"models"` // 선택 가능한 모델 목록 (최신 모델이 맨 앞)
	ModelListSource        string                              `json:"modelListSource,omitempty"`
	ModelListError         string                              `json:"modelListError,omitempty"`
	VideoModel             string                              `json:"videoModel"`
	VideoModels            []string                            `json:"videoModels"`
	VideoModelCapability   gen.VideoModelCapability            `json:"videoModelCapability"`
	VideoModelCapabilities map[string]gen.VideoModelCapability `json:"videoModelCapabilities"`
	VideoModelShowcases    map[string]gen.VideoModelShowcase   `json:"videoModelShowcases,omitempty"`
	SupportsVideo          bool                                `json:"supportsVideo"`
	SupportsImage          bool                                `json:"supportsImage"`
	TextModel              string                              `json:"textModel"`
	TextModels             []string                            `json:"textModels"`
	SupportsText           bool                                `json:"supportsText"`
	VisionModel            string                              `json:"visionModel"`
	VisionModels           []string                            `json:"visionModels"`
	SupportsVision         bool                                `json:"supportsVision"`
}

// SettingsInfo는 프론트엔드에 노출되는 설정 상태입니다.
type SettingsInfo struct {
	Provider                  string                  `json:"provider"`
	ImageProvider             string                  `json:"imageProvider"`
	VideoProvider             string                  `json:"videoProvider"`
	TextProvider              string                  `json:"textProvider"`
	VisionProvider            string                  `json:"visionProvider"`
	AnimationPipeline         string                  `json:"animationPipeline"`
	VideoSeconds              int                     `json:"videoSeconds"`
	VideoFPS                  int                     `json:"videoFps"`
	BackgroundRemovalMode     string                  `json:"backgroundRemovalMode"`
	AnimationVersionWarningGB int                     `json:"animationVersionWarningGb"`
	Providers                 map[string]ProviderInfo `json:"providers"`
}

func keyPreview(key string) string {
	if len(key) > 8 {
		return key[:4] + "····" + key[len(key)-4:]
	}
	return ""
}

// GetSettings는 현재 설정 상태를 반환합니다 (키 원문은 노출하지 않음).
func (a *App) GetSettings() SettingsInfo {
	s := config.Load()
	zetaImageModels, zetaVideoModels, zetaModelStatus := a.zetaTechsDynamicModelLists(s)
	volcengineVideoModels, volcengineModelStatus := a.volcengineDynamicVideoModels(s)
	info := SettingsInfo{
		Provider:                  s.ImageProvider,
		ImageProvider:             s.ImageProvider,
		VideoProvider:             s.VideoProvider,
		TextProvider:              s.TextProvider,
		VisionProvider:            s.VisionProvider,
		AnimationPipeline:         s.AnimationPipeline,
		VideoSeconds:              s.VideoSeconds,
		VideoFPS:                  s.VideoFPS,
		BackgroundRemovalMode:     s.BackgroundRemovalMode,
		AnimationVersionWarningGB: s.AnimationVersionWarningGB,
		Providers:                 map[string]ProviderInfo{},
	}
	for _, p := range gen.SupportedProviders {
		cfg := s.Cfg(p)
		model := cfg.Model
		if model == "" {
			model = gen.DefaultModelFor(p)
		}
		configuredVideoModel := cfg.VideoModel
		videoModel := configuredVideoModel
		if videoModel == "" {
			videoModel = gen.DefaultVideoModelFor(p)
		}
		models := gen.ModelsFor(p)
		videoModels := gen.FilterVerifiedVideoModels(p, gen.VideoModelsFor(p))
		textModel := strings.TrimSpace(cfg.TextModel)
		if textModel == "" {
			textModel = gen.DefaultTextModelFor(p)
		}
		textAPIKey := strings.TrimSpace(cfg.TextAPIKey)
		textUsesProviderKey := textAPIKey == ""
		if textAPIKey == "" {
			textAPIKey = cfg.APIKey
		}
		visionModel := strings.TrimSpace(cfg.VisionModel)
		if visionModel == "" {
			visionModel = gen.DefaultVisionModelFor(p)
		}
		visionModels := gen.VisionModelsFor(p)
		modelListSource := "static"
		modelListError := ""
		if p == gen.ProviderZetaTechs {
			models = gen.MergeModelLists(models, zetaImageModels)
			if zetaModelStatus.Source == "dynamic" {
				videoModels = gen.FilterVerifiedVideoModels(p, zetaVideoModels)
			}
			if configuredVideoModel == "" && len(videoModels) > 0 && !stringInSlice(videoModels, videoModel) {
				videoModel = videoModels[0]
			}
			modelListSource = zetaModelStatus.Source
			modelListError = zetaModelStatus.Error
		} else if p == gen.ProviderVolcengine {
			if volcengineModelStatus.Source == "dynamic" {
				videoModels = gen.FilterVerifiedVideoModels(p, volcengineVideoModels)
			}
			if configuredVideoModel == "" && len(videoModels) > 0 && !stringInSlice(videoModels, videoModel) {
				videoModel = videoModels[0]
			}
			modelListSource = volcengineModelStatus.Source
			modelListError = volcengineModelStatus.Error
		}
		info.Providers[p] = ProviderInfo{
			HasKey:                 cfg.APIKey != "",
			KeyPreview:             keyPreview(cfg.APIKey),
			HasTextKey:             textAPIKey != "",
			TextKeyPreview:         keyPreview(textAPIKey),
			TextUsesProviderKey:    textUsesProviderKey && textAPIKey != "",
			Model:                  model,
			Models:                 models,
			ModelListSource:        modelListSource,
			ModelListError:         modelListError,
			VideoModel:             videoModel,
			VideoModels:            videoModels,
			VideoModelCapability:   gen.VideoModelCapabilityFor(videoModel),
			VideoModelCapabilities: gen.VideoModelCapabilitiesFor(videoModels),
			VideoModelShowcases:    gen.VideoModelShowcasesForModels(p, videoModels),
			SupportsVideo:          len(videoModels) > 0,
			SupportsImage:          len(models) > 0,
			TextModel:              textModel,
			TextModels:             gen.TextModelsFor(p),
			SupportsText:           gen.SupportsText(p),
			VisionModel:            visionModel,
			VisionModels:           visionModels,
			SupportsVision:         gen.SupportsVision(p),
		}
	}
	return info
}

func stringInSlice(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

type modelListStatus struct {
	Source string
	Error  string
}

func (a *App) zetaTechsDynamicModelLists(s config.Settings) (imageModels, videoModels []string, status modelListStatus) {
	status.Source = "static"
	cfg := s.ZetaTechs
	if strings.TrimSpace(cfg.APIKey) == "" {
		status.Error = "ZetaTechs 未配置密钥，当前显示内置模型兜底列表。"
		return nil, nil, status
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	client := gen.NewZetaTechsWithBaseURL(cfg.APIKey, cfg.Model, cfg.BaseURL)
	models, err := client.ListModels(ctx)
	if err != nil {
		status.Error = fmt.Sprintf("无法刷新 ZetaTechs 模型列表：%v", err)
		return nil, nil, status
	}
	status.Source = "dynamic"
	return gen.ZetaTechsImageModelsFrom(models), gen.ZetaTechsVideoModelsFrom(models), status
}

func (a *App) volcengineDynamicVideoModels(s config.Settings) (videoModels []string, status modelListStatus) {
	status.Source = "static"
	cfg := s.Volcengine
	if strings.TrimSpace(cfg.APIKey) == "" {
		status.Error = "火山方舟未配置密钥，当前显示内置模型兜底列表。"
		return nil, status
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	models, err := gen.NewVolcengine(cfg.APIKey, cfg.VideoModel).ListVideoModels(ctx)
	if err != nil {
		status.Error = fmt.Sprintf("无法刷新火山方舟模型列表：%v", err)
		return nil, status
	}
	status.Source = "dynamic"
	return models, status
}

// SaveProviderKey는 특정 프로바이더의 API 키를 검증 후 저장합니다.
func (a *App) SaveProviderKey(provider, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("请输入 API 密钥。")
	}
	baseURL := ""
	if provider == gen.ProviderZetaTechs {
		parsedKey, parsedURL, ok, err := parseZetaTechsConnection(key)
		if err != nil {
			return err
		}
		if ok {
			key = parsedKey
			baseURL = parsedURL
		} else {
			s := config.Load()
			baseURL = s.ZetaTechs.BaseURL
		}
	}
	c, err := newProviderForSettings(provider, key, "", baseURL)
	if err != nil {
		return err
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := c.ValidateKey(ctx); err != nil {
		return err
	}
	s := config.Load()
	cfg := s.Cfg(provider)
	cfg.APIKey = key
	if provider == gen.ProviderZetaTechs && baseURL != "" {
		cfg.BaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	}
	return config.Save(s)
}

// SaveProviderTextKey saves an optional key dedicated to prompt rewriting.
// When it is not configured, the text client reuses the provider's primary key.
func (a *App) SaveProviderTextKey(provider, key string) error {
	if !gen.SupportsText(provider) {
		return fmt.Errorf("%s 暂不支持文本改写。", gen.ProviderLabel(provider))
	}
	key = strings.TrimSpace(key)
	s := config.Load()
	cfg := s.Cfg(provider)
	if key == "" {
		cfg.TextAPIKey = ""
		return config.Save(s)
	}
	baseURL := cfg.BaseURL
	if provider == gen.ProviderZetaTechs {
		parsedKey, parsedURL, ok, err := parseZetaTechsConnection(key)
		if err != nil {
			return err
		}
		if ok {
			key = parsedKey
			baseURL = parsedURL
		}
	}
	client, err := gen.NewTextClient(provider, key, cfg.TextModel, baseURL)
	if err != nil {
		return err
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if _, err := client.ListModels(ctx); err != nil {
		return err
	}
	cfg.TextAPIKey = key
	if provider == gen.ProviderZetaTechs && baseURL != "" {
		cfg.BaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	}
	return config.Save(s)
}

type zetaTechsConnection struct {
	Type string `json:"_type"`
	Key  string `json:"key"`
	URL  string `json:"url"`
}

func parseZetaTechsConnection(input string) (key, baseURL string, ok bool, err error) {
	if !strings.HasPrefix(strings.TrimSpace(input), "{") {
		return "", "", false, nil
	}
	var conn zetaTechsConnection
	if err := json.Unmarshal([]byte(input), &conn); err != nil {
		return "", "", false, errors.New("ZetaTechs 连接 JSON 格式不正确。")
	}
	if conn.Key == "" {
		return "", "", false, errors.New("ZetaTechs 连接 JSON 中缺少 key。")
	}
	return strings.TrimSpace(conn.Key), strings.TrimRight(strings.TrimSpace(conn.URL), "/"), true, nil
}

func newProviderForSettings(provider, apiKey, model, baseURL string) (gen.Provider, error) {
	if provider == gen.ProviderZetaTechs {
		return gen.NewZetaTechsWithBaseURL(apiKey, model, baseURL), nil
	}
	return gen.New(provider, apiKey, model)
}

// SaveProviderModel은 프로바이더의 이미지 모델을 변경합니다 (빈 값이면 기본 모델로 복원).
func (a *App) SaveProviderModel(provider, model string) error {
	switch provider {
	case gen.ProviderGemini, gen.ProviderOpenAI, gen.ProviderOpenRouter, gen.ProviderFal, gen.ProviderBytePlus, gen.ProviderZetaTechs:
	default:
		return fmt.Errorf("不支持的提供方：%s", provider)
	}
	model = strings.TrimSpace(model)
	if model == gen.DefaultModelFor(provider) {
		model = "" // 기본 모델은 빈 값으로 저장해 향후 기본값 변경을 따라가게 함
	}
	s := config.Load()
	s.Cfg(provider).Model = model
	return config.Save(s)
}

// SaveProviderVideoModel은 프로바이더의 비디오 모델을 변경합니다.
func (a *App) SaveProviderVideoModel(provider, model string) error {
	switch provider {
	case gen.ProviderZetaTechs, gen.ProviderVolcengine:
	default:
		return fmt.Errorf("不支持的提供方：%s", provider)
	}
	model = strings.TrimSpace(model)
	if model == gen.DefaultVideoModelFor(provider) {
		model = ""
	}
	s := config.Load()
	s.Cfg(provider).VideoModel = model
	return config.Save(s)
}

func (a *App) SaveProviderTextModel(provider, model string) error {
	if !gen.SupportsText(provider) {
		return fmt.Errorf("%s 暂不支持文本改写。", gen.ProviderLabel(provider))
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return errors.New("请输入文本模型 ID。")
	}
	s := config.Load()
	s.Cfg(provider).TextModel = model
	return config.Save(s)
}

// SaveProviderVisionModel stores the independently selected multimodal model.
func (a *App) SaveProviderVisionModel(provider, model string) error {
	if !gen.SupportsVision(provider) {
		return fmt.Errorf("%s 暂不支持图片理解。", gen.ProviderLabel(provider))
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return errors.New("请输入视觉模型 ID。")
	}
	s := config.Load()
	s.Cfg(provider).VisionModel = model
	return config.Save(s)
}

// SaveAnimationPipeline은 상태 생성에 사용할 기본 애니메이션 파이프라인을 저장합니다.
func (a *App) SaveAnimationPipeline(pipeline string) error {
	pipeline = strings.TrimSpace(pipeline)
	switch pipeline {
	case pipelineImageStrip, pipelineVideoToFrames:
	default:
		return fmt.Errorf("不支持的动画生成管线：%s", pipeline)
	}
	s := config.Load()
	s.AnimationPipeline = pipeline
	return config.Save(s)
}

func (a *App) SaveBackgroundRemovalMode(mode string) error {
	mode = normalizeBackgroundRemovalMode(mode)
	s := config.Load()
	s.BackgroundRemovalMode = mode
	return config.Save(s)
}

func (a *App) SaveAnimationVersionWarningGB(gigabytes int) error {
	if gigabytes < 1 || gigabytes > 1024 {
		return errors.New("版本库容量提醒必须在 1 到 1024 GB 之间。")
	}
	s := config.Load()
	s.AnimationVersionWarningGB = gigabytes
	return config.Save(s)
}

// SaveVideoDefaults는 비디오 생성 기본 시간을 저장합니다.
func (a *App) SaveVideoDefaults(seconds, fps int) error {
	if seconds < 1 || seconds > 15 {
		return errors.New("视频长度必须在 1 到 15 秒之间。")
	}
	if fps < 1 || fps > 30 {
		return errors.New("视频 FPS 必须在 1 到 30 之间。")
	}
	s := config.Load()
	s.VideoSeconds = seconds
	s.VideoFPS = fps
	return config.Save(s)
}

// SetProvider는 활성 프로바이더를 변경합니다.
func (a *App) SetProvider(provider string) error {
	return a.SetImageProvider(provider)
}

func (a *App) SetImageProvider(provider string) error {
	switch provider {
	case gen.ProviderGemini, gen.ProviderOpenAI, gen.ProviderOpenRouter, gen.ProviderFal, gen.ProviderBytePlus, gen.ProviderZetaTechs:
	default:
		return fmt.Errorf("不支持的提供方：%s", provider)
	}
	s := config.Load()
	s.Provider = provider
	s.ImageProvider = provider
	return config.Save(s)
}

func (a *App) SetVideoProvider(provider string) error {
	switch provider {
	case gen.ProviderZetaTechs, gen.ProviderVolcengine:
	default:
		return fmt.Errorf("不支持的视频提供方：%s", provider)
	}
	s := config.Load()
	s.VideoProvider = provider
	return config.Save(s)
}

func (a *App) SetTextProvider(provider string) error {
	if !gen.SupportsText(provider) {
		return fmt.Errorf("不支持的文本提供方：%s", provider)
	}
	s := config.Load()
	s.TextProvider = provider
	return config.Save(s)
}

func (a *App) SetVisionProvider(provider string) error {
	if !gen.SupportsVision(provider) {
		return fmt.Errorf("不支持的视觉提供方：%s", provider)
	}
	s := config.Load()
	s.VisionProvider = provider
	return config.Save(s)
}

// ---------- 세션 저장/복원 ----------

// SaveSession은 현재 작업 상태(JSON)를 디스크에 저장합니다 (앱 재시작 시 복원용).
func (a *App) SaveSession(data string) error {
	path, err := config.SessionPath()
	if err != nil {
		return err
	}
	compacted, _, err := compactSessionJSON([]byte(data))
	if err != nil {
		return fmt.Errorf("会话数据无效: %w", err)
	}
	return atomicWritePrivate(path, compacted)
}

// LoadSession은 저장된 작업 세션 JSON을 반환합니다 (없으면 빈 문자열).
func (a *App) LoadSession() string {
	path, err := config.SessionPath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if migrated, changed, migrateErr := a.migrateSessionToV5(path, data); migrateErr == nil && changed {
		if err := atomicWritePrivate(path, migrated); err == nil {
			data = migrated
			releaseLargeWorkingSet()
		}
	}
	if compacted, changed, err := compactSessionJSON(data); err == nil && changed {
		if err := atomicWritePrivate(path, compacted); err == nil {
			data = compacted
			releaseLargeWorkingSet()
		}
	}
	return string(data)
}

// ClearSession은 저장된 작업 세션을 삭제합니다.
func (a *App) ClearSession() error {
	path, err := config.SessionPath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ---------- 이미지 입출력 헬퍼 ----------

var dataURLRe = regexp.MustCompile(`^data:image/[a-zA-Z+.-]+;base64,`)

func decodeDataURL(dataURL string) ([]byte, error) {
	m := dataURLRe.FindString(dataURL)
	if m == "" {
		return nil, errors.New("图片数据格式不正确。")
	}
	return base64.StdEncoding.DecodeString(dataURL[len(m):])
}

func pngDataURL(img image.Image) (string, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func decodeImage(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("图片解码失败: %w", err)
	}
	return img, nil
}

// PickImage는 파일 선택 대화상자를 열고 선택된 이미지를 dataURL로 반환합니다.
func (a *App) PickImage() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择基础图片",
		Filters: []runtime.FileFilter{
			{DisplayName: "图片 (*.png;*.jpg;*.jpeg;*.webp)", Pattern: "*.png;*.jpg;*.jpeg;*.webp"},
		},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil // 취소
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}
	img, err := decodeImage(data)
	if err != nil {
		return "", err
	}
	return pngDataURL(img)
}

// ---------- 생성 파이프라인 ----------

// GenerateCharacterArgs는 텍스트 → 베이스 캐릭터 생성 요청입니다.
type GenerateCharacterArgs struct {
	Description string `json:"description"`
	StyleKey    string `json:"styleKey"`
	StyleCustom string `json:"styleCustom"`
}

type RewritePromptArgs struct {
	Text string `json:"text"`
}

const complianceRewriteSystemPrompt = `你是游戏素材的合规描述编辑。请把用户输入改写为原创、中性、可直接用于生成游戏角色的中文视觉描述。
必须删除作品名、角色名、品牌名、真人姓名、艺术家姓名、受保护的独特标识和“模仿某作品/某作者风格”的要求。
保留通用且可观察的视觉属性，例如轮廓、服装类别、材质、主色块、比例、装备类别和气质。
把所有专有名词视为不可推断含义的占位符；不得依据训练知识回忆或补全原作外观、型号、配色、标志、道具或世界观设定。
只保留输入中除专有名词之外明确写出的属性；属性不足时采用通用原创设计，不要补回任何已知角色的标志性细节。
不要声称用户拥有授权，不要解释如何规避审核，不要加入“类似于”“致敬”“同款”等影射措辞。
只输出一段改写后的角色描述，不要标题、列表、引号、Markdown 或解释。`

const characterPromptOptimizationSystemPrompt = `你是资深游戏角色生成提示词编辑。请把用户输入优化为清晰、具体、稳定、可直接用于生成基础角色图的一段描述。
完整保留用户明确写出的角色身份、物种、性别表达、年龄感、体型、面部、发型、服装、配色、材质、装备和气质；不得擅自删除、替换或反转这些设定。
优先按“主体与轮廓、外观特征、服装与装备、主辅色块、材质与气质”的顺序组织内容，让图片模型容易建立稳定的角色身份。
可以把模糊形容词改成可观察的视觉属性，但不得凭空增加重要道具、标志、文字、复杂背景、额外角色或故事设定。
不要加入动画分镜、多帧动作、镜头运动、海报排版或水印要求；基础角色应保持完整身体、轮廓清楚、适合后续动画制作。
使用与输入相同的主要语言。只输出优化后的角色描述，不要标题、列表、引号、Markdown、解释或修改说明。`

// RewriteRiskyPrompt uses the independently configured text LLM to turn a
// potentially protected identity description into an original neutral brief.
func (a *App) RewriteRiskyPrompt(args RewritePromptArgs) (string, error) {
	return a.runPromptTextTask(args, complianceRewriteSystemPrompt, "prompt_rewrite", "正在使用文本模型分析并改写描述...", "prompt_rewrite_done", "提示词改写完成")
}

// OptimizeCharacterPrompt improves any character brief without requiring an
// IP-risk match and without changing the user's explicit character design.
func (a *App) OptimizeCharacterPrompt(args RewritePromptArgs) (string, error) {
	return a.runPromptTextTask(args, characterPromptOptimizationSystemPrompt, "prompt_optimize", "正在使用文本模型优化角色描述...", "prompt_optimize_done", "角色描述优化完成")
}

func (a *App) runPromptTextTask(args RewritePromptArgs, systemPrompt, phase, message, donePhase, doneMessage string) (string, error) {
	input := strings.TrimSpace(args.Text)
	if input == "" {
		return "", errors.New("请输入需要改写的角色描述。")
	}
	if len([]rune(input)) > 4000 {
		return "", errors.New("角色描述过长，请控制在 4000 个字符以内。")
	}
	client, err := configuredTextClient()
	if err != nil {
		return "", err
	}

	defer a.emitProgress("idle", "", "", -1, 0, 0)
	a.emitProgress(phase, "", message, 15, 1, 2)
	genCtx, releaseGen := a.genContext()
	defer releaseGen()
	rewritten, err := client.Complete(genCtx, systemPrompt, input)
	if err != nil {
		return "", friendlyErr(err)
	}
	a.emitProgress(donePhase, "", doneMessage, 96, 2, 2)
	rewritten = cleanPromptRewrite(rewritten)
	if rewritten == "" {
		return "", errors.New("文本模型没有返回可用的改写结果。")
	}
	return rewritten, nil
}

func configuredTextClient() (*gen.TextClient, error) {
	settings := config.Load()
	provider := strings.TrimSpace(settings.TextProvider)
	if provider == "" {
		provider = gen.ProviderVolcengine
	}
	cfg := settings.Cfg(provider)
	apiKey := strings.TrimSpace(cfg.TextAPIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(cfg.APIKey)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("%s 文本 LLM 密钥未设置，请在设置的“文本改写模型”中配置。", gen.ProviderLabel(provider))
	}
	client, err := gen.NewTextClient(provider, apiKey, cfg.TextModel, cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func cleanPromptRewrite(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "```") {
		if firstLine := strings.IndexByte(value, '\n'); firstLine >= 0 {
			value = value[firstLine+1:]
		}
		value = strings.TrimSuffix(strings.TrimSpace(value), "```")
	}
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"'“”‘’")
	return strings.TrimSpace(value)
}

// GenerateCharacter는 설명만으로 베이스 캐릭터 이미지를 생성합니다.
func (a *App) GenerateCharacter(args GenerateCharacterArgs) (string, error) {
	if strings.TrimSpace(args.Description) == "" {
		return "", errors.New("请输入角色描述。")
	}
	style := sprite.ResolveStyle(args.StyleKey, args.StyleCustom)
	prompt := sprite.BuildCharacterPrompt(args.Description, style)

	p, err := a.provider()
	if err != nil {
		return "", err
	}
	// 종료 시 진행 표시 제거
	defer a.emitProgress("idle", "", "", -1, 0, 0)

	a.emitProgress("character", "", "正在生成角色...", 8, 1, 4)
	genCtx, releaseGen := a.genContext()
	defer releaseGen()
	raw, err := p.GenerateImage(genCtx, prompt, nil, "1:1")
	if err != nil {
		return "", friendlyErr(err)
	}
	a.emitProgress("character_decode", "", "正在解码角色图片...", 72, 2, 4)
	img, err := decodeImage(raw)
	if err != nil {
		return "", err
	}
	// 배경 제거된 미리보기 제공
	a.emitProgress("character_process", "", "正在移除背景并像素化...", 86, 3, 4)
	remover, _ := appBackgroundRemover("inherit")
	clean, _ := remover.Remove(img)
	if n := sprite.PaletteSizeForStyle(args.StyleKey); n > 0 {
		single := []*image.NRGBA{clean}
		sprite.PixelPostProcess(single, n)
		clean = single[0]
	}
	a.emitProgress("character_save", "", "正在保存角色结果...", 96, 4, 4)
	saveGalleryPNG("character-"+galleryStamp(), clean)
	return pngDataURL(clean)
}

// GenerateStateArgs는 상태별 스트립 생성 요청입니다.
type GenerateStateArgs struct {
	RequestID             string           `json:"requestId"`
	StateID               string           `json:"stateId"`
	BaseImage             string           `json:"baseImage"` // dataURL
	Description           string           `json:"description"`
	StyleKey              string           `json:"styleKey"`
	StyleCustom           string           `json:"styleCustom"`
	CellSize              int              `json:"cellSize"`
	SafeMargin            int              `json:"safeMargin"`
	Feedback              string           `json:"feedback"`
	RefStrip              string           `json:"refStrip"` // 정면(south) 스트립 dataURL — 방향 세트 생성 시 모션 참조용
	Pipeline              string           `json:"pipeline"` // video_to_frames | image_strip
	VideoModel            string           `json:"videoModel"`
	VideoSeconds          int              `json:"videoSeconds"`
	VideoFPS              int              `json:"videoFps"`
	MotionSourceID        string           `json:"motionSourceId"`
	MotionSourceMode      string           `json:"motionSourceMode"`      // none | direct | guide_image_strip | ai_video
	VideoReferenceMode    string           `json:"videoReferenceMode"`    // auto | none | identity_reference | first_frame | start_end_frame
	VideoSubjectMode      string           `json:"videoSubjectMode"`      // auto | include_description | reference_only
	VideoPromptLanguage   string           `json:"videoPromptLanguage"`   // auto | zh | en
	VideoShadowMode       string           `json:"videoShadowMode"`       // avoid | allow_contact
	FrameSamplerMode      string           `json:"frameSamplerMode"`      // candidate_pool | uniform
	AlignmentMode         string           `json:"alignmentMode"`         // preserve_source_canvas | game_anchor_normalize
	FrameFitMode          string           `json:"frameFitMode"`          // contain | auto_expand | crop
	BackgroundRemovalMode string           `json:"backgroundRemovalMode"` // inherit | chroma | neural | hybrid
	TargetFrameIndex      int              `json:"targetFrameIndex"`
	State                 sprite.StateSpec `json:"state"`
}

// ReprocessVideoStateArgs reuses a saved raw video and only reruns local
// frame extraction, background removal, alignment, pixelization, and QA.
type ReprocessVideoStateArgs struct {
	RequestID             string           `json:"requestId"`
	StateID               string           `json:"stateId"`
	BaseImage             string           `json:"baseImage"`
	StyleKey              string           `json:"styleKey"`
	StyleCustom           string           `json:"styleCustom"`
	CellSize              int              `json:"cellSize"`
	SafeMargin            int              `json:"safeMargin"`
	RawVideoPath          string           `json:"rawVideoPath"`
	VideoSeconds          int              `json:"videoSeconds"`
	FrameSamplerMode      string           `json:"frameSamplerMode"`
	AlignmentMode         string           `json:"alignmentMode"`
	FrameFitMode          string           `json:"frameFitMode"`
	BackgroundRemovalMode string           `json:"backgroundRemovalMode"`
	State                 sprite.StateSpec `json:"state"`
}

// StateResult는 상태 생성 결과입니다.
type StateResult struct {
	RequestID          string                      `json:"requestId"`
	Name               string                      `json:"name"`
	Pipeline           string                      `json:"pipeline"`
	Error              string                      `json:"error"`
	RawStrip           string                      `json:"rawStrip"`
	RawVideoPath       string                      `json:"rawVideoPath"`
	RawExtractedFrames []string                    `json:"rawExtractedFrames"`
	Trace              GenerationTrace             `json:"trace"`
	CandidateFrames    []CandidateFrame            `json:"candidateFrames"`
	Frames             []string                    `json:"frames"`
	Expected           int                         `json:"expected"`
	Found              int                         `json:"found"`
	Warnings           []string                    `json:"warnings"`
	Scores             sprite.ScoreResult          `json:"scores"`
	SequenceMetrics    sprite.FrameSequenceMetrics `json:"sequenceMetrics"`
	CellSize           int                         `json:"cellSize"`
}

func normalizeBackgroundRemovalMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "chroma", "neural", "hybrid":
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return "hybrid"
	}
}

func resolveBackgroundRemovalMode(requested string, settings config.Settings) string {
	if strings.EqualFold(strings.TrimSpace(requested), "inherit") || strings.TrimSpace(requested) == "" {
		return normalizeBackgroundRemovalMode(settings.BackgroundRemovalMode)
	}
	return normalizeBackgroundRemovalMode(requested)
}

func applyBackgroundRemovalTrace(trace *GenerationTrace, removal sprite.BackgroundRemovalTrace) {
	if trace == nil {
		return
	}
	trace.BackgroundRemovalMode = removal.ActualMode
	trace.BackgroundRemovalModel = removal.Model
	trace.BackgroundRemovalSHA256 = removal.ModelSHA256
	trace.BackgroundRemovalMs += removal.InferenceMs
	if removal.Fallback != "" {
		trace.BackgroundRemovalFallback = removal.Fallback
	}
}

func normalizeVideoReferenceMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case videoReferenceModeAuto:
		return videoReferenceModeAuto
	case videoReferenceModeNone, videoReferenceModeIdentityReference, videoReferenceModeFirstFrame, videoReferenceModeStartEndFrame:
		return strings.TrimSpace(mode)
	default:
		return videoReferenceModeIdentityReference
	}
}

func normalizeVideoSubjectMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case "auto":
		return "auto"
	case "reference_only":
		return "reference_only"
	case "include_description":
		return "include_description"
	default:
		return "include_description"
	}
}

func normalizeVideoPromptLanguage(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case videoPromptLanguageChinese:
		return videoPromptLanguageChinese
	case videoPromptLanguageEnglish:
		return videoPromptLanguageEnglish
	default:
		return videoPromptLanguageAuto
	}
}

func normalizeVideoShadowMode(mode string) string {
	if strings.TrimSpace(mode) == videoShadowModeAllowContact {
		return videoShadowModeAllowContact
	}
	return videoShadowModeAvoid
}

func resolveVideoPromptLanguage(mode, provider, model string) string {
	mode = normalizeVideoPromptLanguage(mode)
	if mode != videoPromptLanguageAuto {
		return mode
	}
	if isChineseVideoModel(provider, model) {
		return videoPromptLanguageChinese
	}
	return videoPromptLanguageEnglish
}

func isChineseVideoModel(provider, model string) bool {
	if strings.TrimSpace(provider) == gen.ProviderVolcengine {
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(model))
	for _, marker := range []string{
		"seedance", "doubao", "jimeng", "即梦", "wan", "通义万相",
		"kling", "可灵", "vidu", "hailuo", "海螺", "minimax",
		"hunyuan", "混元", "cogvideo", "pixverse",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func translateVideoPrompt(ctx context.Context, prompt, targetLanguage string) (string, error) {
	client, err := configuredTextClient()
	if err != nil {
		return "", err
	}
	target := "简体中文"
	if targetLanguage == videoPromptLanguageEnglish {
		target = "English"
	}
	systemPrompt := fmt.Sprintf(`你是视频生成提示词的严格翻译器。把用户提供的完整提示词忠实翻译为%s。
必须保留原有段落、项目符号、优先级、否定约束、数字、时长、颜色代码、模型可读术语和动作阶段，不得删减、总结、解释、弱化或新增任何要求。
角色描述、动作描述和反馈也必须翻译为目标语言；专有名词、模型名和十六进制颜色值保持原样。
只输出翻译后的完整提示词，不要标题、代码块、引号、前言或说明。`, target)
	translated, err := client.CompleteWithMaxTokens(ctx, systemPrompt, prompt, 5000)
	if err != nil {
		return "", err
	}
	translated = cleanPromptRewrite(translated)
	if translated == "" {
		return "", errors.New("文本模型没有返回可用的视频提示词翻译。")
	}
	return translated, nil
}

func videoNegativePrompt(language, shadowMode string) string {
	if normalizeVideoShadowMode(shadowMode) != videoShadowModeAvoid {
		return ""
	}
	if language == videoPromptLanguageChinese {
		return "脚底阴影，接触阴影，地面投影，椭圆阴影，环境光遮蔽，地面，平台，底座，倒影，角色下方的深色像素或半透明色块"
	}
	return "foot shadow, contact shadow, cast shadow, oval shadow, ambient occlusion under the feet, ground plane, platform, pedestal, reflection, dark or translucent pixels beneath the character"
}

func videoSubjectDescription(description, mode string) string {
	if normalizeVideoSubjectMode(mode) == "reference_only" {
		return ""
	}
	return description
}

func newGenerationTrace(args GenerateStateArgs, pipeline, motionMode string, settings config.Settings) GenerationTrace {
	imageCfg := settings.Cfg(settings.ImageProvider)
	videoCfg := settings.Cfg(settings.VideoProvider)
	imageModel := imageCfg.Model
	if imageModel == "" {
		imageModel = gen.DefaultModelFor(settings.ImageProvider)
	}
	videoModel := strings.TrimSpace(args.VideoModel)
	if videoModel == "" {
		videoModel = videoCfg.VideoModel
	}
	if videoModel == "" {
		videoModel = gen.DefaultVideoModelFor(settings.VideoProvider)
	}
	activeProvider := settings.ImageProvider
	if pipeline == pipelineVideoToFrames {
		activeProvider = settings.VideoProvider
	}
	return GenerationTrace{
		Pipeline:              pipeline,
		Provider:              activeProvider,
		ImageProvider:         settings.ImageProvider,
		VideoProvider:         settings.VideoProvider,
		Model:                 imageModel,
		VideoModel:            videoModel,
		MotionSourceMode:      motionMode,
		MotionSourceID:        strings.TrimSpace(args.MotionSourceID),
		VideoReferenceMode:    normalizeVideoReferenceMode(args.VideoReferenceMode),
		VideoSubjectMode:      normalizeVideoSubjectMode(args.VideoSubjectMode),
		FrameSamplerMode:      sprite.NormalizeFrameSamplerMode(args.FrameSamplerMode),
		AlignmentMode:         defaultAlignmentModeFor(pipeline, motionMode, args.AlignmentMode),
		BackgroundRemovalMode: resolveBackgroundRemovalMode(args.BackgroundRemovalMode, settings),
		StartedAt:             time.Now().Format(time.RFC3339),
		Parameters: map[string]string{
			"requestId":             args.RequestID,
			"stateId":               args.StateID,
			"state":                 args.State.Name,
			"frames":                fmt.Sprintf("%d", args.State.Frames),
			"fps":                   fmt.Sprintf("%d", args.State.FPS),
			"loop":                  fmt.Sprintf("%t", args.State.Loop),
			"facing":                args.State.Facing,
			"videoSeconds":          fmt.Sprintf("%d", args.VideoSeconds),
			"videoFps":              fmt.Sprintf("%d", args.VideoFPS),
			"frameSamplerMode":      sprite.NormalizeFrameSamplerMode(args.FrameSamplerMode),
			"alignmentMode":         defaultAlignmentModeFor(pipeline, motionMode, args.AlignmentMode),
			"frameFitMode":          sprite.NormalizeFrameFitMode(args.FrameFitMode),
			"backgroundRemovalMode": resolveBackgroundRemovalMode(args.BackgroundRemovalMode, settings),
		},
	}
}

func defaultAlignmentModeFor(pipeline, motionMode, requested string) string {
	if strings.TrimSpace(requested) != "" {
		return sprite.NormalizeAlignmentMode(requested)
	}
	if motionMode == motionSourceModeDirect || pipeline == pipelineVideoToFrames {
		return sprite.AlignmentPreserveSourceCanvas
	}
	return sprite.AlignmentGameAnchorNormalize
}

func completeTrace(trace GenerationTrace) GenerationTrace {
	trace.CompletedAt = time.Now().Format(time.RFC3339)
	return trace
}

func videoTaskFailureMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "视频生成失败。"
	}
	if strings.Contains(strings.ToLower(message), "copyright restrictions") {
		result := "Seedance 视频未通过服务商版权审核：参考图或提示词可能涉及受版权保护的角色或作品。本次没有生成原视频。请改用你拥有合法使用权的原创或已授权角色素材后重试。"
		if requestID := providerRequestID(message); requestID != "" {
			result += " 请求 ID：" + requestID
		}
		return result
	}
	return "视频生成失败：" + message
}

func providerRequestID(message string) string {
	lower := strings.ToLower(message)
	const marker = "request id:"
	index := strings.Index(lower, marker)
	if index < 0 {
		return ""
	}
	value := strings.TrimSpace(message[index+len(marker):])
	if fields := strings.Fields(value); len(fields) > 0 {
		return strings.TrimRight(fields[0], ".,;")
	}
	return ""
}

func ensureGenerationRequestID(requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if requestID != "" {
		return requestID
	}
	return "generation-" + galleryStamp()
}

// GenerateState는 한 상태의 스트립을 생성하고 프레임을 추출합니다.
func (a *App) GenerateState(args GenerateStateArgs) (StateResult, error) {
	args.RequestID = ensureGenerationRequestID(args.RequestID)
	res := StateResult{RequestID: args.RequestID, Name: args.State.Name, Expected: args.State.Frames}
	defer a.emitProgress("idle", args.State.Name, "", -1, 0, 0)

	if args.State.Frames < 1 || args.State.Frames > maxAnimationFrames {
		return res, fmt.Errorf("帧数必须在 1 到 %d 之间。", maxAnimationFrames)
	}
	cellSize := args.CellSize
	if cellSize <= 0 {
		cellSize = 256
	}
	margin := args.SafeMargin
	if margin <= 0 {
		margin = max(8, cellSize/12)
	}

	style := sprite.ResolveStyle(args.StyleKey, args.StyleCustom)
	aspect := sprite.AspectForFrames(args.State.Frames)

	pipeline := strings.TrimSpace(args.Pipeline)
	settings := config.Load()
	if pipeline == "" {
		pipeline = settings.AnimationPipeline
	}
	if pipeline == "" {
		pipeline = pipelineVideoToFrames
	}
	motionMode := normalizeMotionSourceMode(args.MotionSourceMode)
	switch motionMode {
	case motionSourceModeDirect, motionSourceModeGuideImageStrip:
		pipeline = pipelineImageStrip
	case motionSourceModeAIVideo:
		pipeline = pipelineVideoToFrames
	}
	res.Pipeline = pipeline
	res.Trace = newGenerationTrace(args, pipeline, motionMode, settings)

	var baseRaw []byte
	var baseN *image.NRGBA
	if strings.TrimSpace(args.BaseImage) != "" {
		var err error
		baseRaw, err = decodeDataURL(args.BaseImage)
		if err != nil {
			return res, fmt.Errorf("基础图片错误: %w", err)
		}
		// 베이스 캐릭터 정체성 검사용 (투명 배경일 때만 InspectFrames가 사용).
		// 뒷면 계열 방향은 정면 베이스와 색 구성이 달라 오탐하므로 검사를 건너뜁니다.
		if !sprite.IsBackFacing(args.State.Facing) {
			if bimg, err := decodeImage(baseRaw); err == nil {
				baseN = sprite.ToNRGBA(bimg)
			}
		}
	}

	if motionMode == motionSourceModeDirect {
		return a.generateMotionLibraryDirectState(args, baseN, args.StyleKey, cellSize, margin, res.Trace)
	}

	if len(baseRaw) == 0 {
		return res, errors.New("请先提供基础角色图；只有“动作库直接使用”可以不调用角色参考图。")
	}

	switch pipeline {
	case pipelineVideoToFrames:
		vp, err := a.videoProvider()
		if err != nil {
			return res, err
		}
		return a.generateVideoState(args, vp, baseRaw, baseN, style, cellSize, margin, res.Trace)
	case pipelineImageStrip:
	default:
		return res, fmt.Errorf("不支持的动画生成管线：%s", pipeline)
	}
	p, err := a.imageProvider()
	if err != nil {
		return res, err
	}

	// 생성 참조 이미지: 베이스 캐릭터 + (선택) 정면 스트립
	refs := [][]byte{baseRaw}
	res.Trace.References = append(res.Trace.References, GenerationReference{
		Kind:    "base_image",
		Label:   "基础角色图",
		Source:  "user",
		Preview: args.BaseImage,
	})
	if motionMode == motionSourceModeGuideImageStrip {
		genCtx := a.ctx
		if genCtx == nil {
			genCtx = context.Background()
		}
		prepared, err := a.prepareMotionSource(genCtx, args.MotionSourceID, args.State.Frames, cellSize, margin, args.StyleKey, args.FrameFitMode, args.State.Loop)
		if err != nil {
			return res, err
		}
		if len(prepared.ReferenceStripBytes) == 0 {
			return res, errors.New("动作库条目无法生成参考条带")
		}
		refs = append(refs, prepared.ReferenceStripBytes)
		res.CandidateFrames = append(res.CandidateFrames, prepared.Candidates...)
		res.Trace.MotionSourceID = prepared.Item.ID
		res.Trace.MotionSourceName = prepared.Item.Name
		res.Trace.References = append(res.Trace.References, GenerationReference{
			Kind:    "motion_library_strip",
			Label:   prepared.Item.Name,
			Source:  prepared.Item.SourceKind,
			ID:      prepared.Item.ID,
			Path:    prepared.Item.SourcePath,
			Preview: prepared.Strip,
		})
	}
	if strings.TrimSpace(args.RefStrip) != "" {
		if refRaw, err := decodeDataURL(args.RefStrip); err == nil {
			refs = append(refs, refRaw)
			res.Trace.References = append(res.Trace.References, GenerationReference{
				Kind:    "direction_motion_reference",
				Label:   "方向组正面动作参考",
				Source:  "state_raw_strip",
				Preview: args.RefStrip,
			})
		}
	}

	// 완벽한 프레임 수가 나올 때까지 자동 재시도 (최대 3회)
	const maxAttempts = 3
	expected := args.State.Frames
	feedback := args.Feedback
	genCtx, releaseGen := a.genContext()
	defer releaseGen()
	res.Trace.CalledAI = true
	var best StateResult
	var bestImgs []*image.NRGBA
	bestScore := -1 << 30
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		prompt := sprite.BuildStripPrompt(args.Description, style, args.State, feedback)
		if len(refs) > 1 {
			prompt += "\nMotion reference: the SECOND attached image is motion timing only. The FIRST attached image remains the canonical identity, costume, palette and style source. Use the second image only to copy the beat order, pose rhythm and action phases frame by frame, then redraw those phases from the required facing direction above. If the two references conflict, preserve the first image's character identity and style.\n"
		}
		res.Trace.Prompt = prompt
		res.Trace.Attempts = append(res.Trace.Attempts, GenerationAttempt{
			Index:      attempt,
			Pipeline:   pipelineImageStrip,
			Provider:   res.Trace.Provider,
			Model:      res.Trace.Model,
			Prompt:     prompt,
			References: append([]GenerationReference(nil), res.Trace.References...),
			Parameters: map[string]string{
				"aspectRatio": aspect,
				"attempt":     fmt.Sprintf("%d", attempt),
			},
		})

		msg := "AI 正在生成帧..."
		if attempt > 1 {
			msg = fmt.Sprintf("正在修正帧数并重新生成... (%d/%d)", attempt, maxAttempts)
		}
		attemptBase := 8 + (attempt-1)*24
		a.emitProgress("generate", args.State.Name, msg, attemptBase, attempt, maxAttempts)

		stripRaw, err := p.GenerateImage(genCtx, prompt, refs, aspect)
		if err != nil {
			lastErr = friendlyErr(err)
			break // API 오류/취소는 재시도 의미 없음 (클라이언트가 자체 재시도함)
		}

		a.emitProgress("extract", args.State.Name, "正在解码并移除背景...", min(92, attemptBase+14), attempt, maxAttempts)
		stripImg, err := decodeImage(stripRaw)
		if err != nil {
			lastErr = err
			continue
		}
		nimg := sprite.ToNRGBA(stripImg)
		bgKey := sprite.DetectBackground(nimg)
		remover, _ := appBackgroundRemover(args.BackgroundRemovalMode)
		clean, removalTrace := remover.Remove(nimg)
		candTrace := res.Trace
		applyBackgroundRemovalTrace(&candTrace, removalTrace)

		cand := StateResult{RequestID: args.RequestID, Name: args.State.Name, Pipeline: pipelineImageStrip, Expected: expected, Trace: candTrace}
		cand.Warnings = append(cand.Warnings, removalTrace.Warnings...)
		if rawURL, err := pngDataURL(clean); err == nil {
			cand.RawStrip = rawURL
			cand.CandidateFrames = append(cand.CandidateFrames, CandidateFrame{
				ID:     "image-strip-" + galleryStamp(),
				Index:  0,
				PNG:    rawURL,
				Source: candidateSourceImageRaw,
				Label:  "图片模型原始条带",
			})
		}
		a.emitProgress("extract", args.State.Name, "正在提取动画帧并质检...", min(95, attemptBase+18), attempt, maxAttempts)
		extracted := sprite.ExtractFramesWithFitMode(clean, expected, cellSize, cellSize, margin, args.FrameFitMode)
		if args.State.Loop {
			dx, dy := sprite.AlignLoopFrameAnchors(extracted.Frames)
			res.Trace.Parameters["loopClosure"] = "foot_anchor_distributed"
			res.Trace.Parameters["loopAnchorCorrection"] = fmt.Sprintf("%d,%d", dx, dy)
		}
		// 프레임별 품질 검사는 양자화 전 원본 프레임에 수행 (양자화로 미세한
		// 정체성 차이가 뭉개지면 drift 감지 민감도가 떨어짐)
		insp := sprite.InspectFrames(extracted.Frames, bgKey, baseN)
		// 픽셀아트 스타일이면 공유 팔레트 양자화 + 픽셀 그리드 스냅으로 "진짜" 픽셀아트화
		a.emitProgress("pixelize", args.State.Name, "正在像素化并统一调色板...", min(98, attemptBase+21), attempt, maxAttempts)
		sprite.PixelPostProcess(extracted.Frames, sprite.PaletteSizeForStyle(args.StyleKey))
		cand.Found = extracted.Found
		if len(extracted.Frames) > 0 {
			cand.CellSize = extracted.Frames[0].Rect.Dx()
		}
		cand.Warnings = extracted.Warnings
		for _, f := range extracted.Frames {
			u, err := pngDataURL(f)
			if err != nil {
				return res, err
			}
			cand.Frames = append(cand.Frames, u)
		}
		cand.CandidateFrames = append(res.CandidateFrames, cand.CandidateFrames...)
		cand.CandidateFrames = append(cand.CandidateFrames, candidateFramesFromURLs(cand.Frames, candidateSourceImageProcessed, "图片模型候选帧")...)
		cand.Warnings = append(cand.Warnings, insp.Errors...)
		cand.Warnings = append(cand.Warnings, insp.Warnings...)
		cand.Scores = sprite.ScoreFrames(extracted.Frames)
		// 인접 프레임 변화가 거의 없으면(사실상 정지) 비차단 경고.
		// 임계값 0.01은 실측 기반: 의도적으로 미동이 적은 meditate(~1.5%)도 통과시키고
		// 0.01 미만, 즉 프레임이 사실상 동일한 "애니메이션이 전혀 움직이지 않는" 결함만 잡습니다.
		if cand.Found >= 2 && sprite.MotionPresence(extracted.Frames) < 0.01 {
			cand.Warnings = append(cand.Warnings,
				"帧与帧之间几乎没有动作。建议补充更明确的动作描述后重新生成。")
		}
		errCount := len(insp.Errors)

		// 프레임 수가 정확하고 심각한 품질 문제가 없으면 즉시 성공
		if cand.Found == expected && insp.Ok() {
			saveGalleryFrames(args.State.Name, extracted.Frames, args.State.FPS, args.State.Loop)
			cand.Trace = completeTrace(res.Trace)
			return cand, nil
		}
		// 최선 후보 갱신: 프레임 수 우선, 같으면 오류 적은 쪽
		score := cand.Found*100 - errCount*10
		if score > bestScore {
			best, bestScore, bestImgs = cand, score, extracted.Frames
		}
		lastErr = nil

		// 다음 시도용 보정 피드백 (사용자 피드백 유지 + 측정 기반 보정 지시)
		var fixes []string
		if cand.Found != expected {
			fixes = append(fixes, fmt.Sprintf(
				"IMPORTANT CORRECTION: the last attempt read as %d poses but EXACTLY %d are required. Redraw as one horizontal row of %d equally sized poses, one clearly separated pose per column, each ringed by a clean magenta gap so none touch or overlap. Do not draw any frame, border, or film strip.",
				cand.Found, expected, expected))
		}
		if len(insp.RetryHints) > 0 {
			fixes = append(fixes, "QUALITY CORRECTIONS detected by automated inspection (fix all of these):")
			fixes = append(fixes, insp.RetryHints...)
		}
		auto := strings.Join(fixes, "\n")
		if args.Feedback != "" {
			feedback = args.Feedback + "\n" + auto
		} else {
			feedback = auto
		}

		// 마지막 시도 직전 메시지 갱신용: 품질 보정 재시도임을 표시
		if cand.Found == expected && errCount > 0 && attempt < maxAttempts {
			a.emitProgress("generate", args.State.Name, fmt.Sprintf("正在修正质量并重新生成... (%d/%d)", attempt+1, maxAttempts), min(85, 18+attempt*24), attempt+1, maxAttempts)
		}
	}

	if best.Found == 0 {
		if lastErr != nil {
			return res, lastErr
		}
		return res, errors.New("无法提取精灵帧。请把角色或动作描述写得更具体一些。")
	}
	if best.Found != expected {
		best.Warnings = append(best.Warnings,
			fmt.Sprintf("自动重试后帧数仍不一致（请求 %d 帧，提取 %d 帧）。已显示最接近的结果。", expected, best.Found))
	} else {
		best.Warnings = append(best.Warnings,
			"自动重试后仍有部分质量问题。请检查帧，需要时带反馈重新生成。")
	}
	saveGalleryFrames(args.State.Name, bestImgs, args.State.FPS, args.State.Loop)
	best.Trace = completeTrace(res.Trace)
	return best, nil
}

func (a *App) generateVideoState(args GenerateStateArgs, vp gen.VideoProvider, baseRaw []byte, baseN *image.NRGBA, style string, cellSize, margin int, trace GenerationTrace) (StateResult, error) {
	res := StateResult{RequestID: args.RequestID, Name: args.State.Name, Pipeline: pipelineVideoToFrames, Expected: args.State.Frames, Trace: trace}
	settings := config.Load()
	videoModel := strings.TrimSpace(args.VideoModel)
	if videoModel == "" {
		videoModel = settings.Cfg(settings.VideoProvider).VideoModel
	}
	if videoModel == "" {
		videoModel = gen.DefaultVideoModelFor(settings.VideoProvider)
	}
	if videoModel == "" {
		return res, errors.New("当前提供方没有可用的视频模型。")
	}
	seconds := args.VideoSeconds
	if seconds <= 0 {
		seconds = settings.VideoSeconds
	}
	if seconds <= 0 {
		seconds = 4
	}
	videoFPS := args.VideoFPS
	if videoFPS <= 0 {
		videoFPS = settings.VideoFPS
	}
	if videoFPS <= 0 {
		videoFPS = 12
	}

	genCtx, releaseGen := a.genContext()
	defer releaseGen()
	emitVideoProgress := func(phase, message string, percent int, current int, total int) {
		if percent < 0 {
			percent = 0
		}
		if percent > 99 {
			percent = 99
		}
		a.emitProgress(phase, args.State.Name, message, percent, current, total)
	}

	subjectMode := normalizeVideoSubjectMode(args.VideoSubjectMode)
	subjectDescription := videoSubjectDescription(args.Description, subjectMode)
	shadowMode := normalizeVideoShadowMode(args.VideoShadowMode)
	sourcePrompt := sprite.BuildVideoPromptWithOptions(
		subjectDescription,
		style,
		args.State,
		args.Feedback,
		seconds,
		shadowMode == videoShadowModeAvoid,
	)
	promptLanguageMode := normalizeVideoPromptLanguage(args.VideoPromptLanguage)
	targetPromptLanguage := resolveVideoPromptLanguage(promptLanguageMode, settings.VideoProvider, videoModel)
	submittedPromptLanguage := targetPromptLanguage
	prompt := sourcePrompt
	promptTranslated := false
	emitVideoProgress("video_prompt_translate", "正在准备视频模型使用的提示词语言...", 2, 1, 7)
	translatedPrompt, translateErr := translateVideoPrompt(genCtx, sourcePrompt, targetPromptLanguage)
	if translateErr != nil {
		if promptLanguageMode != videoPromptLanguageAuto {
			return res, fmt.Errorf("视频提示词翻译失败：%w", friendlyErr(translateErr))
		}
		submittedPromptLanguage = videoPromptLanguageEnglish
	} else {
		prompt = translatedPrompt
		promptTranslated = true
	}
	negativePrompt := videoNegativePrompt(submittedPromptLanguage, shadowMode)
	referenceMode := normalizeVideoReferenceMode(args.VideoReferenceMode)
	videoCap := gen.VideoModelCapabilityFor(videoModel)
	actualReferenceMode, referenceField := actualVideoReferenceMode(videoModel, referenceMode)
	if actualReferenceMode == videoReferenceModeNone && videoModelRequiresStartImage(videoModel) {
		return res, fmt.Errorf("%s 是图生视频模型，必须传入图片。请把“视频参考策略”改为“自动”或“作为首帧”，或者切换到 t2v 文生视频模型。", videoModel)
	}
	videoWorkflow := videoWorkflowForReference(actualReferenceMode)
	videoReference := baseRaw
	if actualReferenceMode != videoReferenceModeNone {
		if framed, err := prepareVideoReferenceForSafeFraming(baseRaw); err == nil && len(framed) > 0 {
			videoReference = framed
		}
	}
	mediaSlots, mediaSlotTrace := videoMediaSlotsForReference(actualReferenceMode, referenceField, videoReference)
	frameSamplerMode := sprite.NormalizeFrameSamplerMode(args.FrameSamplerMode)
	alignmentMode := defaultAlignmentModeFor(pipelineVideoToFrames, motionSourceModeAIVideo, args.AlignmentMode)
	artifactDir, err := ensureGenerationArtifactDir(args.State.Name, args.RequestID)
	if err != nil {
		return res, fmt.Errorf("无法创建生成证据目录: %w", err)
	}
	if res.Trace.Parameters == nil {
		res.Trace.Parameters = map[string]string{}
	}
	res.Trace.Parameters["artifactDir"] = artifactDir
	res.Trace.Parameters["videoPromptLanguageMode"] = promptLanguageMode
	res.Trace.Parameters["videoPromptTargetLanguage"] = targetPromptLanguage
	res.Trace.Parameters["videoPromptLanguage"] = submittedPromptLanguage
	res.Trace.Parameters["videoPromptTranslated"] = fmt.Sprintf("%t", promptTranslated)
	res.Trace.Parameters["videoPromptTranslationFallback"] = fmt.Sprintf("%t", translateErr != nil)
	res.Trace.Parameters["videoShadowMode"] = shadowMode
	res.Trace.Parameters["negativePromptRequested"] = fmt.Sprintf("%t", strings.TrimSpace(negativePrompt) != "")
	res.Trace.CalledAI = true
	res.Trace.Prompt = prompt
	res.Trace.VideoModel = videoModel
	res.Trace.VideoReferenceMode = referenceMode
	res.Trace.VideoSubjectMode = subjectMode
	res.Trace.ActualVideoReferenceMode = actualReferenceMode
	res.Trace.VideoReferenceField = referenceField
	res.Trace.VideoWorkflow = videoWorkflow
	res.Trace.VideoRequestRoute = videoCap.Endpoint
	res.Trace.VideoMediaSlots = mediaSlotTrace
	res.Trace.FrameSamplerMode = frameSamplerMode
	res.Trace.AlignmentMode = alignmentMode
	baseReference := GenerationReference{Kind: "base_image", Label: "基础角色图", Source: "user"}
	if refImage, decodeErr := decodeImage(baseRaw); decodeErr == nil {
		refPath := filepath.Join(artifactDir, "base-reference.png")
		if writeErr := writePNG(refPath, refImage); writeErr == nil {
			baseReference.Path = refPath
		} else {
			baseReference.Preview = args.BaseImage
		}
	} else {
		baseReference.Preview = args.BaseImage
	}
	res.Trace.References = append(res.Trace.References, baseReference)
	res.Trace.Attempts = append(res.Trace.Attempts, GenerationAttempt{
		Index:      1,
		Pipeline:   pipelineVideoToFrames,
		Provider:   res.Trace.Provider,
		Model:      videoModel,
		Prompt:     prompt,
		References: append([]GenerationReference(nil), res.Trace.References...),
		Parameters: map[string]string{
			"seconds":                   fmt.Sprintf("%d", seconds),
			"fps":                       fmt.Sprintf("%d", videoFPS),
			"size":                      "1024x1024",
			"videoReferenceMode":        referenceMode,
			"videoSubjectMode":          subjectMode,
			"videoPromptTargetLanguage": targetPromptLanguage,
			"videoPromptLanguage":       submittedPromptLanguage,
			"videoPromptTranslated":     fmt.Sprintf("%t", promptTranslated),
			"videoShadowMode":           shadowMode,
			"negativePromptRequested":   fmt.Sprintf("%t", strings.TrimSpace(negativePrompt) != ""),
			"descriptionSent":           fmt.Sprintf("%t", strings.TrimSpace(subjectDescription) != ""),
			"actualReferenceMode":       actualReferenceMode,
			"videoReferenceField":       referenceField,
			"videoWorkflow":             videoWorkflow,
			"videoGenerationRoute":      videoCap.Endpoint,
			"providerAdapter":           videoCap.ProviderAdapter,
			"frameSamplerMode":          frameSamplerMode,
			"alignmentMode":             alignmentMode,
			"frameFitMode":              sprite.NormalizeFrameFitMode(args.FrameFitMode),
			"safeReferenceFraming":      fmt.Sprintf("%t", actualReferenceMode != videoReferenceModeNone),
		},
	})
	emitVideoProgress("video_create", "正在创建视频任务...", 5, 1, 6)
	task, err := vp.GenerateVideo(genCtx, gen.VideoRequest{
		Model:           videoModel,
		Prompt:          prompt,
		NegativePrompt:  negativePrompt,
		Workflow:        videoWorkflow,
		ReferenceImage:  videoReference,
		ReferenceMode:   actualReferenceMode,
		MediaSlots:      mediaSlots,
		Seconds:         seconds,
		FPS:             videoFPS,
		Size:            "1024x1024",
		AspectRatio:     "1:1",
		Resolution:      "720P",
		ProviderAdapter: videoCap.ProviderAdapter,
	})
	if err != nil {
		return res, friendlyErr(err)
	}
	if task.ID == "" && task.URL == "" {
		return res, errors.New("视频响应中没有任务 ID 或下载地址。")
	}

	const maxPolls = 90
	for i := 0; !task.Done(); i++ {
		if task.Failed() {
			res.Error = videoTaskFailureMessage(task.Message)
			res.Trace.Parameters["videoTaskId"] = task.ID
			res.Trace.Parameters["videoTaskStatus"] = task.Status
			res.Trace.Parameters["failureReason"] = res.Error
			res.Trace = completeTrace(res.Trace)
			return res, nil
		}
		if task.ID == "" || task.URL != "" {
			break
		}
		if i >= maxPolls {
			return res, errors.New("视频生成超时，请稍后再试。")
		}
		msg := "正在等待视频生成..."
		percent := 10 + min(60, i*60/maxPolls)
		if task.Progress > 0 {
			msg = fmt.Sprintf("正在等待视频生成... %d%%", task.Progress)
			percent = 10 + min(60, task.Progress*60/100)
		}
		emitVideoProgress("video_wait", msg, percent, i+1, maxPolls)
		select {
		case <-genCtx.Done():
			return res, friendlyErr(genCtx.Err())
		case <-time.After(4 * time.Second):
		}
		next, err := vp.PollVideo(genCtx, task.ID)
		if err != nil {
			return res, friendlyErr(err)
		}
		if next.ID == "" {
			next.ID = task.ID
		}
		task = next
	}

	target := task.URL
	if target == "" {
		target = task.ID
	}
	emitVideoProgress("video_download", "正在下载视频...", 72, 3, 6)
	videoBytes, err := vp.DownloadVideo(genCtx, target)
	if err != nil {
		return res, friendlyErr(err)
	}
	res.RawVideoPath, err = saveGalleryVideo(args.State.Name, videoBytes)
	if err != nil {
		return res, fmt.Errorf("视频已下载，但保存原视频失败: %w", err)
	}
	res.Trace.References = append(res.Trace.References, GenerationReference{
		Kind:   "raw_video",
		Label:  "原始生成视频",
		Source: "video_provider",
		Path:   res.RawVideoPath,
	})
	pending := PendingGeneration{
		RequestID:    args.RequestID,
		StateID:      args.StateID,
		StateName:    args.State.Name,
		RawVideoPath: res.RawVideoPath,
		StartedAt:    res.Trace.StartedAt,
	}
	if err := a.recordPendingGeneration(pending); err != nil {
		return res, fmt.Errorf("原视频已保存到 %s，但写入崩溃恢复记录失败: %w", res.RawVideoPath, err)
	}
	a.emit("generation-artifact", pending)

	return a.processRawVideoState(genCtx, args, videoBytes, res.RawVideoPath, baseN, cellSize, margin, seconds, res.Trace, emitVideoProgress, videoProcessProgress{Extract: 82, Process: 90, Pixelize: 96})
}

type videoProgressFunc func(phase, message string, percent, current, total int)

type videoProcessProgress struct {
	Extract  int
	Process  int
	Pixelize int
}

func (a *App) ReprocessVideoState(args ReprocessVideoStateArgs) (StateResult, error) {
	args.RequestID = ensureGenerationRequestID(args.RequestID)
	res := StateResult{RequestID: args.RequestID, Name: args.State.Name, Pipeline: pipelineVideoToFrames, Expected: args.State.Frames}
	defer a.emitProgress("idle", args.State.Name, "", -1, 0, 0)

	if args.State.Frames < 1 || args.State.Frames > maxAnimationFrames {
		return res, fmt.Errorf("帧数必须在 1 到 %d 之间。", maxAnimationFrames)
	}
	rawVideoPath := strings.TrimSpace(args.RawVideoPath)
	if rawVideoPath == "" {
		return res, errors.New("没有可重新处理的原始视频。")
	}
	if strings.HasPrefix(rawVideoPath, animationVersionResourcePrefix) {
		resolved, err := a.animationVersions.ResolveResource(rawVideoPath)
		if err != nil {
			return res, fmt.Errorf("无法读取版本原始视频: %w", err)
		}
		rawVideoPath = resolved
	}
	cellSize := args.CellSize
	if cellSize <= 0 {
		cellSize = 256
	}
	margin := args.SafeMargin
	if margin <= 0 {
		margin = max(8, cellSize/12)
	}
	settings := config.Load()
	seconds := args.VideoSeconds
	if seconds <= 0 {
		seconds = settings.VideoSeconds
	}
	if seconds <= 0 {
		seconds = 4
	}
	genArgs := GenerateStateArgs{
		RequestID:             args.RequestID,
		StateID:               args.StateID,
		BaseImage:             args.BaseImage,
		StyleKey:              args.StyleKey,
		StyleCustom:           args.StyleCustom,
		CellSize:              cellSize,
		SafeMargin:            margin,
		Pipeline:              pipelineVideoToFrames,
		VideoModel:            settings.Cfg(settings.VideoProvider).VideoModel,
		VideoSeconds:          seconds,
		VideoFPS:              settings.VideoFPS,
		MotionSourceMode:      motionSourceModeNone,
		FrameSamplerMode:      sprite.NormalizeFrameSamplerMode(args.FrameSamplerMode),
		AlignmentMode:         sprite.NormalizeAlignmentMode(args.AlignmentMode),
		FrameFitMode:          sprite.NormalizeFrameFitMode(args.FrameFitMode),
		BackgroundRemovalMode: args.BackgroundRemovalMode,
		State:                 args.State,
		VideoReferenceMode:    videoReferenceModeNone,
	}
	trace := newGenerationTrace(genArgs, pipelineVideoToFrames, motionSourceModeNone, settings)
	trace.CalledAI = false
	trace.VideoWorkflow = "reprocess_raw_video"
	trace.VideoRequestRoute = "local_file"
	trace.FrameSamplerMode = sprite.NormalizeFrameSamplerMode(args.FrameSamplerMode)
	trace.AlignmentMode = sprite.NormalizeAlignmentMode(args.AlignmentMode)
	trace.Parameters["frameFitMode"] = sprite.NormalizeFrameFitMode(args.FrameFitMode)
	trace.Parameters["operation"] = "reprocess_raw_video"
	trace.Parameters["rawVideoPath"] = rawVideoPath
	if strings.TrimSpace(args.BaseImage) != "" {
		trace.References = append(trace.References, GenerationReference{
			Kind:    "base_image",
			Label:   "基础角色图",
			Source:  "user",
			Preview: args.BaseImage,
		})
	}

	var baseN *image.NRGBA
	if strings.TrimSpace(args.BaseImage) != "" && !sprite.IsBackFacing(args.State.Facing) {
		if baseRaw, err := decodeDataURL(args.BaseImage); err == nil {
			if bimg, err := decodeImage(baseRaw); err == nil {
				baseN = sprite.ToNRGBA(bimg)
			}
		}
	}

	genCtx, releaseGen := a.genContext()
	defer releaseGen()
	emitReprocessProgress := func(phase, message string, percent int, current int, total int) {
		a.emitProgress(phase, args.State.Name, message, percent, current, total)
	}
	emitReprocessProgress("video_read", "正在读取原始视频...", 6, 1, 4)
	videoBytes, err := readVideoFileForProcessing(rawVideoPath)
	if err != nil {
		return res, err
	}
	return a.processRawVideoState(genCtx, genArgs, videoBytes, rawVideoPath, baseN, cellSize, margin, seconds, trace, emitReprocessProgress, videoProcessProgress{Extract: 24, Process: 58, Pixelize: 84})
}

func readVideoFileForProcessing(path string) ([]byte, error) {
	clean := filepath.Clean(path)
	ext := strings.ToLower(filepath.Ext(clean))
	switch ext {
	case ".mp4", ".webm", ".mov":
	default:
		return nil, errors.New("仅支持重新处理 MP4、WebM 或 MOV 原始视频。")
	}
	info, err := os.Stat(clean)
	if err != nil {
		return nil, fmt.Errorf("无法读取原始视频: %w", err)
	}
	if info.IsDir() {
		return nil, errors.New("原始视频路径指向的是文件夹。")
	}
	if info.Size() > maxVideoFileBytes {
		return nil, fmt.Errorf("原始视频太大 (%.1fMB)，请先压缩或裁剪后再处理。", float64(info.Size())/(1<<20))
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		return nil, fmt.Errorf("无法读取原始视频: %w", err)
	}
	return data, nil
}

func (a *App) processRawVideoState(ctx context.Context, args GenerateStateArgs, videoBytes []byte, rawVideoPath string, baseN *image.NRGBA, cellSize, margin, seconds int, trace GenerationTrace, emitProgress videoProgressFunc, progress videoProcessProgress) (StateResult, error) {
	res, err := a.processRawVideoStateInner(ctx, args, videoBytes, rawVideoPath, baseN, cellSize, margin, seconds, trace, emitProgress, progress)
	releaseLargeWorkingSet()
	return res, err
}

func (a *App) processRawVideoStateInner(ctx context.Context, args GenerateStateArgs, videoBytes []byte, rawVideoPath string, baseN *image.NRGBA, cellSize, margin, seconds int, trace GenerationTrace, emitProgress videoProgressFunc, progress videoProcessProgress) (StateResult, error) {
	res := StateResult{RequestID: args.RequestID, Name: args.State.Name, Pipeline: pipelineVideoToFrames, Expected: args.State.Frames, RawVideoPath: rawVideoPath, Trace: trace}
	artifactDir, err := generationArtifactDir(res.Trace, args.State.Name, args.RequestID)
	if err != nil {
		return res, fmt.Errorf("无法创建视频帧证据目录: %w", err)
	}
	if res.Trace.Parameters == nil {
		res.Trace.Parameters = map[string]string{}
	}
	res.Trace.Parameters["artifactDir"] = artifactDir
	frameSamplerMode := sprite.NormalizeFrameSamplerMode(args.FrameSamplerMode)
	alignmentMode := defaultAlignmentModeFor(pipelineVideoToFrames, motionSourceModeAIVideo, args.AlignmentMode)
	res.Trace.FrameSamplerMode = frameSamplerMode
	res.Trace.AlignmentMode = alignmentMode
	if rawVideoPath != "" && !traceHasReferencePath(res.Trace, "raw_video", rawVideoPath) {
		res.Trace.References = append(res.Trace.References, GenerationReference{
			Kind:   "raw_video",
			Label:  "原始视频",
			Source: "local_video",
			Path:   rawVideoPath,
		})
	}
	if emitProgress != nil {
		emitProgress("video_extract", "正在从视频中抽取候选关键帧...", progress.Extract, 2, 4)
	}
	rawExtract, err := sprite.ExtractVideoFramesWithSamplerForLoop(ctx, videoBytes, args.State.Frames, seconds, frameSamplerMode, args.State.Loop)
	if err != nil {
		return res, friendlyErr(err)
	}
	res.SequenceMetrics = rawExtract.Sequence
	res.Trace.Parameters["sequenceDuplicateRate"] = fmt.Sprintf("%.4f", rawExtract.Sequence.DuplicateRate)
	res.Trace.Parameters["sequenceContinuity"] = fmt.Sprintf("%.4f", rawExtract.Sequence.Continuity)
	res.Trace.Parameters["sequenceMotionCoverage"] = fmt.Sprintf("%.4f", rawExtract.Sequence.MotionCoverage)
	if args.State.Loop {
		res.Trace.Parameters["sequenceLoopClosure"] = fmt.Sprintf("%.4f", rawExtract.Sequence.LoopClosure)
	}
	rawFrames := rawExtract.Candidates
	rawMeta := rawExtract.CandidateMeta
	if len(rawFrames) == 0 {
		rawFrames = rawExtract.Frames
		rawMeta = rawExtract.FrameMeta
	}
	rawPaths, _, err := persistCandidateFrameSet(artifactDir, "raw-candidate", rawFrames, rawMeta, candidateSourceVideoRaw, "视频候选原始帧")
	if err != nil {
		return res, fmt.Errorf("保存视频候选帧失败: %w", err)
	}
	res.RawExtractedFrames = rawPaths
	if emitProgress != nil {
		emitProgress("video_process", "正在移除背景并对齐帧...", progress.Process, 3, 4)
	}
	remover, resolvedRemovalMode := appBackgroundRemover(args.BackgroundRemovalMode)
	res.Trace.BackgroundRemovalMode = resolvedRemovalMode
	processed, removalTrace, err := sprite.PrepareVideoFramesWithBackgroundRemoverContext(ctx, rawExtract.Frames, args.State.Frames, cellSize, cellSize, margin, alignmentMode, args.FrameFitMode, remover, 2, func(completed, total int) {
		if emitProgress != nil && total > 0 {
			span := max(1, progress.Pixelize-progress.Process-2)
			emitProgress("video_process", fmt.Sprintf("正在移除背景并对齐帧... %d/%d", completed, total), progress.Process+completed*span/total, completed, total)
		}
	})
	if err != nil {
		return res, friendlyErr(err)
	}
	applyBackgroundRemovalTrace(&res.Trace, removalTrace)
	res.Warnings = append(res.Warnings, removalTrace.Warnings...)
	if len(processed.Frames) == 0 {
		res.Found = 0
		res.Warnings = append(res.Warnings, rawExtract.Warnings...)
		res.Warnings = append(res.Warnings, processed.Warnings...)
		res.Error = "视频已处理，但没有抽取到可用的角色像素。请在“朔源”页查看原视频与原始抽帧，必要时调整抽帧或对齐策略。"
		res.Trace = completeTrace(res.Trace)
		return res, nil
	}
	if args.State.Loop {
		dx, dy := sprite.AlignLoopFrameAnchors(processed.Frames)
		res.Trace.Parameters["loopClosure"] = "candidate_seam+foot_anchor_distributed"
		res.Trace.Parameters["loopAnchorCorrection"] = fmt.Sprintf("%d,%d", dx, dy)
	}

	if emitProgress != nil {
		emitProgress("pixelize", "正在像素化并检查质量...", progress.Pixelize, 4, 4)
	}
	sprite.PixelPostProcess(processed.Frames, sprite.PaletteSizeForStyle(args.StyleKey))
	bgKey := [3]uint8{255, 0, 255}
	insp := sprite.InspectFrames(processed.Frames, bgKey, baseN)

	res.Found = processed.Found
	if len(processed.Frames) > 0 {
		res.CellSize = processed.Frames[0].Rect.Dx()
	}
	res.Warnings = append(res.Warnings, rawExtract.Warnings...)
	res.Warnings = append(res.Warnings, processed.Warnings...)
	res.Warnings = append(res.Warnings, insp.Errors...)
	res.Warnings = append(res.Warnings, insp.Warnings...)
	res.Scores = sprite.ScoreFrames(processed.Frames)
	if res.Found >= 2 && sprite.MotionPresence(processed.Frames) < 0.01 {
		res.Warnings = append(res.Warnings,
			"帧与帧之间几乎没有动作。建议补充更明确的动作描述后重新生成。")
	}
	_, _, err = persistCandidateFrameSet(artifactDir, "final-frame", processed.Frames, rawExtract.FrameMeta, candidateSourceVideoProcessed, "视频最终帧")
	if err != nil {
		return res, fmt.Errorf("保存视频最终帧失败: %w", err)
	}
	for _, f := range processed.Frames {
		u, err := pngDataURL(f)
		if err != nil {
			return res, err
		}
		res.Frames = append(res.Frames, u)
	}

	// The replacement picker exposes one processed frame per source timestamp.
	// Raw and processed copies must not share the same flat list: doing so looks
	// like accumulation and lets an unprocessed key-color frame replace a final
	// transparent sprite.
	timelinePool, _, err := sprite.PrepareVideoFramesWithBackgroundRemoverContext(ctx, rawFrames, len(rawFrames), cellSize, cellSize, margin, alignmentMode, args.FrameFitMode, remover, 2, nil)
	if err != nil {
		return res, friendlyErr(err)
	}
	timelineMeta := rawMeta
	if len(timelinePool.Frames) < len(timelineMeta) {
		timelineMeta = timelineMeta[:len(timelinePool.Frames)]
	}
	if len(timelinePool.Frames) > 0 {
		sprite.PixelPostProcess(timelinePool.Frames, sprite.PaletteSizeForStyle(args.StyleKey))
		_, res.CandidateFrames, err = persistCandidateFrameSet(
			artifactDir,
			"timeline-candidate",
			timelinePool.Frames,
			timelineMeta,
			candidateSourceVideoProcessed,
			"视频时间轴候选帧",
		)
		if err != nil {
			return res, fmt.Errorf("保存视频时间轴候选帧失败: %w", err)
		}
	}
	if len(res.CandidateFrames) == 0 {
		_, res.CandidateFrames, err = persistCandidateFrameSet(
			artifactDir,
			"timeline-fallback",
			processed.Frames,
			rawExtract.FrameMeta,
			candidateSourceVideoProcessed,
			"视频时间轴候选帧",
		)
		if err != nil {
			return res, fmt.Errorf("保存视频候选帧失败: %w", err)
		}
	}
	selectedCandidateIDs := map[int]string{}
	for _, candidate := range res.CandidateFrames {
		if candidate.Selected {
			selectedCandidateIDs[candidate.TimeMs] = candidate.ID
		}
	}
	for _, meta := range rawExtract.FrameMeta {
		res.Trace.Selections = append(res.Trace.Selections, FrameSelectionTrace{
			TargetFrameIndex: len(res.Trace.Selections),
			CandidateID:      selectedCandidateIDs[meta.TimeMs],
			Source:           candidateSourceVideoProcessed,
			TimeMs:           meta.TimeMs,
			Quality:          meta.Quality,
			Reason:           meta.Reason,
			SelectedAt:       time.Now().Format(time.RFC3339),
		})
	}
	res.Trace.Parameters["candidateFramePolicy"] = "replace_with_current_processed_timeline_pool"
	res.Trace.Parameters["candidateFrameCount"] = fmt.Sprintf("%d", len(res.CandidateFrames))
	if raw := composeStrip(processed.Frames); raw != nil {
		stripPath := filepath.Join(artifactDir, "processed-strip.png")
		if err := writePNG(stripPath, raw); err == nil {
			res.Trace.Parameters["processedStripPath"] = stripPath
		}
		if u, err := pngDataURL(raw); err == nil {
			res.RawStrip = u
		}
	}
	if res.Found != args.State.Frames || len(insp.Errors) > 0 {
		var parts []string
		if res.Found != args.State.Frames {
			parts = append(parts, fmt.Sprintf("从视频中只得到 %d/%d 个可用角色帧", res.Found, args.State.Frames))
		}
		if len(insp.Errors) > 0 {
			parts = append(parts, "质检发现空白、过淡或角色漂移等严重问题")
		}
		res.Error = strings.Join(parts, "；") + "。已保留原视频、原始抽帧和处理后条带用于排查。"
		res.Trace = completeTrace(res.Trace)
		return res, nil
	}
	saveGalleryFrames(args.State.Name, processed.Frames, args.State.FPS, args.State.Loop)
	res.Trace = completeTrace(res.Trace)
	return res, nil
}

func traceHasReferencePath(trace GenerationTrace, kind, path string) bool {
	for _, ref := range trace.References {
		if ref.Kind == kind && ref.Path == path {
			return true
		}
	}
	return false
}

// prepareVideoReferenceForSafeFraming turns a tightly cropped character image
// into a wider staging reference. First-frame video models otherwise inherit
// the original crop and cannot reveal limbs or weapons that start off-canvas.
func prepareVideoReferenceForSafeFraming(raw []byte) ([]byte, error) {
	img, err := decodeImage(raw)
	if err != nil {
		return nil, err
	}
	original := sprite.ToNRGBA(img)
	transparent := 0
	for i := 3; i < len(original.Pix); i += 4 {
		if original.Pix[i] <= 10 {
			transparent++
		}
	}
	clean := original
	if transparent*100 < original.Rect.Dx()*original.Rect.Dy() {
		remover, _ := appBackgroundRemover("inherit")
		clean, _ = remover.Remove(original)
	}
	minX, minY := clean.Rect.Dx(), clean.Rect.Dy()
	maxX, maxY := -1, -1
	for y := 0; y < clean.Rect.Dy(); y++ {
		for x := 0; x < clean.Rect.Dx(); x++ {
			if clean.Pix[clean.PixOffset(x, y)+3] <= 10 {
				continue
			}
			minX, minY = min(minX, x), min(minY, y)
			maxX, maxY = max(maxX, x), max(maxY, y)
		}
	}
	if maxX < minX || maxY < minY {
		return raw, nil
	}
	crop := image.NewNRGBA(image.Rect(0, 0, maxX-minX+1, maxY-minY+1))
	xdraw.Copy(crop, image.Point{}, clean, image.Rect(minX, minY, maxX+1, maxY+1), xdraw.Src, nil)
	const canvasSize = 1024
	const contentLimit = 560
	scale := min(float64(contentLimit)/float64(crop.Rect.Dx()), float64(contentLimit)/float64(crop.Rect.Dy()))
	if scale > 1 {
		scale = 1
	}
	sw := max(1, int(float64(crop.Rect.Dx())*scale+0.5))
	sh := max(1, int(float64(crop.Rect.Dy())*scale+0.5))
	scaled := crop
	if sw != crop.Rect.Dx() || sh != crop.Rect.Dy() {
		scaled = image.NewNRGBA(image.Rect(0, 0, sw, sh))
		xdraw.NearestNeighbor.Scale(scaled, scaled.Rect, crop, crop.Rect, xdraw.Src, nil)
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, canvasSize, canvasSize))
	for i := 0; i < len(canvas.Pix); i += 4 {
		canvas.Pix[i], canvas.Pix[i+1], canvas.Pix[i+2], canvas.Pix[i+3] = 255, 0, 255, 255
	}
	left := (canvasSize - sw) / 2
	top := min(canvasSize-sh-150, (canvasSize-sh)/2)
	xdraw.Copy(canvas, image.Point{X: left, Y: max(120, top)}, scaled, scaled.Rect, xdraw.Over, nil)
	var buf bytes.Buffer
	if err := png.Encode(&buf, canvas); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func actualVideoReferenceMode(model, requested string) (mode, field string) {
	cap := gen.VideoModelCapabilityFor(model)
	firstFrameMode := func() (string, string) {
		if cap.SupportsFirstFrameReference && cap.SupportsReferenceImage {
			if cap.Endpoint == "/api/v3/contents/generations/tasks" {
				return videoReferenceModeFirstFrame, "content[].role=first_frame"
			}
			return videoReferenceModeFirstFrame, "image"
		}
		return videoReferenceModeNone, ""
	}
	switch requested {
	case videoReferenceModeNone:
		return videoReferenceModeNone, ""
	case videoReferenceModeIdentityReference:
		if cap.SupportsIdentityReference && cap.SupportsReferenceImage {
			if cap.Endpoint == "/api/v3/contents/generations/tasks" {
				return videoReferenceModeIdentityReference, "content[].role=reference_image"
			}
			return videoReferenceModeIdentityReference, "input_reference"
		}
		return firstFrameMode()
	case videoReferenceModeFirstFrame:
		return firstFrameMode()
	case videoReferenceModeStartEndFrame:
		if cap.Endpoint == "/api/v3/contents/generations/tasks" && cap.SupportsStartEndFrames {
			return videoReferenceModeStartEndFrame, "content[].role=first_frame,last_frame"
		}
		if cap.Endpoint == "/v1/video/generations" && cap.SupportsStartEndFrames {
			return videoReferenceModeStartEndFrame, "image"
		}
		return firstFrameMode()
	default:
		if cap.SupportsIdentityReference && cap.SupportsReferenceImage {
			return videoReferenceModeIdentityReference, "input_reference"
		}
		return firstFrameMode()
	}
}

func videoWorkflowForReference(mode string) string {
	switch mode {
	case videoReferenceModeIdentityReference:
		return gen.VideoWorkflowReferenceToVideo
	case videoReferenceModeFirstFrame:
		return gen.VideoWorkflowImageToVideo
	case videoReferenceModeStartEndFrame:
		return gen.VideoWorkflowFirstLastToVideo
	default:
		return gen.VideoWorkflowTextToVideo
	}
}

func videoModelRequiresStartImage(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(m, "i2v") || strings.Contains(m, "image-to-video")
}

func videoMediaSlotsForReference(mode, field string, baseRaw []byte) (map[string][]gen.VideoMediaInput, []GenerationMediaSlotTrace) {
	if len(baseRaw) == 0 {
		return nil, nil
	}
	var slot string
	var role string
	switch mode {
	case videoReferenceModeIdentityReference:
		slot = gen.VideoSlotReferenceImages
		role = "reference_image"
	case videoReferenceModeFirstFrame:
		slot = gen.VideoSlotStartImage
		role = "first_frame"
	case videoReferenceModeStartEndFrame:
		start := gen.VideoMediaInput{Data: baseRaw, MIME: "image/png", Label: "基础角色起始帧"}
		end := gen.VideoMediaInput{Data: baseRaw, MIME: "image/png", Label: "基础角色结束帧"}
		return map[string][]gen.VideoMediaInput{
				gen.VideoSlotStartImage: {start},
				gen.VideoSlotEndImage:   {end},
			}, []GenerationMediaSlotTrace{
				{Slot: gen.VideoSlotStartImage, Field: field, Role: "first_frame", Count: 1},
				{Slot: gen.VideoSlotEndImage, Field: field, Role: "last_frame", Count: 1},
			}
	default:
		return nil, nil
	}
	if strings.TrimSpace(field) == "" {
		field = videoFieldForSlot(slot)
	}
	media := gen.VideoMediaInput{Data: baseRaw, MIME: "image/png", Label: "基础角色图"}
	return map[string][]gen.VideoMediaInput{
			slot: []gen.VideoMediaInput{media},
		}, []GenerationMediaSlotTrace{{
			Slot:  slot,
			Field: field,
			Role:  role,
			Count: 1,
		}}
}

func videoFieldForSlot(slot string) string {
	switch slot {
	case gen.VideoSlotReferenceImages:
		return "input_reference"
	case gen.VideoSlotStartImage:
		return "image"
	case gen.VideoSlotEndImage:
		return "image_tail"
	default:
		return slot
	}
}

// ---------- 8방향 세트 ----------

// ListDirections는 8방향 메타데이터 목록을 반환합니다 (3x3 그리드 순서).
func (a *App) ListDirections() []sprite.DirectionInfo {
	return sprite.ListDirections()
}

// ListPresets는 100개 상황 키워드 카탈로그를 반환합니다 (프리셋 선택 UI용).
func (a *App) ListPresets() []sprite.PresetInfo {
	return sprite.ListPresets()
}

// MirrorFrames는 프레임들을 좌우 반전합니다 (east→west 등 미러 방향 생성용).
func (a *App) MirrorFrames(frames []string) ([]string, error) {
	out := make([]string, 0, len(frames))
	for i, fu := range frames {
		raw, err := decodeDataURL(fu)
		if err != nil {
			return nil, fmt.Errorf("第 %d 帧解码失败: %w", i+1, err)
		}
		img, err := decodeImage(raw)
		if err != nil {
			return nil, err
		}
		u, err := pngDataURL(sprite.MirrorNRGBA(sprite.ToNRGBA(img)))
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

// ---------- 내보내기 ----------

// ExportState는 내보내기용 상태 데이터입니다.
type ExportState struct {
	Name   string   `json:"name"`
	FPS    int      `json:"fps"`
	Loop   bool     `json:"loop"`
	Frames []string `json:"frames"` // 선택/정렬된 dataURL
}

// ExportArgs는 프로젝트 내보내기 요청입니다.
type ExportArgs struct {
	Character    string        `json:"character"`
	CellSize     int           `json:"cellSize"`
	FrameFitMode string        `json:"frameFitMode"`
	AtlasColumns int           `json:"atlasColumns"`
	States       []ExportState `json:"states"`
}

type ExportAnimationArgs struct {
	Character string   `json:"character"`
	Name      string   `json:"name"`
	FPS       int      `json:"fps"`
	Loop      bool     `json:"loop"`
	Format    string   `json:"format"` // gif | mp4
	Frames    []string `json:"frames"`
}

// ExportProject는 디렉토리를 선택받아 스프라이트시트/매니페스트/GIF/프레임을 저장합니다.
func (a *App) ExportProject(args ExportArgs) (string, error) {
	if len(args.States) == 0 {
		return "", errors.New("没有可导出的动画。")
	}
	cellSize := args.CellSize
	if cellSize <= 0 {
		cellSize = 256
	}

	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择导出文件夹",
	})
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", nil // 취소
	}

	charName := strings.TrimSpace(args.Character)
	if charName == "" {
		charName = "character"
	}
	safeName := sanitizeName(charName)
	outDir := filepath.Join(dir, safeName)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}

	defer a.emitProgress("idle", "", "", -1, 0, 0)
	a.emitProgress("export_prepare", "", "正在准备导出帧...", 8, 1, 4)

	// 상태 프레임 디코딩
	var stateFrames []sprite.StateFrames
	for _, st := range args.States {
		if len(st.Frames) == 0 {
			continue
		}
		sf := sprite.StateFrames{
			Spec: sprite.StateSpec{Name: st.Name, Frames: len(st.Frames), FPS: st.FPS, Loop: st.Loop},
		}
		for _, fu := range st.Frames {
			raw, err := decodeDataURL(fu)
			if err != nil {
				return "", fmt.Errorf("%s 帧解码失败: %w", st.Name, err)
			}
			img, err := decodeImage(raw)
			if err != nil {
				return "", err
			}
			sf.Frames = append(sf.Frames, sprite.ToNRGBA(img))
		}
		stateFrames = append(stateFrames, sf)
	}
	if len(stateFrames) == 0 {
		return "", errors.New("没有可导出的帧。")
	}
	frameFitMode := sprite.NormalizeFrameFitMode(args.FrameFitMode)
	if frameFitMode == sprite.FrameFitAutoExpand {
		for _, sf := range stateFrames {
			cellSize = sprite.ExpandedCellSize(sf.Frames, cellSize)
		}
	}
	for i := range stateFrames {
		stateFrames[i].Frames = sprite.PrepareFramesForAtlas(stateFrames[i].Frames, cellSize, cellSize, frameFitMode)
	}

	// 파일명 프리픽스: 캐릭터별 산출물이 섞여도 구분되도록 모든 파일 앞에 캐릭터 이름을 붙인다.
	prefix := safeName + "-"

	// 1) 스프라이트시트 + 매니페스트
	a.emitProgress("export_sheet", "", "正在写入精灵表和清单...", 36, 2, 4)
	sheet, manifest := sprite.ComposeAtlasWithColumns(safeName, stateFrames, cellSize, cellSize, args.AtlasColumns)
	if err := writePNG(filepath.Join(outDir, prefix+"sprite-sheet.png"), sheet); err != nil {
		return "", err
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(outDir, prefix+"manifest.json"), manifestJSON, 0o644); err != nil {
		return "", err
	}
	// Aseprite 호환 JSON (Phaser/Unity/Godot 임포터용)
	aseJSON, err := sprite.BuildAsepriteJSON(manifest)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(outDir, prefix+"sprite-sheet.json"), aseJSON, 0o644); err != nil {
		return "", err
	}

	// 2) 상태별 GIF + 개별 프레임 PNG
	for i, sf := range stateFrames {
		percent := 48 + (i * 44 / max(1, len(stateFrames)))
		a.emitProgress("export_state", sf.Spec.Name, "正在导出动画帧、GIF 和 APNG...", percent, i+1, len(stateFrames))
		stateDir := filepath.Join(outDir, "frames", sanitizeName(sf.Spec.Name))
		if err := os.MkdirAll(stateDir, 0o755); err != nil {
			return "", err
		}
		for i, frame := range sf.Frames {
			if err := writePNG(filepath.Join(stateDir, fmt.Sprintf("%sframe-%02d.png", prefix, i)), frame); err != nil {
				return "", err
			}
		}
		stateName := sanitizeName(sf.Spec.Name)
		gifBytes, err := sprite.EncodeGIF(sf.Frames, sf.Spec.FPS, sf.Spec.Loop)
		if err != nil {
			return "", fmt.Errorf("%s GIF 编码失败: %w", sf.Spec.Name, err)
		}
		gifDir := filepath.Join(outDir, "gif")
		if err := os.MkdirAll(gifDir, 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(gifDir, prefix+stateName+".gif"), gifBytes, 0o644); err != nil {
			return "", err
		}
		// APNG: 풀 알파 지원 (GIF의 1-bit 투명도 한계 보완)
		apngBytes, err := sprite.EncodeAPNG(sf.Frames, sf.Spec.FPS, sf.Spec.Loop)
		if err != nil {
			return "", fmt.Errorf("%s APNG 编码失败: %w", sf.Spec.Name, err)
		}
		apngDir := filepath.Join(outDir, "apng")
		if err := os.MkdirAll(apngDir, 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(apngDir, prefix+stateName+".png"), apngBytes, 0o644); err != nil {
			return "", err
		}
	}

	a.emitProgress("export_done", "", "导出完成。", 100, 4, 4)
	return outDir, nil
}

// ExportAnimation writes one processed animation without creating the full
// project package.
func (a *App) ExportAnimation(args ExportAnimationArgs) (string, error) {
	if len(args.Frames) == 0 {
		return "", errors.New("没有可导出的动画帧。")
	}
	format := strings.ToLower(strings.TrimSpace(args.Format))
	if format != "gif" && format != "mp4" {
		return "", fmt.Errorf("不支持的动画格式：%s", args.Format)
	}
	name := sanitizeName(strings.TrimSpace(args.Character) + "-" + strings.TrimSpace(args.Name))
	if name == "" || name == "-" {
		name = "animation"
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "导出处理后的动画",
		DefaultFilename: name + "." + format,
		Filters: []runtime.FileFilter{{
			DisplayName: strings.ToUpper(format) + " 动画 (*." + format + ")",
			Pattern:     "*." + format,
		}},
	})
	if err != nil || path == "" {
		return path, err
	}
	defer a.emitProgress("idle", args.Name, "", -1, 0, 0)
	a.emitProgress("export_animation_decode", args.Name, "正在准备动画帧...", 18, 1, 3)
	frames := make([]*image.NRGBA, 0, len(args.Frames))
	for _, dataURL := range args.Frames {
		raw, err := decodeDataURL(dataURL)
		if err != nil {
			return "", err
		}
		img, err := decodeImage(raw)
		if err != nil {
			return "", err
		}
		frames = append(frames, sprite.ToNRGBA(img))
	}
	a.emitProgress("export_animation_encode", args.Name, "正在编码处理后的动画...", 62, 2, 3)
	var data []byte
	if format == "gif" {
		data, err = sprite.EncodeGIF(frames, args.FPS, args.Loop)
	} else {
		ctx := a.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		data, err = sprite.EncodeMP4(ctx, frames, args.FPS)
	}
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	a.emitProgress("export_animation_done", args.Name, "动画导出完成。", 100, 3, 3)
	return path, nil
}

// RevealInFinder는 내보낸 폴더를 파일 탐색기에서 엽니다.
func (a *App) RevealInFinder(path string) {
	if strings.HasPrefix(strings.TrimSpace(path), animationVersionResourcePrefix) && a.animationVersions != nil {
		if resolved, err := a.animationVersions.ResolveResource(path); err == nil {
			path = resolved
		} else {
			return
		}
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	runtime.BrowserOpenURL(a.ctx, u.String())
}

func writePNG(path string, img image.Image) error {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

var unsafeNameRe = regexp.MustCompile(`[^a-zA-Z0-9가-힣_-]+`)

func sanitizeName(name string) string {
	s := unsafeNameRe.ReplaceAllString(strings.TrimSpace(name), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "character"
	}
	return s
}
