package daneel_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Rafiki81/daneel"
	"github.com/Rafiki81/daneel/content"
	"github.com/Rafiki81/daneel/provider/mock"
)

// ---------- Tool schema: advanced types ----------

func TestToolSchemaSliceAndNested(t *testing.T) {
	type Address struct {
		City string `json:"city" desc:"City name"`
	}
	type Params struct {
		Tags    []string `json:"tags" desc:"List of tags"`
		Address Address  `json:"address"`
	}
	tool := daneel.NewTool("test", "Test",
		func(ctx context.Context, p Params) (string, error) { return "ok", nil },
	)
	def := tool.Def()
	var schema map[string]any
	json.Unmarshal(def.Schema, &schema)
	props := schema["properties"].(map[string]any)

	// tags should be array
	tags := props["tags"].(map[string]any)
	if tags["type"] != "array" {
		t.Fatalf("tags type = %v, want array", tags["type"])
	}

	// address should be object with properties
	addr := props["address"].(map[string]any)
	if addr["type"] != "object" {
		t.Fatalf("address type = %v, want object", addr["type"])
	}
	addrProps := addr["properties"].(map[string]any)
	if addrProps["city"] == nil {
		t.Fatal("address.city should exist")
	}
}

func TestToolSchemaEnum(t *testing.T) {
	type Params struct {
		Color string `json:"color" enum:"red,green,blue" desc:"Pick a color"`
	}
	tool := daneel.NewTool("test", "Test",
		func(ctx context.Context, p Params) (string, error) { return "ok", nil },
	)
	def := tool.Def()
	var schema struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	json.Unmarshal(def.Schema, &schema)
	if len(schema.Properties["color"].Enum) != 3 {
		t.Fatalf("enum len = %d, want 3", len(schema.Properties["color"].Enum))
	}
	if schema.Properties["color"].Enum[0] != "red" {
		t.Fatalf("enum[0] = %q", schema.Properties["color"].Enum[0])
	}
}

func TestToolSchemaDefault(t *testing.T) {
	type Params struct {
		Lang string `json:"lang" default:"en" desc:"Language"`
	}
	tool := daneel.NewTool("test", "Test",
		func(ctx context.Context, p Params) (string, error) { return "ok", nil },
	)
	def := tool.Def()
	var schema struct {
		Properties map[string]struct {
			Default any `json:"default"`
		} `json:"properties"`
	}
	json.Unmarshal(def.Schema, &schema)
	if schema.Properties["lang"].Default != "en" {
		t.Fatalf("default = %v, want en", schema.Properties["lang"].Default)
	}
}

func TestToolSchemaRequired(t *testing.T) {
	type Params struct {
		Name     string  `json:"name"`
		Optional *string `json:"optional"`
	}
	tool := daneel.NewTool("test", "Test",
		func(ctx context.Context, p Params) (string, error) { return "ok", nil },
	)
	def := tool.Def()
	var schema struct {
		Required []string `json:"required"`
	}
	json.Unmarshal(def.Schema, &schema)
	// Name is non-pointer -> required; Optional is pointer -> not required
	hasName := false
	hasOptional := false
	for _, r := range schema.Required {
		if r == "name" {
			hasName = true
		}
		if r == "optional" {
			hasOptional = true
		}
	}
	if !hasName {
		t.Fatal("name should be required")
	}
	if hasOptional {
		t.Fatal("optional should not be required")
	}
}

func TestToolSchemaJsonTagDash(t *testing.T) {
	type Params struct {
		Public  string `json:"public"`
		Private string `json:"-"`
	}
	tool := daneel.NewTool("test", "Test",
		func(ctx context.Context, p Params) (string, error) { return "ok", nil },
	)
	def := tool.Def()
	var schema struct {
		Properties map[string]any `json:"properties"`
	}
	json.Unmarshal(def.Schema, &schema)
	if _, ok := schema.Properties["Private"]; ok {
		t.Fatal("json dash fields should be excluded")
	}
	if _, ok := schema.Properties["public"]; !ok {
		t.Fatal("public field should be present")
	}
}

