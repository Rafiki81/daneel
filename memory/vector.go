package memory

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"

	daneel "github.com/Rafiki81/daneel"
)

// VectorOption configures the Vector memory.
type VectorOption func(*vectorConfig)

type vectorConfig struct {
	topK int
}

// TopK sets the number of similar results to retrieve on each lookup.
// Default: 5.
func TopK(k int) VectorOption {
	return func(c *vectorConfig) {
		if k > 0 {
			c.topK = k
		}
	}
}

// Vector returns a Memory backed by vector similarity search (RAG). On Save,
// each message is embedded and stored. On Retrieve, the query is embedded and
// the most similar messages are returned.
//
//	agent := daneel.NewAgent("assistant",
//	    daneel.WithMemory(memory.Vector(store, embedder, memory.TopK(5))),
//	)
func Vector(store daneel.VectorStore, embedder daneel.Embedder, opts ...VectorOption) daneel.Memory {
	cfg := vectorConfig{topK: 5}
	for _, o := range opts {
		o(&cfg)
	}
	return &vectorMemory{
		store:    store,
		embedder: embedder,
		topK:     cfg.topK,
	}
}

type vectorMemory struct {
	mu       sync.Mutex
	store    daneel.VectorStore
	embedder daneel.Embedder
	topK     int
}

func (v *vectorMemory) Save(ctx context.Context, sessionID string, msgs []daneel.Message) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	for _, m := range msgs {
		if m.Role == daneel.RoleSystem || m.Content == "" {
			continue
		}

		embedding, err := v.embedder.Embed(ctx, m.Content)
		if err != nil {
			return fmt.Errorf("memory vector embed: %w", err)
		}

		id := vectorID(sessionID, m)
		meta := map[string]string{
			"session_id": sessionID,
			"role":       string(m.Role),
			"content":    m.Content,
		}

		if err := v.store.Store(ctx, id, embedding, meta); err != nil {
			return fmt.Errorf("memory vector store: %w", err)
		}
	}
	return nil
}

func (v *vectorMemory) Retrieve(ctx context.Context, sessionID string, query string, limit int) ([]daneel.Message, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if query == "" {
		return nil, nil
	}

	queryVec, err := v.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("memory vector embed query: %w", err)
	}

	k := v.topK
	if limit > 0 && limit < k {
		k = limit
	}

	results, err := v.store.Search(ctx, queryVec, k)
	if err != nil {
		return nil, fmt.Errorf("memory vector search: %w", err)
	}

	var msgs []daneel.Message
	for _, r := range results {
		if r.Metadata["session_id"] != sessionID {
			continue
		}
		msgs = append(msgs, daneel.Message{
			Role:    daneel.Role(r.Metadata["role"]),
			Content: r.Metadata["content"],
		})
	}
	return msgs, nil
}

func (v *vectorMemory) Clear(ctx context.Context, sessionID string) error {
	// Vector stores typically don't support bulk delete by metadata.
	// Retrieve known IDs for this session then delete them.
	vec, err := v.embedder.Embed(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("memory vector clear: embed session ID: %w", err)
	}
	results, err := v.store.Search(ctx, vec, v.topK*10)
	if err != nil {
		return fmt.Errorf("memory vector clear: search: %w", err)
	}
	var ids []string
	for _, r := range results {
		if r.Metadata["session_id"] == sessionID {
			ids = append(ids, r.ID)
		}
	}
	if len(ids) > 0 {
		return v.store.Delete(ctx, ids...)
	}
	return nil
}

// vectorID produces a deterministic ID for a message in a session.
func vectorID(sessionID string, m daneel.Message) string {
	var sb strings.Builder
	sb.WriteString(sessionID)
	sb.WriteString(":")
	sb.WriteString(string(m.Role))
	sb.WriteString(":")
	sb.WriteString(m.Content)
	h := sha256.Sum256([]byte(sb.String()))
	return fmt.Sprintf("%x", h[:12])
}
