package gen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"perfectpixel/internal/safemedia"
)

func TestNewFactory(t *testing.T) {
	cases := []struct {
		provider string
		wantType string
	}{
		{"gemini", "*gen.Client"},
		{"", "*gen.Client"},
		{"openai", "*gen.OpenAI"},
		{"openrouter", "*gen.OpenRouter"},
		{"fal", "*gen.Fal"},
		{"byteplus", "*gen.BytePlus"},
		{"zetatechs", "*gen.ZetaTechs"},
		{"volcengine", "*gen.Volcengine"},
	}
	for _, c := range cases {
		p, err := New(c.provider, "test-key", "")
		if err != nil {
			t.Fatalf("[%s] 팩토리 오류: %v", c.provider, err)
		}
		if got := typeName(p); got != c.wantType {
			t.Fatalf("[%s] 타입 오류: got %s want %s", c.provider, got, c.wantType)
		}
	}
	if _, err := New("unknown", "k", ""); err == nil {
		t.Fatal("알 수 없는 프로바이더는 오류여야 합니다")
	}
}

func typeName(v any) string {
	switch v.(type) {
	case *Client:
		return "*gen.Client"
	case *OpenAI:
		return "*gen.OpenAI"
	case *OpenRouter:
		return "*gen.OpenRouter"
	case *Fal:
		return "*gen.Fal"
	case *BytePlus:
		return "*gen.BytePlus"
	case *ZetaTechs:
		return "*gen.ZetaTechs"
	case *Volcengine:
		return "*gen.Volcengine"
	default:
		return "?"
	}
}

func TestDefaultModelFor(t *testing.T) {
	if DefaultModelFor("gemini") != DefaultModel {
		t.Fatal("gemini 기본 모델 오류")
	}
	if DefaultModelFor("openai") != "gpt-image-2" {
		t.Fatal("openai 기본 모델 오류")
	}
	if DefaultModelFor("openrouter") != "google/gemini-3-pro-image-preview" {
		t.Fatal("openrouter 기본 모델 오류")
	}
	if DefaultModelFor("fal") != "fal-ai/nano-banana-pro" {
		t.Fatal("fal 기본 모델 오류")
	}
	if DefaultModelFor("byteplus") != "seedream-4-0-250828" {
		t.Fatal("byteplus 기본 모델 오류")
	}
	if DefaultModelFor("zetatechs") != "gpt-image-2" {
		t.Fatal("zetatechs 기본 모델 오류")
	}
}

func TestModelsFor(t *testing.T) {
	for _, p := range SupportedProviders {
		models := ModelsFor(p)
		if len(models) == 0 {
			if SupportsVideo(p) {
				continue
			}
			t.Fatalf("[%s] image model list is empty", p)
		}
		// 최신(맨 앞) 모델은 기본 모델과 일치해야 합니다
		if models[0] != DefaultModelFor(p) {
			t.Fatalf("[%s] 최신 모델이 기본 모델과 다릅니다: %s != %s", p, models[0], DefaultModelFor(p))
		}
	}
	if ModelsFor("unknown") != nil {
		t.Fatal("알 수 없는 프로바이더는 nil이어야 합니다")
	}
	geminiModels := ModelsFor(ProviderGemini)
	for _, want := range []string{"gemini-3.1-flash-image-preview", "gemini-3.1-flash-image", "gemini-2.5-flash-image"} {
		if !hasModel(geminiModels, want) {
			t.Fatalf("Gemini 모델 목록에 %s가 없습니다", want)
		}
	}
	zetaModels := ModelsFor(ProviderZetaTechs)
	for _, want := range []string{"gpt-image-2", "gpt-image-1.5", "gpt-image-1-mini", "gemini-3.1-flash-image-preview"} {
		if !hasModel(zetaModels, want) {
			t.Fatalf("zetatechs 모델 목록에 %s가 없습니다", want)
		}
	}
}

func TestVideoModelsFor(t *testing.T) {
	if DefaultVideoModelFor(ProviderZetaTechs) != "wan2.6-t2v" {
		t.Fatal("ZetaTechs 기본 비디오 모델 오류")
	}
	if !SupportsVideo(ProviderZetaTechs) {
		t.Fatal("ZetaTechs는 비디오를 지원해야 합니다")
	}
	if SupportsVideo(ProviderGemini) {
		t.Fatal("Gemini 이미지 provider는 비디오를 지원하지 않아야 합니다")
	}
	models := VideoModelsFor(ProviderZetaTechs)
	for _, want := range []string{"wan2.6-t2v", "wan2.6-i2v", "wan2.5-t2v-preview", "wanx2.1-t2v-turbo"} {
		if !hasModel(models, want) {
			t.Fatalf("ZetaTechs 검증 완료 비디오 모델 목록에 %s가 없습니다", want)
		}
	}
	for _, notShown := range []string{"sora-2", "kling-v2-master", "doubao-seedance-2-0-260128", "wan2.2-kf2v-flash", "wanx2.1-vace-plus"} {
		if hasModel(models, notShown) {
			t.Fatalf("검증 실패/미검증 모델이 제품 목록에 노출됨: %s", notShown)
		}
	}
}

