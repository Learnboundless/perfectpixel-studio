package gen

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestWithoutPaidRequestRetriesSubmitsImageOnce(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"ambiguous upstream failure"}}`))
	}))
	defer server.Close()

	client := NewOpenAI("sk-test-00000000000000000000", "gpt-image-2")
	client.HTTP = server.Client()
	client.generationEndpoint = server.URL
	_, err := client.GenerateImage(WithoutPaidRequestRetries(context.Background()), "test", nil, "1:1")
	if err == nil {
		t.Fatal("expected provider error")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("paid request calls = %d, want 1", got)
	}
}
