package daneel

import (
	"encoding/json"
	"time"
)

// RunResult is the complete result of an agent execution.
type RunResult struct {
	Output      string           // final text response
	Messages    []Message        // full conversation history
	ToolCalls   []ToolCallRecord // all tool calls made
	Turns       int              // number of agent loop iterations
	Usage       Usage            // total token usage across all LLM calls
	Duration    time.Duration    // total wall-clock time
	HandoffFrom string           // if this was a handoff, which agent started it
	AgentName   string           // which agent produced this result
	SessionID   string           // conversation session identifier
}

// ToolCallRecord captures details about a single tool invocation.
type ToolCallRecord struct {
	Name      string          // tool name
	Arguments json.RawMessage // raw JSON arguments
	Result    string          // the tool's output
	IsError   bool            // whether the tool returned an error
	Duration  time.Duration   // how long the tool took
	Permitted bool            // was it allowed by permissions?
}

// StructuredResult wraps RunResult with a parsed typed output for
// RunStructured[T] calls.
type StructuredResult[T any] struct {
	RunResult        // embeds the full run result
	Data      T      // the parsed structured output
	Raw       string // the raw JSON string before parsing
}
