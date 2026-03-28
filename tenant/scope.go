package tenant

import (
	"context"
	"strings"

	"github.com/Rafiki81/daneel"
)

// ScopedSessionStore wraps a daneel.SessionStore, prefixing every session ID
// with "<tenantID>:" to provide namespace isolation between tenants.
type ScopedSessionStore struct {
	inner    daneel.SessionStore
	tenantID string
}

// NewScopedSessionStore creates a session store scoped to tenantID.
func NewScopedSessionStore(inner daneel.SessionStore, tenantID string) *ScopedSessionStore {
	return &ScopedSessionStore{inner: inner, tenantID: tenantID}
}

func (s *ScopedSessionStore) key(sessionID string) string {
	if strings.HasPrefix(sessionID, s.tenantID+":") {
		return sessionID
	}
	return s.tenantID + ":" + sessionID
}

// SaveMessages stores messages under the tenant-scoped session key.
func (s *ScopedSessionStore) SaveMessages(ctx context.Context, sessionID string, msgs []daneel.Message) error {
	return s.inner.SaveMessages(ctx, s.key(sessionID), msgs)
}

// LoadMessages retrieves messages from the tenant-scoped session key.
func (s *ScopedSessionStore) LoadMessages(ctx context.Context, sessionID string) ([]daneel.Message, error) {
	return s.inner.LoadMessages(ctx, s.key(sessionID))
}

// ScopedMemory wraps a daneel.Memory, prefixing every session ID with
// "<tenantID>:" for isolation.
type ScopedMemory struct {
	inner    daneel.Memory
	tenantID string
}

// NewScopedMemory creates a memory store scoped to tenantID.
func NewScopedMemory(inner daneel.Memory, tenantID string) *ScopedMemory {
	return &ScopedMemory{inner: inner, tenantID: tenantID}
}

func (m *ScopedMemory) key(sessionID string) string {
	if strings.HasPrefix(sessionID, m.tenantID+":") {
		return sessionID
	}
	return m.tenantID + ":" + sessionID
}

// Save stores messages under the tenant-scoped session key.
func (m *ScopedMemory) Save(ctx context.Context, sessionID string, msgs []daneel.Message) error {
	return m.inner.Save(ctx, m.key(sessionID), msgs)
}

// Retrieve fetches messages from the tenant-scoped session key.
func (m *ScopedMemory) Retrieve(ctx context.Context, sessionID, query string, limit int) ([]daneel.Message, error) {
	return m.inner.Retrieve(ctx, m.key(sessionID), query, limit)
}

// Clear removes messages for the tenant-scoped session key.
func (m *ScopedMemory) Clear(ctx context.Context, sessionID string) error {
	return m.inner.Clear(ctx, m.key(sessionID))
}
