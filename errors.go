package daneel

import (
	"errors"
	"fmt"
	"time"
)

// Sentinel errors returned by the Runner and related components.
var (
	// ErrPermissionDenied is returned when a tool call is denied by the
	// agent's permission rules and WithStrictPermissions is enabled.
	ErrPermissionDenied = errors.New("daneel: permission denied")

	// ErrMaxTurns is returned when the agent loop exceeds the configured
	// maximum number of turns.
	ErrMaxTurns = errors.New("daneel: maximum turns exceeded")

	// ErrGuardFailed is returned when an input or output guard rejects
	// a message.
	ErrGuardFailed = errors.New("daneel: guard validation failed")

	// ErrHandoff is used internally to signal an agent handoff. It should
	// never be seen by user code.
	ErrHandoff = errors.New("daneel: handoff")

	// ErrNoProvider is returned when Run is called on an agent that has
	// no LLM provider configured.
	ErrNoProvider = errors.New("daneel: no provider configured")

	// ErrApprovalRequired is returned when a tool requires human approval
	// but no Approver was provided to Run.
	ErrApprovalRequired = errors.New("daneel: approval required")

	// ErrApprovalDenied is returned when a human denies a tool call via
	// the Approver interface.
	ErrApprovalDenied = errors.New("daneel: approval denied")

	// ErrToolTimeout is returned when a tool execution exceeds its
	// configured timeout.
	ErrToolTimeout = errors.New("daneel: tool execution timed out")

	// ErrContextOverflow is returned when the conversation history
	// exceeds the model's context window and the ContextError strategy
	// is in use.
	ErrContextOverflow = errors.New("daneel: context window exceeded")
)

// PermissionError provides context about a denied tool call.
type PermissionError struct {
	Agent  string // name of the agent that attempted the call
	Tool   string // name of the tool that was denied
	Reason string // human-readable reason (e.g., "tool in deny list")
}

func (e *PermissionError) Error() string {
	return fmt.Sprintf("daneel: agent %q denied tool %q: %s", e.Agent, e.Tool, e.Reason)
}

func (e *PermissionError) Unwrap() error { return ErrPermissionDenied }

// GuardError provides context about a failed guard validation.
type GuardError struct {
	Agent   string // name of the agent
	Guard   string // "input" or "output"
	Message string // description of what failed
}

func (e *GuardError) Error() string {
	return fmt.Sprintf("daneel: agent %q %s guard failed: %s", e.Agent, e.Guard, e.Message)
}

func (e *GuardError) Unwrap() error { return ErrGuardFailed }

// MaxTurnsError provides context when the agent loop exceeds its turn limit.
type MaxTurnsError struct {
	Agent    string     // name of the agent
	MaxTurns int        // the configured limit
	Partial  *RunResult // partial result up to the limit (may be nil)
}

func (e *MaxTurnsError) Error() string {
	return fmt.Sprintf("daneel: agent %q exceeded %d turns", e.Agent, e.MaxTurns)
}

func (e *MaxTurnsError) Unwrap() error { return ErrMaxTurns }

// ProviderError wraps errors from LLM provider API calls.
type ProviderError struct {
	Provider   string // provider name (e.g., "openai", "anthropic")
	StatusCode int    // HTTP status code (0 if not HTTP-related)
	Message    string // error message from provider
	Retryable  bool   // whether the caller should retry
}

func (e *ProviderError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("daneel: provider %q returned HTTP %d: %s", e.Provider, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("daneel: provider %q error: %s", e.Provider, e.Message)
}

// ErrProvider is the sentinel error for provider failures.
var ErrProvider = errors.New("daneel: provider error")

func (e *ProviderError) Unwrap() error { return ErrProvider }

// ToolTimeoutError provides context about a tool that exceeded its timeout.
type ToolTimeoutError struct {
	Tool    string        // name of the tool
	Timeout time.Duration // the configured timeout
}

func (e *ToolTimeoutError) Error() string {
	return fmt.Sprintf("daneel: tool %q execution timed out after %s", e.Tool, e.Timeout)
}

func (e *ToolTimeoutError) Unwrap() error { return ErrToolTimeout }
