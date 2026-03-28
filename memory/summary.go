package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"

	daneel "github.com/daneel-ai/daneel"
)

// SummaryOption configures the Summary memory.
type SummaryOption func(*summaryConfig)

type summaryConfig struct {
	summarizeEvery int
}

// SummarizeEvery sets how often (in messages) the memory triggers a
// summarization pass. For example, SummarizeEvery(10) means every 10 new
// messages the older ones are compressed into a running summary. Default: 10.
func SummarizeEvery(n int) SummaryOption {
	return func(c *summaryConfig) {
		if n > 0 {
			c.summarizeEvery = n
		}
	}
}

// Summary returns a Memory that periodically summarizes older messages using
// the given LLM provider. A running summary is maintained per session; recent
// messages are kept verbatim while older ones are compressed.
//
//	agent := daneel.NewAgent("assistant",
//	    daneel.WithMemory(memory.Summary(myProvider, memory.SummarizeEvery(10))),
//	)
func Summary(provider daneel.Provider, opts ...SummaryOption) daneel.Memory {
	cfg := summaryConfig{summarizeEvery: 10}
	for _, o := range opts {
		o(&cfg)
	}
	return &summaryMemory{
		provider:       provider,
		summarizeEvery: cfg.summarizeEvery,
		sessions:       make(map[string]*summarySession),
	}
}

type summarySession struct {
	summary string
	recent  []daneel.Message
	counter int
}

type summaryMemory struct {
	mu             sync.Mutex
	provider       daneel.Provider
	summarizeEvery int
	sessions       map[string]*summarySession
}

func (s *summaryMemory) Save(ctx context.Context, sessionID string, msgs []daneel.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[sessionID]
	if !ok {
		sess = &summarySession{}
		s.sessions[sessionID] = sess
	}

	for _, m := range msgs {
		if m.Role == daneel.RoleSystem {
			continue
		}
		sess.recent = append(sess.recent, m)
		sess.counter++
	}

	if sess.counter >= s.summarizeEvery && len(sess.recent) > 2 {
		if err := s.summarize(ctx, sess); err != nil {
			return fmt.Errorf("memory summary: %w", err)
		}
		sess.counter = 0
	}

	return nil
}

func (s *summaryMemory) summarize(ctx context.Context, sess *summarySession) error {
	// Keep only the last 2 messages as "recent", summarize everything before.
	toSummarize := sess.recent[:len(sess.recent)-2]
	keep := sess.recent[len(sess.recent)-2:]

	var sb strings.Builder
	if sess.summary != "" {
		sb.WriteString("Previous summary:\n")
		sb.WriteString(sess.summary)
		sb.WriteString("\n\n")
	}
	sb.WriteString("New messages to incorporate:\n")
	for _, m := range toSummarize {
		sb.WriteString(string(m.Role))
		sb.WriteString(": ")
		sb.WriteString(m.Content)
		sb.WriteString("\n")
	}

	prompt := []daneel.Message{
		{Role: daneel.RoleSystem, Content: "You are a conversation summarizer. Produce a concise summary of the conversation so far, preserving key facts, decisions, and context. Output only the summary, no preamble."},
		{Role: daneel.RoleUser, Content: sb.String()},
	}

	resp, err := s.provider.Chat(ctx, prompt, nil)
	if err != nil {
		return err
	}

	sess.summary = resp.Content
	sess.recent = make([]daneel.Message, len(keep))
	copy(sess.recent, keep)
	return nil
}

func (s *summaryMemory) Retrieve(ctx context.Context, sessionID string, _ string, limit int) ([]daneel.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[sessionID]
	if !ok {
		return nil, nil
	}

	var result []daneel.Message
	if sess.summary != "" {
		result = append(result, daneel.Message{
			Role:    daneel.RoleSystem,
			Content: "Conversation summary:\n" + sess.summary,
		})
	}
	result = append(result, sess.recent...)

	if limit > 0 && len(result) > limit {
		result = result[len(result)-limit:]
	}

	out := make([]daneel.Message, len(result))
	copy(out, result)
	return out, nil
}

func (s *summaryMemory) Clear(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
	return nil
}
