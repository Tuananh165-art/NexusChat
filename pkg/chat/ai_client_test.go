package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tuananh165-art/NexusChat/pkg/config"
)

func TestAIClientRewrite(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/assistant/rewrite" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		var req AIRewriteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Text != "hello" || req.Tone != "formal" {
			t.Fatalf("unexpected request: %+v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"Hello.","provider":"openai-compatible","model":"test-model"}`))
	}))
	defer server.Close()

	client := NewAIClientImpl(&config.Config{
		AI: &config.AIConfig{
			BaseURL:             server.URL,
			RequestTimeoutMilli: 1000,
		},
	})

	resp, err := client.Rewrite(context.Background(), &AIRewriteRequest{
		Text: "hello",
		Tone: "formal",
	})
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if resp.Text != "Hello." || resp.Provider != "openai-compatible" || resp.Model != "test-model" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestAIClientRewriteReturnsStatusError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	client := NewAIClientImpl(&config.Config{
		AI: &config.AIConfig{
			BaseURL:             server.URL,
			RequestTimeoutMilli: 1000,
		},
	})

	_, err := client.Rewrite(context.Background(), &AIRewriteRequest{Text: "hello"})
	if err == nil {
		t.Fatal("expected status error")
	}
}
