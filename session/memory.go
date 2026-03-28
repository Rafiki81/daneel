package session

import (
	"context"
	"sync"
)

// MemoryStore keeps sessions in-process memory (not persistent across restarts).
type MemoryStore struct {
	m sync.Map
}

// NewMemoryStore creates a new in-memory session store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

func (s *MemoryStore) Save(_ context.Context, id string, data SessionData) error {
	s.m.Store(id, data)
	return nil
}

func (s *MemoryStore) Load(_ context.Context, id string) (SessionData, error) {
	v, ok := s.m.Load(id)
	if !ok {
		return SessionData{}, nil
	}
	return v.(SessionData), nil
}

func (s *MemoryStore) List(_ context.Context) ([]string, error) {
	var ids []string
	s.m.Range(func(k, _ any) bool {
		ids = append(ids, k.(string))
		return true
	})
	return ids, nil
}

func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.m.Delete(id)
	return nil
}