func TestZetaTechsVerifiedVideoModelFilterAndShowcases(t *testing.T) {
	dynamic := []string{
		"wan2.2-animate-mix",
		"wan2.2-i2v-plus",
		"wan2.6-t2v",
		"wanx2.1-vace-plus",
		"gpt-image-2",
	}
	got := FilterVerifiedVideoModels(ProviderZetaTechs, dynamic)
	want := []string{"wan2.6-t2v", "wan2.2-i2v-plus"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("검증 모델 필터 오류: got %v want %v", got, want)
	}
	showcases := VideoModelShowcasesForModels(ProviderZetaTechs, got)
	if len(showcases) != len(want) {
		t.Fatalf("쇼케이스 수 오류: got %d want %d", len(showcases), len(want))
	}
	if showcases["wan2.6-t2v"].SampleURL != "/video-model-samples/wan2.6-t2v.mp4" {
		t.Fatalf("쇼케이스 샘플 URL 오류: %+v", showcases["wan2.6-t2v"])
	}
	if _, ok := showcases["wan2.2-animate-mix"]; ok {
		t.Fatal("실패한 모델이 쇼케이스에 포함되었습니다")
	}
}

func TestVolcengineVerifiedVideoModelFilterAndShowcase(t *testing.T) {
	dynamic := []string{"doubao-seedance-2-0-mini-260615", "doubao-seedance-unverified"}
	got := FilterVerifiedVideoModels(ProviderVolcengine, dynamic)
	if strings.Join(got, ",") != "doubao-seedance-2-0-mini-260615" {
		t.Fatalf("Volcengine verified model filter mismatch: %v", got)
	}
	showcases := VideoModelShowcasesForModels(ProviderVolcengine, got)
	sample, ok := showcases["doubao-seedance-2-0-mini-260615"]
	if !ok || sample.SampleURL != "/video-model-samples/doubao-seedance-2-0-mini-260615.mp4" || sample.Workflow != VideoWorkflowReferenceToVideo {
		t.Fatalf("Volcengine showcase mismatch: %#v", sample)
	}
}

func TestVideoModelCapabilityFor(t *testing.T) {
	cases := []struct {
		model      string
		family     string
		refImage   bool
		startEnd   bool
		metadata   bool
		wantTag    string
		notWantTag string
	}{
		{"sora-2", "Sora", true, false, false, "openai_video", ""},
		{"kling-v2-master", "Kling", true, true, true, "start_end_frames", ""},
		{"jimeng_vgfm_i2v_l20", "Jimeng", true, false, true, "multi_image_reference", ""},
		{"viduq1", "Vidu", true, false, true, "image_to_video", ""},
		{"doubao-seedance-1-0-lite-t2v-250428", "Seedance", false, false, true, "text_to_video", "reference_image"},
		{"doubao-seedance-1-0-lite-i2v-250428", "Seedance", true, false, true, "first_frame_reference", ""},
		{"doubao-seedance-2-0-mini-260615", "Seedance", true, true, true, "identity_reference", ""},
		{"wan2.2-kf2v-flash", "Wan", true, true, true, "keyframe_to_video", ""},
		{"wan2.2-s2v", "Wan", true, false, true, "reference_audio", ""},
		{"wan2.6-t2v", "Wan", false, false, true, "text_to_video", "reference_image"},
	}
	for _, tt := range cases {
		t.Run(tt.model, func(t *testing.T) {
			got := VideoModelCapabilityFor(tt.model)
			if got.Family != tt.family {
				t.Fatalf("family mismatch: got %q want %q", got.Family, tt.family)
			}
			if got.SupportsReferenceImage != tt.refImage {
				t.Fatalf("ref image mismatch: got %v want %v (%+v)", got.SupportsReferenceImage, tt.refImage, got)
			}
			if got.SupportsStartEndFrames != tt.startEnd {
				t.Fatalf("start/end mismatch: got %v want %v (%+v)", got.SupportsStartEndFrames, tt.startEnd, got)
			}
			if got.SupportsMetadata != tt.metadata {
				t.Fatalf("metadata mismatch: got %v want %v (%+v)", got.SupportsMetadata, tt.metadata, got)
			}
			if tt.wantTag != "" && !hasModel(got.Tags, tt.wantTag) {
				t.Fatalf("missing tag %q in %+v", tt.wantTag, got.Tags)
			}
			if tt.notWantTag != "" && hasModel(got.Tags, tt.notWantTag) {
				t.Fatalf("unexpected tag %q in %+v", tt.notWantTag, got.Tags)
			}
		})
	}
}

func hasModel(models []string, want string) bool {
	for _, model := range models {
		if model == want {
			return true
		}
	}
	return false
}

func TestBPSizeFor(t *testing.T) {
	if got := bpSizeFor("1:1"); got != "4096x4096" && got != "2048x2048" {
		t.Fatalf("정사각 크기 오류: %s", got)
	}
	if got := bpSizeFor(""); got != "2048x2048" {
		t.Fatalf("빈 종횡비 기본 크기 오류: %s", got)
	}
	// 와이드 스트립: 긴 변 4096, 짧은 변은 최소 1280으로 클램프
	if got := bpSizeFor("7:1"); got != "4096x1280" {
		t.Fatalf("와이드 크기 클램프 오류: %s", got)
	}
}

func TestOpenAISizeFor(t *testing.T) {
	if got := openAISizeFor("1:1"); got != "1024x1024" {
		t.Fatalf("정사각 크기 오류: %s", got)
	}
	if got := openAISizeFor("16:9"); got != "1792x1008" {
		t.Fatalf("16:9 크기 오류: %s", got)
	}
	if got := openAISizeFor("21:9"); got != "1792x768" {
		t.Fatalf("21:9 크기 오류: %s", got)
	}
}

