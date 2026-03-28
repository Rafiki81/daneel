// Package mock provides a deterministic mock Provider for testing.
//
// Usage:
//
//	p := mock.New(
//		mock.Respond("Hello!"),
//		mock.RespondWithToolCall("search", `{"query": "golang"}`),
//		mock.Respond("Here are the results..."),
//	)
//	result, err := daneel.Run(ctx, agent.WithProvider(p), "help me")
package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/daneel-ai/daneel"
)

// Provider is a mock LLM provider that returns pre-configured responses.
// It implements daneel.Provider and tracks all calls made to it.
type Provider struct {
	mu        sync.Mutex
	responses []responseFunc
	index     int
	calls     []Call
}

// Call records a single invocation of Chat.
type Call struct {
	Messages []daneel.Message
	Tools    []daneel.ToolDef
}

// responseFunc generates a response given the conversation so far.
type responseFunc func(msgs []daneel.Message) *daneel.Response

// Option configures the mock provider.
type Option func(*Provider)

// New creates a new mock Provider with the given response options.
func New(opts ...Option) *Provider {
	p := &Provider{}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Respond queues a static text response.
func Respond(text string) Option {
	return func(p *Provider) {
		p.responses = append(p.responses, func(_ []daneel.Message) *daneel.Response {
			return TextResponse(text)
		})
	}
}

// RespondWithToolCall queues a response that requests a tool call.
func RespondWithToolCall(toolName, argsJSON string) Option {
	return func(p *Provider) {
		p.responses = append(p.responses, func(_ []daneel.Message) *daneel.Response {
			return &daneel.Response{
				ToolCalls: []daneel.ToolCall{{
					ID:        fmt.Sprintf("call_%s", toolName),
					Name:      toolName,
					Arguments: json.RawMessage(argsJSON),
				}},
			}
		})
	}
}

// RespondFunc queues a dynamic response function.
func RespondFunc(fn func(msgs []daneel.Message) *daneel.Response) Option {
	return func(p *Provider) {
		p.responses = append(p.responses, fn)
	}
}

// TextResponse creates a simple text Response.
func TextResponse(text string) *daneel.Response {
	return &daneel.Response{
		Content: text,
		Usage: daneel.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}
}

// ErrorResponse creates a Response that simulates an error message.
func ErrorResponse(msg string) *daneel.Response {
	return &daneel.Response{
		Content: fmt.Sprintf("Error: %s", msg),
	}
}

// Chat implements daneel.Provider. Returns the next queued response.
func (p *Provider) Chat(ctx context.Context, messages []daneel.Message, tools []daneel.ToolDef) (*daneel.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Record the call
	p.calls = append(p.calls, Call{
		Messages: append([]daneel.Message(nil), messages...),
		Tools:    append([]daneel.ToolDef(nil), tools...),
	})

	if p.index >= len(p.responses) {
		return nil, fmt.Errorf("mock provider: no more responses (got %d calls, have %d responses)", p.index+1, len(p.responses))
	}

	fn := p.responses[p.index]
	p.index++
	return fn(messages), nil
}

// --- Introspection methods ---

// Calls returns all recorded Chat calls.
func (p *Provider) Calls() []Call {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Call(nil), p.calls...)
}

// CallCount returns the number of Chat calls made.
func (p *Provider) CallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

// Reset clears all recorded calls and resets the response index.
func (p *Provider) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = nil
	p.index = 0
}

// --- Queue methods for test helpers ---

// QueueResponse appends a text response to the queue.
func (p *Provider) QueueResponse(text string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.responses = append(p.responses, func(_ []daneel.Message) *daneel.Response {
		return TextResponse(text)
	})
}

// QueueToolCall appends a tool call response to the queue.
func (p *Provider) QueueToolCall(toolName, argsJSON string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.responses = append(p.responses, func(_ []daneel.Message) *daneel.Response {
		return &daneel.Response{
			ToolCalls: []daneel.ToolCall{{
				ID:        fmt.Sprintf("call_%s", toolName),
				Name:      toolName,
				Arguments: json.RawMessage(argsJSON),
			}},
		}
	})
}

// ToolWasCalled checks if a tool was invoked during any Chat call.
// It inspects tool result messages in the recorded calls.
func (p *Provider) ToolWasCalled(toolName string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, call := range p.calls {
		for _, msg := range call.Messages {
			if msg.Role == daneel.RoleTool && msg.Name == toolName {
				return true
			}
		}
	}
	return false
}

// ToolCallArgs returns the JSON args of the last invocation of a tool.
// Returns empty string if the tool was never called.
func (p *Provider) ToolCallArgs(toolName string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := len(p.calls) - 1; i >= 0; i-- {
		for _, msg := range p.calls[i].Messages {
			for _, tc := range msg.ToolCalls {
				if tc.Name == toolName {
					return string(tc.Arguments)
				}
			}
		}
	}
	return ""
}

// LastMessages returns the messages from the most recent Chat call.
func (p *Provider) LastMessages() []daneel.Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.calls) == 0 {
		return nil
	}
	return p.calls[len(p.calls)-1].Messages
}
