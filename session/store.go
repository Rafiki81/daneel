package session

import (
	"context"
	"time"

	"github.com/daneel-ai/daneel"
)

// Metadata holds information about a persisted session.
type Metadata struct {
	SessionID string
	AgentName string
	CreatedAt time.Time
	UpdatedAt time.Time
	TurnCount int
}

// SessionData is the value persisted by a Store.
type SessionData struct {
	Messages []daneel.Message
	Meta     Metadata
}

// Store is the low-level persistence interface for session data.
type Store interface {
	Save(ctx context.Context, id string, data SessionData) error
	Load(ctx context.Context, id string) (SessionData, error)
	List(ctx context.Context) ([]string, error)
	Delete(ctx context.Context, id string) error
}

// sessionStoreAdapter bridges Store with daneel.SessionStore.
type sessionStoreAdapter struct {
	store     Store
	agentName string
}

// NewSessionStore returns a daneel.SessionStore backed by the given Store.
func NewSessionStore(s Store, agentName string) daneel.SessionStore {
	return &sessionStoreAdapter{store: s, agentName: agentName}
}

func (a *sessionStoreAdapter) LoadMessages(ctx context.Context, sessionID string) ([]daneel.Message, error) {
	data, err := a.store.Load(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return data.Messages, nil
}

func (a *sessionStoreAdapter) SaveMessages(ctx context.Context, sessionID string, msgs []daneel.Message) error {
	existing, _ := a.store.Load(ctx, sessionID)
	var createdAt time.Time
	if existing.Meta.CreatedAt.IsZero() {
		createdAt = time.Now()
	} else {
		createdAt = existing.Meta.CreatedAt
	}
	data := SessionData{
		Messages: msgs,
		Meta: Metadata{
			SessionID: sessionID,
			AgentName: a.agentName,
			CreatedAt: createdAt,
			UpdatedAt: time.Now(),
			TurnCount: existing.Meta.TurnCount + 1,
		},
	}
	return a.store.Save(ctx, sessionID, data)
}
