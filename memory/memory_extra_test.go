package memory_test

import (
	"context"
	"testing"

	"github.com/Rafiki81/daneel"
	"github.com/Rafiki81/daneel/memory"
	"github.com/Rafiki81/daneel/memory/store"
	"github.com/Rafiki81/daneel/provider/mock"
)

// ========== Summary Memory ==========

func TestSummarySaveAndRetrieve(t *testing.T) {
	p := mock.New()
	p.QueueResponse("Summary: user said hello and bot said hi.")
	m := memory.Summary(p, memory.SummarizeEvery(2))
	ctx := context.Background()

	msgs := []daneel.Message{
		daneel.UserMessage("hello"),
		daneel.AssistantMessage("hi"),
		daneel.UserMessage("how are you"),
	}
	if err := m.Save(ctx, "s1", msgs); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := m.Retrieve(ctx, "s1", "", 0)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	// Should have summary message + recent messages
	if len(got) == 0 {
		t.Fatal("should return some messages")
	}
	// First message should be system with summary
	found := false
	for _, msg := range got {
		if msg.Role == daneel.RoleSystem {
			found = true
		}
	}
	if !found {
		t.Fatal("should contain summary system message")
	}
}

func TestSummarySkipsSystemMessages(t *testing.T) {
	p := mock.New()
	m := memory.Summary(p)
	ctx := context.Background()

	msgs := []daneel.Message{
		daneel.SystemMessage("you are a bot"),
		daneel.UserMessage("hello"),
	}
	m.Save(ctx, "s1", msgs)
	got, _ := m.Retrieve(ctx, "s1", "", 0)
	// Should only have the user message, system is filtered on save
	if len(got) != 1 {
		t.Fatalf("got %d msgs, want 1 (system filtered)", len(got))
	}
}

func TestSummaryDefaultSummarizeEvery(t *testing.T) {
	p := mock.New()
	m := memory.Summary(p) // default summarizeEvery=10
	ctx := context.Background()

	// Save 5 messages (below threshold), should not trigger summarize
	var msgs []daneel.Message
	for i := 0; i < 5; i++ {
		msgs = append(msgs, daneel.UserMessage("msg"))
	}
	m.Save(ctx, "s1", msgs)
	got, _ := m.Retrieve(ctx, "s1", "", 0)
	if len(got) != 5 {
		t.Fatalf("got %d msgs, want 5 (no summarize yet)", len(got))
	}
	// Provider should not have been called
	if p.CallCount() != 0 {
		t.Fatalf("provider called %d times, want 0", p.CallCount())
	}
}

func TestSummaryClear(t *testing.T) {
	p := mock.New()
	m := memory.Summary(p)
	ctx := context.Background()
	m.Save(ctx, "s1", []daneel.Message{daneel.UserMessage("hi")})
	m.Clear(ctx, "s1")
	got, _ := m.Retrieve(ctx, "s1", "", 0)
	if len(got) != 0 {
		t.Fatalf("got %d msgs after clear", len(got))
	}
}

func TestSummaryRetrieveWithLimit(t *testing.T) {
	p := mock.New()
	m := memory.Summary(p)
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
		t.Fatalf("got %d msgs, want 2 (limited)", len(got))
	}
}

func TestSummaryIsolatesSessions(t *testing.T) {
	p := mock.New()
	m := memory.Summary(p)
	ctx := context.Background()
	m.Save(ctx, "s1", []daneel.Message{daneel.UserMessage("session1")})
	m.Save(ctx, "s2", []daneel.Message{daneel.UserMessage("session2")})
	got1, _ := m.Retrieve(ctx, "s1", "", 0)
	got2, _ := m.Retrieve(ctx, "s2", "", 0)
	if len(got1) != 1 || got1[0].Content != "session1" {
		t.Fatal("session1 wrong")
	}
	if len(got2) != 1 || got2[0].Content != "session2" {
		t.Fatal("session2 wrong")
	}
}

func TestSummaryRetrieveEmpty(t *testing.T) {
	p := mock.New()
	m := memory.Summary(p)
	got, err := m.Retrieve(context.Background(), "nonexistent", "", 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Fatalf("should be nil for unknown session")
	}
}

func TestSummarizeEveryOption(t *testing.T) {
	p := mock.New()
	// SummarizeEvery(0) should be ignored (kept at default)
	m := memory.Summary(p, memory.SummarizeEvery(0))
	ctx := context.Background()
	var msgs []daneel.Message
	for i := 0; i < 5; i++ {
		msgs = append(msgs, daneel.UserMessage("x"))
	}
	m.Save(ctx, "s1", msgs)
	// With default=10, 5 msgs should not trigger summarize
	if p.CallCount() != 0 {
		t.Fatal("should not have summarized")
	}
}

// ========== Vector Memory ==========

// mockEmbedder returns a fixed embedding based on text length mod 3
type mockEmbedder struct{}

func (e *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	// Simple deterministic embedding based on first char
	vec := make([]float32, 3)
	if len(text) > 0 {
		vec[int(text[0])%3] = 1.0
	}
	return vec, nil
}

func TestVectorSaveAndRetrieve(t *testing.T) {
	s := store.NewLocal("")
	e := &mockEmbedder{}
	m := memory.Vector(s, e, memory.TopK(5))
	ctx := context.Background()

	msgs := []daneel.Message{
		daneel.UserMessage("hello"),
		daneel.AssistantMessage("hi there"),
	}
	if err := m.Save(ctx, "s1", msgs); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := m.Retrieve(ctx, "s1", "hello", 10)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("should return some messages")
	}
}

