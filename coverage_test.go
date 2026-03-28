package daneel_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/daneel-ai/daneel"
	"github.com/daneel-ai/daneel/content"
	"github.com/daneel-ai/daneel/provider/mock"
)

// ========== Registry ==========

func TestRegistryEmpty(t *testing.T) {
	r := daneel.NewRegistry()
	if len(r.Agents()) != 0 {
		t.Fatal("should be empty")
	}
	if len(r.Platforms()) != 0 {
		t.Fatal("should be empty")
	}
	if len(r.Tools()) != 0 {
		t.Fatal("should be empty")
	}
}

func TestRegistryRegisterAgent(t *testing.T) {
	p := mock.New()
	tool := daneel.NewTool("search", "Search",
		func(ctx context.Context, p struct{ Q string `json:"q"` }) (string, error) {
			return p.Q, nil
		},
	)
	agent := daneel.New("bot",
		daneel.WithProvider(p),
		daneel.WithInstructions("Be helpful"),
		daneel.WithTools(tool),
		daneel.WithMaxTurns(10),
	)

	r := daneel.NewRegistry()
	r.RegisterAgent(agent)

	agents := r.Agents()
	if len(agents) != 1 {
		t.Fatalf("agents = %d", len(agents))
	}
	if agents[0].Name != "bot" {
		t.Fatalf("name = %q", agents[0].Name)
	}
	if agents[0].Instructions != "Be helpful" {
		t.Fatalf("instructions = %q", agents[0].Instructions)
	}
	if agents[0].MaxTurns != 10 {
		t.Fatalf("maxTurns = %d", agents[0].MaxTurns)
	}
	if len(agents[0].Tools) != 1 {
		t.Fatalf("tools = %d", len(agents[0].Tools))
	}
	if agents[0].Tools[0].Name != "search" {
		t.Fatalf("tool name = %q", agents[0].Tools[0].Name)
	}
}

func TestRegistryRegisterPlatform(t *testing.T) {
	tool := daneel.NewTool("tweet", "Tweet",
		func(ctx context.Context, p struct{}) (string, error) { return "", nil },
	)
	r := daneel.NewRegistry()
	r.RegisterPlatform("twitter", []daneel.Tool{tool})

	platforms := r.Platforms()
	if len(platforms) != 1 {
		t.Fatalf("platforms = %d", len(platforms))
	}
	if platforms[0].Name != "twitter" {
		t.Fatalf("name = %q", platforms[0].Name)
	}
	if len(platforms[0].Tools) != 1 {
		t.Fatalf("tools = %d", len(platforms[0].Tools))
	}
}

func TestRegistryToolsDedup(t *testing.T) {
	p := mock.New()
	tool := daneel.NewTool("shared", "Shared tool",
		func(ctx context.Context, p struct{}) (string, error) { return "", nil },
	)
	agent := daneel.New("a", daneel.WithProvider(p), daneel.WithTools(tool))

	r := daneel.NewRegistry()
	r.RegisterAgent(agent)
	r.RegisterPlatform("plat", []daneel.Tool{tool})

	tools := r.Tools()
	if len(tools) != 1 {
		t.Fatalf("tools = %d, want 1 (deduped)", len(tools))
	}
}

func TestRegistryFindAgent(t *testing.T) {
	p := mock.New()
	agent := daneel.New("finder", daneel.WithProvider(p))
	r := daneel.NewRegistry()
	r.RegisterAgent(agent)

	found := r.FindAgent("finder")
	if found == nil {
		t.Fatal("should find agent")
	}
	if found.Name != "finder" {
		t.Fatalf("name = %q", found.Name)
	}

	notFound := r.FindAgent("nonexistent")
	if notFound != nil {
		t.Fatal("should not find nonexistent")
	}
}

func TestRegistryFindTool(t *testing.T) {
	p := mock.New()
	tool := daneel.NewTool("mytool", "My tool",
		func(ctx context.Context, p struct{}) (string, error) { return "", nil },
	)
	agent := daneel.New("a", daneel.WithProvider(p), daneel.WithTools(tool))
	r := daneel.NewRegistry()
	r.RegisterAgent(agent)

	found := r.FindTool("mytool")
	if found == nil {
		t.Fatal("should find tool")
	}
	if found.Name != "mytool" {
		t.Fatalf("name = %q", found.Name)
	}

	notFound := r.FindTool("nonexistent")
	if notFound != nil {
		t.Fatal("should not find nonexistent")
	}
}

