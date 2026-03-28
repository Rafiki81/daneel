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
	"bufio"
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

	// Convert tool calls, applying model-quirk normalisations.
	var toolCalls []daneel.ToolCall
	for i, tc := range chatResp.Message.ToolCalls {
		toolCalls = append(toolCalls, daneel.ToolCall{
			ID:        fmt.Sprintf("call_%d", i),
			Name:      tc.Function.Name,
			Arguments: normalizeToolArguments(tc.Function.Arguments),
		})
	}

	// Fallback: some models (e.g. qwen2.5-coder) emit tool calls as plain text
	// in message.content instead of using the structured tool_calls field.
	if len(toolCalls) == 0 && chatResp.Message.Content != "" {
		if extracted := extractToolCallsFromContent(chatResp.Message.Content); len(extracted) > 0 {
			toolCalls = extracted
			chatResp.Message.Content = ""
		}
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

// normalizeToolArguments unwraps double-encoded JSON arguments produced by
// some models (e.g. llama3.2). Those models wrap the argument object inside
// a JSON string, yielding e.g.:
//
//	{"city": "{\"city\":\"Madrid\"}"}
//
// instead of the correct:
//
//	{"city": "Madrid"}
//
// The heuristic is: if any string value is itself valid JSON (starts with '{'
// or '['), replace the whole arguments blob with the unwrapped object.
func normalizeToolArguments(args json.RawMessage) json.RawMessage {
	if len(args) == 0 {
		return args
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(args, &m); err != nil || len(m) != 1 {
		return args
	}
	for _, v := range m {
		// If the single value is a quoted string that contains JSON, unwrap it.
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return args // value is not a string — nothing to unwrap
		}
		trimmed := strings.TrimSpace(s)
		if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
			return args // string doesn't look like JSON
		}
		// Validate the inner JSON before accepting it.
		if json.Valid([]byte(trimmed)) {
			return json.RawMessage(trimmed)
		}
	}
	return args
}

// extractToolCallsFromContent attempts to parse tool calls that some models
// (e.g. qwen2.5-coder) emit as plain text in message.content instead of using
// the structured tool_calls field. It handles three formats:
//
//  1. A single JSON object: {"name":"get_weather","arguments":{"city":"Madrid"}}
//  2. A JSON array:         [{"name":"get_weather","arguments":{...}}, ...]
//  3. Multiple JSON objects separated by whitespace/newlines (one per call)
func extractToolCallsFromContent(content string) []daneel.ToolCall {
	trimmed := strings.TrimSpace(content)
	if len(trimmed) == 0 {
		return nil
	}

	type inlineCall struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}

	// Try array first.
	if trimmed[0] == '[' {
		var calls []inlineCall
		if err := json.Unmarshal([]byte(trimmed), &calls); err == nil && len(calls) > 0 {
			out := make([]daneel.ToolCall, len(calls))
			for i, c := range calls {
				if c.Name == "" {
					return nil
				}
				out[i] = daneel.ToolCall{
					ID:        fmt.Sprintf("call_%d", i),
					Name:      c.Name,
					Arguments: c.Arguments,
				}
			}
			return out
		}
	}

	// Try single object.
	if trimmed[0] == '{' {
		var call inlineCall
		if err := json.Unmarshal([]byte(trimmed), &call); err == nil && call.Name != "" {
			return []daneel.ToolCall{{
				ID:        "call_0",
				Name:      call.Name,
				Arguments: call.Arguments,
			}}
		}

		// Multiple JSON objects separated by whitespace: scan with brace depth.
		var out []daneel.ToolCall
		remaining := trimmed
		idx := 0
		for len(remaining) > 0 {
			remaining = strings.TrimSpace(remaining)
			if len(remaining) == 0 || remaining[0] != '{' {
				break
			}
			// Find the end of this JSON object.
			depth, end := 0, -1
			inStr := false
			for i, ch := range remaining {
				switch {
				case inStr:
					if ch == '\\' {
						// skip next char — handled by next iteration naturally
						// (range gives runes; we just set a flag)
					} else if ch == '"' {
						inStr = false
					}
				case ch == '"':
					inStr = true
				case ch == '{':
					depth++
				case ch == '}':
					depth--
					if depth == 0 {
						end = i
					}
				}
				if end >= 0 {
					break
				}
			}
			if end < 0 {
				break
			}
			obj := remaining[:end+1]
			remaining = remaining[end+1:]
			var call inlineCall
			if err := json.Unmarshal([]byte(obj), &call); err != nil || call.Name == "" {
				return nil // not tool call objects
			}
			out = append(out, daneel.ToolCall{
				ID:        fmt.Sprintf("call_%d", idx),
				Name:      call.Name,
				Arguments: call.Arguments,
			})
			idx++
		}
		if len(out) > 0 {
			return out
		}
	}

	return nil
}