func TestZetaTechsSizeFor(t *testing.T) {
	if got := zetaTechsSizeFor("1:1"); got != "1024x1024" {
		t.Fatalf("정사각 크기 오류: %s", got)
	}
	if got := zetaTechsSizeFor("21:9"); got != "1536x1024" {
		t.Fatalf("가로 크기 오류: %s", got)
	}
	if got := zetaTechsSizeFor("3:4"); got != "1024x1536" {
		t.Fatalf("세로 크기 오류: %s", got)
	}
}

func TestZetaTechsWanSpecialModelsIncludeOfficialInputShape(t *testing.T) {
	cases := []struct {
		name      string
		req       VideoRequest
		wantInput []string
	}{
		{
			name: "animate uses image and driver video input",
			req: VideoRequest{
				Model:         "wan2.2-animate-move",
				Prompt:        "animate",
				Workflow:      VideoWorkflowMotionTransfer,
				ReferenceMode: VideoReferenceModeFirstFrame,
				MediaSlots: map[string][]VideoMediaInput{
					VideoSlotStartImage:  {{URL: "https://example.test/start.jpg"}},
					VideoSlotDriverVideo: {{URL: "https://example.test/driver.mp4"}},
				},
			},
			wantInput: []string{"image_url", "video_url"},
		},
		{
			name: "keyframe uses first and last frame input",
			req: VideoRequest{
				Model:         "wan2.2-kf2v-flash",
				Prompt:        "animate",
				Workflow:      VideoWorkflowFirstLastToVideo,
				ReferenceMode: VideoReferenceModeStartEndFrame,
				MediaSlots: map[string][]VideoMediaInput{
					VideoSlotStartImage: {{URL: "https://example.test/first.png"}},
					VideoSlotEndImage:   {{URL: "https://example.test/last.png"}},
				},
			},
			wantInput: []string{"prompt", "first_frame_url", "last_frame_url"},
		},
		{
			name: "s2v uses image and audio input",
			req: VideoRequest{
				Model:         "wan2.2-s2v",
				Prompt:        "speak",
				Workflow:      VideoWorkflowImageToVideo,
				ReferenceMode: VideoReferenceModeFirstFrame,
				MediaSlots: map[string][]VideoMediaInput{
					VideoSlotStartImage:      {{URL: "https://example.test/person.jpg"}},
					VideoSlotReferenceAudios: {{URL: "https://example.test/audio.mp3"}},
				},
			},
			wantInput: []string{"prompt", "image_url", "audio_url"},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			body := buildVideoGenerationBody(tt.req)
			input, ok := body["input"].(map[string]any)
			if !ok {
				t.Fatalf("Wan official input missing: %#v", body)
			}
			for _, key := range tt.wantInput {
				if _, ok := input[key]; !ok {
					t.Fatalf("missing input.%s in %#v", key, body)
				}
			}
			if _, ok := body["parameters"].(map[string]any); !ok {
				t.Fatalf("Wan official parameters missing: %#v", body)
			}
		})
	}
}

func TestZetaTechsValidateKey(t *testing.T) {
	if err := NewZetaTechs("short", "").ValidateKey(context.Background()); err == nil {
		t.Fatal("짧은 ZetaTechs 키는 오류여야 합니다")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("모델 확인 경로 오류: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-zeta-test-token" {
			t.Fatalf("인증 헤더 오류: %s", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()
	if err := NewZetaTechsWithBaseURL("sk-zeta-test-token", "", srv.URL).ValidateKey(context.Background()); err != nil {
		t.Fatalf("ZetaTechs 키 형식 검증 실패: %v", err)
	}
}

func TestZetaTechsGenerateUsesConfiguredGatewayAndOmitsFormatParameters(t *testing.T) {
	payload := []byte("png")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("생성 경로 오류: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-zeta-test-token" {
			t.Fatalf("인증 헤더 오류: %s", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("요청 JSON 파싱 실패: %v", err)
		}
		if _, ok := body["response_format"]; ok {
			t.Fatal("ZetaTechs 图片生成请求不应发送 response_format")
		}
		if _, ok := body["output_format"]; ok {
			t.Fatal("ZetaTechs 图片生成请求不应发送 output_format")
		}
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(payload) + `"}]}`))
	}))
	defer srv.Close()

	c := NewZetaTechsWithBaseURL("sk-zeta-test-token", "gpt-image-2", srv.URL)
	got, err := c.GenerateImage(context.Background(), "draw test", nil, "1:1")
	if err != nil {
		t.Fatalf("ZetaTechs 생성 실패: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("응답 이미지 오류: %q", got)
	}
}

func TestZetaTechsEditOmitsUnsupportedResponseFormat(t *testing.T) {
	payload := []byte("png")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("편집 경로 오류: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-zeta-test-token" {
			t.Fatalf("인증 헤더 오류: %s", r.Header.Get("Authorization"))
		}
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			t.Fatalf("multipart 파싱 실패: %v", err)
		}
		if _, ok := r.MultipartForm.Value["response_format"]; ok {
			t.Fatal("ZetaTechs 이미지 편집 요청에는 response_format을 보내면 안 됩니다")
		}
		if _, ok := r.MultipartForm.Value["output_format"]; ok {
			t.Fatal("ZetaTechs 이미지 편집 요청에는 output_format을 보내면 안 됩니다")
		}
		if r.MultipartForm.Value["model"][0] != "gpt-image-2" {
			t.Fatalf("모델 오류: %#v", r.MultipartForm.Value["model"])
		}
		if len(r.MultipartForm.File["image[]"]) != 1 {
			t.Fatalf("참조 이미지 누락: %#v", r.MultipartForm.File)
		}
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(payload) + `"}]}`))
	}))
	defer srv.Close()

	c := NewZetaTechsWithBaseURL("sk-zeta-test-token", "gpt-image-2", srv.URL)
	got, err := c.GenerateImage(context.Background(), "edit test", [][]byte{[]byte("ref png")}, "1:1")
	if err != nil {
		t.Fatalf("ZetaTechs 편집 실패: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("응답 이미지 오류: %q", got)
	}
}

