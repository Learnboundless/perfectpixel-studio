package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"perfectpixel/internal/config"
	"perfectpixel/internal/gen"
	"perfectpixel/internal/sprite"
)

// TestSessionRoundTrip은 세션 저장 → 복원 → 삭제 흐름을 검증합니다.
// HOME을 임시 디렉토리로 격리해 실제 사용자 세션을 건드리지 않습니다.
func TestSessionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", dir)
	a := NewApp()

	// 저장 전에는 빈 문자열
	if got := a.LoadSession(); got != "" {
		t.Fatalf("초기 세션이 비어있지 않음: %q", got)
	}

	payload := `{"v":1,"character":{"name":"hero"},"cellSize":256,"states":[]}`
	if err := a.SaveSession(payload); err != nil {
		t.Fatalf("세션 저장 실패: %v", err)
	}
	if got := a.LoadSession(); got != payload {
		t.Fatalf("세션 복원 불일치: %q", got)
	}

	// 덮어쓰기
	payload2 := `{"v":1,"character":{"name":"slime"},"cellSize":128,"states":[]}`
	if err := a.SaveSession(payload2); err != nil {
		t.Fatalf("세션 덮어쓰기 실패: %v", err)
	}
	if got := a.LoadSession(); got != payload2 {
		t.Fatalf("덮어쓴 세션 불일치: %q", got)
	}

	// 삭제 후 빈 문자열, 중복 삭제도 에러 없어야 함
	if err := a.ClearSession(); err != nil {
		t.Fatalf("세션 삭제 실패: %v", err)
	}
	if got := a.LoadSession(); got != "" {
		t.Fatalf("삭제 후 세션이 남아있음: %q", got)
	}
	if err := a.ClearSession(); err != nil {
		t.Fatalf("중복 삭제 에러: %v", err)
	}
}

func TestVideoTaskFailureMessageLocalizesCopyrightRestriction(t *testing.T) {
	const requestID = "02178506425762000000000000000000000ffffac193b008d452c"
	got := videoTaskFailureMessage("The request failed because the output video may be related to copyright restrictions. Request id: " + requestID)
	if !strings.Contains(got, "版权审核") {
		t.Fatalf("copyright failure was not localized: %q", got)
	}
	if !strings.Contains(got, requestID) {
		t.Fatalf("request id was lost: %q", got)
	}
	if strings.Contains(got, "copyright restrictions") {
		t.Fatalf("raw provider message leaked into user-facing error: %q", got)
	}
}

func TestVideoTaskFailureMessageKeepsUnknownProviderFailure(t *testing.T) {
	got := videoTaskFailureMessage("quota exhausted")
	if got != "视频生成失败：quota exhausted" {
		t.Fatalf("unexpected generic failure: %q", got)
	}
}

func TestVideoSubjectDescriptionCanOmitCharacterDescription(t *testing.T) {
	const description = "licensed character identity"
	if got := videoSubjectDescription(description, "reference_only"); got != "" {
		t.Fatalf("reference-only mode sent the character description: %q", got)
	}
	prompt := sprite.BuildVideoPrompt(
		videoSubjectDescription(description, "reference_only"),
		"pixel art",
		sprite.StateSpec{Name: "dash", Action: "quick dash", Frames: 8, FPS: 12},
		"",
		4,
	)
	if strings.Contains(prompt, description) || strings.Contains(prompt, "Subject notes:") {
		t.Fatalf("reference-only prompt still contains character description: %q", prompt)
	}
	if got := videoSubjectDescription(description, "include_description"); got != description {
		t.Fatalf("include mode changed the character description: %q", got)
	}
}