func TestToolSchemaMap(t *testing.T) {
	type Params struct {
		Meta map[string]string `json:"meta" desc:"Metadata"`
	}
	tool := daneel.NewTool("test", "Test",
		func(ctx context.Context, p Params) (string, error) { return "ok", nil },
	)
	def := tool.Def()
	var schema struct {
		Properties map[string]struct {
			Type                 string `json:"type"`
			AdditionalProperties struct {
				Type string `json:"type"`
			} `json:"additionalProperties"`
		} `json:"properties"`
	}
	json.Unmarshal(def.Schema, &schema)
	if schema.Properties["meta"].Type != "object" {
		t.Fatalf("meta type = %q", schema.Properties["meta"].Type)
	}
	if schema.Properties["meta"].AdditionalProperties.Type != "string" {
		t.Fatal("additionalProperties should be string")
	}
}

// ---------- MergeTools ----------

func TestMergeTools(t *testing.T) {
	t1 := daneel.NewTool("t1", "T1",
		func(ctx context.Context, p struct{}) (string, error) { return "", nil },
	)
	t2 := daneel.NewTool("t2", "T2",
		func(ctx context.Context, p struct{}) (string, error) { return "", nil },
	)
	t3 := daneel.NewTool("t3", "T3",
		func(ctx context.Context, p struct{}) (string, error) { return "", nil },
	)
	merged := daneel.MergeTools([]daneel.Tool{t1}, []daneel.Tool{t2, t3})
	if len(merged) != 3 {
		t.Fatalf("merged len = %d, want 3", len(merged))
	}
	if merged[0].Name != "t1" || merged[1].Name != "t2" || merged[2].Name != "t3" {
		t.Fatal("wrong order")
	}
}

func TestMergeToolsEmpty(t *testing.T) {
	merged := daneel.MergeTools()
	if len(merged) != 0 {
		t.Fatal("should be empty")
	}
}

// ---------- NewToolRaw ----------

