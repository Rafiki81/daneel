package daneel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

// Run executes an agent with the given user input and returns the result.
//
//	result, err := daneel.Run(ctx, agent, "What's the weather?")
//	fmt.Println(result.Output)
func Run(ctx context.Context, agent *Agent, input string, opts ...RunOption) (*RunResult, error) {
	// Resolve convenience provider if none set explicitly
	a := resolveProvider(agent)
	return run(ctx, a, input, opts...)
}

// RunStructured executes an agent and parses the response into a typed struct.
//
//	result, err := daneel.RunStructured[SentimentAnalysis](ctx, agent, "Review: Amazing!")
//	fmt.Println(result.Data.Sentiment)
//
// After a successful parse, the result is validated against the JSON Schema
// derived from T (required fields, enum constraints). If validation fails the
// agent is called once more with a concise description of the violations so it
// can self-correct.
func RunStructured[T any](ctx context.Context, agent *Agent, input string, opts ...RunOption) (*StructuredResult[T], error) {
	// Generate schema for T and add it to run options
	schema := generateSchema[T]()
	opts = append(opts, func(c *runConfig) {
		c.responseFormat = JSON
		// Store schema info for the provider
		c.responseSchema = schema
	})

	result, err := Run(ctx, agent, input, opts...)
	if err != nil {
		return nil, err
	}

	var data T
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		return nil, fmt.Errorf("daneel: failed to parse structured output: %w", err)
	}

	// Validate required fields and enum constraints against the schema.
	if violations := validateSchemaConstraints([]byte(result.Output), schema); len(violations) > 0 {
		retryInput := fmt.Sprintf(
			"%s\n\n[Your previous JSON response had validation errors: %v. Please correct them.]",
			input, violations,
		)
		retryResult, retryErr := Run(ctx, agent, retryInput, opts...)
		if retryErr != nil {
			return nil, fmt.Errorf("daneel: structured retry failed: %w (original violations: %v)", retryErr, violations)
		}
		var retryData T
		if err := json.Unmarshal([]byte(retryResult.Output), &retryData); err != nil {
			return nil, fmt.Errorf("daneel: retry parse failed: %w (original violations: %v)", err, violations)
		}
		return &StructuredResult[T]{
			RunResult: *retryResult,
			Data:      retryData,
			Raw:       retryResult.Output,
		}, nil
	}

	return &StructuredResult[T]{
		RunResult: *result,
		Data:      data,
		Raw:       result.Output,
	}, nil
}

// validateSchemaConstraints checks dataJSON against the JSON Schema constraints
// (required fields and enum values).  Returns a list of violation messages, or
// nil when the data is valid.
func validateSchemaConstraints(dataJSON []byte, schema json.RawMessage) []string {
	var s struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(schema, &s); err != nil {
		return nil
	}

	var obj map[string]any
	if err := json.Unmarshal(dataJSON, &obj); err != nil {
		return nil
	}

	var violations []string

	for _, req := range s.Required {
		v, exists := obj[req]
		if !exists || v == nil {
			violations = append(violations, fmt.Sprintf("required field %q is missing", req))
			continue
		}
		if sv, ok := v.(string); ok && sv == "" {
			violations = append(violations, fmt.Sprintf("required field %q must not be empty", req))
		}
	}

	for fname, fprop := range s.Properties {
		if len(fprop.Enum) == 0 {
			continue
		}
		v, ok := obj[fname]
		if !ok {
			continue
		}
		sv, ok := v.(string)
		if !ok {
			continue
		}
		valid := false
		for _, e := range fprop.Enum {
			if e == sv {
				valid = true
				break
			}
		}
		if !valid {
			violations = append(violations, fmt.Sprintf("field %q must be one of %v, got %q", fname, fprop.Enum, sv))
		}
	}

	return violations
}

// resolveProvider ensures the agent has a provider. If the agent was configured
// with WithModel/WithOpenAI/WithOllama/WithLocalAI, the built-in minimal
// client is used.
func resolveProvider(agent *Agent) *Agent {
	if agent.config.provider != nil {
		return agent
	}
	if agent.config.model == "" {
		return agent
	}
	// Use built-in minimal OpenAI-compat client
	cp := agent.clone()
	cp.config.provider = &miniClient{
		baseURL: "https://api.openai.com/v1",
		apiKey:  os.Getenv("OPENAI_API_KEY"),
		model:   agent.config.model,
	}
	return cp
}

// --- Convenience provider shortcuts (AgentOptions) ---

// WithModel sets the model name and uses the built-in minimal OpenAI-compat
// client. Reads OPENAI_API_KEY from the environment.
//
//	daneel.New("assistant", daneel.WithModel("gpt-4o"))
func WithModel(model string) AgentOption {
	return func(c *agentConfig) {
		c.model = model
	}
}

// WithOpenAI sets the API key and uses the built-in minimal OpenAI client.
// If apiKey is empty, reads OPENAI_API_KEY from the environment.
func WithOpenAI(apiKey string) AgentOption {
	return func(c *agentConfig) {
		if apiKey == "" {
			apiKey = os.Getenv("OPENAI_API_KEY")
		}
		c.provider = &miniClient{
			baseURL: "https://api.openai.com/v1",
			apiKey:  apiKey,
			model:   c.model,
		}
	}
}

