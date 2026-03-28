// Package openai implements the daneel.Provider interface for OpenAI and
// compatible APIs (Groq, Together, Fireworks, DeepSeek, etc.).
//
// Usage:
//
//	p := openai.New(
//	    openai.WithAPIKey(os.Getenv("OPENAI_API_KEY")),
//	    openai.WithModel("gpt-4o"),
//	)
//	agent := daneel.New("assistant", daneel.WithProvider(p))
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	daneel "github.com/Rafiki81/daneel"
)

// KnownModels maps model names to their capabilities. Looked up from
// this built-in table — no network call needed.
var KnownModels = map[string]daneel.ModelInfo{
	"gpt-4o":        {ContextWindow: 128_000, MaxOutput: 16_384, SupportsVision: true, SupportsTools: true, SupportsJSON: true},
	"gpt-4o-mini":   {ContextWindow: 128_000, MaxOutput: 16_384, SupportsVision: true, SupportsTools: true, SupportsJSON: true},
	"gpt-4-turbo":   {ContextWindow: 128_000, MaxOutput: 4_096, SupportsVision: true, SupportsTools: true, SupportsJSON: true},
	"gpt-4":         {ContextWindow: 8_192, MaxOutput: 8_192, SupportsVision: false, SupportsTools: true, SupportsJSON: false},
	"gpt-3.5-turbo": {ContextWindow: 16_385, MaxOutput: 4_096, SupportsVision: false, SupportsTools: true, SupportsJSON: true},
	"o1":            {ContextWindow: 200_000, MaxOutput: 100_000, SupportsVision: true, SupportsTools: true, SupportsJSON: true},
	"o1-mini":       {ContextWindow: 128_000, MaxOutput: 65_536, SupportsVision: false, SupportsTools: true, SupportsJSON: true},
	"o3-mini":       {ContextWindow: 200_000, MaxOutput: 100_000, SupportsVision: false, SupportsTools: true, SupportsJSON: true},
}

// Option configures the OpenAI provider.
type Option func(*config)

type config struct {
	apiKey       string
	model        string
	baseURL      string
	organization string
	maxTokens    int
	temperature  *float64
	timeout      time.Duration
	retry        RetryConfig
	client       *http.Client
}

// RetryConfig controls retry behavior for failed API calls.
type RetryConfig struct {
	MaxRetries  int
	InitialWait time.Duration
	MaxWait     time.Duration
	RetryOn     []int // HTTP status codes to retry on
}

func defaultConfig() config {
	return config{
		apiKey:  os.Getenv("OPENAI_API_KEY"),
		model:   "gpt-4o",
		baseURL: "https://api.openai.com/v1",
		timeout: 120 * time.Second,
		retry: RetryConfig{
			MaxRetries:  3,
			InitialWait: 1 * time.Second,
			MaxWait:     30 * time.Second,
			RetryOn:     []int{429, 500, 502, 503},
		},
	}
}

// WithAPIKey sets the OpenAI API key. If empty, reads OPENAI_API_KEY from env.
func WithAPIKey(key string) Option {
	return func(c *config) {
		if key != "" {
			c.apiKey = key
		}
	}
}

// WithModel sets the model name (e.g., "gpt-4o", "gpt-4o-mini").
func WithModel(model string) Option {
	return func(c *config) { c.model = model }
}

// WithBaseURL sets the API base URL. Use this for compatible APIs
// (Groq, Together, Fireworks, LocalAI, Ollama OpenAI-compat, etc.).
func WithBaseURL(url string) Option {
	return func(c *config) { c.baseURL = url }
}

// WithOrganization sets the OpenAI organization ID.
func WithOrganization(org string) Option {
	return func(c *config) { c.organization = org }
}

// WithMaxTokens sets the maximum output tokens.
func WithMaxTokens(n int) Option {
	return func(c *config) { c.maxTokens = n }
}

// WithTemperature sets the sampling temperature (0.0 to 2.0).
func WithTemperature(t float64) Option {
	return func(c *config) { c.temperature = &t }
}

// WithTimeout sets the HTTP request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// WithRetry sets the retry configuration.
func WithRetry(r RetryConfig) Option {
	return func(c *config) { c.retry = r }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *config) { c.client = client }
}

// Provider implements daneel.Provider for OpenAI and compatible APIs.
type Provider struct {
	cfg    config
	client *http.Client
}

// New creates a new OpenAI provider.
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
	return p.chatWithRetry(ctx, messages, tools)
}

// ModelInfo implements daneel.ModelInfoProvider.
func (p *Provider) ModelInfo(_ context.Context) (daneel.ModelInfo, error) {
	if info, ok := KnownModels[p.cfg.model]; ok {
		return info, nil
	}
	// Unknown model — return conservative defaults
	return daneel.ModelInfo{
		ContextWindow:  4096,
		MaxOutput:      4096,
		SupportsVision: false,
		SupportsTools:  true,
		SupportsJSON:   false,
	}, nil
}

// --- Request/Response types ---

type chatRequest struct {
	Model       string           `json:"model"`
	Messages    []requestMessage `json:"messages"`
	Tools       []requestTool    `json:"tools,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature *float64         `json:"temperature,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
}

