package ollama_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	daneel "github.com/Rafiki81/daneel"
	"github.com/Rafiki81/daneel/provider/ollama"
)

func fakeOllamaChatResponse(content string) map[string]any {
	return map[string]any{
		"model": "llama3.2",
		"message": map[string]any{
			"role":    "assistant",
			"content": content,
		},
		"done":              true,
		"prompt_eval_count": 10,
		"eval_count":        5,
	}
}

func TestOllama_Chat_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fakeOllamaChatResponse("Hello from Ollama mock"))
	}))
	defer srv.Close()

	p := ollama.New(
		ollama.WithModel("llama3.2"),
		ollama.WithBaseURL(srv.URL),
		ollama.WithHTTPClient(srv.Client()),
	)

	resp, err := p.Chat(context.Background(), []daneel.Message{
		{Role: daneel.RoleUser, Content: "Hello"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello from Ollama mock" {
		t.Errorf("content: want %q, got %q", "Hello from Ollama mock", resp.Content)
	}
}

func TestOllama_Chat_serverError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model not found", http.StatusNotFound)
	}))
	defer srv.Close()

	p := ollama.New(
		ollama.WithModel("no-such-model"),
		ollama.WithBaseURL(srv.URL),
		ollama.WithHTTPClient(srv.Client()),
	)

	_, err := p.Chat(context.Background(), []daneel.Message{
		{Role: daneel.RoleUser, Content: "Hello"},
	}, nil)
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

func TestOllama_Chat_cancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fakeOllamaChatResponse("too late"))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := ollama.New(
		ollama.WithBaseURL(srv.URL),
		ollama.WithHTTPClient(srv.Client()),
	)

	_, err := p.Chat(ctx, []daneel.Message{
		{Role: daneel.RoleUser, Content: "Hello"},
	}, nil)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}
