package google_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	daneel "github.com/Rafiki81/daneel"
	"github.com/Rafiki81/daneel/provider/google"
)

func fakeGoogleResponse(text string) map[string]any {
	return map[string]any{
		"candidates": []map[string]any{{
			"content": map[string]any{
				"role":  "model",
				"parts": []map[string]any{{"text": text}},
			},
			"finishReason": "STOP",
		}},
		"usageMetadata": map[string]any{
			"promptTokenCount":     10,
			"candidatesTokenCount": 5,
			"totalTokenCount":      15,
		},
	}
}

func TestGoogle_Chat_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "generateContent") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fakeGoogleResponse("Hello from Google mock"))
	}))
	defer srv.Close()

	p := google.New(
		google.WithAPIKey("test-key"),
		google.WithBaseURL(srv.URL),
		google.WithModel("gemini-1.5-flash"),
		google.WithHTTPClient(srv.Client()),
	)

	resp, err := p.Chat(context.Background(), []daneel.Message{
		{Role: daneel.RoleUser, Content: "Hello"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello from Google mock" {
		t.Errorf("content: want %q, got %q", "Hello from Google mock", resp.Content)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("total tokens: want 15, got %d", resp.Usage.TotalTokens)
	}
}

func TestGoogle_Chat_httpError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"API key not valid"}}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := google.New(
		google.WithAPIKey("bad-key"),
		google.WithBaseURL(srv.URL),
		google.WithHTTPClient(srv.Client()),
	)

	_, err := p.Chat(context.Background(), []daneel.Message{
		{Role: daneel.RoleUser, Content: "Hello"},
	}, nil)
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
}

func TestGoogle_ModelInfo_knownModel(t *testing.T) {
	p := google.New(google.WithModel("gemini-1.5-pro"))
	info, err := p.ModelInfo(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ContextWindow == 0 {
		t.Error("expected non-zero ContextWindow for gemini-1.5-pro")
	}
}