func TestZetaTechsValidateKeyReportsGatewayMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid token"}}`))
	}))
	defer srv.Close()
	err := NewZetaTechsWithBaseURL("sk-zeta-test-token", "", srv.URL).ValidateKey(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Invalid token") {
		t.Fatalf("401 오류 메시지 누락: %v", err)
	}
}

func TestZetaTechsOpenAIVideoLifecycle(t *testing.T) {
	video := []byte("mp4-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-zeta-test-token" {
			t.Fatalf("인증 헤더 오류: %s", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/videos":
			if err := r.ParseMultipartForm(8 << 20); err != nil {
				t.Fatalf("multipart 파싱 실패: %v", err)
			}
			if r.MultipartForm.Value["model"][0] != "sora-2" {
				t.Fatalf("비디오 모델 오류: %#v", r.MultipartForm.Value["model"])
			}
			if r.MultipartForm.Value["seconds"][0] != "4" {
				t.Fatalf("seconds 오류: %#v", r.MultipartForm.Value["seconds"])
			}
			if len(r.MultipartForm.File["input_reference"]) != 1 {
				t.Fatalf("참조 이미지 누락: %#v", r.MultipartForm.File)
			}
			_, _ = w.Write([]byte(`{"id":"video_123","status":"queued","progress":10}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/videos/video_123":
			_, _ = w.Write([]byte(`{"id":"video_123","status":"completed","progress":100}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/videos/video_123/content":
			_, _ = w.Write(video)
		default:
			t.Fatalf("unexpected video request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewZetaTechsWithBaseURL("sk-zeta-test-token", "gpt-image-2", srv.URL)
	task, err := c.GenerateVideo(context.Background(), VideoRequest{
		Model:          "sora-2",
		Prompt:         "animate",
		ReferenceImage: []byte("png"),
		Seconds:        4,
		FPS:            12,
	})
	if err != nil {
		t.Fatalf("비디오 생성 요청 실패: %v", err)
	}
	if task.ID != "video_123" || task.Status != VideoStatusQueued || task.Progress != 10 {
		t.Fatalf("비디오 생성 응답 파싱 오류: %+v", task)
	}
	task, err = c.PollVideo(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("비디오 폴링 실패: %v", err)
	}
	if !task.Done() {
		t.Fatalf("비디오 완료 상태 파싱 실패: %+v", task)
	}
	got, err := c.DownloadVideo(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("비디오 다운로드 실패: %v", err)
	}
	if string(got) != string(video) {
		t.Fatalf("비디오 다운로드 바이트 오류: %q", got)
	}
}

func TestZetaTechsVideoGenerationEndpointLifecycle(t *testing.T) {
	video := []byte("video-generation-mp4")
	var baseURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/video/generations":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("JSON 파싱 실패: %v", err)
			}
			if body["model"] != "wan2.6-i2v" {
				t.Fatalf("비디오 모델 오류: %#v", body["model"])
			}
			if !strings.HasPrefix(body["image"].(string), "data:image/png;base64,") {
				t.Fatalf("참조 이미지 data URL 누락: %#v", body["image"])
			}
			_, _ = w.Write([]byte(`{"data":{"task_id":"abc","status":"processing"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/video/generations/abc":
			_, _ = w.Write([]byte(`{"data":{"task_id":"abc","status":"succeeded","url":"` + baseURL + `/video.mp4"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/video.mp4":
			_, _ = w.Write(video)
		default:
			t.Fatalf("unexpected video generation request: %s %s", r.Method, r.URL.Path)
		}
	}))
	baseURL = srv.URL
	defer srv.Close()

	c := NewZetaTechsWithBaseURL("sk-zeta-test-token", "gpt-image-2", srv.URL)
	task, err := c.GenerateVideo(context.Background(), VideoRequest{
		Model:          "wan2.6-i2v",
		Prompt:         "animate",
		ReferenceImage: []byte("png"),
		ReferenceMode:  VideoReferenceModeFirstFrame,
		Seconds:        4,
		FPS:            12,
	})
	if err != nil {
		t.Fatalf("비디오 생성 요청 실패: %v", err)
	}
	if task.ID != "video-generation:abc" || task.Status != VideoStatusProcessing {
		t.Fatalf("비디오 task 파싱 오류: %+v", task)
	}
	task, err = c.PollVideo(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("비디오 폴링 실패: %v", err)
	}
	if !task.Done() || task.URL == "" {
		t.Fatalf("비디오 완료 응답 파싱 실패: %+v", task)
	}
	got, err := c.DownloadVideo(context.Background(), task.URL)
	if err != nil {
		t.Fatalf("비디오 다운로드 실패: %v", err)
	}
	if string(got) != string(video) {
		t.Fatalf("비디오 다운로드 바이트 오류: %q", got)
	}
}

func TestZetaTechsWanVideoGenerationUsesProviderResolutionShape(t *testing.T) {
	cases := []struct {
		name       string
		model      string
		workflow   string
		wantSize   string
		wantRes    string
		wantNoSize bool
	}{
		{
			name:     "t2v size uses star separator",
			model:    "wan2.6-t2v",
			workflow: VideoWorkflowTextToVideo,
			wantSize: "960*960",
		},
		{
			name:       "i2v resolution is top level",
			model:      "wan2.6-i2v",
			workflow:   VideoWorkflowImageToVideo,
			wantRes:    "720P",
			wantNoSize: true,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			body := buildVideoGenerationBody(VideoRequest{
				Model:       tt.model,
				Prompt:      "animate",
				Workflow:    tt.workflow,
				Seconds:     4,
				FPS:         12,
				Size:        "1024x1024",
				AspectRatio: "1:1",
				Resolution:  "720p",
			})
			if tt.wantSize != "" && body["size"] != tt.wantSize {
				t.Fatalf("size mismatch: got %#v want %s body=%#v", body["size"], tt.wantSize, body)
			}
			if tt.wantNoSize {
				if _, ok := body["size"]; ok {
					t.Fatalf("i2v request must not send image size as video resolution: %#v", body)
				}
			}
			if tt.wantRes != "" && body["resolution"] != tt.wantRes {
				t.Fatalf("resolution mismatch: got %#v want %s body=%#v", body["resolution"], tt.wantRes, body)
			}
		})
	}
}

func TestZetaTechsVideoGenerationFailureStatusParsesTerminal(t *testing.T) {
	got := parseVideoResult([]byte(`{"data":{"status":"NOT_START","progress":"100%","fail_reason":"task failed","data":{"output":{"task_status":"FAILED","message":"inner failed"}}}}`))
	if !got.Failed() {
		t.Fatalf("FAILURE status must be terminal failed: %+v", got)
	}
	if got.Progress != 100 {
		t.Fatalf("string percentage progress was not parsed: %+v", got)
	}
	if got.Message != "task failed" {
		t.Fatalf("failure reason missing: %+v", got)
	}
}

func TestZetaTechsVideoGenerationOmitsReferenceForTextToVideoModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/video/generations" {
			t.Fatalf("unexpected video generation request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("JSON 파싱 실패: %v", err)
		}
		if body["model"] != "wan2.6-t2v" {
			t.Fatalf("비디오 모델 오류: %#v", body["model"])
		}
		if _, ok := body["image"]; ok {
			t.Fatalf("t2v 모델에는 참조 이미지 필드를 보내면 안 됩니다: %#v", body)
		}
		if _, ok := body["duration"]; ok {
			t.Fatalf("Wan video/generations should not send custom duration: %#v", body)
		}
		if _, ok := body["fps"]; ok {
			t.Fatalf("Wan video/generations should not send custom fps: %#v", body)
		}
		if body["size"] != "960*960" {
			t.Fatalf("Wan t2v should use an upstream-accepted smoke size, got %#v in %#v", body["size"], body)
		}
		_, _ = w.Write([]byte(`{"data":{"task_id":"abc","status":"processing"}}`))
	}))
	defer srv.Close()

	c := NewZetaTechsWithBaseURL("sk-zeta-test-token", "gpt-image-2", srv.URL)
	if _, err := c.GenerateVideo(context.Background(), VideoRequest{
		Model:          "wan2.6-t2v",
		Prompt:         "animate",
		ReferenceImage: []byte("png"),
		ReferenceMode:  VideoReferenceModeFirstFrame,
		Seconds:        4,
		FPS:            12,
	}); err != nil {
		t.Fatalf("비디오 생성 요청 실패: %v", err)
	}
}

