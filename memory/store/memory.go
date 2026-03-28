// Package store provides local vector store implementations.
package store

import (
	"context"
	"encoding/gob"
	"math"
	"os"
	"sort"
	"sync"

	daneel "github.com/daneel-ai/daneel"
)

// entry is a single vector stored locally.
type entry struct {
	ID        string
	Embedding []float32
	Metadata  map[string]string
}

// Local is an in-memory vector store with optional gob persistence.
// It uses brute-force cosine similarity search — suitable for small to
// medium datasets (< 100k vectors).
type Local struct {
	mu      sync.RWMutex
	entries []entry
	path    string // empty = no persistence
}

// NewLocal creates a local vector store. If path is non-empty, the store
// will load existing data on creation and persist on every Store call.
// Pass "" for a purely in-memory store.
func NewLocal(path string) *Local {
	s := &Local{path: path}
	if path != "" {
		_ = s.load()
	}
	return s
}

// Store adds or updates a vector entry.
func (s *Local) Store(ctx context.Context, id string, embedding []float32, metadata map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update existing entry if ID matches.
	for i := range s.entries {
		if s.entries[i].ID == id {
			s.entries[i].Embedding = embedding
			s.entries[i].Metadata = metadata
			return s.persist()
		}
	}

	s.entries = append(s.entries, entry{
		ID:        id,
		Embedding: embedding,
		Metadata:  metadata,
	})
	return s.persist()
}

// Search returns the topK most similar vectors by cosine similarity.
func (s *Local) Search(ctx context.Context, query []float32, topK int) ([]daneel.VectorResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	type scored struct {
		idx   int
		score float32
	}

	scores := make([]scored, 0, len(s.entries))
	for i, e := range s.entries {
		sim := cosineSimilarity(query, e.Embedding)
		scores = append(scores, scored{idx: i, score: sim})
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	if topK > len(scores) {
		topK = len(scores)
	}

	results := make([]daneel.VectorResult, topK)
	for i := 0; i < topK; i++ {
		e := s.entries[scores[i].idx]
		results[i] = daneel.VectorResult{
			ID:       e.ID,
			Score:    scores[i].score,
			Metadata: e.Metadata,
		}
	}
	return results, nil
}

// Delete removes entries by ID.
func (s *Local) Delete(ctx context.Context, ids ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	filtered := s.entries[:0]
	for _, e := range s.entries {
		if !idSet[e.ID] {
			filtered = append(filtered, e)
		}
	}
	s.entries = filtered
	return s.persist()
}

func (s *Local) persist() error {
	if s.path == "" {
		return nil
	}
	f, err := os.Create(s.path)
	if err != nil {
		return err
	}
	defer f.Close()
	return gob.NewEncoder(f).Encode(s.entries)
}

func (s *Local) load() error {
	f, err := os.Open(s.path)
	if err != nil {
		return err
	}
	defer f.Close()
	return gob.NewDecoder(f).Decode(&s.entries)
}

// cosineSimilarity returns the cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return float32(dot / denom)
}