type requestMessage struct {
	Role       string            `json:"role"`
	Content    string            `json:"content"`
	ToolCalls  []requestToolCall `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
}

type requestTool struct {
	Type     string          `json:"type"`
	Function requestFunction `json:"function"`
}

type requestFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type requestToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Role      string            `json:"role"`
			Content   string            `json:"content"`
			ToolCalls []requestToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// --- Internal methods ---

func (p *Provider) chatWithRetry(ctx context.Context, messages []daneel.Message, tools []daneel.ToolDef) (*daneel.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= p.cfg.retry.MaxRetries; attempt++ {
		if attempt > 0 {
			wait := p.backoffDuration(attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}

		resp, statusCode, err := p.doChat(ctx, messages, tools)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Check if we should retry
		if !p.shouldRetry(statusCode) {
			return nil, err
		}
	}

	return nil, lastErr
}

func (p *Provider) doChat(ctx context.Context, messages []daneel.Message, tools []daneel.ToolDef) (*daneel.Response, int, error) {
	// Build request
	reqMsgs := make([]requestMessage, len(messages))
	for i, m := range messages {
		rm := requestMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			rm.ToolCalls = append(rm.ToolCalls, requestToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{
					Name:      tc.Name,
					Arguments: string(tc.Arguments),
				},
			})
		}
		reqMsgs[i] = rm
	}

	var reqTools []requestTool
	for _, td := range tools {
		reqTools = append(reqTools, requestTool{
			Type: "function",
			Function: requestFunction{
				Name:        td.Name,
				Description: td.Description,
				Parameters:  td.Schema,
			},
		})
	}

	reqBody := chatRequest{
		Model:       p.cfg.model,
		Messages:    reqMsgs,
		Tools:       reqTools,
		MaxTokens:   p.cfg.maxTokens,
		Temperature: p.cfg.temperature,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("openai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.cfg.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("openai: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.cfg.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.apiKey)
	}
	if p.cfg.organization != "" {
		req.Header.Set("OpenAI-Organization", p.cfg.organization)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("openai: HTTP request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("openai: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, &daneel.ProviderError{
			Provider:   "openai",
			StatusCode: resp.StatusCode,
			Message:    string(respBody),
			Retryable:  resp.StatusCode == 429 || resp.StatusCode >= 500,
		}
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("openai: unmarshal response: %w", err)
	}

	if chatResp.Error != nil {
		return nil, resp.StatusCode, &daneel.ProviderError{
			Provider: "openai",
			Message:  chatResp.Error.Message,
		}
	}

	if len(chatResp.Choices) == 0 {
		return nil, resp.StatusCode, &daneel.ProviderError{
			Provider: "openai",
			Message:  "no choices in response",
		}
	}

	choice := chatResp.Choices[0]

	// Convert tool calls
	var toolCalls []daneel.ToolCall
	for _, tc := range choice.Message.ToolCalls {
		toolCalls = append(toolCalls, daneel.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: json.RawMessage(tc.Function.Arguments),
		})
	}

	return &daneel.Response{
		Content:   choice.Message.Content,
		ToolCalls: toolCalls,
		Usage: daneel.Usage{
			PromptTokens:     chatResp.Usage.PromptTokens,
			CompletionTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:      chatResp.Usage.TotalTokens,
		},
	}, resp.StatusCode, nil
}

// --- Streaming response types ---

type streamChunkResponse struct {
	Choices []struct {
		Delta        streamDelta `json:"delta"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
}

type streamDelta struct {
	Content   string            `json:"content"`
	ToolCalls []streamToolDelta `json:"tool_calls"`
}

type streamToolDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ChatStream implements daneel.StreamProvider. It uses the OpenAI streaming
// API to emit text tokens as they arrive, then emits complete ToolCall chunks
// once the full stream has been received.
func (p *Provider) ChatStream(ctx context.Context, messages []daneel.Message, tools []daneel.ToolDef) (<-chan daneel.StreamChunk, error) {
	ch := make(chan daneel.StreamChunk, 32)
	go func() {
		defer close(ch)
		if err := p.doStream(ctx, messages, tools, ch); err != nil {
			select {
			case ch <- daneel.StreamChunk{Type: daneel.StreamError, Error: err}:
			case <-ctx.Done():
			}
		}
	}()
	return ch, nil
}