func TestZetaTechsWanReferenceVideoUsesTopLevelReferenceFields(t *testing.T) {
	body := buildVideoGenerationBody(VideoRequest{
		Model:         "wan2.6-r2v",
		Prompt:        "animate",
		Workflow:      VideoWorkflowReferenceToVideo,
		ReferenceMode: VideoReferenceModeNone,
		MediaSlots: map[string][]VideoMediaInput{
			VideoSlotReferenceVideos: {{URL: "https://example.test/reference.mp4", MIME: "video/mp4"}},
		},
		Resolution: "720p",
	})
	for _, key := range []string{"reference_video_urls", "reference_urls"} {
		urls, ok := body[key].([]string)
		if !ok || len(urls) != 1 || urls[0] != "https://example.test/reference.mp4" {
			t.Fatalf("Wan reference video field %s missing: %#v", key, body)
		}
	}
	metadata, ok := body["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("Wan reference video metadata missing: %#v", body)
	}
	for _, key := range []string{"reference_video_urls", "reference_urls"} {
		urls, ok := metadata[key].([]string)
		if !ok || len(urls) != 1 || urls[0] != "https://example.test/reference.mp4" {
			t.Fatalf("Wan reference video metadata field %s missing: %#v", key, body)
		}
	}
}

func TestZetaTechsVideoGenerationPollToleratesEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/video/generations/task-empty" {
			t.Fatalf("unexpected poll request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`null`))
	}))
	defer srv.Close()

	c := NewZetaTechsWithBaseURL("sk-zeta-test-token", "gpt-image-2", srv.URL)
	task, err := c.PollVideo(context.Background(), "video-generation:task-empty")
	if err != nil {
		t.Fatalf("empty poll body should be treated as retryable queued state: %v", err)
	}
	if task.ID != "video-generation:task-empty" || task.Status != VideoStatusQueued {
		t.Fatalf("unexpected fallback task: %+v", task)
	}
}