func TestSaveSessionExternalizesRegenerationEvidenceWithoutReplacingOldFrames(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", dir)
	a := NewApp()

	img := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	for y := 0; y < 96; y++ {
		for x := 0; x < 96; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 2), G: uint8(y * 2), B: uint8((x + y) % 255), A: 255})
		}
	}
	largeEvidence, err := pngDataURL(img)
	if err != nil {
		t.Fatal(err)
	}
	oldFrame, err := pngDataURL(image.NewNRGBA(image.Rect(0, 0, 8, 8)))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"v": 2,
		"states": []any{map[string]any{
			"id":                 "attack-card",
			"name":               "attack",
			"status":             "generating",
			"items":              []any{map[string]any{"id": "old", "png": oldFrame, "selected": true}},
			"rawExtractedFrames": []string{largeEvidence, largeEvidence},
			"candidateFrames": []any{map[string]any{
				"id": "candidate-1", "index": 0, "png": largeEvidence,
				"source": "video_raw", "label": "视频候选原始帧",
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SaveSession(string(payload)); err != nil {
		t.Fatal(err)
	}

	var saved struct {
		States []struct {
			Items []struct {
				PNG string `json:"png"`
			} `json:"items"`
			RawExtractedFrames []string         `json:"rawExtractedFrames"`
			CandidateFrames    []CandidateFrame `json:"candidateFrames"`
		} `json:"states"`
	}
	if err := json.Unmarshal([]byte(a.LoadSession()), &saved); err != nil {
		t.Fatal(err)
	}
	if got := saved.States[0].Items[0].PNG; got != oldFrame {
		t.Fatal("重新生成快照覆盖了旧的最终帧")
	}
	candidate := saved.States[0].CandidateFrames[0]
	if candidate.PNG != "" {
		t.Fatal("候选帧仍以内嵌 DataURL 写入会话")
	}
	if candidate.SourcePath == "" {
		t.Fatal("候选帧没有可恢复的本地文件路径")
	}
	if _, err := os.Stat(candidate.SourcePath); err != nil {
		t.Fatalf("候选帧本地文件不存在: %v", err)
	}
	for _, ref := range saved.States[0].RawExtractedFrames {
		if _, err := os.Stat(ref); err != nil {
			t.Fatalf("原始抽帧没有外置保存: %v", err)
		}
	}
	if got := len(a.LoadSession()); got >= len(payload) {
		t.Fatalf("会话压缩未生效: before=%d after=%d", len(payload), got)
	}
}

func TestSaveSessionExternalizesNestedCharacterWorkspaceEvidence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", dir)
	a := NewApp()

	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 3), G: uint8(y * 3), B: 120, A: 255})
		}
	}
	evidence, err := pngDataURL(img)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"v": 4,
		"workspaces": []any{map[string]any{
			"id": "character-1",
			"states": []any{map[string]any{
				"id":                 "walk",
				"rawExtractedFrames": []string{evidence},
				"candidateFrames": []any{map[string]any{
					"id": "candidate-1", "index": 0, "png": evidence,
				}},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SaveSession(string(payload)); err != nil {
		t.Fatal(err)
	}

	var saved struct {
		Workspaces []struct {
			States []struct {
				RawExtractedFrames []string         `json:"rawExtractedFrames"`
				CandidateFrames    []CandidateFrame `json:"candidateFrames"`
			} `json:"states"`
		} `json:"workspaces"`
	}
	if err := json.Unmarshal([]byte(a.LoadSession()), &saved); err != nil {
		t.Fatal(err)
	}
	state := saved.Workspaces[0].States[0]
	if len(state.RawExtractedFrames) != 1 {
		t.Fatalf("nested workspace evidence count mismatch: %#v", state.RawExtractedFrames)
	}
	if _, err := os.Stat(state.RawExtractedFrames[0]); err != nil {
		t.Fatalf("nested workspace raw frame was not externalized: %v", err)
	}
	if state.CandidateFrames[0].PNG != "" || state.CandidateFrames[0].SourcePath == "" {
		t.Fatalf("nested workspace candidate was not compacted: %#v", state.CandidateFrames[0])
	}
}

func TestImageAndVideoProvidersAreSelectedIndependently(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", dir)
	a := NewApp()

	if err := a.SetImageProvider(gen.ProviderOpenAI); err != nil {
		t.Fatal(err)
	}
	if err := a.SetVideoProvider(gen.ProviderVolcengine); err != nil {
		t.Fatal(err)
	}
	if err := a.SetTextProvider(gen.ProviderVolcengine); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveProviderTextModel(gen.ProviderVolcengine, "glm-5.2"); err != nil {
		t.Fatal(err)
	}
	settings := config.Load()
	if settings.ImageProvider != gen.ProviderOpenAI {
		t.Fatalf("image provider mismatch: %q", settings.ImageProvider)
	}
	if settings.VideoProvider != gen.ProviderVolcengine {
		t.Fatalf("video provider mismatch: %q", settings.VideoProvider)
	}
	if settings.TextProvider != gen.ProviderVolcengine || settings.Volcengine.TextModel != "glm-5.2" {
		t.Fatalf("text provider mismatch: %#v", settings)
	}
	if settings.Provider != gen.ProviderOpenAI {
		t.Fatalf("legacy provider should follow image provider: %q", settings.Provider)
	}

	trace := newGenerationTrace(GenerateStateArgs{
		RequestID:    "request-1",
		VideoSeconds: 4,
		VideoFPS:     12,
		State:        sprite.StateSpec{Name: "attack", Frames: 6, FPS: 12},
	}, pipelineVideoToFrames, motionSourceModeNone, settings)
	if trace.Provider != gen.ProviderVolcengine || trace.ImageProvider != gen.ProviderOpenAI || trace.VideoProvider != gen.ProviderVolcengine {
		t.Fatalf("generation trace provider fields mismatch: %#v", trace)
	}
}

func TestCleanPromptRewriteRemovesFenceAndQuotes(t *testing.T) {
	got := cleanPromptRewrite("```text\n“原创白色机甲驾驶员”\n```")
	if got != "原创白色机甲驾驶员" {
		t.Fatalf("unexpected rewrite cleanup: %q", got)
	}
}

func TestRewriteRiskyPromptUsesIndependentTextProvider(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", dir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected text endpoint: %s", r.URL.Path)
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			ResponseFormat any `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "glm-5-2-260617" {
			t.Fatalf("unexpected text model: %q", body.Model)
		}
		if body.ResponseFormat != nil {
			t.Fatalf("text request unexpectedly sent response_format: %#v", body.ResponseFormat)
		}
		if len(body.Messages) != 2 || body.Messages[1].Content != "高达0079 阿姆罗的MS" {
			t.Fatalf("unexpected messages: %#v", body.Messages)
		}
		if !strings.Contains(body.Messages[0].Content, "不要解释如何规避审核") {
			t.Fatalf("compliance system prompt missing: %q", body.Messages[0].Content)
		}
		if !strings.Contains(body.Messages[0].Content, "不得依据训练知识回忆或补全原作外观") {
			t.Fatalf("anti-inference guard missing: %q", body.Messages[0].Content)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"原创白色装甲驾驶员，红蓝色块点缀。"}}]}`))
	}))
	defer server.Close()

	if err := config.Save(config.Settings{
		TextProvider: gen.ProviderVolcengine,
		Volcengine: config.ProviderCfg{
			APIKey:    "ark-test",
			TextModel: "glm-5-2-260617",
			BaseURL:   server.URL,
		},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := NewApp().RewriteRiskyPrompt(RewritePromptArgs{Text: "高达0079 阿姆罗的MS"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "原创白色装甲驾驶员，红蓝色块点缀。" {
		t.Fatalf("unexpected rewritten prompt: %q", got)
	}
}

func TestOptimizeCharacterPromptWorksWithoutIPRiskTerms(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", dir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected text endpoint: %s", r.URL.Path)
		}
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Messages) != 2 || body.Messages[1].Content != "短发的原创机械师，穿黄色工装" {
			t.Fatalf("unexpected messages: %#v", body.Messages)
		}
		if !strings.Contains(body.Messages[0].Content, "完整保留用户明确写出的角色身份") || !strings.Contains(body.Messages[0].Content, "不得凭空增加重要道具") {
			t.Fatalf("optimization guard missing: %q", body.Messages[0].Content)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"一名短发的原创机械师，身穿轮廓利落的黄色工装，配色清晰，完整身体可见。"}}]}`))
	}))
	defer server.Close()

	if err := config.Save(config.Settings{
		TextProvider: gen.ProviderVolcengine,
		Volcengine: config.ProviderCfg{
			APIKey:    "ark-test",
			TextModel: "glm-5-2-260617",
			BaseURL:   server.URL,
		},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := NewApp().OptimizeCharacterPrompt(RewritePromptArgs{Text: "短发的原创机械师，穿黄色工装"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "一名短发的原创机械师，身穿轮廓利落的黄色工装，配色清晰，完整身体可见。" {
		t.Fatalf("unexpected optimized prompt: %q", got)
	}
}

func TestSeedance20ReferenceModesUseArkMediaRoles(t *testing.T) {
	model := "doubao-seedance-2-0-mini-260615"
	mode, field := actualVideoReferenceMode(model, videoReferenceModeIdentityReference)
	if mode != videoReferenceModeIdentityReference || field != "content[].role=reference_image" {
		t.Fatalf("identity reference mapping mismatch: mode=%q field=%q", mode, field)
	}
	mode, field = actualVideoReferenceMode(model, videoReferenceModeStartEndFrame)
	if mode != videoReferenceModeStartEndFrame || field != "content[].role=first_frame,last_frame" {
		t.Fatalf("start/end mapping mismatch: mode=%q field=%q", mode, field)
	}
	slots, trace := videoMediaSlotsForReference(mode, field, []byte("png"))
	if len(slots[gen.VideoSlotStartImage]) != 1 || len(slots[gen.VideoSlotEndImage]) != 1 || len(trace) != 2 {
		t.Fatalf("start/end media slots mismatch: slots=%#v trace=%#v", slots, trace)
	}
}

func TestVideoGenerationDefaultsUseFullDescriptionAndIdentityReference(t *testing.T) {
	if got := normalizeVideoReferenceMode(""); got != videoReferenceModeIdentityReference {
		t.Fatalf("default video reference mode=%q want=%q", got, videoReferenceModeIdentityReference)
	}
	if got := normalizeVideoSubjectMode(""); got != "include_description" {
		t.Fatalf("default video subject mode=%q want=include_description", got)
	}
	if got := normalizeVideoReferenceMode(videoReferenceModeAuto); got != videoReferenceModeAuto {
		t.Fatalf("explicit auto reference mode should remain available, got=%q", got)
	}
	if got := normalizeVideoSubjectMode("auto"); got != "auto" {
		t.Fatalf("explicit auto subject mode should remain available, got=%q", got)
	}
}

func TestVideoPromptLanguageAutoUsesChineseForChineseModels(t *testing.T) {
	tests := []struct {
		provider string
		model    string
		want     string
	}{
		{gen.ProviderVolcengine, "doubao-seedance-2-0-mini-260615", videoPromptLanguageChinese},
		{gen.ProviderZetaTechs, "wan2.6-i2v", videoPromptLanguageChinese},
		{gen.ProviderZetaTechs, "kling-v2.6", videoPromptLanguageChinese},
		{gen.ProviderOpenAI, "sora-2", videoPromptLanguageEnglish},
	}
	for _, tc := range tests {
		if got := resolveVideoPromptLanguage(videoPromptLanguageAuto, tc.provider, tc.model); got != tc.want {
			t.Fatalf("provider=%q model=%q language=%q want=%q", tc.provider, tc.model, got, tc.want)
		}
	}
	if got := resolveVideoPromptLanguage(videoPromptLanguageEnglish, gen.ProviderVolcengine, "seedance"); got != videoPromptLanguageEnglish {
		t.Fatalf("explicit English mode was not respected: %q", got)
	}
}

func TestVideoShadowModeControlsPromptAndNegativePrompt(t *testing.T) {
	spec := sprite.StateSpec{Name: "walk", Action: "walking", Loop: true}
	avoid := sprite.BuildVideoPromptWithOptions("原创角色", "pixel art", spec, "", 4, true)
	if !strings.Contains(avoid, "Every pixel beneath and around the feet") {
		t.Fatalf("avoid-shadow prompt is missing the strict foot-shadow constraint: %q", avoid)
	}
	allow := sprite.BuildVideoPromptWithOptions("原创角色", "pixel art", spec, "", 4, false)
	if !strings.Contains(allow, "small stable contact shadow") || strings.Contains(allow, "Every pixel beneath and around the feet") {
		t.Fatalf("allow-contact prompt has the wrong shadow policy: %q", allow)
	}
	if got := videoNegativePrompt(videoPromptLanguageChinese, videoShadowModeAvoid); !strings.Contains(got, "脚底阴影") {
		t.Fatalf("Chinese negative prompt missing foot shadow: %q", got)
	}
	if got := videoNegativePrompt(videoPromptLanguageEnglish, videoShadowModeAllowContact); got != "" {
		t.Fatalf("allow-contact mode should not send a shadow negative prompt: %q", got)
	}
}

func TestTranslateVideoPromptUsesConfiguredTextModelAndLargeOutputLimit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", dir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["max_tokens"] != float64(5000) {
			t.Fatalf("video prompt translation max_tokens=%v want=5000", body["max_tokens"])
		}
		messages, ok := body["messages"].([]any)
		if !ok || len(messages) != 2 {
			t.Fatalf("unexpected messages: %#v", body["messages"])
		}
		system := messages[0].(map[string]any)["content"].(string)
		if !strings.Contains(system, "简体中文") || !strings.Contains(system, "不得删减") {
			t.Fatalf("translation guard missing: %q", system)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"生成一个 4 秒的角色行走动画。"}}]}`))
	}))
	defer server.Close()

	if err := config.Save(config.Settings{
		TextProvider: gen.ProviderVolcengine,
		Volcengine: config.ProviderCfg{
			APIKey:    "ark-test",
			TextModel: "glm-5-2-260617",
			BaseURL:   server.URL,
		},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := translateVideoPrompt(context.Background(), "Create a 4-second walking animation.", videoPromptLanguageChinese)
	if err != nil {
		t.Fatal(err)
	}
	if got != "生成一个 4 秒的角色行走动画。" {
		t.Fatalf("unexpected translated prompt: %q", got)
	}
}

func TestPendingGenerationSurvivesRestartUntilSessionCommit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", dir)
	a := NewApp()
	videoPath := dir + string(os.PathSeparator) + "attack.mp4"
	if err := os.WriteFile(videoPath, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	pending := PendingGeneration{
		RequestID:    "gen-attack-2",
		StateID:      "attack-card",
		StateName:    "attack",
		RawVideoPath: videoPath,
	}
	if err := a.recordPendingGeneration(pending); err != nil {
		t.Fatal(err)
	}

	restarted := NewApp()
	items, err := restarted.ListPendingGenerations()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].RawVideoPath != videoPath {
		t.Fatalf("重启后未恢复待提交视频: %#v", items)
	}
	if err := restarted.AcknowledgeGeneration(pending.RequestID); err != nil {
		t.Fatal(err)
	}
	items, err = restarted.ListPendingGenerations()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("会话提交后待恢复记录仍然存在: %#v", items)
	}
}

