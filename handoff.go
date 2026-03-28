package daneel

import (
	"context"
	"encoding/json"
	"fmt"
)

const handoffPrefix = "handoff_to_"

// handoffParams is the parameter struct for synthetic handoff tools.
type handoffParams struct {
	Reason string `json:"reason" desc:"Why the handoff is happening"`
}

// makeHandoffTools creates synthetic tools for each handoff target agent.
// The Runner recognizes these by the "handoff_to_" prefix.
func makeHandoffTools(targets []*Agent) []Tool {
	tools := make([]Tool, len(targets))
	for i, target := range targets {
		name := handoffPrefix + target.Name()

		// Auto-generate description from target's instructions (first 200 chars)
		desc := target.Instructions()
		if len(desc) > 200 {
			desc = desc[:200] + "..."
		}
		if desc == "" {
			desc = fmt.Sprintf("Hand off to the %s agent", target.Name())
		} else {
			desc = fmt.Sprintf("Hand off to %s: %s", target.Name(), desc)
		}

		tools[i] = NewTool[handoffParams](name, desc,
			func(ctx context.Context, p handoffParams) (string, error) {
				// This function is never actually called — the Runner
				// intercepts handoff tools before execution.
				return "", ErrHandoff
			},
		)
	}
	return tools
}

// isHandoffTool returns true if the tool name is a synthetic handoff tool.
func isHandoffTool(name string) bool {
	return len(name) > len(handoffPrefix) && name[:len(handoffPrefix)] == handoffPrefix
}

// handoffTargetName extracts the target agent name from a handoff tool name.
func handoffTargetName(toolName string) string {
	return toolName[len(handoffPrefix):]
}

// findHandoffTarget finds the target agent by name from the handoff list.
func findHandoffTarget(handoffs []*Agent, name string) *Agent {
	for _, a := range handoffs {
		if a.Name() == name {
			return a
		}
	}
	return nil
}

// prepareHandoffHistory prepares conversation history for the target agent
// based on the configured HandoffHistory mode.
func prepareHandoffHistory(msgs []Message, mode HandoffHistory) []Message {
	n := mode.count()
	switch {
	case n == 0: // FullHistory
		cp := make([]Message, len(msgs))
		copy(cp, msgs)
		return cp
	case n == -1: // SummaryHistory — the Runner handles this via LLM call
		return msgs
	default: // LastN
		if n >= len(msgs) {
			cp := make([]Message, len(msgs))
			copy(cp, msgs)
			return cp
		}
		cp := make([]Message, n)
		copy(cp, msgs[len(msgs)-n:])
		return cp
	}
}

// HandoffResult is the internal result of a handoff, used by the Runner.
type HandoffResult struct {
	TargetAgent string
	Reason      string
	Result      *RunResult
}

// parseHandoffArgs extracts the reason from handoff tool arguments.
func parseHandoffArgs(args json.RawMessage) string {
	var p handoffParams
	if err := json.Unmarshal(args, &p); err != nil {
		return ""
	}
	return p.Reason
}
