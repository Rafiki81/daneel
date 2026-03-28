package memory

import (
	"context"
	"sync"

	daneel "github.com/daneel-ai/daneel"
)

// Sliding returns a Memory that keeps the last n non-system messages per
// session. When saved messages exceed n, the oldest are discarded. This is
// the simplest and most predictable memory strategy.
//
//	agent := daneel.NewAgent("assistant",
//	    daneel.WithMemory(memory.Sliding(20)),
//	)
func Sliding(n int) daneel.Memory {
	if n <= 0 {
		n = 20
	}
	return &slidingMemory{
		maxMessages: n,
		sessions:    make(map[string][]daneel.Message),
	}
}

type slidingMemory struct {
	mu          sync.RWMutex
	maxMessages int
	sessions    map[string][]daneel.Message
}

func (s *slidingMemory) Save(ctx context.Context, sessionID string, msgs []daneel.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Filter out system messages, then replace the session entirely.
	// The runner passes the full conversation each time, so we store
	// only the last maxMessages non-system messages.
	var filtered []daneel.Message
	for _, m := range msgs {
		if m.Role == daneel.RoleSystem {
			continue
		}
		filtered = append(filtered, m)
	}

	if len(filtered) > s.maxMessages {
		filtered = filtered[len(filtered)-s.maxMessages:]
	}

	s.sessions[sessionID] = filtered
	return nil
}

func (s *slidingMemory) Retrieve(ctx context.Context, sessionID string, _ string, limit int) ([]daneel.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	msgs := s.sessions[sessionID]
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}

	out := make([]daneel.Message, len(msgs))
	copy(out, msgs)
	return out, nil
}

func (s *slidingMemory) Clear(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
	return nil
}