func TestPersistCandidateFrameSetKeepsFullFramesOffTheBridge(t *testing.T) {
	dir := t.TempDir()
	frame := image.NewNRGBA(image.Rect(0, 0, 1024, 768))
	for y := 0; y < 768; y++ {
		for x := 0; x < 1024; x++ {
			frame.SetNRGBA(x, y, color.NRGBA{R: uint8(x % 251), G: uint8(y % 241), B: uint8((x + y) % 239), A: 255})
		}
	}
	paths, candidates, err := persistCandidateFrameSet(dir, "raw", []*image.NRGBA{frame}, nil, "video_raw", "原始帧")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || len(candidates) != 1 {
		t.Fatalf("候选帧保存数量错误: paths=%d candidates=%d", len(paths), len(candidates))
	}
	if candidates[0].SourcePath != paths[0] {
		t.Fatalf("候选帧没有指向完整本地证据: %#v", candidates[0])
	}
	if _, err := os.Stat(paths[0]); err != nil {
		t.Fatal(err)
	}
	thumbRaw, err := decodeDataURL(candidates[0].PNG)
	if err != nil {
		t.Fatal(err)
	}
	thumb, err := decodeImage(thumbRaw)
	if err != nil {
		t.Fatal(err)
	}
	if max(thumb.Bounds().Dx(), thumb.Bounds().Dy()) > candidateThumbnailMaxDim {
		t.Fatalf("跨桥缩略图尺寸过大: %v", thumb.Bounds())
	}
}

