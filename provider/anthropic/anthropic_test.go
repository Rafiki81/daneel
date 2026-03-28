package anthropic_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	daneel "github.com/Rafiki81/daneel"
	"github.com/Rafiki81/daneel/provider/anthropic"
)

func fakeAnthropicResponse(content string) map[string]any {
	return map[string]any{
		"id":   "msg_test",
		"type": "message",
		"role": "assistant",
		"content": []map[string]any{
			{"type": "text", "text": content},
		},
		"usage": map[string]any{
			"input_tokens":  10,
			"output_tokens": 5,
		},
		"stop_reason": "end_turn",
	}
}

func TestAnthropic_Chat_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fakeAnthropicResponse("Hello from Anthropic mock"))
	}))
	defer srv.Close()

	p := anthropic.New(
		anthropic.WithAPIKey("test-key"),
		anthropic.WithBaseURL(srv.URL),
		anthropic.WithModel("claude-3-5-sonnet-20241022"),
		anthropic.WithHTTPClient(srv.Client()),
	)

	resp, err := p.Chat(context.Background(), []daneel.Message{
		{Role: daneel.RoleUser, Content: "Hello"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello from Anthropic mock" {
		t.Errorf("content: want %q, got %q", "Hello from Anthropic mock", resp.Content)
	}
}

func TestAnthropic_Chat_httpError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"type":"overloaded_error","message":"overloaded"}}`, http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	p := anthropic.New(
		anthropic.WithAPIKey("test-key"),
		anthropic.WithBaseURL(srv.URL),
		anthropic.WithHTTPClient(srv.Client()),
	)

	_, err := p.Chat(context.Background(), []daneel.Message{
		{Role: daneel.RoleUser, Content: "Hello"},
	}, nil)
	if err == nil {
		t.Fatal("expected error for 503, got nil")
	}
}

func TestAnthropic_ModelInfo(t *testing.T) {
	p := anthropic.New(anthropic.WithModel("claude-3-5-sonnet-20241022"))
	info, err := p.ModelInfo(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = info
}
