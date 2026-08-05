// Package config는 앱 설정 영속화를 담당합니다.
package config

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// ProviderCfg는 프로바이더별 키/모델 설정입니다.
type ProviderCfg struct {
	APIKey      string `json:"apiKey"`
	Model       string `json:"model"`
	VideoModel  string `json:"videoModel,omitempty"`
	TextAPIKey  string `json:"textApiKey,omitempty"`
	TextModel   string `json:"textModel,omitempty"`
	VisionModel string `json:"visionModel,omitempty"`
	BaseURL     string `json:"baseUrl,omitempty"`
}

// Settings는 사용자 설정입니다.
type Settings struct {
	Provider          string      `json:"provider,omitempty"`          // 旧版图片 provider，保留用于兼容迁移
	ImageProvider     string      `json:"imageProvider,omitempty"`     // gemini | openai | openrouter | fal | byteplus | zetatechs
	VideoProvider     string      `json:"videoProvider,omitempty"`     // zetatechs | volcengine
	TextProvider      string      `json:"textProvider,omitempty"`      // volcengine | zetatechs | openai | openrouter
	VisionProvider    string      `json:"visionProvider,omitempty"`    // zetatechs | openai | openrouter
	AnimationPipeline string      `json:"animationPipeline,omitempty"` // video_to_frames | image_strip
	VideoSeconds      int         `json:"videoSeconds,omitempty"`
	VideoFPS          int         `json:"videoFps,omitempty"`
	Gemini            ProviderCfg `json:"gemini"`
	OpenAI            ProviderCfg `json:"openai"`
	OpenRouter        ProviderCfg `json:"openrouter"`
	Fal               ProviderCfg `json:"fal"`
	BytePlus          ProviderCfg `json:"byteplus"`
	ZetaTechs         ProviderCfg `json:"zetatechs"`
	Volcengine        ProviderCfg `json:"volcengine"`

	// 레거시 필드 (v1 → 마이그레이션용)
	LegacyAPIKey string `json:"apiKey,omitempty"`
	LegacyModel  string `json:"model,omitempty"`
}

// Cfg는 프로바이더 이름으로 해당 설정을 반환합니다.
func (s *Settings) Cfg(provider string) *ProviderCfg {
	switch provider {
	case "openai":
		return &s.OpenAI
	case "openrouter":
		return &s.OpenRouter
	case "fal":
		return &s.Fal
	case "byteplus":
		return &s.BytePlus
	case "zetatechs":
		return &s.ZetaTechs
	case "volcengine":
		return &s.Volcengine
	default:
		return &s.Gemini
	}
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "perfectpixel", "config.json"), nil
}

// SessionPath는 작업 세션 스냅샷 파일 경로를 반환합니다.
func SessionPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "perfectpixel", "session.json"), nil
}

// GenerationArtifactsDir stores large generation evidence outside session.json.
func GenerationArtifactsDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "perfectpixel", "generation-artifacts"), nil
}

// PendingGenerationsPath stores downloaded videos until the frontend commits
// the corresponding card state to session.json.
func PendingGenerationsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "perfectpixel", "pending-generations.json"), nil
}

// GalleryDir는 생성 이미지가 자동 보관되는 갤러리 디렉토리 경로를 반환합니다.
func GalleryDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "perfectpixel", "gallery"), nil
}

// MotionLibraryDir는 재사용 가능한 동작 소스와 색인 파일이 저장되는 경로를 반환합니다.
func MotionLibraryDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "perfectpixel", "motion-library"), nil
}

// CharacterLibraryDir stores reusable character identities independently of a session.
func CharacterLibraryDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "perfectpixel", "character-library"), nil
}