func TestProcessRawVideoStateReturnsBoundedRecoverablePayload(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", dir)
	videoFrames := make([]*image.NRGBA, 0, 12)
	for i := 0; i < 12; i++ {
		frame := image.NewNRGBA(image.Rect(0, 0, 320, 320))
		for y := 0; y < 320; y++ {
			for x := 0; x < 320; x++ {
				frame.SetNRGBA(x, y, color.NRGBA{R: 255, B: 255, A: 255})
			}
		}
		startX := 90 + i*5
		for y := 105; y < 245; y++ {
			for x := startX; x < startX+55; x++ {
				frame.SetNRGBA(x, y, color.NRGBA{R: 32, G: 96, B: 210, A: 255})
			}
		}
		videoFrames = append(videoFrames, frame)
	}
	videoBytes, err := sprite.EncodeMP4(context.Background(), videoFrames, 12)
	if err != nil {
		t.Fatal(err)
	}
	videoPath := dir + string(os.PathSeparator) + "source.mp4"
	if err := os.WriteFile(videoPath, videoBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	args := GenerateStateArgs{
		RequestID:        "bounded-payload",
		StateID:          "attack-card",
		StyleKey:         "pixel",
		CellSize:         128,
		FrameSamplerMode: sprite.FrameSamplerCandidatePool,
		AlignmentMode:    sprite.AlignmentPreserveSourceCanvas,
		FrameFitMode:     sprite.FrameFitContain,
		State:            sprite.StateSpec{Name: "attack", Frames: 4, FPS: 12},
	}
	trace := GenerationTrace{Parameters: map[string]string{}}
	res, err := NewApp().processRawVideoState(context.Background(), args, videoBytes, videoPath, nil, 128, 12, 1, trace, nil, videoProcessProgress{})
	if err != nil {
		t.Fatal(err)
	}
	if res.RawVideoPath != videoPath || len(res.RawExtractedFrames) < args.State.Frames {
		t.Fatalf("原视频或候选帧朔源缺失: video=%q raw=%d", res.RawVideoPath, len(res.RawExtractedFrames))
	}
	for _, path := range res.RawExtractedFrames {
		if len(path) >= 5 && path[:5] == "data:" {
			t.Fatal("原始全分辨率抽帧仍通过 WebView DataURL 返回")
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("原始抽帧路径不可恢复: %v", err)
		}
	}
	for _, candidate := range res.CandidateFrames {
		if candidate.SourcePath == "" {
			t.Fatal("候选帧缺少本地证据路径")
		}
	}
	payload, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) > 4<<20 {
		t.Fatalf("视频处理结果跨桥载荷过大: %.1f MB", float64(len(payload))/(1<<20))
	}
}