func TestZetaTechsVideoGenerationReferenceModeNoneOmitsImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/video/generations" {
			t.Fatalf("unexpected video generation request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("JSON 파싱 실패: %v", err)
		}
		if body["model"] != "doubao-seedance-1-0-lite-i2v-250428" {
			t.Fatalf("비디오 모델 오류: %#v", body["model"])
		}
		if _, ok := body["image"]; ok {
			t.Fatalf("none 전략에서는 image 필드를 보내면 안 됩니다: %#v", body)
		}
		_, _ = w.Write([]byte(`{"data":{"task_id":"abc","status":"processing"}}`))
	}))
	defer srv.Close()

	c := NewZetaTechsWithBaseURL("sk-zeta-test-token", "gpt-image-2", srv.URL)
	if _, err := c.GenerateVideo(context.Background(), VideoRequest{
		Model:          "doubao-seedance-1-0-lite-i2v-250428",
		Prompt:         "animate",
		ReferenceImage: []byte("png"),
		ReferenceMode:  VideoReferenceModeNone,
		Seconds:        4,
		FPS:            12,
	}); err != nil {
		t.Fatalf("비디오 생성 요청 실패: %v", err)
	}
}

func TestZetaTechsVideoGenerationReferenceModeAutoOmitsFirstFrameImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/video/generations" {
			t.Fatalf("unexpected video generation request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("JSON 파싱 실패: %v", err)
		}
		if _, ok := body["image"]; ok {
			t.Fatalf("auto 전략에서는 video/generations image 필드를 암묵적으로 보내면 안 됩니다: %#v", body)
		}
		_, _ = w.Write([]byte(`{"data":{"task_id":"abc","status":"processing"}}`))
	}))
	defer srv.Close()

	c := NewZetaTechsWithBaseURL("sk-zeta-test-token", "gpt-image-2", srv.URL)
	if _, err := c.GenerateVideo(context.Background(), VideoRequest{
		Model:          "doubao-seedance-1-0-lite-i2v-250428",
		Prompt:         "animate",
		ReferenceImage: []byte("png"),
		ReferenceMode:  VideoReferenceModeAuto,
		Seconds:        4,
		FPS:            12,
	}); err != nil {
		t.Fatalf("비디오 생성 요청 실패: %v", err)
	}
}

func TestZetaTechsVideoGenerationUsesExplicitStartImageSlot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/video/generations" {
			t.Fatalf("unexpected video generation request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("JSON 파싱 실패: %v", err)
		}
		if !strings.HasPrefix(body["image"].(string), "data:image/png;base64,") {
			t.Fatalf("start_image 슬롯이 image 필드로 전송되지 않았습니다: %#v", body)
		}
		_, _ = w.Write([]byte(`{"data":{"task_id":"abc","status":"processing"}}`))
	}))
	defer srv.Close()

	c := NewZetaTechsWithBaseURL("sk-zeta-test-token", "gpt-image-2", srv.URL)
	if _, err := c.GenerateVideo(context.Background(), VideoRequest{
		Model:         "doubao-seedance-1-0-lite-i2v-250428",
		Prompt:        "animate",
		ReferenceMode: VideoReferenceModeNone,
		MediaSlots: map[string][]VideoMediaInput{
			VideoSlotStartImage: {{Data: []byte("png"), MIME: "image/png"}},
		},
		Seconds: 4,
		FPS:     12,
	}); err != nil {
		t.Fatalf("비디오 생성 요청 실패: %v", err)
	}
}

func TestZetaTechsVideoGenerationDoesNotTreatReferenceSlotAsFirstFrame(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/video/generations" {
			t.Fatalf("unexpected video generation request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("JSON 파싱 실패: %v", err)
		}
		if _, ok := body["image"]; ok {
			t.Fatalf("reference_images 슬롯은 image(첫 프레임) 필드로 전송되면 안 됩니다: %#v", body)
		}
		_, _ = w.Write([]byte(`{"data":{"task_id":"abc","status":"processing"}}`))
	}))
	defer srv.Close()

	c := NewZetaTechsWithBaseURL("sk-zeta-test-token", "gpt-image-2", srv.URL)
	if _, err := c.GenerateVideo(context.Background(), VideoRequest{
		Model:         "doubao-seedance-1-0-lite-i2v-250428",
		Prompt:        "animate",
		ReferenceMode: VideoReferenceModeIdentityReference,
		MediaSlots: map[string][]VideoMediaInput{
			VideoSlotReferenceImages: {{Data: []byte("png"), MIME: "image/png"}},
		},
		Seconds: 4,
		FPS:     12,
	}); err != nil {
		t.Fatalf("비디오 생성 요청 실패: %v", err)
	}
}

func TestZetaTechsVideoGenerationSendsEndFrameInMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/video/generations" {
			t.Fatalf("unexpected video generation request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("JSON 파싱 실패: %v", err)
		}
		if _, ok := body["image"]; !ok {
			t.Fatalf("start_image 누락: %#v", body)
		}
		metadata, ok := body["metadata"].(map[string]any)
		if !ok || !strings.HasPrefix(metadata["image_tail"].(string), "data:image/png;base64,") {
			t.Fatalf("end_image가 metadata.image_tail로 전송되지 않았습니다: %#v", body)
		}
		_, _ = w.Write([]byte(`{"data":{"task_id":"abc","status":"processing"}}`))
	}))
	defer srv.Close()

	c := NewZetaTechsWithBaseURL("sk-zeta-test-token", "gpt-image-2", srv.URL)
	if _, err := c.GenerateVideo(context.Background(), VideoRequest{
		Model:         "kling-v2-master",
		Prompt:        "animate",
		ReferenceMode: VideoReferenceModeStartEndFrame,
		MediaSlots: map[string][]VideoMediaInput{
			VideoSlotStartImage: {{Data: []byte("start"), MIME: "image/png"}},
			VideoSlotEndImage:   {{Data: []byte("end"), MIME: "image/png"}},
		},
		Seconds: 4,
		FPS:     12,
	}); err != nil {
		t.Fatalf("비디오 생성 요청 실패: %v", err)
	}
}

