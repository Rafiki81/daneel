package finetune

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/daneel-ai/daneel"
)

// Format specifies the output format for training data.
type Format int

const (
	ShareGPT  Format = iota // Multi-turn conversation (Unsloth/Axolotl)
	Alpaca                  // instruction/input/output triplets
	OpenAIFmt               // OpenAI fine-tuning format
	ChatML                  // Raw ChatML template
)

// Collector captures agent conversations for fine-tuning.
// It is thread-safe and can be shared across multiple agents.
type Collector struct {
	mu               sync.Mutex
	format           Format
	storage          string
	minTurns         int
	includeToolCalls bool
	count            int
}

// CollectorOption configures a Collector.
type CollectorOption func(*Collector)

// WithFormat sets the output format.
func WithFormat(f Format) CollectorOption {
	return func(c *Collector) { c.format = f }
}

// WithStorage sets the directory for captured data.
func WithStorage(dir string) CollectorOption {
	return func(c *Collector) { c.storage = dir }
}

// WithMinTurns skips conversations shorter than n turns.
func WithMinTurns(n int) CollectorOption {
	return func(c *Collector) { c.minTurns = n }
}

// WithIncludeToolCalls includes tool call data in training samples.
func WithIncludeToolCalls(b bool) CollectorOption {
	return func(c *Collector) { c.includeToolCalls = b }
}

// NewCollector creates a training data collector.
func NewCollector(opts ...CollectorOption) *Collector {
	c := &Collector{
		format:           ShareGPT,
		storage:          "./training_data",
		minTurns:         1,
		includeToolCalls: true,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Capture is a callback for daneel.WithOnConversationEnd.
func (c *Collector) Capture(ctx context.Context, result daneel.RunResult) {
	if result.Turns < c.minTurns {
		return
	}

	var data any
	switch c.format {
	case ShareGPT:
		data = c.toShareGPT(result)
	case Alpaca:
		data = c.toAlpaca(result)
	case OpenAIFmt:
		data = c.toOpenAI(result)
	case ChatML:
		data = c.toChatML(result)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := os.MkdirAll(c.storage, 0o755); err != nil {
		return
	}

	c.count++
	filename := filepath.Join(c.storage, fmt.Sprintf("conv_%d_%d.json", time.Now().Unix(), c.count))

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(filename, b, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "finetune: failed to write %s: %v\n", filename, err)
	}
}

// Count returns the number of captured conversations.
func (c *Collector) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

// --- Format converters ---

type shareGPTConv struct {
	Conversations []shareGPTMsg `json:"conversations"`
}

type shareGPTMsg struct {
	From  string `json:"from"`
	Value string `json:"value"`
}

func (c *Collector) toShareGPT(r daneel.RunResult) shareGPTConv {
	var msgs []shareGPTMsg
	for _, m := range r.Messages {
		switch m.Role {
		case daneel.RoleSystem:
			msgs = append(msgs, shareGPTMsg{From: "system", Value: m.Content})
		case daneel.RoleUser:
			msgs = append(msgs, shareGPTMsg{From: "human", Value: m.Content})
		case daneel.RoleAssistant:
			value := m.Content
			if c.includeToolCalls && len(m.ToolCalls) > 0 {
				b, _ := json.Marshal(m.ToolCalls)
				value += "\n<tool_calls>" + string(b) + "</tool_calls>"
			}
			msgs = append(msgs, shareGPTMsg{From: "gpt", Value: value})
		case daneel.RoleTool:
			if c.includeToolCalls {
				msgs = append(msgs, shareGPTMsg{From: "tool", Value: m.Content})
			}
		}
	}
	return shareGPTConv{Conversations: msgs}
}

type alpacaSample struct {
	Instruction string `json:"instruction"`
	Input       string `json:"input"`
	Output      string `json:"output"`
}

func (c *Collector) toAlpaca(r daneel.RunResult) alpacaSample {
	var instruction, input, output string
	for _, m := range r.Messages {
		switch m.Role {
		case daneel.RoleSystem:
			instruction = m.Content
		case daneel.RoleUser:
			input = m.Content
		case daneel.RoleAssistant:
			output = m.Content
		}
	}
	return alpacaSample{Instruction: instruction, Input: input, Output: output}
}

type openAISample struct {
	Messages []openAIMsg `json:"messages"`
}

type openAIMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (c *Collector) toOpenAI(r daneel.RunResult) openAISample {
	var msgs []openAIMsg
	for _, m := range r.Messages {
		if m.Role == daneel.RoleTool && !c.includeToolCalls {
			continue
		}
		msgs = append(msgs, openAIMsg{Role: string(m.Role), Content: m.Content})
	}
	return openAISample{Messages: msgs}
}

func (c *Collector) toChatML(r daneel.RunResult) string {
	var s string
	for _, m := range r.Messages {
		if m.Role == daneel.RoleTool && !c.includeToolCalls {
			continue
		}
		s += fmt.Sprintf("<|im_start|>%s\n%s<|im_end|>\n", m.Role, m.Content)
	}
	return s
}