func TestSaveProviderKeyStoresZetaTechsNewAPIConnection(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", dir)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected validation path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-zeta-test-token" {
			t.Fatalf("unexpected authorization header: %s", got)
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	a := NewApp()
	a.startup(context.Background())
	conn := `{"_type":"newapi_channel_conn","key":"sk-zeta-test-token","url":"` + srv.URL + `"}`
	if err := a.SaveProviderKey("zetatechs", conn); err != nil {
		t.Fatalf("save ZetaTechs NewAPI connection failed: %v", err)
	}

	cfg := config.Load().ZetaTechs
	if cfg.APIKey != "sk-zeta-test-token" {
		t.Fatal("ZetaTechs API key was not extracted from NewAPI connection JSON")
	}
	if cfg.BaseURL != srv.URL {
		t.Fatalf("ZetaTechs base URL mismatch: got %q want %q", cfg.BaseURL, srv.URL)
	}
}

func TestActualVideoReferenceModeMapsWanIdentityToFirstFrame(t *testing.T) {
	mode, field := actualVideoReferenceMode("wan2.6-i2v", videoReferenceModeIdentityReference)
	if mode != videoReferenceModeFirstFrame || field != "image" {
		t.Fatalf("Wan i2v identity request should be sent as first frame: mode=%q field=%q", mode, field)
	}
	if workflow := videoWorkflowForReference(mode); workflow != gen.VideoWorkflowImageToVideo {
		t.Fatalf("Wan i2v should use image-to-video workflow, got %q", workflow)
	}
	if !videoModelRequiresStartImage("wan2.6-i2v") {
		t.Fatal("Wan i2v should require a start image")
	}
	if videoModelRequiresStartImage("wan2.6-t2v") {
		t.Fatal("Wan t2v should not require a start image")
	}
}

func TestPrepareVideoReferenceAddsSafeFraming(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 300, 700))
	for y := 5; y < 695; y++ {
		for x := 5; x < 295; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: 20, G: 80, B: 190, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, src); err != nil {
		t.Fatal(err)
	}
	framed, err := prepareVideoReferenceForSafeFraming(encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(framed))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 1024 || img.Bounds().Dy() != 1024 {
		t.Fatalf("安全参考图尺寸=%v", img.Bounds())
	}
	minX, minY, maxX, maxY := 1024, 1024, -1, -1
	for y := 0; y < 1024; y++ {
		for x := 0; x < 1024; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r>>8 == 255 && g>>8 == 0 && b>>8 == 255 {
				continue
			}
			minX, minY = min(minX, x), min(minY, y)
			maxX, maxY = max(maxX, x), max(maxY, y)
		}
	}
	if maxX-minX+1 > 560 || maxY-minY+1 > 560 {
		t.Fatalf("角色未缩进中央安全区: bbox=(%d,%d)-(%d,%d)", minX, minY, maxX, maxY)
	}
	if minX < 120 || minY < 120 || maxX > 903 || maxY > 903 {
		t.Fatalf("角色四周留白不足: bbox=(%d,%d)-(%d,%d)", minX, minY, maxX, maxY)
	}
}

