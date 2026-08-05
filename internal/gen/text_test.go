package gen

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTextClientUsesOpenAICompatibleChatWithoutResponseFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer ark-test" {
			t.Fatalf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "glm-5.2" {
			t.Fatalf("unexpected model: %#v", body["model"])
		}
		if _, ok := body["response_format"]; ok {
			t.Fatal("text request must not send response_format")
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"原创机甲驾驶员，白色装甲与红蓝点缀。"}}]}`))
	}))
	defer server.Close()

	client, err := NewTextClient(ProviderVolcengine, "ark-test", "glm-5.2", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.Complete(context.Background(), "rewrite", "高达")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "原创机甲") {
		t.Fatalf("unexpected completion: %q", got)
	}
}

func TestDecodeTextContentSupportsArray(t *testing.T) {
	got := decodeTextContent(json.RawMessage(`[{"type":"text","text":"第一段"},{"type":"text","text":"第二段"}]`))
	if got != "第一段\n第二段" {
		t.Fatalf("unexpected content: %q", got)
	}
}