func TestNewToolRaw(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}}}`)
	tool := daneel.NewToolRaw("raw", "Raw tool", schema,
		func(ctx context.Context, args string) (string, error) {
			return "got:" + args, nil
		},
	)
	if tool.Name != "raw" {
		t.Fatalf("name = %q", tool.Name)
	}
	result, err := tool.Run(context.Background(), json.RawMessage(`{"x":"test"}`))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(result, "test") {
		t.Fatalf("result = %q", result)
	}
}

// ---------- ToolDef ----------

func TestToolDef(t *testing.T) {
	tool := daneel.NewTool("mytool", "My description",
		func(ctx context.Context, p struct {
			X int `json:"x" desc:"Value"`
		}) (string, error) {
			return "", nil
		},
	)
	def := tool.Def()
	if def.Name != "mytool" {
		t.Fatalf("name = %q", def.Name)
	}
	if def.Description != "My description" {
		t.Fatalf("desc = %q", def.Description)
	}
	if len(def.Schema) == 0 {
		t.Fatal("schema should not be empty")
	}
}

// ---------- Error messages ----------

func TestPermissionErrorFormat(t *testing.T) {
	e := &daneel.PermissionError{Agent: "bot", Tool: "exec", Reason: "in deny list"}
	if !strings.Contains(e.Error(), "bot") {
		t.Fatal("should contain agent name")
	}
	if !strings.Contains(e.Error(), "exec") {
		t.Fatal("should contain tool name")
	}
}

func TestGuardErrorFormat(t *testing.T) {
	e := &daneel.GuardError{Agent: "bot", Guard: "input", Message: "rejected"}
	if !strings.Contains(e.Error(), "input guard") {
		t.Fatal("should contain guard type")
	}
	if !strings.Contains(e.Error(), "rejected") {
		t.Fatal("should contain message")
	}
}

func TestMaxTurnsErrorFormat(t *testing.T) {
	e := &daneel.MaxTurnsError{Agent: "bot", MaxTurns: 10}
	if !strings.Contains(e.Error(), "10") {
		t.Fatal("should contain max turns")
	}
}

func TestProviderErrorFormat(t *testing.T) {
	e := &daneel.ProviderError{Provider: "openai", StatusCode: 429, Message: "rate limited"}
	if !strings.Contains(e.Error(), "429") {
		t.Fatal("should contain status code")
	}
	if !strings.Contains(e.Error(), "openai") {
		t.Fatal("should contain provider")
	}

	// Without status code
	e2 := &daneel.ProviderError{Provider: "anthropic", Message: "error"}
	if strings.Contains(e2.Error(), "HTTP") {
		t.Fatal("should not contain HTTP when status is 0")
	}
}

func TestToolTimeoutErrorFormat(t *testing.T) {
	e := &daneel.ToolTimeoutError{Tool: "slow", Timeout: 5000000000}
	if !strings.Contains(e.Error(), "slow") {
		t.Fatal("should contain tool name")
	}
	if !strings.Contains(e.Error(), "5s") {
		t.Fatal("should contain timeout duration")
	}
}

// ---------- ToolResult.ToMessage ----------

func TestToolResultToMessage(t *testing.T) {
	r := daneel.ToolResult{
		ToolCallID: "call_1",
		Name:       "search",
		Content:    "found 3 results",
		IsError:    false,
	}
	msg := r.ToMessage()
	if msg.Role != daneel.RoleTool {
		t.Fatalf("role = %q", msg.Role)
	}
	if msg.Content != "found 3 results" {
		t.Fatalf("content = %q", msg.Content)
	}
	if msg.Name != "search" {
		t.Fatalf("name = %q", msg.Name)
	}
}

func TestToolResultToMessageError(t *testing.T) {
	r := daneel.ToolResult{
		ToolCallID: "call_1",
		Name:       "search",
		Content:    "not found",
		IsError:    true,
	}
	msg := r.ToMessage()
	if !strings.HasPrefix(msg.Content, "Error: ") {
		t.Fatalf("error content = %q, want Error: prefix", msg.Content)
	}
}

// ---------- MultiModalMessage ----------

func TestMultiModalMessage(t *testing.T) {
	img := content.ImageContent([]byte{1, 2, 3}, "image/png")
	txt := content.TextContent("describe this")
	msg := daneel.MultiModalMessage("look", img, txt)
	if msg.Role != daneel.RoleUser {
		t.Fatalf("role = %q", msg.Role)
	}
	if msg.Content != "look" {
		t.Fatalf("content = %q", msg.Content)
	}
	if len(msg.ContentParts) != 2 {
		t.Fatalf("parts = %d, want 2", len(msg.ContentParts))
	}
	if msg.ContentParts[0].Type != content.ContentImage {
		t.Fatal("first part should be image")
	}
}

// ---------- Streaming ----------

// TestStreamingTextChunks verifies that StreamText chunks arrive incrementally
// via the WithStreaming callback when the provider implements StreamProvider.
func TestStreamingTextChunks(t *testing.T) {
	p := mock.New(mock.RespondStream("Hello", ", ", "world", "!"))
	agent := daneel.New("streamer", daneel.WithProvider(p))

	var received []string
	var chunkTypes []daneel.StreamChunkType

	result, err := daneel.Run(context.Background(), agent, "hi",
		daneel.WithStreaming(func(chunk daneel.StreamChunk) {
			chunkTypes = append(chunkTypes, chunk.Type)
			if chunk.Type == daneel.StreamText {
				received = append(received, chunk.Text)
			}
		}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify full output is the concatenation of all tokens.
	if result.Output != "Hello, world!" {
		t.Fatalf("output = %q, want %q", result.Output, "Hello, world!")
	}

	// Verify we received all 4 text tokens.
	want := []string{"Hello", ", ", "world", "!"}
	if len(received) != len(want) {
		t.Fatalf("got %d text chunks, want %d: %v", len(received), len(want), received)
	}
	for i, tok := range want {
		if received[i] != tok {
			t.Errorf("chunk[%d] = %q, want %q", i, received[i], tok)
		}
	}

	// The last event should be StreamDone.
	if len(chunkTypes) == 0 || chunkTypes[len(chunkTypes)-1] != daneel.StreamDone {
		t.Fatalf("last chunk type = %v, want StreamDone", chunkTypes)
	}
}

// TestStreamingFinalOutput verifies that result.Output is correct even when
// the response arrives in streaming chunks.
func TestStreamingFinalOutput(t *testing.T) {
	p := mock.New(mock.RespondStream("Go", " is", " great"))
	agent := daneel.New("a", daneel.WithProvider(p))

	result, err := daneel.Run(context.Background(), agent, "opinion?",
		daneel.WithStreaming(func(daneel.StreamChunk) {}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "Go is great" {
		t.Fatalf("output = %q, want %q", result.Output, "Go is great")
	}
}

// TestStreamingWithToolCalls verifies that StreamToolCallStart fires before
// StreamToolCallDone when a streaming provider triggers a tool call.
func TestStreamingWithToolCalls(t *testing.T) {
	searchTool := daneel.NewTool("search", "web search",
		func(_ context.Context, args struct {
			Query string `json:"query"`
		}) (string, error) {
			return "result for: " + args.Query, nil
		},
	)

	p := mock.New(
		mock.RespondWithToolCall("search", `{"query":"golang"}`),
		mock.RespondStream("Found something!"),
	)
	agent := daneel.New("searcher",
		daneel.WithProvider(p),
		daneel.WithTools(searchTool),
	)

	var events []daneel.StreamChunkType

	result, err := daneel.Run(context.Background(), agent, "search for golang",
		daneel.WithStreaming(func(chunk daneel.StreamChunk) {
			events = append(events, chunk.Type)
		}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "Found something!" {
		t.Fatalf("output = %q, want %q", result.Output, "Found something!")
	}

	// Verify event ordering: StreamToolCallStart must precede StreamToolCallDone.
	var startIdx, doneIdx = -1, -1
	for i, e := range events {
		if e == daneel.StreamToolCallStart && startIdx < 0 {
			startIdx = i
		}
		if e == daneel.StreamToolCallDone && doneIdx < 0 {
			doneIdx = i
		}
	}
	if startIdx < 0 {
		t.Fatal("StreamToolCallStart not fired")
	}
	if doneIdx < 0 {
		t.Fatal("StreamToolCallDone not fired")
	}
	if startIdx >= doneIdx {
		t.Fatalf("StreamToolCallStart (index %d) must come before StreamToolCallDone (index %d)", startIdx, doneIdx)
	}
}

// TestStreamingFallback verifies that a non-streaming provider still fires
// StreamDone at the end when WithStreaming is configured.
func TestStreamingFallback(t *testing.T) {
	// mock.New with Respond (not RespondStream) is a non-streaming provider
	// when ChatStream is also implemented — here we use plain Respond which
	// the mock's ChatStream handles by emitting the whole content as one chunk.
	p := mock.New(mock.Respond("plain response"))
	agent := daneel.New("a", daneel.WithProvider(p))

	var events []daneel.StreamChunkType
	result, err := daneel.Run(context.Background(), agent, "hi",
		daneel.WithStreaming(func(chunk daneel.StreamChunk) {
			events = append(events, chunk.Type)
		}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "plain response" {
		t.Fatalf("output = %q, want %q", result.Output, "plain response")
	}

	// Must receive at least StreamText followed by StreamDone.
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d: %v", len(events), events)
	}
	if events[len(events)-1] != daneel.StreamDone {
		t.Fatalf("last event = %v, want StreamDone", events[len(events)-1])
	}
	hasTxt := false
	for _, e := range events {
		if e == daneel.StreamText {
			hasTxt = true
		}
	}
	if !hasTxt {
		t.Fatal("expected at least one StreamText event")
	}
}

// TestStreamingCancellation verifies that a cancelled context stops processing
// cleanly without a panic or deadlock.
func TestStreamingCancellation(t *testing.T) {
	p := mock.New(mock.RespondStream("tok1", "tok2", "tok3"))
	agent := daneel.New("a", daneel.WithProvider(p))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// The run should return an error (context cancelled) rather than hanging.
	_, err := daneel.Run(ctx, agent, "hi",
		daneel.WithStreaming(func(daneel.StreamChunk) {}),
	)
	if err == nil {
		// Some implementations may have already completed before the cancel
		// takes effect — that's acceptable.
		return
	}
	if !strings.Contains(err.Error(), "context") {
		t.Logf("error (acceptable): %v", err)
	}
}
