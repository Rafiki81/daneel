package memory_test

import (
	"context"
	"testing"

	"github.com/Rafiki81/daneel"
	"github.com/Rafiki81/daneel/memory"
	"github.com/Rafiki81/daneel/memory/store"
)

// ---------- Sliding ----------

func TestSlidingSaveAndRetrieve(t *testing.T) {
	m := memory.Sliding(3)
	ctx := context.Background()
	msgs := []daneel.Message{
		daneel.UserMessage("one"),
		daneel.AssistantMessage("two"),
		daneel.UserMessage("three"),
		daneel.AssistantMessage("four"),
		daneel.UserMessage("five"),
	}
	if err := m.Save(ctx, "s1", msgs); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := m.Retrieve(ctx, "s1", "", 0)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d msgs, want 3", len(got))
	}
	if got[0].Content != "three" {
		t.Fatalf("first msg = %q, want three", got[0].Content)
	}
}

func TestSlidingFiltersSystemMessages(t *testing.T) {
	m := memory.Sliding(10)
	ctx := context.Background()
	msgs := []daneel.Message{
		daneel.SystemMessage("instructions"),
		daneel.UserMessage("hello"),
		daneel.AssistantMessage("hi"),
	}
	m.Save(ctx, "s1", msgs)
	got, _ := m.Retrieve(ctx, "s1", "", 0)
	for _, msg := range got {
		if msg.Role == daneel.RoleSystem {
			t.Fatal("system messages should be filtered out")
		}
	}
	if len(got) != 2 {
		t.Fatalf("got %d msgs, want 2", len(got))
	}
}

func TestSlidingDefaultN(t *testing.T) {
	m := memory.Sliding(0) // should default to 20
	ctx := context.Background()
	var msgs []daneel.Message
	for i := 0; i < 25; i++ {
		msgs = append(msgs, daneel.UserMessage("msg"))
	}
	m.Save(ctx, "s1", msgs)
	got, _ := m.Retrieve(ctx, "s1", "", 0)
	if len(got) != 20 {
		t.Fatalf("got %d msgs, want 20 (default)", len(got))
	}
}

func TestSlidingRetrieveWithLimit(t *testing.T) {
	m := memory.Sliding(10)
	ctx := context.Background()
	msgs := []daneel.Message{
		daneel.UserMessage("a"),
		daneel.AssistantMessage("b"),
		daneel.UserMessage("c"),
		daneel.AssistantMessage("d"),
	}
	m.Save(ctx, "s1", msgs)
	got, _ := m.Retrieve(ctx, "s1", "", 2)
	if len(got) != 2 {
		t.Fatalf("got %d msgs, want 2", len(got))
	}
	if got[0].Content != "c" {
		t.Fatalf("first = %q, want c", got[0].Content)
	}
}

func TestSlidingClear(t *testing.T) {
	m := memory.Sliding(10)
	ctx := context.Background()
	m.Save(ctx, "s1", []daneel.Message{daneel.UserMessage("hi")})
	m.Clear(ctx, "s1")
	got, _ := m.Retrieve(ctx, "s1", "", 0)
	if len(got) != 0 {
		t.Fatalf("got %d msgs after clear, want 0", len(got))
	}
}

func TestSlidingIsolatesSessions(t *testing.T) {
	m := memory.Sliding(10)
	ctx := context.Background()
	m.Save(ctx, "s1", []daneel.Message{daneel.UserMessage("s1-msg")})
	m.Save(ctx, "s2", []daneel.Message{daneel.UserMessage("s2-msg")})
	got1, _ := m.Retrieve(ctx, "s1", "", 0)
	got2, _ := m.Retrieve(ctx, "s2", "", 0)
	if len(got1) != 1 || got1[0].Content != "s1-msg" {
		t.Fatal("session 1 data wrong")
	}
	if len(got2) != 1 || got2[0].Content != "s2-msg" {
		t.Fatal("session 2 data wrong")
	}
}

// ---------- Composite ----------

func TestCompositeSingleBackend(t *testing.T) {
	s := memory.Sliding(5)
	c := memory.Composite(s)
	// Should return the same backend, not wrap it
	ctx := context.Background()
	c.Save(ctx, "s1", []daneel.Message{daneel.UserMessage("hi")})
	got, _ := c.Retrieve(ctx, "s1", "", 0)
	if len(got) != 1 {
		t.Fatalf("got %d msgs, want 1", len(got))
	}
}

