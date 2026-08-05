// Command ppgen은 GUI 없이 캐릭터 + 애니메이션 상태를 생성하고
// 게임 엔진에 바로 쓸 수 있는 번들(스프라이트시트 · manifest.json · Aseprite JSON ·
// 상태별 GIF/APNG · 개별 프레임 PNG)을 디스크로 내보내는 헤드리스 CLI입니다.
//
// 설치형 Wails 앱의 GenerateState + ExportProject 로직을 동일하게 재현하되,
// 파일 대화상자 없이 -out 디렉토리로 바로 내보내고, 결과 요약을 stdout에
// JSON으로 출력하여 스킬/스크립트에서 호출하기 좋게 만들었습니다.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	_ "image/gif"
	_ "image/jpeg"

	_ "golang.org/x/image/webp"

	"perfectpixel/internal/config"
	"perfectpixel/internal/gen"
	"perfectpixel/internal/sprite"
)

// options는 CLI 플래그를 묶은 실행 옵션입니다.
type options struct {
	desc     string
	style    string
	states   string
	percat   int
	all      bool
	dirset   string
	out      string
	provider string
	key      string
	model    string
	attempts int
	timeout  time.Duration
	jsonOut  bool
	quiet    bool
	baseOnly bool
}

func main() {
	var (
		opt  options
		dump = flag.Bool("dump", false, "프리셋+방향 카탈로그를 JSON으로 출력하고 종료")
	)
	flag.StringVar(&opt.desc, "desc", "a small knight with silver armor and a blue plume on the helmet", "캐릭터 설명")
	flag.StringVar(&opt.style, "style", "pixel", "스타일 키 (pixel | chibi | cartoon | retro16)")
	flag.StringVar(&opt.states, "states", "idle,walk", "쉼표로 구분된 생성할 상태 이름 목록")
	flag.IntVar(&opt.percat, "percat", 0, "0보다 크면 카테고리당 N개 프리셋을 자동 선택 (states 무시)")
	flag.BoolVar(&opt.all, "all", false, "전체 프리셋 생성 (states/percat 무시)")
	flag.StringVar(&opt.dirset, "dirset", "", "8방향 세트를 추가 생성할 상태 이름 (선택)")
	flag.StringVar(&opt.out, "out", "./perfectpixel-out", "출력 디렉토리")
	flag.StringVar(&opt.provider, "provider", "", "프로바이더 강제 지정 (gemini|openai|openrouter|fal|byteplus|zetatechs)")
	flag.StringVar(&opt.key, "key", "", "API 키 강제 지정 (설정/환경변수보다 우선)")
	flag.StringVar(&opt.model, "model", "", "모델 강제 지정")
	flag.IntVar(&opt.attempts, "attempts", 3, "상태별 품질 보정 재생성 최대 시도 횟수")
	flag.DurationVar(&opt.timeout, "timeout", 30*time.Minute, "전체 타임아웃")
	flag.BoolVar(&opt.jsonOut, "json", false, "사람이 읽는 로그 대신 결과 요약 JSON만 stdout에 출력")
	flag.BoolVar(&opt.quiet, "quiet", false, "진행 로그 억제 (-json과 함께 쓰기 좋음)")
	flag.BoolVar(&opt.baseOnly, "baseonly", false, "베이스 캐릭터(base.png)만 생성하고 상태/번들은 건너뜀")
	flag.Parse()

	if *dump {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{
			"presets":    sprite.ListPresets(),
			"directions": sprite.ListDirections(),
			"styles":     []string{"pixel", "chibi", "cartoon", "retro16"},
			"providers":  gen.SupportedProviders,
		})
		return
	}

	if err := runGen(opt); err != nil {
		fmt.Fprintf(os.Stderr, "ppgen 실패: %v\n", err)
		os.Exit(1)
	}
}

// resolveProvider는 설정/환경변수를 읽고 CLI 오버라이드를 적용해 프로바이더를 만듭니다.
func resolveProvider(opt options) (gen.Provider, string, string, error) {
	s := config.Load()
	provider := s.Provider
	if opt.provider != "" {
		provider = opt.provider
	}
	cfg := s.Cfg(provider)
	key := cfg.APIKey
	if opt.key != "" {
		key = opt.key
	}
	model := cfg.Model
	if opt.model != "" {
		model = opt.model
	}
	if key == "" {
		return nil, "", "", fmt.Errorf("프로바이더 %q에 API 키가 없습니다 (config.json, .env, 환경변수 또는 -key 사용)", provider)
	}
	var p gen.Provider
	var err error
	if provider == gen.ProviderZetaTechs {
		p = gen.NewZetaTechsWithBaseURL(key, model, cfg.BaseURL)
	} else {
		p, err = gen.New(provider, key, model)
	}
	if err != nil {
		return nil, "", "", err
	}
	if model == "" {
		model = gen.DefaultModelFor(provider)
	}
	return p, provider, model, nil
}

// selectStates는 -all / -percat / -states 우선순위로 생성 대상 프리셋을 고릅니다.
func selectStates(opt options) ([]sprite.PresetInfo, error) {
	byName := map[string]sprite.PresetInfo{}
	for _, p := range sprite.Presets {
		byName[p.Name] = p
	}
	if opt.all {
		return append([]sprite.PresetInfo(nil), sprite.Presets...), nil
	}
	if opt.percat > 0 {
		count := map[string]int{}
		var out []sprite.PresetInfo
		for _, p := range sprite.Presets {
			if count[p.Category] < opt.percat {
				out = append(out, p)
				count[p.Category]++
			}
		}
		return out, nil
	}
	var out []sprite.PresetInfo
	var missing []string
	for _, n := range strings.Split(opt.states, ",") {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if p, ok := byName[n]; ok {
			out = append(out, p)
		} else {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("알 수 없는 상태 이름: %s (목록은 -dump 참고)", strings.Join(missing, ", "))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("생성할 상태가 없습니다 (-states, -percat, 또는 -all 지정)")
	}
	return out, nil
}
