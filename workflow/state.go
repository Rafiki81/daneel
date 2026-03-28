package workflow

import (
	"context"

	daneel "github.com/Rafiki81/daneel"
)

// Transition defines an edge in the FSM: when event is matched in the agent's
// output, control moves to Target state.
type Transition struct {
	Event  string // keyword to match (case-insensitive substring)
	Target string
	Fn     func(ctx context.Context, output string) bool // custom matcher; overrides Event
}

// stateOpt configures a StateDef.
type stateOpt func(*StateDef)

// StateDef represents a single state in the FSM: an agent plus its outgoing transitions.
type StateDef struct {
	name        string
	agent       *daneel.Agent
	transitions []Transition
}

// On registers a keyword-based transition: if the agent output contains event
// (case-insensitive), the FSM moves to target.
func On(event, target string) stateOpt {
	return func(s *StateDef) {
		s.transitions = append(s.transitions, Transition{Event: event, Target: target})
	}
}

// TransitionFn registers a custom transition using fn to decide whether to move
// to target. fn receives the agent's output text.
func TransitionFn(target string, fn func(ctx context.Context, output string) bool) stateOpt {
	return func(s *StateDef) {
		s.transitions = append(s.transitions, Transition{Target: target, Fn: fn})
	}
}

// FSMOption configures an FSM.
type FSMOption func(*fsmConfig)

// State defines a named state in the FSM. It returns an FSMOption so it can be
// passed directly to NewFSM.
func State(name string, agent *daneel.Agent, opts ...stateOpt) FSMOption {
	return func(cfg *fsmConfig) {
		def := &StateDef{name: name, agent: agent}
		for _, o := range opts {
			o(def)
		}
		cfg.states[name] = def
		cfg.order = append(cfg.order, name)
	}
}

// WithInitialState sets the first state the FSM enters. If not set, the first
// State() declared is used.
func WithInitialState(name string) FSMOption {
	return func(cfg *fsmConfig) { cfg.initialState = name }
}

// WithMaxTransitions sets the maximum number of state transitions before the
// FSM returns an error. Default is 50.
func WithMaxTransitions(n int) FSMOption {
	return func(cfg *fsmConfig) { cfg.maxTransitions = n }
}
