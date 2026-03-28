// Package google implements the daneel.Provider interface for Google's
// Gemini API (generativelanguage.googleapis.com).
//
// Usage:
//
//	p := google.New(
//	    google.WithAPIKey(os.Getenv("GOOGLE_API_KEY")),
//	    google.WithModel("gemini-2.0-flash"),
//	)
//	agent := daneel.New("assistant", daneel.WithProvider(p))
package google

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

	daneel "github.com/Rafiki81/daneel"
)

// SafetySetting controls content safety filtering.
type SafetySetting string

const (
	BlockNone        SafetySetting = "BLOCK_NONE"
	BlockLowAndAbove SafetySetting = "BLOCK_LOW_AND_ABOVE"
	BlockMedAndAbove SafetySetting = "BLOCK_MEDIUM_AND_ABOVE"
	BlockHighOnly    SafetySetting = "BLOCK_ONLY_HIGH"
)

// KnownModels maps Gemini model names to their capabilities.
var KnownModels = map[string]daneel.ModelInfo{
	"gemini-2.0-flash":      {ContextWindow: 1_048_576, MaxOutput: 8_192, SupportsVision: true, SupportsTools: true, SupportsJSON: true},
	"gemini-2.0-flash-lite": {ContextWindow: 1_048_576, MaxOutput: 8_192, SupportsVision: true, SupportsTools: true, SupportsJSON: true},
	"gemini-1.5-pro":        {ContextWindow: 2_097_152, MaxOutput: 8_192, SupportsVision: true, SupportsTools: true, SupportsJSON: true},
	"gemini-1.5-flash":      {ContextWindow: 1_048_576, MaxOutput: 8_192, SupportsVision: true, SupportsTools: true, SupportsJSON: true},
}

// Option configures the Google Gemini provider.
type Option func(*config)

type config struct {
	apiKey         string
	model          string
	baseURL        string
	maxTokens      int
	temperature    *float64
	timeout        time.Duration
	retry          RetryConfig
	client         *http.Client
	safetySettings SafetySetting
}

// RetryConfig controls retry behavior.
type RetryConfig struct {
	MaxRetries  int
	InitialWait time.Duration
	MaxWait     time.Duration
	RetryOn     []int
}

func defaultConfig() config {
	return config{
		apiKey:  os.Getenv("GOOGLE_API_KEY"),
		model:   "gemini-2.0-flash",
		baseURL: "https://generativelanguage.googleapis.com",
		timeout: 120 * time.Second,
		retry: RetryConfig{
			MaxRetries: 3, InitialWait: 1 * time.Second,
			MaxWait: 30 * time.Second, RetryOn: []int{429, 500, 502, 503},
		},
	}
}

// WithAPIKey sets the Google API key.
func WithAPIKey(key string) Option {
	return func(c *config) {
		if key != "" {
			c.apiKey = key
		}
	}
}

// WithModel sets the model name.
func WithModel(model string) Option { return func(c *config) { c.model = model } }

// WithBaseURL sets the API base URL.
func WithBaseURL(url string) Option { return func(c *config) { c.baseURL = url } }

// WithMaxTokens sets the maximum output tokens.
func WithMaxTokens(n int) Option { return func(c *config) { c.maxTokens = n } }

// WithTemperature sets the sampling temperature.
func WithTemperature(t float64) Option { return func(c *config) { c.temperature = &t } }

// WithTimeout sets the HTTP request timeout.
func WithTimeout(d time.Duration) Option { return func(c *config) { c.timeout = d } }

// WithRetry sets the retry configuration.
func WithRetry(r RetryConfig) Option { return func(c *config) { c.retry = r } }

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(cl *http.Client) Option { return func(c *config) { c.client = cl } }

// WithSafetySettings sets the safety filtering level.
func WithSafetySettings(s SafetySetting) Option { return func(c *config) { c.safetySettings = s } }

// Provider implements daneel.Provider for Google Gemini.
type Provider struct {
	cfg    config
	client *http.Client
}

// New creates a new Google Gemini provider.
func New(opts ...Option) *Provider {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	cl := cfg.client
	if cl == nil {
		cl = &http.Client{Timeout: cfg.timeout}
	}
	return &Provider{cfg: cfg, client: cl}
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
		ContextWindow: 1_048_576, MaxOutput: 8_192,
		SupportsVision: true, SupportsTools: true, SupportsJSON: true,
	}, nil
}

// --- Gemini API types ---

type generateRequest struct {
	Contents          []geminiContent      `json:"contents"`
	SystemInstruction *geminiContent       `json:"system_instruction,omitempty"`
	Tools             []geminiTool         `json:"tools,omitempty"`
	GenerationConfig  *generationConfig    `json:"generation_config,omitempty"`
	SafetySettings    []safetySettingEntry `json:"safety_settings,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string            `json:"text,omitempty"`
	FunctionCall     *geminiFnCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFnResponse `json:"functionResponse,omitempty"`
}

type geminiFnCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type geminiFnResponse struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFnDecl `json:"function_declarations"`
}

type geminiFnDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type generationConfig struct {
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
}

type safetySettingEntry struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

type generateResponse struct {
	Candidates []struct {
		Content      geminiContent `json:"content"`
		FinishReason string        `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// --- Retry ---

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
		resp, sc, err := p.doChat(ctx, messages, tools)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !shouldRetry(sc, p.cfg.retry.RetryOn) {
			return nil, err
		}
	}
	return nil, lastErr
}

