package daneel_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/daneel-ai/daneel"
	"github.com/daneel-ai/daneel/provider/mock"
)

// ---------- Agent creation ----------

func TestNewAgent(t *testing.T) {
	a := daneel.New("test-agent",
		daneel.WithInstructions("You are helpful"),
		daneel.WithMaxTurns(5),
	)
	if a.Name() != "test-agent" {
		t.Fatalf("got name %q, want %q", a.Name(), "test-agent")
	}
	if a.Instructions() != "You are helpful" {
		t.Fatalf("got instructions %q", a.Instructions())
	}
}

func TestAgentCopyOnModify(t *testing.T) {
	a := daneel.New("original", daneel.WithInstructions("v1"))
	b := a.WithName("clone")
	if a.Name() == b.Name() {
		t.Fatal("WithName must return a new agent")
	}
	if a.Name() != "original" {
		t.Fatal("original agent mutated")
	}
}

func TestAgentWithProvider(t *testing.T) {
	p1 := mock.New(mock.Respond("p1"))
	p2 := mock.New(mock.Respond("p2"))
	a := daneel.New("a", daneel.WithProvider(p1))
	b := a.WithProvider(p2)
	if a.Provider() == b.Provider() {
		t.Fatal("WithProvider must return a new agent with different provider")
	}
}

// ---------- Basic Run ----------

func TestRunSimpleResponse(t *testing.T) {
	p := mock.New(mock.Respond("Hello!"))
	agent := daneel.New("greeter", daneel.WithProvider(p))
	result, err := daneel.Run(context.Background(), agent, "Hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "Hello!" {
		t.Fatalf("got output %q, want %q", result.Output, "Hello!")
	}
	if result.AgentName != "greeter" {
		t.Fatalf("got agent name %q", result.AgentName)
	}
	if result.SessionID == "" {
		t.Fatal("session ID should not be empty")
	}
	if result.Turns != 1 {
		t.Fatalf("got %d turns, want 1", result.Turns)
	}
}

