// Package workflow provides multi-agent orchestration patterns for Daneel.
//
// Available patterns:
//   - Chain — sequential pipeline: output of one agent feeds into the next
//   - Parallel — run multiple tasks concurrently and collect results
//   - Router — classify input and route to the appropriate agent
//   - Orchestrator — boss agent decomposes work, workers execute, boss synthesizes
package workflow

import (
	"context"
	"fmt"

	daneel "github.com/daneel-ai/daneel"
)

// Chain runs agents sequentially, passing the output of each as input to the
// next. Returns the final agent's result. If any agent fails, the chain stops.
//
//	result, err := workflow.Chain(ctx, "Translate this to French: Hello!",
//	    translator, editor, formatter,
//	)
func Chain(ctx context.Context, input string, agents ...*daneel.Agent) (*daneel.RunResult, error) {
	if len(agents) == 0 {
		return nil, fmt.Errorf("workflow chain: no agents provided")
	}

	var result *daneel.RunResult
	current := input

	for i, agent := range agents {
		var err error
		result, err = daneel.Run(ctx, agent, current)
		if err != nil {
			return nil, fmt.Errorf("workflow chain step %d (%s): %w", i, agent.Name(), err)
		}
		current = result.Output
	}

	return result, nil
}
