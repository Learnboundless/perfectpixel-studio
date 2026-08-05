package gen

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
)

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestVisionAnalyzeJSONBuildsMultimodalRequest(t *testing.T) {
	var received visionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"regions\":[]}"}}]}`))
	}))
	defer server.Close()

	client, err := NewVisionClient(ProviderOpenAI, "test-key", "gpt-4o-mini", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client.chatPath = "/analyze"
	result, err := client.AnalyzeJSON(context.Background(), "system", "inspect", [][]byte{tinyPNG(t)}, 1200)
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != `{"regions":[]}` {
		t.Fatalf("result = %s", result)
	}
	if received.Model != "gpt-4o-mini" || len(received.Messages) != 2 {
		t.Fatalf("unexpected request: %+v", received)
	}
	parts := received.Messages[1].Content
	if len(parts) != 2 || parts[1].ImageURL == nil || parts[1].ImageURL.Detail != "high" {
		t.Fatalf("unexpected multimodal parts: %+v", parts)
	}
	if received.ResponseFormat["type"] != "json_object" {
		t.Fatalf("response format = %+v", received.ResponseFormat)
	}
}

func TestVisionAnalyzeJSONRejectsInvalidOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"not json"}}]}`))
	}))
	defer server.Close()
	client, _ := NewVisionClient(ProviderOpenAI, "test-key", "gpt-4o-mini", server.URL)
	client.chatPath = ""
	if _, err := client.AnalyzeJSON(context.Background(), "system", "inspect", [][]byte{tinyPNG(t)}, 1200); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestImageModelCapabilityMaskSupport(t *testing.T) {
	if !ImageModelCapabilityFor(ProviderZetaTechs, "gpt-image-2").SupportsMaskEdit {
		t.Fatal("gpt-image-2 should expose mask editing")
	}
	if ImageModelCapabilityFor(ProviderFal, "fal-ai/flux/dev").MaxReferenceImages != 1 {
		t.Fatal("flux reference limit should be conservative")
	}
}
