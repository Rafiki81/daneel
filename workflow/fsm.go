package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	daneel "github.com/Rafiki81/daneel"
)

// fsmConfig holds all configuration for an FSM.
type fsmConfig struct {
	states         map[string]*StateDef
	order          []string // insertion-ordered state names
	initialState   string
	maxTransitions int
}

// FSM is a finite state machine where each state is handled by a daneel Agent.
// Transitions are triggered by keyword matching on the agent's output.
type FSM struct {
	name string
	cfg  fsmConfig
}

// FSMResult is the outcome of a completed FSM run.
type FSMResult struct {
	FinalState string
	Path       []string    // sequence of states visited
	Output     string      // last agent output
	Duration   time.Duration
}

// NewFSM creates a new FSM with the given name and options.
//
// Example:
//
//	fsm := workflow.NewFSM("support",
//	    workflow.State("triage", triageAgent,
//	        workflow.On("escalate", "escalation"),
//	        workflow.On("resolve", "done"),
//	    ),
//	    workflow.State("escalation", escalationAgent,
//	        workflow.On("resolved", "done"),
//	    ),
//	    workflow.State("done", resolutionAgent),
//	    workflow.WithInitialState("triage"),
//	    workflow.WithMaxTransitions(20),
//	)
func NewFSM(name string, opts ...FSMOption) *FSM {
	cfg := fsmConfig{
		states:         make(map[string]*StateDef),
		maxTransitions: 50,
	}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.initialState == "" && len(cfg.order) > 0 {
		cfg.initialState = cfg.order[0]
	}
	return &FSM{name: name, cfg: cfg}
}

// Run executes the FSM starting from the initial state, passing input to each
// state's agent and routing based on the output.
func (f *FSM) Run(ctx context.Context, input string) (*FSMResult, error) {
	start := time.Now()
	current := f.cfg.initialState
	if current == "" {
		return nil, fmt.Errorf("fsm %q: no initial state defined", f.name)
	}

	path := []string{current}
	var lastOutput string

	for step := 0; step < f.cfg.maxTransitions; step++ {
		state, ok := f.cfg.states[current]
		if !ok {
			return nil, fmt.Errorf("fsm %q: unknown state %q", f.name, current)
		}

		result, err := daneel.Run(ctx, state.agent, input)
		if err != nil {
			return nil, fmt.Errorf("fsm %q state %q: %w", f.name, current, err)
		}
		lastOutput = result.Output

		// Attempt to find a matching transition
		next := matchFSMTransition(ctx, state.transitions, result.Output)
		if next == "" {
			// No transition matched → this is a terminal state
			return &FSMResult{
				FinalState: current,
				Path:       path,
				Output:     lastOutput,
				Duration:   time.Since(start),
			}, nil
		}

		// Move to next state, pass the current output as new input
		current = next
		input = lastOutput
		path = append(path, current)
	}

	return nil, fmt.Errorf("fsm %q: exceeded max transitions (%d)", f.name, f.cfg.maxTransitions)
}

// matchFSMTransition returns the target state of the first matching transition,
// or an empty string if none match.
func matchFSMTransition(ctx context.Context, transitions []Transition, output string) string {
	lower := strings.ToLower(output)
	for _, t := range transitions {
		if t.Fn != nil {
			if t.Fn(ctx, output) {
				return t.Target
			}
		} else if strings.Contains(lower, strings.ToLower(t.Event)) {
			return t.Target
		}
	}
	return ""
}