func TestReprocessVideoReturnsOneProcessedCandidatePerCurrentTimelineFrame(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", dir)

	sourceFrames := make([]*image.NRGBA, 12)
	for i := range sourceFrames {
		frame := image.NewNRGBA(image.Rect(0, 0, 64, 64))
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				frame.SetNRGBA(x, y, color.NRGBA{R: 255, B: 255, A: 255})
			}
		}
		left := 6 + i*3
		for y := 18; y < 50; y++ {
			for x := left; x < min(left+14, 63); x++ {
				frame.SetNRGBA(x, y, color.NRGBA{R: 30, G: 100, B: 220, A: 255})
			}
		}
		sourceFrames[i] = frame
	}
	video, err := sprite.EncodeMP4(context.Background(), sourceFrames, 12)
	if err != nil {
		t.Fatalf("encode local video fixture: %v", err)
	}

	args := GenerateStateArgs{
		RequestID:        "reprocess-candidate-test",
		StateID:          "walk",
		StyleKey:         "pixel",
		CellSize:         64,
		SafeMargin:       4,
		VideoSeconds:     1,
		FrameSamplerMode: sprite.FrameSamplerCandidatePool,
		AlignmentMode:    sprite.AlignmentPreserveSourceCanvas,
		FrameFitMode:     sprite.FrameFitContain,
		State: sprite.StateSpec{
			Name:   "walk",
			Frames: 4,
			FPS:    8,
			Action: "walk from left to right",
		},
	}
	trace := GenerationTrace{
		Pipeline:   pipelineVideoToFrames,
		Parameters: map[string]string{},
	}
	app := NewApp()
	res, err := app.processRawVideoStateInner(
		context.Background(),
		args,
		video,
		"fixture.mp4",
		nil,
		64,
		4,
		1,
		trace,
		nil,
		videoProcessProgress{},
	)
	if err != nil {
		t.Fatalf("reprocess local video: %v", err)
	}
	if len(res.RawExtractedFrames) == 0 {
		t.Fatal("expected a current-run raw candidate pool")
	}
	if len(res.CandidateFrames) != len(res.RawExtractedFrames) {
		t.Fatalf("timeline candidates should replace, not append to, the raw pool: processed=%d raw=%d", len(res.CandidateFrames), len(res.RawExtractedFrames))
	}
	if len(res.CandidateFrames) <= args.State.Frames {
		t.Fatalf("candidate pool should retain alternatives: candidates=%d final=%d", len(res.CandidateFrames), args.State.Frames)
	}
	seen := map[string]bool{}
	for _, candidate := range res.CandidateFrames {
		if candidate.Source != candidateSourceVideoProcessed {
			t.Fatalf("candidate is not replacement-ready: id=%s source=%s", candidate.ID, candidate.Source)
		}
		if seen[candidate.ID] {
			t.Fatalf("duplicate candidate id returned: %s", candidate.ID)
		}
		seen[candidate.ID] = true
	}
}
