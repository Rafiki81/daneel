package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileStore persists sessions as JSON files in a directory.
type FileStore struct {
	dir string
	ttl time.Duration
}

// FileStoreOption configures a FileStore.
type FileStoreOption func(*FileStore)

// WithTTL sets a maximum age for stored sessions; older sessions are removed on List.
func WithTTL(d time.Duration) FileStoreOption {
	return func(s *FileStore) { s.ttl = d }
}

// NewFileStore creates a FileStore rooted at dir.
func NewFileStore(dir string, opts ...FileStoreOption) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("session: create directory: %w", err)
	}
	s := &FileStore{dir: dir}
	for _, o := range opts {
		o(s)
	}
	return s, nil
}

func (s *FileStore) path(id string) string {
	return filepath.Join(s.dir, id+".json")
}

func (s *FileStore) Save(_ context.Context, id string, data SessionData) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("session: marshal: %w", err)
	}
	tmp := s.path(id) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("session: write temp: %w", err)
	}
	if err := os.Rename(tmp, s.path(id)); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("session: rename: %w", err)
	}
	return nil
}

func (s *FileStore) Load(_ context.Context, id string) (SessionData, error) {
	b, err := os.ReadFile(s.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return SessionData{}, nil
	}
	if err != nil {
		return SessionData{}, fmt.Errorf("session: read: %w", err)
	}
	var data SessionData
	if err := json.Unmarshal(b, &data); err != nil {
		return SessionData{}, fmt.Errorf("session: unmarshal: %w", err)
	}
	return data, nil
}

func (s *FileStore) List(_ context.Context) ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("session: readdir: %w", err)
	}
	var ids []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		if s.ttl > 0 {
			info, err := e.Info()
			if err == nil && time.Since(info.ModTime()) > s.ttl {
				_ = os.Remove(filepath.Join(s.dir, name))
				continue
			}
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *FileStore) Delete(_ context.Context, id string) error {
	err := os.Remove(s.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
