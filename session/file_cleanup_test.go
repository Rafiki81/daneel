package session_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Rafiki81/daneel/session"
)

func TestFileStore_StartCleanup_RemovesExpired(t *testing.T) {
	dir := t.TempDir()

	// Create a store with a 50ms TTL so cleanup happens quickly.
	store, err := session.NewFileStore(dir, session.WithTTL(50*time.Millisecond))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	// Write a dummy session file directly so we can backdate its mod time.
	filePath := filepath.Join(dir, "old-session.json")
	if err := os.WriteFile(filePath, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Backdate the file so it is already expired.
	past := time.Now().Add(-1 * time.Second)
	if err := os.Chtimes(filePath, past, past); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	// Start cleanup with a short interval.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.StartCleanup(ctx, 20*time.Millisecond)

	// Wait enough time for at least one cleanup tick.
	time.Sleep(100 * time.Millisecond)

	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("expired session file still exists; expected cleanup to remove it")
	}
}

func TestFileStore_StartCleanup_IdempotentCalls(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewFileStore(dir, session.WithTTL(time.Minute))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Multiple calls must not panic or launch duplicate goroutines.
	store.StartCleanup(ctx, time.Minute)
	store.StartCleanup(ctx, time.Minute)
	store.StartCleanup(ctx, time.Minute)
}

func TestFileStore_StartCleanup_NoopWithoutTTL(t *testing.T) {
	dir := t.TempDir()
	// No TTL set — StartCleanup should be a no-op.
	store, err := session.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Should not panic.
	store.StartCleanup(ctx, 10*time.Millisecond)
}
