package workflow

import (
	"context"
	"fmt"
	"strings"

	daneel "github.com/Rafiki81/daneel"
)

// Route maps a label to an agent. The triage agent's output is matched against
// route labels to select the appropriate handler.
type Route struct {
	Label string
	Agent *daneel.Agent
}

// Router classifies the input using a triage agent, then dispatches to the
// matching route's agent. The triage agent should output a single label
// matching one of the route labels.
//
//	result, err := workflow.Router(ctx, userInput, triageAgent,
//	    workflow.Route{Label: "billing", Agent: billingAgent},
//	    workflow.Route{Label: "support", Agent: supportAgent},
//	    workflow.Route{Label: "sales",   Agent: salesAgent},
//	)
func Router(ctx context.Context, input string, triage *daneel.Agent, routes ...Route) (*daneel.RunResult, error) {
	if len(routes) == 0 {
		return nil, fmt.Errorf("workflow router: no routes provided")
	}

	// Build the triage prompt with available labels.
	labels := make([]string, len(routes))
	for i, r := range routes {
		labels[i] = r.Label
	}

	triagePrompt := fmt.Sprintf(
		"Classify the following input into exactly one of these categories: [%s]. "+
			"Output ONLY the category label, nothing else.\n\nInput: %s",
		strings.Join(labels, ", "), input,
	)

	triageResult, err := daneel.Run(ctx, triage, triagePrompt)
	if err != nil {
		return nil, fmt.Errorf("workflow router triage: %w", err)
	}

	chosen := normalizeRoute(triageResult.Output)

	// Find the matching route.
	for _, r := range routes {
		if normalizeRoute(r.Label) == chosen {
			return daneel.Run(ctx, r.Agent, input)
		}
	}

	// Fuzzy fallback: find closest match.
	if best, ok := findClosestRoute(chosen, routes); ok {
		return daneel.Run(ctx, best.Agent, input)
	}

	return nil, fmt.Errorf("workflow router: triage output %q did not match any route %v", triageResult.Output, labels)
}

// normalizeRoute lowercases and trims whitespace for matching.
func normalizeRoute(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// findClosestRoute does a simple substring match as a fallback.
func findClosestRoute(chosen string, routes []Route) (Route, bool) {
	for _, r := range routes {
		norm := normalizeRoute(r.Label)
		if strings.Contains(chosen, norm) || strings.Contains(norm, chosen) {
			return r, true
		}
	}
	return Route{}, false
}