func TestZetaTechsModelGenerationLifecycle(t *testing.T) {
	video := []byte("model-generation-mp4")
	var baseURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/model/v1/generation":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("JSON 파싱 실패: %v", err)
			}
			if body["model"] != "spark_dance_v2_0" {
				t.Fatalf("모델 오류: %#v", body["model"])
			}
			input := body["input"].([]any)[0].(map[string]any)["params"].(map[string]any)
			content := input["content"].([]any)
			if len(content) < 2 || content[1].(map[string]any)["role"] != "first_frame" {
				t.Fatalf("content multimodal 첫 프레임 역할 누락: %#v", input)
			}
			_, _ = w.Write([]byte(`{"taskId":"task-1","status":"queued"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/model/v1/generation/task/task-1":
			_, _ = w.Write([]byte(`{"task_status":"success","output":{"video_url":"` + baseURL + `/video.mp4"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/video.mp4":
			_, _ = w.Write(video)
		default:
			t.Fatalf("unexpected model generation request: %s %s", r.Method, r.URL.Path)
		}
	}))
	baseURL = srv.URL
	defer srv.Close()

	c := NewZetaTechsWithBaseURL("sk-zeta-test-token", "gpt-image-2", srv.URL)
	task, err := c.GenerateVideo(context.Background(), VideoRequest{
		Model:         "spark_dance_v2_0",
		Prompt:        "animate",
		ReferenceMode: VideoReferenceModeFirstFrame,
		MediaSlots: map[string][]VideoMediaInput{
			VideoSlotStartImage: {{Data: []byte("png"), MIME: "image/png"}},
		},
		Seconds:         4,
		FPS:             12,
		ProviderAdapter: "vtrix.content_multimodal",
	})
	if err != nil {
		t.Fatalf("모델 생성 요청 실패: %v", err)
	}
	if task.ID != "model-generation:task-1" {
		t.Fatalf("모델 생성 task ID 파싱 실패: %+v", task)
	}
	task, err = c.PollVideo(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("모델 생성 폴링 실패: %v", err)
	}
	if !task.Done() || task.URL == "" {
		t.Fatalf("모델 생성 완료 응답 파싱 실패: %+v", task)
	}
	got, err := c.DownloadVideo(context.Background(), task.URL)
	if err != nil {
		t.Fatalf("모델 생성 다운로드 실패: %v", err)
	}
	if string(got) != string(video) {
		t.Fatalf("모델 생성 다운로드 바이트 오류: %q", got)
	}
}

func TestZetaTechsVideoGenerationPrefersTaskIDOverGenericID(t *testing.T) {
	polled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/video/generations":
			_, _ = w.Write([]byte(`{"id":"request-log-id","data":{"id":"inner-generic-id","task_id":"real-task-123","status":"processing"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/video/generations/real-task-123":
			polled = true
			_, _ = w.Write([]byte(`{"data":{"task_id":"real-task-123","status":"succeeded","url":"https://example.test/video.mp4"}}`))
		default:
			t.Fatalf("unexpected video generation request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewZetaTechsWithBaseURL("sk-zeta-test-token", "gpt-image-2", srv.URL)
	task, err := c.GenerateVideo(context.Background(), VideoRequest{
		Model:          "wan2.6-i2v",
		Prompt:         "animate",
		ReferenceImage: []byte("png"),
		ReferenceMode:  VideoReferenceModeFirstFrame,
		Seconds:        4,
		FPS:            12,
	})
	if err != nil {
		t.Fatalf("비디오 생성 요청 실패: %v", err)
	}
	if task.ID != "video-generation:real-task-123" {
		t.Fatalf("task_id 우선 파싱 실패: %+v", task)
	}
	task, err = c.PollVideo(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("비디오 폴링 실패: %v", err)
	}
	if !polled {
		t.Fatal("실제 task_id로 폴링하지 않았습니다")
	}
	if !task.Done() || task.URL == "" {
		t.Fatalf("비디오 완료 응답 파싱 실패: %+v", task)
	}
}

func TestZetaTechsVideoGenerationEndpointModelRouting(t *testing.T) {
	for _, model := range []string{
		"kling-v2-master",
		"jimeng_vgfm_i2v_l20",
		"viduq1",
		"doubao-seedance-2-0-260128",
		"doubao-seedance-1-0-lite-i2v-250428",
		"wan2.6-i2v",
		"wanx2.1-t2v-turbo",
	} {
		if !usesVideoGenerationsEndpoint(model) {
			t.Fatalf("%s 모델은 /v1/video/generations 라우팅이어야 합니다", model)
		}
	}
	if usesVideoGenerationsEndpoint("sora-2") {
		t.Fatal("sora-2는 OpenAI 호환 /v1/videos 라우팅이어야 합니다")
	}
}

func TestZetaTechsListModelsParsesV1Models(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("모델 목록 경로 오류: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-zeta-test-token" {
			t.Fatalf("인증 헤더 오류: %s", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"doubao-seedance-2-0-260128","object":"model"},{"id":"wan2.6-i2v","object":"model"}]}`))
	}))
	defer srv.Close()

	models, err := NewZetaTechsWithBaseURL("sk-zeta-test-token", "", srv.URL).ListModels(context.Background())
	if err != nil {
		t.Fatalf("모델 목록 조회 실패: %v", err)
	}
	for _, want := range []string{"doubao-seedance-2-0-260128", "wan2.6-i2v"} {
		if !hasModel(models, want) {
			t.Fatalf("모델 목록에 %s가 없습니다: %v", want, models)
		}
	}
}