// Load는 설정을 읽고 레거시 마이그레이션 및 환경변수 폴백을 적용합니다.
func Load() Settings {
	var s Settings
	if path, err := configPath(); err == nil {
		if data, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(data, &s)
		}
	}

	// v1 단일 키 → Gemini로 마이그레이션
	if s.LegacyAPIKey != "" && s.Gemini.APIKey == "" {
		s.Gemini.APIKey = s.LegacyAPIKey
		if s.LegacyModel != "" {
			s.Gemini.Model = s.LegacyModel
		}
	}
	s.LegacyAPIKey = ""
	s.LegacyModel = ""

	// 환경변수 / .env 파일 폴백 (설정 파일이 우선)
	env := loadEnvFallback()
	if s.Gemini.APIKey == "" {
		s.Gemini.APIKey = firstNonEmpty(env["GEMINI_API_KEY"], env["GOOGLE_API_KEY"])
	}
	if s.OpenAI.APIKey == "" {
		s.OpenAI.APIKey = env["OPENAI_API_KEY"]
	}
	if s.OpenRouter.APIKey == "" {
		s.OpenRouter.APIKey = env["OPENROUTER_API_KEY"]
	}
	if s.Fal.APIKey == "" {
		s.Fal.APIKey = firstNonEmpty(env["FAL_KEY"], env["FAL_API_KEY"])
	}
	if s.BytePlus.APIKey == "" {
		s.BytePlus.APIKey = firstNonEmpty(env["BYTEPLUS_API_KEY"], env["ARK_API_KEY"])
	}
	if s.ZetaTechs.APIKey == "" {
		s.ZetaTechs.APIKey = firstNonEmpty(env["ZETATECHS_API_KEY"], env["ZETA_API_KEY"])
	}
	if s.ZetaTechs.BaseURL == "" {
		s.ZetaTechs.BaseURL = firstNonEmpty(env["ZETATECHS_BASE_URL"], env["ZETATECHS_API_BASE_URL"], env["ZETA_BASE_URL"], env["ZETA_API_BASE_URL"])
	}
	if s.ZetaTechs.VideoModel == "" {
		s.ZetaTechs.VideoModel = firstNonEmpty(env["ZETATECHS_VIDEO_MODEL"], env["ZETA_VIDEO_MODEL"])
	}
	if s.Volcengine.APIKey == "" {
		s.Volcengine.APIKey = firstNonEmpty(env["VOLCENGINE_ARK_API_KEY"], env["ARK_CN_API_KEY"])
	}
	if s.Volcengine.VideoModel == "" {
		s.Volcengine.VideoModel = env["VOLCENGINE_VIDEO_MODEL"]
	}
	if s.Volcengine.TextAPIKey == "" {
		s.Volcengine.TextAPIKey = env["VOLCENGINE_TEXT_API_KEY"]
	}
	if s.Volcengine.TextModel == "" {
		s.Volcengine.TextModel = env["VOLCENGINE_TEXT_MODEL"]
	}
	if s.ZetaTechs.TextAPIKey == "" {
		s.ZetaTechs.TextAPIKey = env["ZETATECHS_TEXT_API_KEY"]
	}
	if s.ZetaTechs.TextModel == "" {
		s.ZetaTechs.TextModel = env["ZETATECHS_TEXT_MODEL"]
	}
	if s.ZetaTechs.VisionModel == "" {
		s.ZetaTechs.VisionModel = env["ZETATECHS_VISION_MODEL"]
	}

	if s.AnimationPipeline == "" {
		s.AnimationPipeline = "video_to_frames"
	}
	if s.VideoSeconds <= 0 {
		s.VideoSeconds = 4
	}
	if s.VideoFPS <= 0 {
		s.VideoFPS = 12
	}

	// v3 단일 provider는 이미지 provider로 마이그레이션합니다.
	if s.ImageProvider == "" {
		s.ImageProvider = s.Provider
	}
	if s.ImageProvider == "" {
		switch {
		case s.Gemini.APIKey != "":
			s.ImageProvider = "gemini"
		case s.OpenAI.APIKey != "":
			s.ImageProvider = "openai"
		case s.OpenRouter.APIKey != "":
			s.ImageProvider = "openrouter"
		case s.Fal.APIKey != "":
			s.ImageProvider = "fal"
		case s.BytePlus.APIKey != "":
			s.ImageProvider = "byteplus"
		case s.ZetaTechs.APIKey != "":
			s.ImageProvider = "zetatechs"
		default:
			s.ImageProvider = "gemini"
		}
	}
	// 계속 구버전 필드도 동기화해 이전 바이너리에서 설정을 읽을 수 있게 합니다.
	s.Provider = s.ImageProvider
	if s.VideoProvider == "" {
		switch {
		case s.Volcengine.APIKey != "":
			s.VideoProvider = "volcengine"
		case s.ZetaTechs.APIKey != "":
			s.VideoProvider = "zetatechs"
		default:
			s.VideoProvider = "zetatechs"
		}
	}
	if s.TextProvider == "" {
		switch {
		case s.Volcengine.TextAPIKey != "" || s.Volcengine.APIKey != "":
			s.TextProvider = "volcengine"
		case s.ZetaTechs.TextAPIKey != "" || s.ZetaTechs.APIKey != "":
			s.TextProvider = "zetatechs"
		case s.OpenAI.TextAPIKey != "" || s.OpenAI.APIKey != "":
			s.TextProvider = "openai"
		case s.OpenRouter.TextAPIKey != "" || s.OpenRouter.APIKey != "":
			s.TextProvider = "openrouter"
		default:
			s.TextProvider = "volcengine"
		}
	}
	if s.VisionProvider == "" {
		switch {
		case s.ZetaTechs.APIKey != "":
			s.VisionProvider = "zetatechs"
		case s.OpenAI.APIKey != "":
			s.VisionProvider = "openai"
		case s.OpenRouter.APIKey != "":
			s.VisionProvider = "openrouter"
		default:
			s.VisionProvider = "zetatechs"
		}
	}
	return s
}

// Save는 설정을 저장합니다 (0600 권한).
func Save(s Settings) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// loadEnvFallback은 OS 환경변수와 실행 위치 주변의 .env/.env.local을 읽습니다.
func loadEnvFallback() map[string]string {
	out := map[string]string{}

	// 1) .env 파일 (작업 디렉토리 + 실행 파일 디렉토리)
	var dirs []string
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd)
	}
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}
	for _, dir := range dirs {
		for _, name := range []string{".env", ".env.local"} {
			parseEnvFile(filepath.Join(dir, name), out)
		}
	}

	// 2) OS 환경변수 (파일보다 우선)
	for _, key := range []string{"GEMINI_API_KEY", "GOOGLE_API_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY", "FAL_KEY", "FAL_API_KEY", "BYTEPLUS_API_KEY", "ARK_API_KEY", "ZETATECHS_API_KEY", "ZETA_API_KEY", "ZETATECHS_BASE_URL", "ZETATECHS_API_BASE_URL", "ZETA_BASE_URL", "ZETA_API_BASE_URL", "ZETATECHS_VIDEO_MODEL", "ZETA_VIDEO_MODEL", "ZETATECHS_TEXT_API_KEY", "ZETATECHS_TEXT_MODEL", "VOLCENGINE_ARK_API_KEY", "ARK_CN_API_KEY", "VOLCENGINE_VIDEO_MODEL", "VOLCENGINE_TEXT_API_KEY", "VOLCENGINE_TEXT_MODEL"} {
		if v := os.Getenv(key); v != "" {
			out[key] = v
		}
	}
	return out
}

func parseEnvFile(path string, out map[string]string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if k != "" && v != "" {
			out[k] = v
		}
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
