// Package anthropic implements the daneel.Provider interface for the
// Anthropic Messages API (Claude models).
//
// Usage:
//
//	p := anthropic.New(
//	    anthropic.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")),
//	    anthropic.WithModel("claude-sonnet-4-20250514"),
//	)
//	agent := daneel.New("assistant", daneel.WithProvider(p))
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	daneel "github.com/daneel-ai/daneel"
)

// KnownModels maps Anthropic model names to their capabilities.
var KnownModels = map[string]daneel.ModelInfo{
	"claude-sonnet-4-20250514":   {ContextWindow: 200_000, MaxOutput: 8_192, SupportsVision: true, SupportsTools: true, SupportsJSON: true},
	"claude-opus-4-20250514":     {ContextWindow: 200_000, MaxOutput: 8_192, SupportsVision: true, SupportsTools: true, SupportsJSON: true},
	"claude-3-5-sonnet-20241022": {ContextWindow: 200_000, MaxOutput: 8_192, SupportsVision: true, SupportsTools: true, SupportsJSON: true},
	"claude-3-5-haiku-20241022":  {ContextWindow: 200_000, MaxOutput: 8_192, SupportsVision: true, SupportsTools: true, SupportsJSON: true},
	"claude-3-opus-20240229":     {ContextWindow: 200_000, MaxOutput: 4_096, SupportsVision: true, SupportsTools: true, SupportsJSON: true},
	"claude-3-haiku-20240307":    {ContextWindow: 200_000, MaxOutput: 4_096, SupportsVision: true, SupportsTools: true, SupportsJSON: true},
}

// Option configures the Anthropic provider.
type Option func(*config)

type config struct {
	apiKey      string
	model       string
	baseURL     string
	maxTokens   int
	temperature *float64
	timeout     time.Duration
	retry       RetryConfig
	client      *http.Client
	betaHeaders []string
}

// RetryConfig controls retry behavior for failed API calls.
type RetryConfig struct {
	MaxRetries  int
	InitialWait time.Duration
	MaxWait     time.Duration
	RetryOn     []int
}

func defaultConfig() config {
	return config{
		apiKey:    os.Getenv("ANTHROPIC_API_KEY"),
		model:     "claude-sonnet-4-20250514",
		baseURL:   "https://api.anthropic.com",
		maxTokens: 4096,
		timeout:   120 * time.Second,
		retry: RetryConfig{
			MaxRetries:  3,
			InitialWait: 1 * time.Second,
			MaxWait:     30 * time.Second,
			RetryOn:     []int{429, 500, 502, 503, 529},
		},
	}
}

// WithAPIKey sets the Anthropic API key.
func WithAPIKey(key string) Option {
	return func(c *config) {
		if key != "" {
			c.apiKey = key
		}
	}
}

// WithModel sets the model name.
func WithModel(model string) Option {
	return func(c *config) { c.model = model }
}

// WithBaseURL sets the API base URL.
func WithBaseURL(url string) Option {
	return func(c *config) { c.baseURL = url }
}

// WithMaxTokens sets the maximum output tokens.
func WithMaxTokens(n int) Option {
	return func(c *config) { c.maxTokens = n }
}

// WithTemperature sets the sampling temperature.
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

// WithBetaHeaders adds beta feature headers.
func WithBetaHeaders(headers ...string) Option {
	return func(c *config) { c.betaHeaders = append(c.betaHeaders, headers...) }
}

// Provider implements daneel.Provider for Anthropic.
type Provider struct {
	cfg    config
	client *http.Client
}

// New creates a new Anthropic provider.
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
	return daneel.ModelInfo{
		ContextWindow: 200_000, MaxOutput: 4_096,
		SupportsVision: true, SupportsTools: true, SupportsJSON: true,
	}, nil
}

// --- API types ---

type messagesRequest struct {
	Model       string           `json:"model"`
	Messages    []requestMessage `json:"messages"`
	System      string           `json:"system,omitempty"`
	MaxTokens   int              `json:"max_tokens"`
	Temperature *float64         `json:"temperature,omitempty"`
	Tools       []requestTool    `json:"tools,omitempty"`
}

type requestMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

type requestTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type messagesResponse struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Role       string         `json:"role"`
	Content    []contentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// --- Retry logic ---