func TestCompositeMergesAndDeduplicates(t *testing.T) {
	s1 := memory.Sliding(10)
	s2 := memory.Sliding(10)
	c := memory.Composite(s1, s2)
	ctx := context.Background()

	msgs := []daneel.Message{
		daneel.UserMessage("hello"),
		daneel.AssistantMessage("world"),
	}
	// Save to composite saves to both backends
	c.Save(ctx, "s1", msgs)

	got, err := c.Retrieve(ctx, "s1", "", 0)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	// Both backends have same msgs, composite should dedup
	if len(got) != 2 {
		t.Fatalf("got %d msgs, want 2 (deduped)", len(got))
	}
}

func TestCompositeClear(t *testing.T) {
	s1 := memory.Sliding(10)
	s2 := memory.Sliding(10)
	c := memory.Composite(s1, s2)
	ctx := context.Background()
	c.Save(ctx, "s1", []daneel.Message{daneel.UserMessage("hi")})
	c.Clear(ctx, "s1")
	got1, _ := s1.Retrieve(ctx, "s1", "", 0)
	got2, _ := s2.Retrieve(ctx, "s1", "", 0)
	if len(got1)+len(got2) != 0 {
		t.Fatal("clear should affect all backends")
	}
}

// ---------- Local vector store ----------

func TestLocalStoreAndSearch(t *testing.T) {
	s := store.NewLocal("")
	ctx := context.Background()

	// Store three vectors
	s.Store(ctx, "v1", []float32{1, 0, 0}, map[string]string{"text": "hello"})
	s.Store(ctx, "v2", []float32{0, 1, 0}, map[string]string{"text": "world"})
	s.Store(ctx, "v3", []float32{1, 0.1, 0}, map[string]string{"text": "hi"})

	// Search with query similar to v1 and v3
	results, err := s.Search(ctx, []float32{1, 0, 0}, 2)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	// v1 should be most similar (exact match)
	if results[0].ID != "v1" {
		t.Fatalf("first result = %q, want v1", results[0].ID)
	}
	if results[0].Score < 0.99 {
		t.Fatalf("v1 score = %f, want ~1.0", results[0].Score)
	}
	// v3 should be second
	if results[1].ID != "v3" {
		t.Fatalf("second result = %q, want v3", results[1].ID)
	}
}

func TestLocalDelete(t *testing.T) {
	s := store.NewLocal("")
	ctx := context.Background()
	s.Store(ctx, "v1", []float32{1, 0}, map[string]string{"a": "b"})
	s.Store(ctx, "v2", []float32{0, 1}, map[string]string{"c": "d"})
	s.Delete(ctx, "v1")
	results, _ := s.Search(ctx, []float32{1, 0}, 10)
	if len(results) != 1 {
		t.Fatalf("got %d results after delete, want 1", len(results))
	}
	if results[0].ID != "v2" {
		t.Fatalf("remaining = %q, want v2", results[0].ID)
	}
}

func TestLocalUpdateExisting(t *testing.T) {
	s := store.NewLocal("")
	ctx := context.Background()
	s.Store(ctx, "v1", []float32{1, 0}, map[string]string{"version": "1"})
	s.Store(ctx, "v1", []float32{0, 1}, map[string]string{"version": "2"})
	results, _ := s.Search(ctx, []float32{0, 1}, 10)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1 (no dup)", len(results))
	}
	if results[0].Metadata["version"] != "2" {
		t.Fatalf("version = %q, want 2", results[0].Metadata["version"])
	}
}

func TestLocalTopKExceedsEntries(t *testing.T) {
	s := store.NewLocal("")
	ctx := context.Background()
	s.Store(ctx, "v1", []float32{1, 0}, nil)
	results, _ := s.Search(ctx, []float32{1, 0}, 100)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
}

func TestLocalPersistence(t *testing.T) {
	path := t.TempDir() + "/vectors.gob"
	ctx := context.Background()

	// Store data with persistence
	s1 := store.NewLocal(path)
	s1.Store(ctx, "v1", []float32{1, 0, 0}, map[string]string{"text": "hello"})

	// Load from same path
	s2 := store.NewLocal(path)
	results, err := s2.Search(ctx, []float32{1, 0, 0}, 1)
	if err != nil {
		t.Fatalf("search after reload: %v", err)
	}
	if len(results) != 1 || results[0].ID != "v1" {
		t.Fatal("persistence failed")
	}
}

func TestLocalEmptySearch(t *testing.T) {
	s := store.NewLocal("")
	ctx := context.Background()
	results, err := s.Search(ctx, []float32{1, 0}, 5)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("got %d results from empty store", len(results))
	}
}
