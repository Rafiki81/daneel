package daneel_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Rafiki81/daneel"
	"github.com/Rafiki81/daneel/provider/mock"
)

// ---- WithMaxContextTokens ----------------------------------------------------------------

func TestWithMaxContextTokens(t *testing.T) {
	p := mock.New(mock.Respond("pong"))
	agent := daneel.New("agent",
		daneel.WithProvider(p),
		daneel.WithMaxContextTokens(200_000),
	)
	result, err := daneel.Run(context.Background(), agent, "ping")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "pong" {
		t.Fatalf("output = %q, want %q", result.Output, "pong")
	}
}

func TestWithMaxContextTokensZeroIgnored(t *testing.T) {
	p := mock.New(mock.Respond("ok"))
	agent := daneel.New("agent",
		daneel.WithProvider(p),
		daneel.WithMaxContextTokens(0),
	)
	if _, err := daneel.Run(context.Background(), agent, "hi"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---- ContextSlidingWindow ----------------------------------------------------------------

func TestSlidingWindow_NoTrimWhenFits(t *testing.T) {
	p := mock.New(mock.Respond("ok"))
	agent := daneel.New("agent",
		daneel.WithProvider(p),
		daneel.WithMaxContextTokens(10_000),
		daneel.WithContextStrategy(daneel.ContextSlidingWindow),
	)
	history := []daneel.Message{
		daneel.UserMessage("question"),
		daneel.AssistantMessage("answer"),
	}
	_, err := daneel.Run(context.Background(), agent, "follow-up", daneel.WithHistory(history))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := p.LastMessages()
	if len(got) != 3 {
		t.Errorf("expected 3 messages (no trimming), got %d", len(got))
	}
}

func TestSlidingWindow_TrimsOldMessages(t *testing.T) {
	p := mock.New(mock.Respond("ok"))
	agent := daneel.New("agent",
		daneel.WithProvider(p),
		daneel.WithMaxContextTokens(40),
		daneel.WithContextStrategy(daneel.ContextSlidingWindow),
	)
	history := make([]daneel.Message, 0, 40)
	for i := 0; i < 20; i++ {
		history = append(history, daneel.UserMessage("hello world again today"))
		history = append(history, daneel.AssistantMessage("yes indeed sounds good"))
	}
	_, err := daneel.Run(context.Background(), agent, "what now", daneel.WithHistory(history))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := p.LastMessages()
	original := len(history) + 1
	if len(got) >= original {
		t.Errorf("sliding window should trim: got %d msgs, original was %d", len(got), original)
	}
}

func TestSlidingWindow_KeepsSystemPrompt(t *testing.T) {
	p := mock.New(mock.Respond("ok"))
	agent := daneel.New("agent",
		daneel.WithInstructions("You are a concise assistant."),
		daneel.WithProvider(p),
		daneel.WithMaxContextTokens(40),
		daneel.WithContextStrategy(daneel.ContextSlidingWindow),
	)
	history := make([]daneel.Message, 0, 40)
	for i := 0; i < 20; i++ {
		history = append(history, daneel.UserMessage("hello world again today"))
		history = append(history, daneel.AssistantMessage("yes indeed sounds good"))
	}
	_, err := daneel.Run(context.Background(), agent, "go", daneel.WithHistory(history))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := p.LastMessages()
	if len(got) == 0 {
		t.Fatal("message list is empty")
	}
	if got[0].Role != daneel.RoleSystem {
		t.Errorf("first message role = %q, want system", got[0].Role)
	}
}

// ---- ContextError ----------------------------------------------------------------

func TestContextError_DoesNotTrim(t *testing.T) {
	p := mock.New(mock.Respond("ok"))
	agent := daneel.New("agent",
		daneel.WithProvider(p),
		daneel.WithMaxContextTokens(1),
		daneel.WithContextStrategy(daneel.ContextError),
	)
	history := []daneel.Message{
		daneel.UserMessage("message one"),
		daneel.AssistantMessage("response one"),
		daneel.UserMessage("message two"),
		daneel.AssistantMessage("response two"),
	}
	_, err := daneel.Run(context.Background(), agent, "new one", daneel.WithHistory(history))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := p.LastMessages()
	want := len(history) + 1
	if len(got) != want {
		t.Errorf("ContextError must not trim: got %d messages, want %d", len(got), want)
	}
}

// ---- ContextSummarize ----------------------------------------------------------------

func TestContextSummarize_CondensesHistory(t *testing.T) {
	p := mock.New(
		mock.Respond("These are the key points from the earlier messages."),
		mock.Respond("final answer"),
	)
	agent := daneel.New("agent",
		daneel.WithProvider(p),
		daneel.WithMaxContextTokens(40),
		daneel.WithContextStrategy(daneel.ContextSummarize),
	)
	history := make([]daneel.Message, 0, 40)
	for i := 0; i < 20; i++ {
		history = append(history, daneel.UserMessage("hello world again today"))
		history = append(history, daneel.AssistantMessage("yes indeed sounds good"))
	}
	result, err := daneel.Run(context.Background(), agent, "what next", daneel.WithHistory(history))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "final answer" {
		t.Fatalf("output = %q, want %q", result.Output, "final answer")
	}
	if p.CallCount() < 2 {
		t.Errorf("expected >= 2 calls (summary + agent), got %d", p.CallCount())
	}
	hasSummary := false
	for _, m := range p.LastMessages() {
		if strings.Contains(m.Content, "Earlier conversation summary") {
			hasSummary = true
			break
		}
	}
	if !hasSummary {
		t.Error("expected summary message in final provider call context")
	}
}

func TestContextSummarize_FallsBackToSlidingWindow(t *testing.T) {
	p := mock.New(
		mock.Respond(""),
		mock.Respond("done"),
	)
	agent := daneel.New("agent",
		daneel.WithProvider(p),
		daneel.WithMaxContextTokens(5),
		daneel.WithContextStrategy(daneel.ContextSummarize),
	)
	history := make([]daneel.Message, 0, 20)
	for i := 0; i < 10; i++ {
		history = append(history, daneel.UserMessage("hello world again today"))
		history = append(history, daneel.AssistantMessage("yes indeed sounds good"))
	}
	_, err := daneel.Run(context.Background(), agent, "continue", daneel.WithHistory(history))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := p.LastMessages()
	original := len(history) + 1
	if len(got) >= original {
		t.Errorf("expected trimming after fallback: got %d, original %d", len(got), original)
	}
}