// WithOllama sets up the built-in client pointing at a local Ollama instance
// using the OpenAI-compatible endpoint.
//
//	daneel.New("local", daneel.WithOllama("llama3.3:70b"))
func WithOllama(model string) AgentOption {
	return func(c *agentConfig) {
		c.model = model
		c.provider = &miniClient{
			baseURL: "http://localhost:11434/v1",
			model:   model,
		}
	}
}

// WithLocalAI sets up the built-in client pointing at a local LocalAI instance.
//
//	daneel.New("local", daneel.WithLocalAI("my-model"))
func WithLocalAI(model string) AgentOption {
	return func(c *agentConfig) {
		c.model = model
		c.provider = &miniClient{
			baseURL: "http://localhost:8080/v1",
			model:   model,
		}
	}
}

// --- QuickAgent ---

// QuickAgent wraps an Agent with pre-configured connectors for easy
// setup. It embeds *Agent so it can be used anywhere an Agent is expected.
type QuickAgent struct {
	*Agent
	connectors []Connector
}

// Connectors returns the connectors registered via Quick().
func (qa *QuickAgent) Connectors() []Connector { return qa.connectors }

// Quick creates a QuickAgent with convenience shortcuts for rapid prototyping.
//
//	qa := daneel.Quick("assistant",
//	    daneel.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
//	    daneel.WithModel("gpt-4o"),
//	)
//	result, err := daneel.Run(ctx, qa.Agent, "Hello")
func Quick(name string, opts ...AgentOption) *QuickAgent {
	agent := New(name, opts...)
	return &QuickAgent{Agent: agent}
}

// --- Built-in minimal OpenAI-compatible HTTP client (~80 LOC) ---
// Handles basic /v1/chat/completions calls. Supports OpenAI, Ollama
// (compat endpoint), and LocalAI. No streaming, no retry — use the
// full provider packages for those features.

type miniClient struct {
	baseURL string
	apiKey  string
	model   string
	client  http.Client
	once    sync.Once
}

type miniRequest struct {
	Model    string        `json:"model"`
	Messages []miniMessage `json:"messages"`
	Tools    []miniTool    `json:"tools,omitempty"`
}

type miniMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []miniToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type miniTool struct {
	Type     string `json:"type"`
	Function miniFn `json:"function"`
}

type miniFn struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type miniToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type miniResponse struct {
	Choices []struct {
		Message struct {
			Content   string         `json:"content"`
			ToolCalls []miniToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (mc *miniClient) Chat(ctx context.Context, messages []Message, tools []ToolDef) (*Response, error) {
	model := mc.model
	if model == "" {
		model = "gpt-4o"
	}

	// Convert messages
	miniMsgs := make([]miniMessage, len(messages))
	for i, m := range messages {
		mm := miniMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			mm.ToolCalls = append(mm.ToolCalls, miniToolCall{
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
		miniMsgs[i] = mm
	}

	// Convert tools
	var miniTools []miniTool
	for _, td := range tools {
		miniTools = append(miniTools, miniTool{
			Type: "function",
			Function: miniFn{
				Name:        td.Name,
				Description: td.Description,
				Parameters:  td.Schema,
			},
		})
	}

	reqBody := miniRequest{
		Model:    model,
		Messages: miniMsgs,
		Tools:    miniTools,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("daneel: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", mc.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("daneel: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if mc.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+mc.apiKey)
	}

	mc.once.Do(func() {
		if mc.client.Timeout == 0 {
			mc.client.Timeout = 120 * time.Second
		}
	})

	resp, err := mc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("daneel: HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("daneel: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &ProviderError{
			Provider:   "openai-compat",
			StatusCode: resp.StatusCode,
			Message:    string(respBody),
			Retryable:  resp.StatusCode == 429 || resp.StatusCode >= 500,
		}
	}

	var miniResp miniResponse
	if err := json.Unmarshal(respBody, &miniResp); err != nil {
		return nil, fmt.Errorf("daneel: unmarshal response: %w", err)
	}

	if miniResp.Error != nil {
		return nil, &ProviderError{
			Provider: "openai-compat",
			Message:  miniResp.Error.Message,
		}
	}

	if len(miniResp.Choices) == 0 {
		return nil, &ProviderError{
			Provider: "openai-compat",
			Message:  "no choices in response",
		}
	}

	choice := miniResp.Choices[0]

	// Convert tool calls
	var toolCalls []ToolCall
	for _, tc := range choice.Message.ToolCalls {
		toolCalls = append(toolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: json.RawMessage(tc.Function.Arguments),
		})
	}

	return &Response{
		Content:   choice.Message.Content,
		ToolCalls: toolCalls,
		Usage: Usage{
			PromptTokens:     miniResp.Usage.PromptTokens,
			CompletionTokens: miniResp.Usage.CompletionTokens,
			TotalTokens:      miniResp.Usage.TotalTokens,
		},
	}, nil
}
