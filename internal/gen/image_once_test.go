package gen

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIGenerateImageOnceDoesNotReplayPaidPost(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, `{"error":{"message":"temporary failure"}}`, http.StatusInternalServerError)
	}))
	defer server.Close()
	client := NewOpenAI("test-key", "gpt-image-2")
	client.generationEndpoint = server.URL
	client.HTTP = server.Client()
	if _, err := client.GenerateImageOnce(context.Background(), "test", nil, "1:1"); err == nil {
		t.Fatal("expected provider error")
	}
	if calls != 1 {
		t.Fatalf("paid POST calls = %d, want 1", calls)
	}
}

func TestZetaTechsPromotesSingleAttemptImageMethod(t *testing.T) {
	var provider Provider = NewZetaTechsWithBaseURL("test-key", "gpt-image-2", "https://example.com")
	if _, ok := provider.(SingleAttemptImageProvider); !ok {
		t.Fatal("ZetaTechs should expose the embedded single-attempt image method")
	}
}