func (p *Provider) chatWithRetry(ctx context.Context, messages []daneel.Message, tools []daneel.ToolDef) (*daneel.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= p.cfg.retry.MaxRetries; attempt++ {
		if attempt > 0 {
			wait := p.cfg.retry.InitialWait * time.Duration(math.Pow(2, float64(attempt-1)))
			if wait > p.cfg.retry.MaxWait {
				wait = p.cfg.retry.MaxWait
			}
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
		if !shouldRetry(statusCode, p.cfg.retry.RetryOn) {
			return nil, err
		}
	}
	return nil, lastErr
}

func shouldRetry(statusCode int, codes []int) bool {
	if statusCode == 0 {
		return false
	}
	for _, code := range codes {
		if statusCode == code {
			return true
		}
	}
	return false
}

func (p *Provider) doChat(ctx context.Context, messages []daneel.Message, tools []daneel.ToolDef) (*daneel.Response, int, error) {
	var systemPrompt string
	var convMsgs []daneel.Message
	for _, m := range messages {
		if m.Role == daneel.RoleSystem {
			if systemPrompt != "" {
				systemPrompt += "\n\n"
			}
			systemPrompt += m.Content
		} else {
			convMsgs = append(convMsgs, m)
		}
	}

	reqMsgs := convertMessages(convMsgs)

	var reqTools []requestTool
	for _, td := range tools {
		reqTools = append(reqTools, requestTool{
			Name: td.Name, Description: td.Description, InputSchema: td.Schema,
		})
	}

	reqBody := messagesRequest{
		Model: p.cfg.model, Messages: reqMsgs, System: systemPrompt,
		MaxTokens: p.cfg.maxTokens, Temperature: p.cfg.temperature, Tools: reqTools,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("anthropic: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.cfg.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("anthropic: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.cfg.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	if len(p.cfg.betaHeaders) > 0 {
		req.Header.Set("anthropic-beta", strings.Join(p.cfg.betaHeaders, ","))
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("anthropic: HTTP: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("anthropic: read: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, &daneel.ProviderError{
			Provider: "anthropic", StatusCode: resp.StatusCode,
			Message: string(respBody), Retryable: resp.StatusCode == 429 || resp.StatusCode >= 500,
		}
	}

	var msgResp messagesResponse
	if err := json.Unmarshal(respBody, &msgResp); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("anthropic: unmarshal: %w", err)
	}
	if msgResp.Error != nil {
		return nil, resp.StatusCode, &daneel.ProviderError{
			Provider: "anthropic", Message: msgResp.Error.Message,
		}
	}

	var textParts []string
	var toolCalls []daneel.ToolCall
	for _, block := range msgResp.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			toolCalls = append(toolCalls, daneel.ToolCall{
				ID: block.ID, Name: block.Name, Arguments: block.Input,
			})
		}
	}

	return &daneel.Response{
		Content: strings.Join(textParts, ""), ToolCalls: toolCalls,
		Usage: daneel.Usage{
			PromptTokens: msgResp.Usage.InputTokens, CompletionTokens: msgResp.Usage.OutputTokens,
			TotalTokens: msgResp.Usage.InputTokens + msgResp.Usage.OutputTokens,
		},
	}, resp.StatusCode, nil
}

// --- Message conversion ---

func convertMessages(msgs []daneel.Message) []requestMessage {
	var result []requestMessage
	for _, m := range msgs {
		switch m.Role {
		case daneel.RoleUser:
			block := contentBlock{Type: "text", Text: m.Content}
			raw, _ := json.Marshal([]contentBlock{block})
			result = append(result, requestMessage{Role: "user", Content: raw})
		case daneel.RoleAssistant:
			var blocks []contentBlock
			if m.Content != "" {
				blocks = append(blocks, contentBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, contentBlock{
					Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: tc.Arguments,
				})
			}
			if len(blocks) == 0 {
				blocks = append(blocks, contentBlock{Type: "text", Text: ""})
			}
			raw, _ := json.Marshal(blocks)
			result = append(result, requestMessage{Role: "assistant", Content: raw})
		case daneel.RoleTool:
			block := contentBlock{Type: "tool_result", ToolUseID: m.ToolCallID, Content: m.Content}
			raw, _ := json.Marshal([]contentBlock{block})
			result = append(result, requestMessage{Role: "user", Content: raw})
		}
	}
	return mergeConsecutive(result)
}

func mergeConsecutive(msgs []requestMessage) []requestMessage {
	if len(msgs) <= 1 {
		return msgs
	}
	var merged []requestMessage
	merged = append(merged, msgs[0])
	for i := 1; i < len(msgs); i++ {
		last := &merged[len(merged)-1]
		if last.Role == msgs[i].Role {
			var lb, nb []contentBlock
			_ = json.Unmarshal(last.Content, &lb)
			_ = json.Unmarshal(msgs[i].Content, &nb)
			combined, _ := json.Marshal(append(lb, nb...))
			last.Content = combined
		} else {
			merged = append(merged, msgs[i])
		}
	}
	return merged
}

// --- Pricing ---

// ModelCost holds input and output costs per 1M tokens.
type ModelCost struct {
	Input  float64
	Output float64
}

// AnthropicPricing contains known pricing.
var AnthropicPricing = map[string]ModelCost{
	"claude-sonnet-4-20250514":   {Input: 3.00, Output: 15.00},
	"claude-opus-4-20250514":     {Input: 15.00, Output: 75.00},
	"claude-3-5-sonnet-20241022": {Input: 3.00, Output: 15.00},
	"claude-3-5-haiku-20241022":  {Input: 0.80, Output: 4.00},
	"claude-3-opus-20240229":     {Input: 15.00, Output: 75.00},
	"claude-3-haiku-20240307":    {Input: 0.25, Output: 1.25},
}

// Compile-time interface checks.
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
	return fmt.Sprintf("Anthropic{model: %s, key: %s}", p.cfg.model, key)
}
