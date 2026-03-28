package ollama_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rafiki81/daneel/provider/ollama"
)

func TestNewEmbedder_defaults(t *testing.T) {
	e := ollama.NewEmbedder()
	if e == nil {
		t.Fatal("expected non-nil Embedder")
	}
}

func TestEmbedder_Embed_success(t *testing.T) {
	want := []float32{0.1, 0.2, 0.3}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Model string `json:"model"`
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.Model == "" || req.Input == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"embeddings": [][]float32{want},
		})
	}))
	defer srv.Close()

	e := ollama.NewEmbedder(
		ollama.EmbedBaseURL(srv.URL),
		ollama.EmbedModel("nomic-embed-text"),
		ollama.EmbedHTTPClient(srv.Client()),
	)

	got, err := e.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d dimensions, got %d", len(want), len(got))
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("dim[%d]: want %v, got %v", i, v, got[i])
		}
	}
}

func TestEmbedder_Embed_serverError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model not found", http.StatusNotFound)
	}))
	defer srv.Close()

	e := ollama.NewEmbedder(
		ollama.EmbedBaseURL(srv.URL),
		ollama.EmbedHTTPClient(srv.Client()),
	)

	_, err := e.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for non-200 status, got nil")
	}
}

func TestEmbedder_Embed_emptyEmbeddings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"embeddings": [][]float32{},
		})
	}))
	defer srv.Close()

	e := ollama.NewEmbedder(
		ollama.EmbedBaseURL(srv.URL),
		ollama.EmbedHTTPClient(srv.Client()),
	)

	_, err := e.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for empty embeddings, got nil")
	}
}

func TestEmbedder_Embed_cancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"embeddings": [][]float32{{1.0}},
		})
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	e := ollama.NewEmbedder(
		ollama.EmbedBaseURL(srv.URL),
		ollama.EmbedHTTPClient(srv.Client()),
	)

	_, err := e.Embed(ctx, "test")
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}

func TestEmbedModel_option(t *testing.T) {
	want := []float32{0.5}
	captured := ""

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
			Input string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		captured = req.Model
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"embeddings": [][]float32{want},
		})
	}))
	defer srv.Close()

	const model = "mxbai-embed-large"
	e := ollama.NewEmbedder(
		ollama.EmbedBaseURL(srv.URL),
		ollama.EmbedModel(model),
		ollama.EmbedHTTPClient(srv.Client()),
	)
	if _, err := e.Embed(context.Background(), "x"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured != model {
		t.Errorf("model: want %q, got %q", model, captured)
	}
}