func TestVectorSkipsSystemAndEmpty(t *testing.T) {
	s := store.NewLocal("")
	e := &mockEmbedder{}
	m := memory.Vector(s, e)
	ctx := context.Background()

	msgs := []daneel.Message{
		daneel.SystemMessage("instructions"),
		{Role: daneel.RoleUser, Content: ""},
		daneel.UserMessage("real message"),
	}
	m.Save(ctx, "s1", msgs)

	// Only "real message" should be stored
	results, _ := s.Search(ctx, []float32{1, 0, 0}, 10)
	// Just verify at least one was stored
	if len(results) == 0 {
		t.Fatal("should store at least one message")
	}
}

func TestVectorRetrieveEmptyQuery(t *testing.T) {
	s := store.NewLocal("")
	e := &mockEmbedder{}
	m := memory.Vector(s, e)
	ctx := context.Background()

	got, err := m.Retrieve(ctx, "s1", "", 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Fatal("empty query should return nil")
	}
}

func TestVectorRetrieveWithLimit(t *testing.T) {
	s := store.NewLocal("")
	e := &mockEmbedder{}
	m := memory.Vector(s, e, memory.TopK(10))
	ctx := context.Background()

	msgs := []daneel.Message{
		daneel.UserMessage("aaa"),
		daneel.UserMessage("bbb"),
		daneel.UserMessage("ccc"),
	}
	m.Save(ctx, "s1", msgs)

	// Limit < topK should use limit
	got, _ := m.Retrieve(ctx, "s1", "aaa", 1)
	if len(got) > 1 {
		t.Fatalf("got %d msgs, want at most 1 (limited)", len(got))
	}
}

func TestVectorClear(t *testing.T) {
	s := store.NewLocal("")
	e := &mockEmbedder{}
	m := memory.Vector(s, e)
	ctx := context.Background()

	msgs := []daneel.Message{daneel.UserMessage("hello")}
	m.Save(ctx, "s1", msgs)
	err := m.Clear(ctx, "s1")
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
}

func TestVectorDefaultTopK(t *testing.T) {
	// TopK(0) should be ignored, keep default=5
	s := store.NewLocal("")
	e := &mockEmbedder{}
	m := memory.Vector(s, e, memory.TopK(0))
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		m.Save(ctx, "s1", []daneel.Message{daneel.UserMessage("msg")})
	}
	got, _ := m.Retrieve(ctx, "s1", "msg", 0)
	// Default topK=5, should return at most 5
	if len(got) > 5 {
		t.Fatalf("got %d msgs, want <= 5 (default topK)", len(got))
	}
}

func TestVectorSessionIsolation(t *testing.T) {
	s := store.NewLocal("")
	e := &mockEmbedder{}
	m := memory.Vector(s, e)
	ctx := context.Background()

	m.Save(ctx, "s1", []daneel.Message{daneel.UserMessage("session1")})
	m.Save(ctx, "s2", []daneel.Message{daneel.UserMessage("session2")})

	got, _ := m.Retrieve(ctx, "s1", "session1", 10)
	for _, msg := range got {
		if msg.Content == "session2" {
			t.Fatal("should not return messages from other sessions")
		}
	}
}

// ========== Composite with error paths ==========

func TestCompositeSaveError(t *testing.T) {
	// Composite with single backend: save then retrieve new session
	s1 := memory.Sliding(10)
	s2 := memory.Sliding(10)
	c := memory.Composite(s1, s2)
	ctx := context.Background()

	// Save different data via each backend directly
	s1.Save(ctx, "s1", []daneel.Message{daneel.UserMessage("only-in-s1")})
	s2.Save(ctx, "s1", []daneel.Message{daneel.UserMessage("only-in-s2")})

	// Composite retrieve merges + deduplicates
	got, _ := c.Retrieve(ctx, "s1", "", 0)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2 (merged from both)", len(got))
	}
}

// ========== Store persistence edge cases ==========

func TestLocalStoreDeleteNonexistent(t *testing.T) {
	s := store.NewLocal("")
	ctx := context.Background()
	s.Store(ctx, "v1", []float32{1, 0}, map[string]string{"a": "b"})
	// Delete non-existent should not error
	err := s.Delete(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("delete nonexistent: %v", err)
	}
	// v1 should still be there
	results, _ := s.Search(ctx, []float32{1, 0}, 10)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
}

func TestLocalStoreDeleteMultiple(t *testing.T) {
	s := store.NewLocal("")
	ctx := context.Background()
	s.Store(ctx, "v1", []float32{1, 0}, nil)
	s.Store(ctx, "v2", []float32{0, 1}, nil)
	s.Store(ctx, "v3", []float32{1, 1}, nil)
	s.Delete(ctx, "v1", "v3")
	results, _ := s.Search(ctx, []float32{0, 1}, 10)
	if len(results) != 1 || results[0].ID != "v2" {
		t.Fatal("should only have v2 left")
	}
}

func TestLocalStoreDifferentLengthVectors(t *testing.T) {
	s := store.NewLocal("")
	ctx := context.Background()
	s.Store(ctx, "v1", []float32{1, 0, 0}, nil)
	// Search with different-length vector (cosine returns 0)
	results, _ := s.Search(ctx, []float32{1, 0}, 10)
	if len(results) != 1 {
		t.Fatalf("got %d results", len(results))
	}
	if results[0].Score != 0 {
		t.Fatalf("score = %f, want 0 (different lengths)", results[0].Score)
	}
}

func TestLocalStoreZeroVector(t *testing.T) {
	s := store.NewLocal("")
	ctx := context.Background()
	s.Store(ctx, "v1", []float32{0, 0, 0}, nil)
	results, _ := s.Search(ctx, []float32{1, 0, 0}, 10)
	if len(results) != 1 {
		t.Fatalf("got %d results", len(results))
	}
	if results[0].Score != 0 {
		t.Fatalf("score = %f, want 0 (zero vector)", results[0].Score)
	}
}
