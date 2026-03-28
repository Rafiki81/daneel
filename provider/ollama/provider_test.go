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

// TestOllama_Chat_toolCallDoubleEncodedArgs verifies that when a model (e.g.
// llama3.2) wraps the entire argument object inside a JSON-encoded string, the
// provider unwraps it so callers receive a clean JSON object.
func TestOllama_Chat_toolCallDoubleEncodedArgs(t *testing.T) {
	// The server returns a structured tool_call whose arguments value is a
	// JSON-string wrapping another JSON object — the double-encoding bug.
	resp := map[string]any{
		"model": "llama3.2",
		"message": map[string]any{
			"role":    "assistant",
			"content": "",
			"tool_calls": []any{
				map[string]any{
					"function": map[string]any{
						"name": "get_weather",
						// double-encoded: the string value IS valid JSON.
						"arguments": map[string]any{
							"city": `{"city":"Madrid"}`,
						},
					},
				},
			},
		},
		"done": true,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := ollama.New(
		ollama.WithModel("llama3.2"),
		ollama.WithBaseURL(srv.URL),
		ollama.WithHTTPClient(srv.Client()),
	)

	chatResp, err := p.Chat(context.Background(), []daneel.Message{
		{Role: daneel.RoleUser, Content: "What is the weather in Madrid?"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chatResp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(chatResp.ToolCalls))
	}
	tc := chatResp.ToolCalls[0]
	if tc.Name != "get_weather" {
		t.Errorf("tool name: want %q, got %q", "get_weather", tc.Name)
	}

	// After unwrapping, Arguments must be the inner object {"city":"Madrid"}.
	var args map[string]string
	if err := json.Unmarshal(tc.Arguments, &args); err != nil {
		t.Fatalf("unmarshal args: %v", err)
	}
	if args["city"] != "Madrid" {
		t.Errorf("args[city]: want %q, got %q", "Madrid", args["city"])
	}
}

// TestOllama_Chat_toolCallInContent verifies that when a model (e.g.
// qwen2.5-coder) emits tool calls as a JSON object in message.content instead
// of using the structured tool_calls field, the provider parses and returns
// them correctly.
func TestOllama_Chat_toolCallInContent(t *testing.T) {
	toolJSON := `{"name":"get_weather","arguments":{"city":"Paris"}}`
	resp := map[string]any{
		"model": "qwen2.5-coder",
		"message": map[string]any{
			"role":       "assistant",
			"content":    toolJSON,
			"tool_calls": []any{},
		},
		"done": true,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := ollama.New(
		ollama.WithModel("qwen2.5-coder"),
		ollama.WithBaseURL(srv.URL),
		ollama.WithHTTPClient(srv.Client()),
	)

	chatResp, err := p.Chat(context.Background(), []daneel.Message{
		{Role: daneel.RoleUser, Content: "What is the weather in Paris?"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chatResp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(chatResp.ToolCalls))
	}
	tc := chatResp.ToolCalls[0]
	if tc.Name != "get_weather" {
		t.Errorf("tool name: want %q, got %q", "get_weather", tc.Name)
	}
	// Content should be cleared when a tool call is extracted from it.
	if chatResp.Content != "" {
		t.Errorf("content should be empty after tool call extraction, got %q", chatResp.Content)
	}

	var args map[string]string
	if err := json.Unmarshal(tc.Arguments, &args); err != nil {
		t.Fatalf("unmarshal args: %v", err)
	}
	if args["city"] != "Paris" {
		t.Errorf("args[city]: want %q, got %q", "Paris", args["city"])
	}
}

// TestOllama_Chat_toolCallStructured verifies that normal structured tool calls
// (correct format) continue to work without regression.
func TestOllama_Chat_toolCallStructured(t *testing.T) {
	resp := map[string]any{
		"model": "llama3.2",
		"message": map[string]any{
			"role":    "assistant",
			"content": "",
			"tool_calls": []any{
				map[string]any{
					"function": map[string]any{
						"name":      "get_weather",
						"arguments": map[string]any{"city": "Tokyo"},
					},
				},
			},
		},
		"done": true,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := ollama.New(
		ollama.WithModel("llama3.2"),
		ollama.WithBaseURL(srv.URL),
		ollama.WithHTTPClient(srv.Client()),
	)

	chatResp, err := p.Chat(context.Background(), []daneel.Message{
		{Role: daneel.RoleUser, Content: "Weather in Tokyo?"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chatResp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(chatResp.ToolCalls))
	}
	tc := chatResp.ToolCalls[0]
	if tc.Name != "get_weather" {
		t.Errorf("tool name: want %q, got %q", "get_weather", tc.Name)
	}
	var args map[string]string
	if err := json.Unmarshal(tc.Arguments, &args); err != nil {
		t.Fatalf("unmarshal args: %v", err)
	}
	if args["city"] != "Tokyo" {
		t.Errorf("args[city]: want %q, got %q", "Tokyo", args["city"])
	}
}
