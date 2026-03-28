package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Embedder implements daneel.Embedder using Ollama's /api/embed endpoint.
//
// Usage:
//
//	e := ollama.NewEmbedder(
//	    ollama.EmbedModel("nomic-embed-text"),
//	)
//	vec, err := e.Embed(ctx, "some text")
type Embedder struct {
	baseURL string
	model   string
	client  *http.Client
}

// EmbedderOption configures the Ollama Embedder.
type EmbedderOption func(*Embedder)

// EmbedModel sets the embedding model name.
// Recommended models: "nomic-embed-text" (768d), "mxbai-embed-large" (1024d), "all-minilm" (384d).
func EmbedModel(m string) EmbedderOption {
	return func(e *Embedder) { e.model = m }
}

// EmbedBaseURL sets the Ollama server URL for the embedder.
// Defaults to "http://localhost:11434".
func EmbedBaseURL(u string) EmbedderOption {
	return func(e *Embedder) { e.baseURL = strings.TrimRight(u, "/") }
}

// EmbedHTTPClient sets a custom HTTP client.
func EmbedHTTPClient(c *http.Client) EmbedderOption {
	return func(e *Embedder) { e.client = c }
}

// NewEmbedder creates an Ollama-backed Embedder.
// Default model is "nomic-embed-text"; default base URL is "http://localhost:11434".
func NewEmbedder(opts ...EmbedderOption) *Embedder {
	e := &Embedder{
		baseURL: "http://localhost:11434",
		model:   "nomic-embed-text",
		client:  &http.Client{Timeout: 30 * time.Second},
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Embed returns the embedding vector for the given text.
// It calls POST /api/embed on the Ollama server.
func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(map[string]string{
		"model": e.model,
		"input": text,
	})
	if err != nil {
		return nil, fmt.Errorf("ollama embed: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama embed: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama embed: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama embed: decode response: %w", err)
	}
	if len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
		return nil, fmt.Errorf("ollama embed: empty embeddings in response")
	}
	return result.Embeddings[0], nil
}