func (p *Provider) doStream(ctx context.Context, messages []daneel.Message, tools []daneel.ToolDef, ch chan<- daneel.StreamChunk) error {
	reqMsgs := make([]requestMessage, len(messages))
	for i, m := range messages {
		rm := requestMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			rm.ToolCalls = append(rm.ToolCalls, requestToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{
					Name:      tc.Name,
					Arguments: string(tc.Arguments),
				},
			})
		}
		reqMsgs[i] = rm
	}

	var reqTools []requestTool
	for _, td := range tools {
		reqTools = append(reqTools, requestTool{
			Type: "function",
			Function: requestFunction{
				Name:        td.Name,
				Description: td.Description,
				Parameters:  td.Schema,
			},
		})
	}

	reqBody := chatRequest{
		Model:       p.cfg.model,
		Messages:    reqMsgs,
		Tools:       reqTools,
		MaxTokens:   p.cfg.maxTokens,
		Temperature: p.cfg.temperature,
		Stream:      true,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("openai: marshal stream request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.cfg.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("openai: create stream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if p.cfg.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.apiKey)
	}
	if p.cfg.organization != "" {
		req.Header.Set("OpenAI-Organization", p.cfg.organization)
	}

	// Streaming responses have no fixed duration; rely on context for cancellation.
	streamClient := &http.Client{Transport: p.client.Transport}
	if streamClient.Transport == nil {
		streamClient.Transport = http.DefaultTransport
	}

	resp, err := streamClient.Do(req)
	if err != nil {
		return fmt.Errorf("openai: stream HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return &daneel.ProviderError{
			Provider:   "openai",
			StatusCode: resp.StatusCode,
			Message:    string(b),
			Retryable:  resp.StatusCode == 429 || resp.StatusCode >= 500,
		}
	}

	// Accumulate tool call fragments by index.
	type toolAccum struct {
		id   string
		name string
		args strings.Builder
	}
	accumulated := map[int]*toolAccum{}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "data: [DONE]" {
			break
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var chunk streamChunkResponse
		if err := json.Unmarshal([]byte(line[6:]), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta

		// Emit text token immediately.
		if delta.Content != "" {
			select {
			case ch <- daneel.StreamChunk{Type: daneel.StreamText, Text: delta.Content}:
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Accumulate tool call fragments keyed by index.
		for _, tc := range delta.ToolCalls {
			if _, ok := accumulated[tc.Index]; !ok {
				accumulated[tc.Index] = &toolAccum{}
			}
			acc := accumulated[tc.Index]
			if tc.ID != "" {
				acc.id = tc.ID
			}
			if tc.Function.Name != "" {
				acc.name = tc.Function.Name
			}
			acc.args.WriteString(tc.Function.Arguments)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("openai: stream scan: %w", err)
	}

	// Emit complete tool calls in index order.
	for i := 0; i < len(accumulated); i++ {
		acc, ok := accumulated[i]
		if !ok {
			continue
		}
		select {
		case ch <- daneel.StreamChunk{
			Type: daneel.StreamToolCallStart,
			ToolCall: &daneel.ToolCall{
				ID:        acc.id,
				Name:      acc.name,
				Arguments: json.RawMessage(acc.args.String()),
			},
		}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (p *Provider) shouldRetry(statusCode int) bool {
	if statusCode == 0 {
		return false
	}
	for _, code := range p.cfg.retry.RetryOn {
		if statusCode == code {
			return true
		}
	}
	return false
}

func (p *Provider) backoffDuration(attempt int) time.Duration {
	wait := p.cfg.retry.InitialWait * time.Duration(math.Pow(2, float64(attempt-1)))
	if wait > p.cfg.retry.MaxWait {
		wait = p.cfg.retry.MaxWait
	}
	return wait
}

// --- Pricing ---

// Pricing maps model names to their per-1M-token costs.
type Pricing map[string]ModelCost

// ModelCost holds input and output costs per 1M tokens.
type ModelCost struct {
	Input  float64 // cost per 1M input tokens
	Output float64 // cost per 1M output tokens
}

// OpenAIPricing contains known OpenAI model pricing.
var OpenAIPricing = Pricing{
	"gpt-4o":        {Input: 2.50, Output: 10.00},
	"gpt-4o-mini":   {Input: 0.15, Output: 0.60},
	"gpt-4-turbo":   {Input: 10.00, Output: 30.00},
	"gpt-3.5-turbo": {Input: 0.50, Output: 1.50},
	"o1":            {Input: 15.00, Output: 60.00},
	"o1-mini":       {Input: 3.00, Output: 12.00},
	"o3-mini":       {Input: 1.10, Output: 4.40},
}

// EstimatedCost calculates the estimated cost for the given usage.
func (p Pricing) EstimatedCost(model string, usage daneel.Usage) float64 {
	cost, ok := p[model]
	if !ok {
		return 0
	}
	return (float64(usage.PromptTokens) * cost.Input / 1_000_000) +
		(float64(usage.CompletionTokens) * cost.Output / 1_000_000)
}

// Ensure Provider implements the interfaces at compile time.
var (
	_ daneel.Provider          = (*Provider)(nil)
	_ daneel.ModelInfoProvider = (*Provider)(nil)
)

// String returns a redacted representation for logging.
func (p *Provider) String() string {
	key := p.cfg.apiKey
	if len(key) > 8 {
		key = key[:4] + "..." + key[len(key)-4:]
	} else if key != "" {
		key = "***"
	}
	return fmt.Sprintf("OpenAI{model: %s, key: %s, base: %s}", p.cfg.model, key, p.cfg.baseURL)
}

// Compile-time interface checks.
var (
	_ daneel.Provider       = (*Provider)(nil)
	_ daneel.StreamProvider = (*Provider)(nil)
)

func init() {
	_ = strconv.Itoa // keep import alive
}