// Compile-time interface checks.
var (
	_ daneel.Provider       = (*Provider)(nil)
	_ daneel.StreamProvider = (*Provider)(nil)
)

// String returns a representation for logging.
func (p *Provider) String() string {
	return fmt.Sprintf("Ollama{model: %s, base: %s}", p.cfg.model, p.cfg.baseURL)
}

// ChatStream implements daneel.StreamProvider. It uses Ollama's native
// streaming API (NDJSON) to emit text tokens as they arrive, then emits
// complete ToolCall chunks from the final done message.
func (p *Provider) ChatStream(ctx context.Context, messages []daneel.Message, tools []daneel.ToolDef) (<-chan daneel.StreamChunk, error) {
	ch := make(chan daneel.StreamChunk, 32)
	go func() {
		defer close(ch)
		if err := p.doStreamChat(ctx, messages, tools, ch); err != nil {
			select {
			case ch <- daneel.StreamChunk{Type: daneel.StreamError, Error: err}:
			case <-ctx.Done():
			}
		}
	}()
	return ch, nil
}

func (p *Provider) doStreamChat(ctx context.Context, messages []daneel.Message, tools []daneel.ToolDef, ch chan<- daneel.StreamChunk) error {
	var ollamaMsgs []ollamaMessage
	for _, m := range messages {
		om := ollamaMessage{Role: string(m.Role), Content: m.Content}
		if m.Role == daneel.RoleTool {
			om.Role = "tool"
		}
		for _, tc := range m.ToolCalls {
			om.ToolCalls = append(om.ToolCalls, ollamaToolCall{
				Function: struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				}{Name: tc.Name, Arguments: tc.Arguments},
			})
		}
		ollamaMsgs = append(ollamaMsgs, om)
	}

	var ollamaTools []ollamaTool
	for _, td := range tools {
		ollamaTools = append(ollamaTools, ollamaTool{
			Type: "function",
			Function: ollamaFunction{
				Name: td.Name, Description: td.Description, Parameters: td.Schema,
			},
		})
	}

	reqBody := chatRequest{
		Model:    p.cfg.model,
		Messages: ollamaMsgs,
		Tools:    ollamaTools,
		Stream:   true,
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
		return fmt.Errorf("ollama: marshal stream request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.cfg.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("ollama: create stream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Streaming responses have no fixed duration; rely on context for cancellation.
	streamClient := &http.Client{Transport: p.client.Transport}
	if streamClient.Transport == nil {
		streamClient.Transport = http.DefaultTransport
	}

	resp, err := streamClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama: stream HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return &daneel.ProviderError{
			Provider:   "ollama",
			StatusCode: resp.StatusCode,
			Message:    string(b),
			Retryable:  resp.StatusCode >= 500,
		}
	}

	// Buffer content tokens during scanning so we can apply the content-as-
	// tool-call fallback (some models emit tool calls as plain text in content)
	// before forwarding chunks to the caller.
	var contentBuf strings.Builder
	var toolCalls []daneel.ToolCall
	var tcIdx int

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var chunk chatResponse
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue
		}
		if chunk.Error != "" {
			return &daneel.ProviderError{Provider: "ollama", Message: chunk.Error}
		}
		contentBuf.WriteString(chunk.Message.Content)
		for _, tc := range chunk.Message.ToolCalls {
			toolCalls = append(toolCalls, daneel.ToolCall{
				ID:        fmt.Sprintf("call_%d", tcIdx),
				Name:      tc.Function.Name,
				Arguments: normalizeToolArguments(tc.Function.Arguments),
			})
			tcIdx++
		}
		if chunk.Done {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("ollama: stream scan: %w", err)
	}

	// Fallback: if no structured tool calls but the accumulated content is a
	// JSON tool call, extract it (same heuristic as the non-streaming path).
	accContent := contentBuf.String()
	if len(toolCalls) == 0 && accContent != "" {
		if extracted := extractToolCallsFromContent(accContent); len(extracted) > 0 {
			toolCalls = extracted
			accContent = ""
		}
	}

	// Forward buffered text tokens.
	if accContent != "" {
		select {
		case ch <- daneel.StreamChunk{Type: daneel.StreamText, Text: accContent}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Emit complete tool calls.
	for i := range toolCalls {
		select {
		case ch <- daneel.StreamChunk{Type: daneel.StreamToolCallStart, ToolCall: &toolCalls[i]}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
