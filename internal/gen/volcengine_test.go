package gen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildVolcengineVideoBodyUsesReferenceImage(t *testing.T) {
	data, err := buildVolcengineVideoBody(VideoRequest{
		Model:         "doubao-seedance-2-0-mini-260615",
		Prompt:        "fixed camera pixel sprite attack",
		ReferenceMode: VideoReferenceModeIdentityReference,
		MediaSlots: map[string][]VideoMediaInput{
			VideoSlotReferenceImages: {{Data: []byte("png"), MIME: "image/png"}},
		},
		Seconds:     2,
		Resolution:  "1080P",
		AspectRatio: "1:1",
	})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	if body["duration"] != float64(4) {
		t.Fatalf("Seedance 2.0 Mini minimum duration should be 4, got %#v", body["duration"])
	}
	if body["resolution"] != "720p" {
		t.Fatalf("Mini should downgrade unsupported 1080p, got %#v", body["resolution"])
	}
	content := body["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("expected text and one reference image, got %d", len(content))
	}
	image := content[1].(map[string]any)
	if image["role"] != "reference_image" {
		t.Fatalf("identity reference role mismatch: %#v", image["role"])
	}
	imageURL := image["image_url"].(map[string]any)["url"].(string)
	if !strings.HasPrefix(imageURL, "data:image/png;base64,") {
		t.Fatalf("reference image should be sent as data URL: %q", imageURL)
	}
}

func TestBuildVolcengineVideoBodyDoesNotDuplicateSlotWithoutPrompt(t *testing.T) {
	data, err := buildVolcengineVideoBody(VideoRequest{
		Model:          "doubao-seedance-2-0-mini-260615",
		ReferenceImage: []byte("legacy-png"),
		ReferenceMode:  VideoReferenceModeIdentityReference,
		MediaSlots: map[string][]VideoMediaInput{
			VideoSlotReferenceImages: {{Data: []byte("slot-png"), MIME: "image/png"}},
		},
		Seconds: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	content := body["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("expected exactly one reference image, got %d: %#v", len(content), content)
	}
}

func TestBuildVolcengineVideoBodyUpscalesSmallPNGReference(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	src.SetNRGBA(10, 10, color.NRGBA{R: 255, A: 255})
	var source bytes.Buffer
	if err := png.Encode(&source, src); err != nil {
		t.Fatal(err)
	}
	data, err := buildVolcengineVideoBody(VideoRequest{
		Model:         "doubao-seedance-2-0-mini-260615",
		Prompt:        "animate",
		ReferenceMode: VideoReferenceModeIdentityReference,
		MediaSlots: map[string][]VideoMediaInput{
			VideoSlotReferenceImages: {{Data: source.Bytes(), MIME: "image/png"}},
		},
		Seconds: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	content := body["content"].([]any)
	url := content[1].(map[string]any)["image_url"].(map[string]any)["url"].(string)
	encoded := strings.TrimPrefix(url, "data:image/png;base64,")
	resized, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(resized))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() < 300 || decoded.Bounds().Dy() < 300 {
		t.Fatalf("Ark reference image remained too small: %v", decoded.Bounds())
	}
	if decoded.Bounds().Dx()%256 != 0 || decoded.Bounds().Dy()%256 != 0 {
		t.Fatalf("pixel reference should use integer nearest-neighbor scaling: %v", decoded.Bounds())
	}
}

func TestVolcengineTaskLifecycle(t *testing.T) {
	var createAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/contents/generations/tasks":
			createAuth = r.Header.Get("Authorization")
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"generate_audio":false`) {
				t.Fatalf("expected silent video request, got %s", body)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"cgt-test","status":"queued"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/contents/generations/tasks/cgt-test":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"cgt-test","status":"succeeded","content":{"video_url":"` + serverURLForRequest(r) + `/result.mp4"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/result.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewVolcengineWithBaseURL("ark-test", "doubao-seedance-2-0-mini-260615", server.URL+"/api/v3")
	task, err := client.GenerateVideo(context.Background(), VideoRequest{
		Prompt:        "sprite walk",
		ReferenceMode: VideoReferenceModeNone,
		Seconds:       4,
		Resolution:    "480p",
		AspectRatio:   "1:1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "cgt-test" || task.Status != VideoStatusQueued {
		t.Fatalf("unexpected create result: %#v", task)
	}
	if createAuth != "Bearer ark-test" {
		t.Fatalf("authorization header missing: %q", createAuth)
	}
	task, err = client.PollVideo(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !task.Done() || task.URL == "" {
		t.Fatalf("unexpected poll result: %#v", task)
	}
	video, err := client.DownloadVideo(context.Background(), task.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(video) != "video" {
		t.Fatalf("unexpected downloaded content: %q", video)
	}
}

func TestVolcengineListVideoModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/models" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"data":[
			{"id":"doubao-seedance-2-0-mini-260615","domain":"VideoGeneration","modalities":{"output_modalities":["video"]}},
			{"id":"doubao-seed-1-6","domain":"Chat","modalities":{"output_modalities":["text"]}}
		]}`))
	}))
	defer server.Close()
	client := NewVolcengineWithBaseURL("ark-test", "", server.URL+"/api/v3")
	models, err := client.ListVideoModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0] != "doubao-seedance-2-0-mini-260615" {
		t.Fatalf("unexpected model list: %#v", models)
	}
}

func serverURLForRequest(r *http.Request) string {
	return "http://" + r.Host
}
