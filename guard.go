package daneel

import "context"

// InputGuard validates user input before it is sent to the LLM.
// Return a non-nil error to reject the input. The error message
// is wrapped in a GuardError.
type InputGuard func(ctx context.Context, input string) error

// OutputGuard validates the LLM's response before returning it
// to the caller. Return a non-nil error to reject the output.
type OutputGuard func(ctx context.Context, output string) error
