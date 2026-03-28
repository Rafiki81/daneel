// Package memory provides conversation memory implementations for Daneel agents.
//
// All memory operations are scoped by session ID. The Runner passes the
// current session ID automatically.
//
// Available implementations:
//   - Sliding(n) — keep last N messages per session
//   - Summary(provider, opts...) — periodically summarize older messages with LLM
//   - Vector(store, embedder, opts...) — RAG: search relevant context before each turn
//   - Composite(memories...) — combine multiple memory backends
package memory

import (
	"context"
	"fmt"

	daneel "github.com/daneel-ai/daneel"
)

// Composite combines multiple memory implementations. On Save, all backends
// are called. On Retrieve, results from all backends are merged and
// deduplicated. On Clear, all backends are cleared.
//
//	m := memory.Composite(memory.Sliding(20), memory.Vector(store, embedder))
func Composite(backends ...daneel.Memory) daneel.Memory {
	if len(backends) == 1 {
		return backends[0]
	}
	return &compositeMemory{backends: backends}
}

type compositeMemory struct {
	backends []daneel.Memory
}

func (c *compositeMemory) Save(ctx context.Context, sessionID string, msgs []daneel.Message) error {
	for _, b := range c.backends {
		if err := b.Save(ctx, sessionID, msgs); err != nil {
			return fmt.Errorf("memory composite save: %w", err)
		}
	}
	return nil
}

func (c *compositeMemory) Retrieve(ctx context.Context, sessionID string, query string, limit int) ([]daneel.Message, error) {
	var all []daneel.Message
	seen := make(map[string]bool)

	for _, b := range c.backends {
		msgs, err := b.Retrieve(ctx, sessionID, query, limit)
		if err != nil {
			return nil, fmt.Errorf("memory composite retrieve: %w", err)
		}
		for _, m := range msgs {
			key := string(m.Role) + ":" + m.Content
			if !seen[key] {
				seen[key] = true
				all = append(all, m)
			}
		}
	}

	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func (c *compositeMemory) Clear(ctx context.Context, sessionID string) error {
	for _, b := range c.backends {
		if err := b.Clear(ctx, sessionID); err != nil {
			return fmt.Errorf("memory composite clear: %w", err)
		}
	}
	return nil
}