func TestRunWithSessionID(t *testing.T) {
	p := mock.New(mock.Respond("ok"))
	agent := daneel.New("a", daneel.WithProvider(p))
	result, err := daneel.Run(context.Background(), agent, "hi", daneel.WithSessionID("my-session"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SessionID != "my-session" {
		t.Fatalf("got session %q, want %q", result.SessionID, "my-session")
	}
}

func TestRunNoProvider(t *testing.T) {
	agent := daneel.New("noprov")
	_, err := daneel.Run(context.Background(), agent, "hi")
	if !errors.Is(err, daneel.ErrNoProvider) {
		t.Fatalf("expected ErrNoProvider, got %v", err)
	}
}

// ---------- Tool execution ----------

func TestRunWithToolCall(t *testing.T) {
	called := false
	tool := daneel.NewTool("greet", "Greets a person",
		func(ctx context.Context, p struct {
			Name string `json:"name" desc:"Person name"`
		}) (string, error) {
			called = true
			return "Hello, " + p.Name + "!", nil
		},
	)

	p := mock.New(
		mock.RespondWithToolCall("greet", `{"name":"Alice"}`),
		mock.Respond("I greeted Alice for you!"),
	)
	agent := daneel.New("helper",
		daneel.WithProvider(p),
		daneel.WithTools(tool),
	)

	result, err := daneel.Run(context.Background(), agent, "Say hi to Alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("tool was not called")
	}
	if result.Output != "I greeted Alice for you!" {
		t.Fatalf("got output %q", result.Output)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(result.ToolCalls))
	}
	if result.ToolCalls[0].Name != "greet" {
		t.Fatalf("tool call name = %q", result.ToolCalls[0].Name)
	}
	if result.Turns != 2 {
		t.Fatalf("got %d turns, want 2", result.Turns)
	}
}

func TestToolError(t *testing.T) {
	tool := daneel.NewTool("fail", "Always fails",
		func(ctx context.Context, p struct{}) (string, error) {
			return "", fmt.Errorf("boom")
		},
	)
	p := mock.New(
		mock.RespondWithToolCall("fail", `{}`),
		mock.Respond("The tool failed"),
	)
	agent := daneel.New("a", daneel.WithProvider(p), daneel.WithTools(tool))
	result, err := daneel.Run(context.Background(), agent, "do it")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ToolCalls[0].IsError != true {
		t.Fatal("tool call should be marked as error")
	}
}

func TestToolTimeout(t *testing.T) {
	tool := daneel.NewTool("slow", "Slow tool",
		func(ctx context.Context, p struct{}) (string, error) {
			select {
			case <-time.After(5 * time.Second):
				return "done", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		},
		daneel.WithToolTimeout(50*time.Millisecond),
	)
	p := mock.New(
		mock.RespondWithToolCall("slow", `{}`),
		mock.Respond("It timed out"),
	)
	agent := daneel.New("a", daneel.WithProvider(p), daneel.WithTools(tool))
	result, err := daneel.Run(context.Background(), agent, "run slow")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToolCalls[0].IsError {
		t.Fatal("expected tool call to be an error due to timeout")
	}
}

// ---------- Permissions ----------

func TestDenyTool(t *testing.T) {
	tool := daneel.NewTool("secret", "Secret tool",
		func(ctx context.Context, p struct{}) (string, error) {
			return "secret data", nil
		},
	)
	p := mock.New(
		mock.RespondWithToolCall("secret", `{}`),
		mock.Respond("Access denied"),
	)
	agent := daneel.New("a",
		daneel.WithProvider(p),
		daneel.WithTools(tool),
		daneel.WithPermissions(daneel.DenyTools("secret")),
	)
	result, err := daneel.Run(context.Background(), agent, "use secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ToolCalls[0].Permitted {
		t.Fatal("tool call should be denied")
	}
}

func TestAllowToolsWhitelist(t *testing.T) {
	allowed := daneel.NewTool("allowed", "Allowed",
		func(ctx context.Context, p struct{}) (string, error) { return "ok", nil },
	)
	blocked := daneel.NewTool("blocked", "Blocked",
		func(ctx context.Context, p struct{}) (string, error) { return "ok", nil },
	)
	p := mock.New(
		mock.RespondWithToolCall("blocked", `{}`),
		mock.Respond("denied"),
	)
	agent := daneel.New("a",
		daneel.WithProvider(p),
		daneel.WithTools(allowed, blocked),
		daneel.WithPermissions(daneel.AllowTools("allowed")),
	)
	result, err := daneel.Run(context.Background(), agent, "use blocked")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.ToolCalls[0].Permitted {
		t.Fatal("blocked tool should not be permitted")
	}
}

// ---------- Guards ----------

func TestInputGuard(t *testing.T) {
	p := mock.New(mock.Respond("ok"))
	agent := daneel.New("a",
		daneel.WithProvider(p),
		daneel.WithInputGuard(func(ctx context.Context, input string) error {
			if input == "bad" {
				return fmt.Errorf("rejected")
			}
			return nil
		}),
	)
	_, err := daneel.Run(context.Background(), agent, "bad")
	if err == nil {
		t.Fatal("expected guard error")
	}
	var guardErr *daneel.GuardError
	if !errors.As(err, &guardErr) {
		t.Fatalf("expected GuardError, got %T: %v", err, err)
	}
	if guardErr.Guard != "input" {
		t.Fatalf("guard type = %q", guardErr.Guard)
	}
}

func TestOutputGuard(t *testing.T) {
	p := mock.New(mock.Respond("forbidden content"))
	agent := daneel.New("a",
		daneel.WithProvider(p),
		daneel.WithOutputGuard(func(ctx context.Context, output string) error {
			if output == "forbidden content" {
				return fmt.Errorf("blocked")
			}
			return nil
		}),
	)
	_, err := daneel.Run(context.Background(), agent, "hi")
	if err == nil {
		t.Fatal("expected guard error")
	}
	var guardErr *daneel.GuardError
	if !errors.As(err, &guardErr) {
		t.Fatalf("expected GuardError, got %T: %v", err, err)
	}
	if guardErr.Guard != "output" {
		t.Fatalf("guard type = %q", guardErr.Guard)
	}
}

// ---------- Max turns ----------

func TestMaxTurns(t *testing.T) {
	p := mock.New()
	for i := 0; i < 5; i++ {
		p.QueueToolCall("noop", `{}`)
	}
	tool := daneel.NewTool("noop", "Does nothing",
		func(ctx context.Context, p struct{}) (string, error) { return "ok", nil },
	)
	agent := daneel.New("a",
		daneel.WithProvider(p),
		daneel.WithTools(tool),
		daneel.WithMaxTurns(3),
	)
	_, err := daneel.Run(context.Background(), agent, "loop")
	if err == nil {
		t.Fatal("expected MaxTurnsError")
	}
	var maxErr *daneel.MaxTurnsError
	if !errors.As(err, &maxErr) {
		t.Fatalf("expected MaxTurnsError, got %T: %v", err, err)
	}
	if maxErr.MaxTurns != 3 {
		t.Fatalf("max turns = %d, want 3", maxErr.MaxTurns)
	}
	if !errors.Is(err, daneel.ErrMaxTurns) {
		t.Fatal("MaxTurnsError should Unwrap to ErrMaxTurns")
	}
}

// ---------- OnConversationEnd callback ----------

func TestOnConversationEnd(t *testing.T) {
	var captured daneel.RunResult
	var callCount int32

	p := mock.New(mock.Respond("done"))
	agent := daneel.New("a",
		daneel.WithProvider(p),
		daneel.WithOnConversationEnd(func(ctx context.Context, result daneel.RunResult) {
			captured = result
			atomic.AddInt32(&callCount, 1)
		}),
	)
	result, err := daneel.Run(context.Background(), agent, "hi")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if atomic.LoadInt32(&callCount) != 1 {
		t.Fatalf("callback called %d times, want 1", callCount)
	}
	if captured.Output != result.Output {
		t.Fatalf("captured output %q != result output %q", captured.Output, result.Output)
	}
}

// ---------- Context functions ----------

func TestContextFunc(t *testing.T) {
	p := mock.New(mock.Respond("ok"))
	agent := daneel.New("a",
		daneel.WithProvider(p),
		daneel.WithInstructions("Base instructions"),
		daneel.WithContextFunc(func(ctx context.Context) (string, error) {
			return "Current time: 12:00", nil
		}),
	)
	_, err := daneel.Run(context.Background(), agent, "what time?")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	calls := p.Calls()
	if len(calls) == 0 {
		t.Fatal("no calls recorded")
	}
	sysMsg := calls[0].Messages[0]
	if sysMsg.Role != daneel.RoleSystem {
		t.Fatalf("first message role = %q, want system", sysMsg.Role)
	}
	expected := "Base instructions\n\nCurrent time: 12:00"
	if sysMsg.Content != expected {
		t.Fatalf("system prompt = %q, want %q", sysMsg.Content, expected)
	}
}

func TestContextFuncError(t *testing.T) {
	p := mock.New(mock.Respond("ok"))
	agent := daneel.New("a",
		daneel.WithProvider(p),
		daneel.WithContextFunc(func(ctx context.Context) (string, error) {
			return "", fmt.Errorf("context failed")
		}),
	)
	_, err := daneel.Run(context.Background(), agent, "hi")
	if err == nil {
		t.Fatal("expected error from context func")
	}
}

// ---------- Handoffs ----------

func TestHandoff(t *testing.T) {
	p2 := mock.New(mock.Respond("I am the specialist"))
	specialist := daneel.New("specialist",
		daneel.WithProvider(p2),
		daneel.WithInstructions("I handle specialized tasks"),
	)

	p1 := mock.New(
		mock.RespondWithToolCall("handoff_to_specialist", `{"reason":"needs expert help"}`),
	)
	router := daneel.New("router",
		daneel.WithProvider(p1),
		daneel.WithHandoffs(specialist),
	)

	result, err := daneel.Run(context.Background(), router, "I need expert help")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.AgentName != "specialist" {
		t.Fatalf("agent name = %q, want specialist", result.AgentName)
	}
	if result.HandoffFrom != "router" {
		t.Fatalf("handoff from = %q, want router", result.HandoffFrom)
	}
	if result.Output != "I am the specialist" {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestHandoffInheritsProvider(t *testing.T) {
	specialist := daneel.New("specialist",
		daneel.WithInstructions("I am a specialist"),
	)
	p := mock.New(
		mock.RespondWithToolCall("handoff_to_specialist", `{"reason":"help"}`),
		mock.Respond("specialist reply"),
	)
	router := daneel.New("router",
		daneel.WithProvider(p),
		daneel.WithHandoffs(specialist),
	)
	result, err := daneel.Run(context.Background(), router, "help me")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.AgentName != "specialist" {
		t.Fatalf("agent name = %q", result.AgentName)
	}
}

// ---------- Error types ----------

func TestErrorUnwrap(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		sentinel error
	}{
		{"PermissionError", &daneel.PermissionError{Agent: "a", Tool: "t"}, daneel.ErrPermissionDenied},
		{"GuardError", &daneel.GuardError{Agent: "a", Guard: "input"}, daneel.ErrGuardFailed},
		{"MaxTurnsError", &daneel.MaxTurnsError{Agent: "a", MaxTurns: 5}, daneel.ErrMaxTurns},
		{"ProviderError", &daneel.ProviderError{Provider: "p"}, daneel.ErrProvider},
		{"ToolTimeoutError", &daneel.ToolTimeoutError{Tool: "t"}, daneel.ErrToolTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.err, tt.sentinel) {
				t.Fatalf("%T does not Unwrap to %v", tt.err, tt.sentinel)
			}
		})
	}
}

// ---------- Response format ----------

func TestResponseFormatJSON(t *testing.T) {
	p := mock.New(mock.Respond(`{"key":"value"}`))
	agent := daneel.New("a",
		daneel.WithProvider(p),
		daneel.WithInstructions("Help me"),
	)
	_, err := daneel.Run(context.Background(), agent, "give json",
		daneel.WithResponseFormat(daneel.JSON),
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	calls := p.Calls()
	sysMsg := calls[0].Messages[0]
	if sysMsg.Role != daneel.RoleSystem {
		t.Fatal("first msg should be system")
	}
	if len(sysMsg.Content) <= len("Help me") {
		t.Fatal("system prompt should include JSON format instructions")
	}
}

// ---------- RunStructured ----------

func TestRunStructured(t *testing.T) {
	type Sentiment struct {
		Score float64 `json:"score"`
		Label string  `json:"label"`
	}
	p := mock.New(mock.Respond(`{"score":0.95,"label":"positive"}`))
	agent := daneel.New("analyzer",
		daneel.WithProvider(p),
		daneel.WithInstructions("Analyze sentiment"),
	)
	result, err := daneel.RunStructured[Sentiment](context.Background(), agent, "Great product!")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Data.Label != "positive" {
		t.Fatalf("label = %q", result.Data.Label)
	}
	if result.Data.Score != 0.95 {
		t.Fatalf("score = %f", result.Data.Score)
	}
}

// ---------- Tool schema: description + desc tags ----------

func TestToolSchemaDescriptionTag(t *testing.T) {
	type Params struct {
		Name string `json:"name" description:"The name"`
		Age  int    `json:"age" desc:"Age in years"`
	}
	tool := daneel.NewTool("test", "Test tool",
		func(ctx context.Context, p Params) (string, error) { return "ok", nil },
	)
	def := tool.Def()
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(def.Schema, &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	if schema.Properties["name"].Description != "The name" {
		t.Fatalf("name description = %q", schema.Properties["name"].Description)
	}
	if schema.Properties["age"].Description != "Age in years" {
		t.Fatalf("age description = %q", schema.Properties["age"].Description)
	}
}

// ---------- Parallel tool execution ----------

func TestParallelToolExecution(t *testing.T) {
	var count int32
	tool := daneel.NewTool("inc", "Increments counter",
		func(ctx context.Context, p struct{}) (string, error) {
			atomic.AddInt32(&count, 1)
			return "done", nil
		},
	)
	p := mock.New(
		mock.RespondFunc(func(msgs []daneel.Message) *daneel.Response {
			return &daneel.Response{
				ToolCalls: []daneel.ToolCall{
					{ID: "1", Name: "inc", Arguments: json.RawMessage(`{}`)},
					{ID: "2", Name: "inc", Arguments: json.RawMessage(`{}`)},
					{ID: "3", Name: "inc", Arguments: json.RawMessage(`{}`)},
				},
			}
		}),
		mock.Respond("All done"),
	)
	agent := daneel.New("a",
		daneel.WithProvider(p),
		daneel.WithTools(tool),
		daneel.WithToolExecution(daneel.Parallel),
	)
	result, err := daneel.Run(context.Background(), agent, "increment 3 times")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if atomic.LoadInt32(&count) != 3 {
		t.Fatalf("count = %d, want 3", count)
	}
	if len(result.ToolCalls) != 3 {
		t.Fatalf("tool calls = %d, want 3", len(result.ToolCalls))
	}
}

// ---------- Sessions ----------

func TestSessionID(t *testing.T) {
	id := daneel.NewSessionID()
	if len(id) == 0 {
		t.Fatal("session ID should not be empty")
	}
	id2 := daneel.NewSessionID()
	if id == id2 {
		t.Fatal("session IDs should be unique")
	}
}

func TestDeterministicSessionID(t *testing.T) {
	a := daneel.DeterministicSessionID("telegram", "user1", "chat1")
	b := daneel.DeterministicSessionID("telegram", "user1", "chat1")
	c := daneel.DeterministicSessionID("telegram", "user2", "chat1")
	if a != b {
		t.Fatal("same inputs should produce same ID")
	}
	if a == c {
		t.Fatal("different inputs should produce different IDs")
	}
}

// ---------- Messages ----------

func TestMessages(t *testing.T) {
	sys := daneel.SystemMessage("system")
	usr := daneel.UserMessage("user")
	ast := daneel.AssistantMessage("assistant")
	if sys.Role != daneel.RoleSystem {
		t.Fatal("wrong role")
	}
	if usr.Role != daneel.RoleUser {
		t.Fatal("wrong role")
	}
	if ast.Role != daneel.RoleAssistant {
		t.Fatal("wrong role")
	}
}

// ---------- Usage tracking ----------

func TestUsageTracking(t *testing.T) {
	p := mock.New(mock.Respond("hello"))
	agent := daneel.New("a", daneel.WithProvider(p))
	result, err := daneel.Run(context.Background(), agent, "hi")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Usage.TotalTokens != 15 {
		t.Fatalf("total tokens = %d, want 15", result.Usage.TotalTokens)
	}
}

// ---------- Duration ----------

func TestDuration(t *testing.T) {
	p := mock.New(mock.Respond("ok"))
	agent := daneel.New("a", daneel.WithProvider(p))
	result, err := daneel.Run(context.Background(), agent, "hi")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Duration <= 0 {
		t.Fatal("duration should be positive")
	}
}

// ---------- Mock provider ----------

func TestMockProviderCalls(t *testing.T) {
	p := mock.New(mock.Respond("r1"), mock.Respond("r2"))
	agent := daneel.New("a", daneel.WithProvider(p))
	daneel.Run(context.Background(), agent, "first")
	daneel.Run(context.Background(), agent, "second")
	if p.CallCount() != 2 {
		t.Fatalf("call count = %d, want 2", p.CallCount())
	}
	last := p.LastMessages()
	if len(last) == 0 {
		t.Fatal("no last messages")
	}
}

func TestMockProviderReset(t *testing.T) {
	p := mock.New(mock.Respond("r1"))
	agent := daneel.New("a", daneel.WithProvider(p))
	daneel.Run(context.Background(), agent, "hi")
	p.Reset()
	if p.CallCount() != 0 {
		t.Fatal("call count should be 0 after reset")
	}
}

func TestMockProviderExhausted(t *testing.T) {
	p := mock.New()
	agent := daneel.New("a", daneel.WithProvider(p))
	_, err := daneel.Run(context.Background(), agent, "hi")
	if err == nil {
		t.Fatal("expected error when mock has no responses")
	}
}

// ---------- Approval ----------

func TestApprovalDenied(t *testing.T) {
	tool := daneel.NewTool("dangerous", "Dangerous operation",
		func(ctx context.Context, p struct{}) (string, error) {
			return "executed", nil
		},
		daneel.WithApprovalRequired(),
	)
	p := mock.New(
		mock.RespondWithToolCall("dangerous", `{}`),
		mock.Respond("Denied"),
	)
	agent := daneel.New("a", daneel.WithProvider(p), daneel.WithTools(tool))
	result, err := daneel.Run(context.Background(), agent, "do it",
		daneel.WithApprover(daneel.ApproverFunc(func(ctx context.Context, req daneel.ApprovalRequest) (bool, error) {
			return false, nil
		})),
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.ToolCalls[0].Result != "tool call denied by approver" {
		t.Fatalf("result = %q", result.ToolCalls[0].Result)
	}
}

func TestApprovalApproved(t *testing.T) {
	tool := daneel.NewTool("safe", "Safe operation",
		func(ctx context.Context, p struct{}) (string, error) {
			return "success", nil
		},
		daneel.WithApprovalRequired(),
	)
	p := mock.New(
		mock.RespondWithToolCall("safe", `{}`),
		mock.Respond("Done"),
	)
	agent := daneel.New("a", daneel.WithProvider(p), daneel.WithTools(tool))
	result, err := daneel.Run(context.Background(), agent, "do it",
		daneel.WithApprover(daneel.ApproverFunc(func(ctx context.Context, req daneel.ApprovalRequest) (bool, error) {
			return true, nil
		})),
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.ToolCalls[0].Result != "success" {
		t.Fatalf("result = %q", result.ToolCalls[0].Result)
	}
}

// ---------- WithExtraTools / WithExtraInstructions ----------

func TestWithExtraTools(t *testing.T) {
	tool1 := daneel.NewTool("t1", "Tool 1",
		func(ctx context.Context, p struct{}) (string, error) { return "ok", nil },
	)
	tool2 := daneel.NewTool("t2", "Tool 2",
		func(ctx context.Context, p struct{}) (string, error) { return "ok", nil },
	)
	a := daneel.New("a", daneel.WithTools(tool1))
	b := a.WithExtraTools(tool2)
	if len(a.Tools()) != 1 {
		t.Fatalf("original should have 1 tool, got %d", len(a.Tools()))
	}
	if len(b.Tools()) != 2 {
		t.Fatalf("derived should have 2 tools, got %d", len(b.Tools()))
	}
}

func TestWithExtraInstructions(t *testing.T) {
	a := daneel.New("a", daneel.WithInstructions("base"))
	b := a.WithExtraInstructions("extra")
	if a.Instructions() != "base" {
		t.Fatal("original mutated")
	}
	expected := "base\n\nextra"
	if b.Instructions() != expected {
		t.Fatalf("instructions = %q, want %q", b.Instructions(), expected)
	}
}