func TestRegistryAgentWithHandoffs(t *testing.T) {
	p := mock.New()
	target := daneel.New("helper", daneel.WithProvider(p))
	agent := daneel.New("main",
		daneel.WithProvider(p),
		daneel.WithHandoffs(target),
	)
	r := daneel.NewRegistry()
	r.RegisterAgent(agent)

	agents := r.Agents()
	if len(agents[0].Handoffs) != 1 {
		t.Fatalf("handoffs = %d", len(agents[0].Handoffs))
	}
	if agents[0].Handoffs[0] != "helper" {
		t.Fatalf("handoff = %q", agents[0].Handoffs[0])
	}
}

// ========== Quick ==========

func TestQuick(t *testing.T) {
	qa := daneel.Quick("quickbot")
	if qa.Name() != "quickbot" {
		t.Fatalf("name = %q", qa.Name())
	}
	if qa.Connectors() != nil {
		t.Fatal("should have no connectors by default")
	}
}

// ========== RunOption coverage ==========

func TestWithRunMaxTurns(t *testing.T) {
	p := mock.New()
	p.QueueResponse("done")
	agent := daneel.New("bot", daneel.WithProvider(p))
	result, err := daneel.Run(context.Background(), agent, "hi",
		daneel.WithRunMaxTurns(1),
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output == "" {
		t.Fatal("should have content")
	}
}

func TestWithHistory(t *testing.T) {
	p := mock.New()
	p.QueueResponse("I remember")
	agent := daneel.New("bot", daneel.WithProvider(p))
	history := []daneel.Message{
		daneel.UserMessage("earlier message"),
		daneel.AssistantMessage("earlier reply"),
	}
	result, err := daneel.Run(context.Background(), agent, "recall?",
		daneel.WithHistory(history),
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output != "I remember" {
		t.Fatalf("content = %q", result.Output)
	}
	// Messages should include history + user + assistant
	if len(result.Messages) < 4 {
		t.Fatalf("messages = %d, want >= 4", len(result.Messages))
	}
}

func TestWithImageURL(t *testing.T) {
	p := mock.New()
	p.QueueResponse("I see a cat")
	agent := daneel.New("bot", daneel.WithProvider(p))
	result, err := daneel.Run(context.Background(), agent, "what is this?",
		daneel.WithImageURL("https://example.com/cat.jpg"),
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output != "I see a cat" {
		t.Fatalf("content = %q", result.Output)
	}
}

func TestWithImageData(t *testing.T) {
	p := mock.New()
	p.QueueResponse("I see an image")
	agent := daneel.New("bot", daneel.WithProvider(p))
	result, err := daneel.Run(context.Background(), agent, "describe",
		daneel.WithImageData([]byte{0x89, 0x50}, "image/png"),
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output != "I see an image" {
		t.Fatalf("content = %q", result.Output)
	}
}

func TestWithResponseSchema(t *testing.T) {
	p := mock.New()
	p.QueueResponse(`{"name":"John","age":30}`)
	agent := daneel.New("bot", daneel.WithProvider(p))
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	result, err := daneel.Run(context.Background(), agent, "who?",
		daneel.WithResponseSchema(Person{}),
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(result.Output, "John") {
		t.Fatalf("content = %q", result.Output)
	}
}

func TestWithStreaming(t *testing.T) {
	p := mock.New()
	p.QueueResponse("stream test")
	agent := daneel.New("bot", daneel.WithProvider(p))
	var chunks int
	_, err := daneel.Run(context.Background(), agent, "hi",
		daneel.WithStreaming(func(chunk daneel.StreamChunk) {
			chunks++
		}),
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// Mock does not stream, chunks may be 0, just ensure no panic
}

// ========== HandoffHistory modes ==========

func TestHandoffHistoryLastN(t *testing.T) {
	p := mock.New()
	// First agent hands off to second
	p.QueueToolCall("handoff_to_helper", `{"reason":"need help"}`)
	p.QueueResponse("helped!")

	helper := daneel.New("helper", daneel.WithProvider(p))
	agent := daneel.New("main",
		daneel.WithProvider(p),
		daneel.WithHandoffs(helper),
		daneel.WithHandoffHistory(daneel.LastN(2)),
	)

	result, err := daneel.Run(context.Background(), agent, "help me")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output != "helped!" {
		t.Fatalf("content = %q", result.Output)
	}
}

// ========== Permission patterns ==========

func TestPermissionWildcard(t *testing.T) {
	p := mock.New()
	p.QueueToolCall("mcp.github.search", `{}`)
	p.QueueResponse("done")

	tool := daneel.NewTool("mcp.github.search", "Search GitHub",
		func(ctx context.Context, p struct{}) (string, error) { return "found", nil },
	)
	agent := daneel.New("bot",
		daneel.WithProvider(p),
		daneel.WithTools(tool),
		daneel.WithPermissions(daneel.AllowTools("mcp.github.*")),
	)
	result, err := daneel.Run(context.Background(), agent, "search")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output != "done" {
		t.Fatalf("content = %q", result.Output)
	}
}

func TestPermissionAllowHandoffs(t *testing.T) {
	p := mock.New()
	p.QueueToolCall("handoff_to_allowed", `{"reason":"ok"}`)
	p.QueueResponse("handled")

	allowed := daneel.New("allowed", daneel.WithProvider(p))
	blocked := daneel.New("blocked", daneel.WithProvider(p))
	agent := daneel.New("main",
		daneel.WithProvider(p),
		daneel.WithHandoffs(allowed, blocked),
		daneel.WithPermissions(daneel.AllowHandoffs("allowed")),
	)

	result, err := daneel.Run(context.Background(), agent, "go")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output != "handled" {
		t.Fatalf("content = %q", result.Output)
	}
}

// ========== NewToolTyped ==========

func TestNewToolTyped(t *testing.T) {
	type In struct {
		X int `json:"x"`
	}
	type Out struct {
		Result int `json:"result"`
	}
	tool := daneel.NewToolTyped[In, Out]("double", "Double the input",
		func(ctx context.Context, p In) (Out, error) {
			return Out{Result: p.X * 2}, nil
		},
	)
	if tool.Name != "double" {
		t.Fatalf("name = %q", tool.Name)
	}
	result, err := tool.Run(context.Background(), json.RawMessage(`{"x":5}`))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	var out Out
	json.Unmarshal([]byte(result), &out)
	if out.Result != 10 {
		t.Fatalf("result = %d, want 10", out.Result)
	}
}

// ========== NewToolWithContent ==========

func TestNewToolWithContent(t *testing.T) {
	tool := daneel.NewToolWithContent("img", "Get image",
		func(ctx context.Context, p struct{}) (content.Content, error) {
			return content.TextContent("hello world"), nil
		},
	)
	if tool.Name != "img" {
		t.Fatalf("name = %q", tool.Name)
	}
	result, err := tool.Run(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(result, "hello world") {
		t.Fatalf("result = %q", result)
	}
}

// ========== AgentOption coverage ==========

func TestWithDefaultToolTimeout(t *testing.T) {
	p := mock.New()
	p.QueueResponse("ok")
	agent := daneel.New("bot",
		daneel.WithProvider(p),
		daneel.WithDefaultToolTimeout(5*time.Second),
	)
	result, err := daneel.Run(context.Background(), agent, "hi")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output != "ok" {
		t.Fatalf("content = %q", result.Output)
	}
}

func TestWithStrictPermissions(t *testing.T) {
	p := mock.New()
	p.QueueToolCall("forbidden", `{}`)

	tool := daneel.NewTool("forbidden", "Forbidden tool",
		func(ctx context.Context, p struct{}) (string, error) { return "bad", nil },
	)
	agent := daneel.New("bot",
		daneel.WithProvider(p),
		daneel.WithTools(tool),
		daneel.WithPermissions(daneel.DenyTools("forbidden")),
		daneel.WithStrictPermissions(),
	)
	_, err := daneel.Run(context.Background(), agent, "do it")
	if err == nil {
		t.Fatal("should error with strict permissions")
	}
}

func TestWithContextStrategy(t *testing.T) {
	p := mock.New()
	p.QueueResponse("ok")
	agent := daneel.New("bot",
		daneel.WithProvider(p),
		daneel.WithContextStrategy(daneel.ContextError),
	)
	result, err := daneel.Run(context.Background(), agent, "hi")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output != "ok" {
		t.Fatalf("content = %q", result.Output)
	}
}

func TestWithRateLimit(t *testing.T) {
	p := mock.New()
	p.QueueToolCall("t1", `{}`)
	p.QueueResponse("done")

	tool := daneel.NewTool("t1", "Tool 1",
		func(ctx context.Context, p struct{}) (string, error) { return "ok", nil },
	)
	agent := daneel.New("bot",
		daneel.WithProvider(p),
		daneel.WithTools(tool),
		daneel.WithRateLimit(1000), // very high to not block
	)
	result, err := daneel.Run(context.Background(), agent, "go")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output != "done" {
		t.Fatalf("content = %q", result.Output)
	}
}

func TestWithMemory(t *testing.T) {
	p := mock.New()
	p.QueueResponse("ok")
	// Simple in-memory store for testing
	agent := daneel.New("bot",
		daneel.WithProvider(p),
		daneel.WithMemory(nil), // nil memory = no-op
	)
	result, err := daneel.Run(context.Background(), agent, "hi")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output != "ok" {
		t.Fatalf("content = %q", result.Output)
	}
}

// ========== ToolExecution modes ==========

func TestParallelN(t *testing.T) {
	p := mock.New()
	p.QueueToolCall("t1", `{}`)
	p.QueueResponse("done")

	tool := daneel.NewTool("t1", "Tool 1",
		func(ctx context.Context, p struct{}) (string, error) { return "ok", nil },
	)
	agent := daneel.New("bot",
		daneel.WithProvider(p),
		daneel.WithTools(tool),
		daneel.WithToolExecution(daneel.ParallelN(4)),
	)
	result, err := daneel.Run(context.Background(), agent, "go")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output != "done" {
		t.Fatalf("content = %q", result.Output)
	}
}

// ========== WithModel ==========

func TestWithModel(t *testing.T) {
	p := mock.New()
	p.QueueResponse("ok")
	agent := daneel.New("bot",
		daneel.WithProvider(p),
		daneel.WithModel("gpt-4o-mini"),
	)
	result, err := daneel.Run(context.Background(), agent, "hi")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output != "ok" {
		t.Fatalf("content = %q", result.Output)
	}
}

// ========== SessionID ==========

func TestWithSessionID(t *testing.T) {
	p := mock.New()
	p.QueueResponse("ok")
	agent := daneel.New("bot", daneel.WithProvider(p))
	result, err := daneel.Run(context.Background(), agent, "hi",
		daneel.WithSessionID("my-session-123"),
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.SessionID != "my-session-123" {
		t.Fatalf("sessionID = %q", result.SessionID)
	}
}

// ========== Tool invalid args ==========

func TestToolInvalidArgs(t *testing.T) {
	tool := daneel.NewTool("test", "Test",
		func(ctx context.Context, p struct{ X int `json:"x"` }) (string, error) {
			return "", nil
		},
	)
	_, err := tool.Run(context.Background(), json.RawMessage(`{invalid}`))
	if err == nil {
		t.Fatal("should error on invalid JSON")
	}
}

// ========== NewToolTyped invalid args ==========

func TestNewToolTypedInvalidArgs(t *testing.T) {
	type In struct{ X int `json:"x"` }
	type Out struct{ R int `json:"r"` }
	tool := daneel.NewToolTyped[In, Out]("t", "T",
		func(ctx context.Context, p In) (Out, error) { return Out{}, nil },
	)
	_, err := tool.Run(context.Background(), json.RawMessage(`not json`))
	if err == nil {
		t.Fatal("should error on invalid JSON")
	}
}

// ========== HandoffHistory FullHistory vs LastN ==========

func TestHandoffFullHistory(t *testing.T) {
	p := mock.New()
	p.QueueToolCall("handoff_to_target", `{"reason":"test"}`)
	p.QueueResponse("from target")

	target := daneel.New("target", daneel.WithProvider(p))
	agent := daneel.New("main",
		daneel.WithProvider(p),
		daneel.WithHandoffs(target),
		daneel.WithHandoffHistory(daneel.FullHistory),
	)
	result, err := daneel.Run(context.Background(), agent, "go")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output != "from target" {
		t.Fatalf("content = %q", result.Output)
	}
}
