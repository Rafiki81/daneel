package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	daneel "github.com/Rafiki81/daneel"
	"github.com/Rafiki81/daneel/provider/openai"
)

func fakeChatResponse(content string) map[string]any {
	return map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"role": "assistant", "content": content}},
		},
		"usage": map[string]any{
			"prompt_tokens":     10,
			"completion_tokens": 5,
			"total_tokens":      15,
		},
	}
}

func TestOpenAI_Chat_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fakeChatResponse("Hello from OpenAI mock"))
	}))
	defer srv.Close()

	p := openai.New(
		openai.WithAPIKey("test-key"),
		openai.WithBaseURL(srv.URL),
		openai.WithModel("gpt-4o"),
		openai.WithHTTPClient(srv.Client()),
	)

	resp, err := p.Chat(context.Background(), []daneel.Message{
		{Role: daneel.RoleUser, Content: "Hello"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello from OpenAI mock" {
		t.Errorf("content: want %q, got %q", "Hello from OpenAI mock", resp.Content)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("total tokens: want 15, got %d", resp.Usage.TotalTokens)
	}
}

func TestOpenAI_Chat_httpError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"rate limit exceeded"}}`, http.StatusTooManyRequests)
	}))
	defer srv.Close()

	p := openai.New(
		openai.WithAPIKey("test-key"),
		openai.WithBaseURL(srv.URL),
		openai.WithHTTPClient(srv.Client()),
	)

	_, err := p.Chat(context.Background(), []daneel.Message{
		{Role: daneel.RoleUser, Content: "Hello"},
	}, nil)
	if err == nil {
		t.Fatal("expected error for 429, got nil")
	}
}

func TestOpenAI_ModelInfo_knownModel(t *testing.T) {
	p := openai.New(openai.WithModel("gpt-4o"))
	info, err := p.ModelInfo(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ContextWindow == 0 {
		t.Error("expected non-zero ContextWindow for gpt-4o")
	}
	if !info.SupportsTools {
		t.Error("expected SupportsTools=true for gpt-4o")
	}
}

func TestOpenAI_ModelInfo_unknownModel(t *testing.T) {
	p := openai.New(openai.WithModel("unknown-model-xyz"))
	info, err := p.ModelInfo(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = info
}