func TestParseZetaTechsAPIModelsShape(t *testing.T) {
	models := parseModelList([]byte(`{"success":true,"data":{"1":["gpt-image-2","wan2.6-i2v"],"2":["doubao-seedance-2-0-260128","wan2.6-i2v"]}}`))
	for _, want := range []string{"gpt-image-2", "wan2.6-i2v", "doubao-seedance-2-0-260128"} {
		if !hasModel(models, want) {
			t.Fatalf("모델 목록에 %s가 없습니다: %v", want, models)
		}
	}
	if countModel(models, "wan2.6-i2v") != 1 {
		t.Fatalf("중복 모델 제거 실패: %v", models)
	}
}

func TestZetaTechsModelClassification(t *testing.T) {
	models := []string{
		"gpt-image-2",
		"wan2.6-image",
		"wan2.6-t2v",
		"wan2.6-i2v",
		"wan2.2-t2i-plus",
		"doubao-seedance-2-0-260128",
	}
	video := ZetaTechsVideoModelsFrom(models)
	for _, want := range []string{"wan2.6-t2v", "wan2.6-i2v", "doubao-seedance-2-0-260128"} {
		if !hasModel(video, want) {
			t.Fatalf("비디오 모델 분류 누락: %s in %v", want, video)
		}
	}
	for _, notVideo := range []string{"gpt-image-2", "wan2.6-image", "wan2.2-t2i-plus"} {
		if hasModel(video, notVideo) {
			t.Fatalf("이미지 모델이 비디오 목록에 포함됨: %s in %v", notVideo, video)
		}
	}
	image := ZetaTechsImageModelsFrom(models)
	for _, want := range []string{"gpt-image-2", "wan2.6-image", "wan2.2-t2i-plus"} {
		if !hasModel(image, want) {
			t.Fatalf("이미지 모델 분류 누락: %s in %v", want, image)
		}
	}
}

func countModel(models []string, want string) int {
	count := 0
	for _, model := range models {
		if model == want {
			count++
		}
	}
	return count
}

func TestFalEndpoint(t *testing.T) {
	c := NewFal("k", "fal-ai/nano-banana")
	if got := c.endpoint(false); got != "https://fal.run/fal-ai/nano-banana" {
		t.Fatalf("기본 엔드포인트 오류: %s", got)
	}
	if got := c.endpoint(true); got != "https://fal.run/fal-ai/nano-banana/edit" {
		t.Fatalf("edit 엔드포인트 오류: %s", got)
	}
	// 이미 /edit이 붙은 모델은 중복 추가하지 않음
	c.Model = "fal-ai/nano-banana/edit"
	if got := c.endpoint(true); got != "https://fal.run/fal-ai/nano-banana/edit" {
		t.Fatalf("edit 중복 방지 실패: %s", got)
	}
}

func TestDecodeDataOrDownload(t *testing.T) {
	payload := []byte("hello-png-bytes")

	// data: URL 디코딩
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(payload)
	got, err := decodeDataOrDownload(http.DefaultClient, dataURL)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("data URL 디코딩 실패: %v %q", err, got)
	}

	// Remote output must be HTTPS and must not resolve to loopback/private IPs.
	var unsafeRequests atomic.Int32
	httpServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		unsafeRequests.Add(1)
	}))
	defer httpServer.Close()
	if _, err := decodeDataOrDownload(httpServer.Client(), httpServer.URL); !errors.Is(err, safemedia.ErrUnsafeRemoteURL) {
		t.Fatalf("HTTP output URL should be rejected, got %v", err)
	}

	privateTLSServer := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		unsafeRequests.Add(1)
	}))
	defer privateTLSServer.Close()
	if _, err := decodeDataOrDownload(privateTLSServer.Client(), privateTLSServer.URL); !errors.Is(err, safemedia.ErrUnsafeRemoteURL) {
		t.Fatalf("loopback HTTPS output URL should be rejected, got %v", err)
	}
	if got := unsafeRequests.Load(); got != 0 {
		t.Fatalf("unsafe output URL made %d network requests", got)
	}

	// 잘못된 data URL
	if _, err := decodeDataOrDownload(http.DefaultClient, "data:image/png;hex,00"); err == nil {
		t.Fatal("base64 누락 data URL은 오류여야 합니다")
	}
}

func TestOpenAIImageResultURLUsesSafeDownloader(t *testing.T) {
	requests := make(chan string, 1)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Method + " " + r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://127.0.0.1/private-result.png"}]}`))
	}))
	defer api.Close()

	client := NewOpenAI("sk-test", "gpt-image-1")
	client.generationEndpoint = api.URL + "/images"
	_, err := client.GenerateImageOnce(context.Background(), "test", nil, "1:1")
	if !errors.Is(err, safemedia.ErrUnsafeRemoteURL) {
		t.Fatalf("private provider image URL should be rejected, got %v", err)
	}
	if got := <-requests; got != "POST /images" {
		t.Fatalf("unexpected provider request: %s", got)
	}
}

func TestAspectHint(t *testing.T) {
	if h := aspectHint("1:1"); h != "Render on a square 1:1 canvas." {
		t.Fatalf("1:1 힌트 오류: %s", h)
	}
	if h := aspectHint("21:9"); h == "" || !contains(h, "21:9") {
		t.Fatalf("와이드 힌트 오류: %s", h)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
