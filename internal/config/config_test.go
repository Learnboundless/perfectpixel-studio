package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env.local")
	content := "# 주석\n" +
		"FAL_KEY=abc123:secret\n" +
		"export OPENROUTER_API_KEY=\"sk-or-test\"\n" +
		"OPENAI_API_KEY='sk-openai-test'\n" +
		"GEMINI_API_KEY='AIza-test'\n" +
		"ZETATECHS_API_KEY='sk-zeta-test'\n" +
		"ZETATECHS_BASE_URL='https://ent.zetatechs.com'\n" +
		"ZETATECHS_VIDEO_MODEL='sora-2'\n" +
		"VOLCENGINE_ARK_API_KEY='ark-cn-test'\n" +
		"VOLCENGINE_VIDEO_MODEL='doubao-seedance-2-0-mini-260615'\n" +
		"VOLCENGINE_TEXT_API_KEY='ark-text-test'\n" +
		"VOLCENGINE_TEXT_MODEL='glm-5-2-260617'\n" +
		"INVALID_LINE\n" +
		"EMPTY=\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	out := map[string]string{}
	parseEnvFile(path, out)

	if out["FAL_KEY"] != "abc123:secret" {
		t.Fatalf("FAL_KEY 파싱 실패: %q", out["FAL_KEY"])
	}
	if out["OPENROUTER_API_KEY"] != "sk-or-test" {
		t.Fatalf("export + 따옴표 파싱 실패: %q", out["OPENROUTER_API_KEY"])
	}
	if out["OPENAI_API_KEY"] != "sk-openai-test" {
		t.Fatalf("OPENAI_API_KEY 파싱 실패: %q", out["OPENAI_API_KEY"])
	}
	if out["GEMINI_API_KEY"] != "AIza-test" {
		t.Fatalf("작은따옴표 파싱 실패: %q", out["GEMINI_API_KEY"])
	}
	if out["ZETATECHS_API_KEY"] != "sk-zeta-test" {
		t.Fatalf("ZETATECHS_API_KEY 파싱 실패: %q", out["ZETATECHS_API_KEY"])
	}
	if out["ZETATECHS_BASE_URL"] != "https://ent.zetatechs.com" {
		t.Fatalf("ZETATECHS_BASE_URL 파싱 실패: %q", out["ZETATECHS_BASE_URL"])
	}
	if out["ZETATECHS_VIDEO_MODEL"] != "sora-2" {
		t.Fatalf("ZETATECHS_VIDEO_MODEL 파싱 실패: %q", out["ZETATECHS_VIDEO_MODEL"])
	}
	if out["VOLCENGINE_ARK_API_KEY"] != "ark-cn-test" || out["VOLCENGINE_VIDEO_MODEL"] != "doubao-seedance-2-0-mini-260615" {
		t.Fatalf("Volcengine env parsing failed: %#v", out)
	}
	if out["VOLCENGINE_TEXT_API_KEY"] != "ark-text-test" || out["VOLCENGINE_TEXT_MODEL"] != "glm-5-2-260617" {
		t.Fatalf("Volcengine text env parsing failed: %#v", out)
	}
	if _, ok := out["EMPTY"]; ok {
		t.Fatal("빈 값은 무시되어야 합니다")
	}
}

func TestSettingsCfg(t *testing.T) {
	s := Settings{
		Gemini:     ProviderCfg{APIKey: "g"},
		OpenAI:     ProviderCfg{APIKey: "ai"},
		OpenRouter: ProviderCfg{APIKey: "o"},
		Fal:        ProviderCfg{APIKey: "f"},
		ZetaTechs:  ProviderCfg{APIKey: "z", VideoModel: "sora-2", BaseURL: "https://ent.zetatechs.com"},
		Volcengine: ProviderCfg{APIKey: "v", VideoModel: "doubao-seedance-2-0-mini-260615"},
	}
	if s.Cfg("openai").APIKey != "ai" || s.Cfg("openrouter").APIKey != "o" || s.Cfg("fal").APIKey != "f" || s.Cfg("zetatechs").APIKey != "z" || s.Cfg("volcengine").APIKey != "v" {
		t.Fatal("프로바이더별 설정 매핑 오류")
	}
	// 알 수 없는 프로바이더는 gemini로 폴백
	if s.Cfg("unknown").APIKey != "g" {
		t.Fatal("기본 폴백 오류")
	}
	// 포인터 반환이므로 수정이 반영되어야 함
	s.Cfg("fal").Model = "m"
	if s.Fal.Model != "m" {
		t.Fatal("Cfg는 포인터를 반환해야 합니다")
	}
	if s.Cfg("zetatechs").BaseURL != "https://ent.zetatechs.com" {
		t.Fatal("ZetaTechs BaseURL 매핑 오류")
	}
	if s.Cfg("zetatechs").VideoModel != "sora-2" {
		t.Fatal("ZetaTechs VideoModel 매핑 오류")
	}
}