func shouldRetry(sc int, codes []int) bool {
	for _, c := range codes {
		if sc == c {
			return true
		}
	}
	return false
}

// --- Core request ---

func (p *Provider) doChat(ctx context.Context, messages []daneel.Message, tools []daneel.ToolDef) (*daneel.Response, int, error) {
	var sysParts []geminiPart
	var contents []geminiContent

	for _, m := range messages {
		switch m.Role {
		case daneel.RoleSystem:
			sysParts = append(sysParts, geminiPart{Text: m.Content})
		case daneel.RoleUser:
			contents = append(contents, geminiContent{
				Role: "user", Parts: []geminiPart{{Text: m.Content}},
			})
		case daneel.RoleAssistant:
			var parts []geminiPart
			if m.Content != "" {
				parts = append(parts, geminiPart{Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				parts = append(parts, geminiPart{
					FunctionCall: &geminiFnCall{Name: tc.Name, Args: tc.Arguments},
				})
			}
			if len(parts) == 0 {
				parts = []geminiPart{{Text: ""}}
			}
			contents = append(contents, geminiContent{Role: "model", Parts: parts})
		case daneel.RoleTool:
			rd, _ := json.Marshal(map[string]string{"result": m.Content})
			contents = append(contents, geminiContent{
				Role: "user",
				Parts: []geminiPart{{
					FunctionResponse: &geminiFnResponse{Name: m.ToolCallID, Response: rd},
				}},
			})
		}
	}

	contents = mergeContents(contents)

	var gTools []geminiTool
	if len(tools) > 0 {
		var decls []geminiFnDecl
		for _, td := range tools {
			decls = append(decls, geminiFnDecl{
				Name: td.Name, Description: td.Description, Parameters: td.Schema,
			})
		}
		gTools = []geminiTool{{FunctionDeclarations: decls}}
	}

	reqBody := generateRequest{Contents: contents, Tools: gTools}
	if len(sysParts) > 0 {
		reqBody.SystemInstruction = &geminiContent{Parts: sysParts}
	}
	if p.cfg.maxTokens > 0 || p.cfg.temperature != nil {
		reqBody.GenerationConfig = &generationConfig{
			MaxOutputTokens: p.cfg.maxTokens, Temperature: p.cfg.temperature,
		}
	}
	if p.cfg.safetySettings != "" {
		for _, cat := range []string{
			"HARM_CATEGORY_HARASSMENT", "HARM_CATEGORY_HATE_SPEECH",
			"HARM_CATEGORY_SEXUALLY_EXPLICIT", "HARM_CATEGORY_DANGEROUS_CONTENT",
		} {
			reqBody.SafetySettings = append(reqBody.SafetySettings, safetySettingEntry{
				Category: cat, Threshold: string(p.cfg.safetySettings),
			})
		}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("google: marshal: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s",
		p.cfg.baseURL, p.cfg.model, p.cfg.apiKey)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("google: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("google: HTTP: %w", err)
	}
	defer resp.Body.Close()

	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("google: read: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, &daneel.ProviderError{
			Provider: "google", StatusCode: resp.StatusCode,
			Message: string(rb), Retryable: resp.StatusCode == 429 || resp.StatusCode >= 500,
		}
	}

	var gr generateResponse
	if err := json.Unmarshal(rb, &gr); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("google: unmarshal: %w", err)
	}
	if gr.Error != nil {
		return nil, gr.Error.Code, &daneel.ProviderError{
			Provider: "google", StatusCode: gr.Error.Code, Message: gr.Error.Message,
		}
	}
	if len(gr.Candidates) == 0 {
		return nil, resp.StatusCode, &daneel.ProviderError{
			Provider: "google", Message: "no candidates in response",
		}
	}

	cand := gr.Candidates[0]
	var texts []string
	var tcs []daneel.ToolCall
	for _, part := range cand.Content.Parts {
		if part.Text != "" {
			texts = append(texts, part.Text)
		}
		if part.FunctionCall != nil {
			tcs = append(tcs, daneel.ToolCall{
				ID: part.FunctionCall.Name, Name: part.FunctionCall.Name,
				Arguments: part.FunctionCall.Args,
			})
		}
	}

	return &daneel.Response{
		Content: strings.Join(texts, ""), ToolCalls: tcs,
		Usage: daneel.Usage{
			PromptTokens:     gr.UsageMetadata.PromptTokenCount,
			CompletionTokens: gr.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      gr.UsageMetadata.TotalTokenCount,
		},
	}, resp.StatusCode, nil
}

func mergeContents(cs []geminiContent) []geminiContent {
	if len(cs) <= 1 {
		return cs
	}
	merged := []geminiContent{cs[0]}
	for i := 1; i < len(cs); i++ {
		last := &merged[len(merged)-1]
		if last.Role == cs[i].Role {
			last.Parts = append(last.Parts, cs[i].Parts...)
		} else {
			merged = append(merged, cs[i])
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

// GeminiPricing contains known pricing.
var GeminiPricing = map[string]ModelCost{
	"gemini-2.0-flash":      {Input: 0.10, Output: 0.40},
	"gemini-2.0-flash-lite": {Input: 0.075, Output: 0.30},
	"gemini-1.5-pro":        {Input: 1.25, Output: 5.00},
	"gemini-1.5-flash":      {Input: 0.075, Output: 0.30},
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
	return fmt.Sprintf("Google{model: %s, key: %s}", p.cfg.model, key)
}
