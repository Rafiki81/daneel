// Package ollama implements the daneel.Provider interface for Ollama's
// native /api/chat endpoint.
//
// For Ollama's OpenAI-compatible endpoint, use the openai provider with
// a custom base URL instead:
//
//	openai.New(openai.WithBaseURL("http://localhost:11434/v1"))
//
// This package uses Ollama's native API for full feature support:
//
//	p := ollama.New(
//	    ollama.WithModel("llama3.3:70b"),
//	    ollama.WithBaseURL("http://localhost:11434"),
//	)
//	agent := daneel.New("assistant", daneel.WithProvider(p))
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	daneel "github.com/Rafiki81/daneel"
)

// Option configures the Ollama provider.
type Option func(*config)

type config struct {
	model     string
	baseURL   string
	keepAlive *time.Duration
	timeout   time.Duration
	client    *http.Client
	numCtx    int // context window override
}

func defaultConfig() config {
	return config{
		model:   "llama3.3",
		baseURL: "http://localhost:11434",
		timeout: 300 * time.Second, // local models can be slow
	}
}

// WithModel sets the model name (e.g., "llama3.3:70b", "mistral", "codellama").
func WithModel(model string) Option {
	return func(c *config) { c.model = model }
}

// WithBaseURL sets the Ollama server URL (default: "http://localhost:11434").
func WithBaseURL(url string) Option {
	return func(c *config) { c.baseURL = strings.TrimRight(url, "/") }
}

// WithKeepAlive sets how long Ollama keeps the model loaded in memory
// after the last request. Default is determined by Ollama server config.
func WithKeepAlive(d time.Duration) Option {
	return func(c *config) { c.keepAlive = &d }
}

// WithTimeout sets the HTTP request timeout. Default: 300s (local models
// can take longer, especially for first load).
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *config) { c.client = client }
}

// WithNumCtx overrides the context window size for the model.
func WithNumCtx(n int) Option {
	return func(c *config) { c.numCtx = n }
}

// Provider implements daneel.Provider for Ollama.
type Provider struct {
	cfg    config
	client *http.Client
}

// New creates a new Ollama provider.
func New(opts ...Option) *Provider {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	client := cfg.client
	if client == nil {
		client = &http.Client{Timeout: cfg.timeout}
	}

	return &Provider{cfg: cfg, client: client}
}

// Chat implements daneel.Provider.
func (p *Provider) Chat(ctx context.Context, messages []daneel.Message, tools []daneel.ToolDef) (*daneel.Response, error) {
	return p.doChat(ctx, messages, tools)
}

// --- Ollama native API types ---

type chatRequest struct {
	Model     string          `json:"model"`
	Messages  []ollamaMessage `json:"messages"`
	Tools     []ollamaTool    `json:"tools,omitempty"`
	Stream    bool            `json:"stream"`
	Options   *ollamaOptions  `json:"options,omitempty"`
	KeepAlive *string         `json:"keep_alive,omitempty"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaTool struct {
	Type     string         `json:"type"`
	Function ollamaFunction `json:"function"`
}

type ollamaFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ollamaToolCall struct {
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

type ollamaOptions struct {
	NumCtx int `json:"num_ctx,omitempty"`
}

type chatResponse struct {
	Model   string        `json:"model"`
	Message ollamaMessage `json:"message"`
	Done    bool          `json:"done"`
	// Token usage (Ollama provides these in the final response).
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
	Error           string `json:"error,omitempty"`
}

// --- Internal methods ---

func (p *Provider) doChat(ctx context.Context, messages []daneel.Message, tools []daneel.ToolDef) (*daneel.Response, error) {
	// Convert messages.
	var ollamaMsgs []ollamaMessage
	for _, m := range messages {
		om := ollamaMessage{
			Role:    string(m.Role),
			Content: m.Content,
		}

		// Tool results use role "tool" in Ollama's native API.
		if m.Role == daneel.RoleTool {
			om.Role = "tool"
		}

		// Assistant messages with tool calls.
		for _, tc := range m.ToolCalls {
			om.ToolCalls = append(om.ToolCalls, ollamaToolCall{
				Function: struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				}{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			})
		}

		ollamaMsgs = append(ollamaMsgs, om)
	}

	// Convert tools.
	var ollamaTools []ollamaTool
	for _, td := range tools {
		ollamaTools = append(ollamaTools, ollamaTool{
			Type: "function",
			Function: ollamaFunction{
				Name:        td.Name,
				Description: td.Description,
				Parameters:  td.Schema,
			},
		})
	}

	reqBody := chatRequest{
		Model:    p.cfg.model,
		Messages: ollamaMsgs,
		Tools:    ollamaTools,
		Stream:   false, // Non-streaming for Provider interface
	}

	if p.cfg.numCtx > 0 {
		reqBody.Options = &ollamaOptions{NumCtx: p.cfg.numCtx}
	}

	if p.cfg.keepAlive != nil {
		ka := fmt.Sprintf("%ds", int(p.cfg.keepAlive.Seconds()))
		reqBody.KeepAlive = &ka
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.cfg.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: HTTP request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &daneel.ProviderError{
			Provider:   "ollama",
			StatusCode: resp.StatusCode,
			Message:    string(respBody),
			Retryable:  resp.StatusCode >= 500,
		}
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("ollama: unmarshal response: %w", err)
	}

	if chatResp.Error != "" {
		return nil, &daneel.ProviderError{
			Provider: "ollama",
			Message:  chatResp.Error,
		}
	}

	// Convert tool calls.
	var toolCalls []daneel.ToolCall
	for i, tc := range chatResp.Message.ToolCalls {
		toolCalls = append(toolCalls, daneel.ToolCall{
			ID:        fmt.Sprintf("call_%d", i),
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	return &daneel.Response{
		Content:   chatResp.Message.Content,
		ToolCalls: toolCalls,
		Usage: daneel.Usage{
			PromptTokens:     chatResp.PromptEvalCount,
			CompletionTokens: chatResp.EvalCount,
			TotalTokens:      chatResp.PromptEvalCount + chatResp.EvalCount,
		},
	}, nil
}

// Compile-time interface check.
var _ daneel.Provider = (*Provider)(nil)

// String returns a representation for logging.
func (p *Provider) String() string {
	return fmt.Sprintf("Ollama{model: %s, base: %s}", p.cfg.model, p.cfg.baseURL)
}
