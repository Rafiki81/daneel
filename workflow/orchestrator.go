package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	daneel "github.com/daneel-ai/daneel"
)

// Orchestrator implements a boss/worker pattern. The boss agent decomposes the
// input into subtasks, worker agents execute them in parallel, and the boss
// synthesizes the results into a final output.
//
//	result, err := workflow.Orchestrator(ctx, complexTask, bossAgent,
//	    researcher, coder, reviewer,
//	)
func Orchestrator(ctx context.Context, input string, boss *daneel.Agent, workers ...*daneel.Agent) (*daneel.RunResult, error) {
	if len(workers) == 0 {
		return nil, fmt.Errorf("workflow orchestrator: no workers provided")
	}

	// Phase 1: Boss decomposes the task.
	workerNames := make([]string, len(workers))
	for i, w := range workers {
		workerNames[i] = w.Name()
	}

	decomposePrompt := fmt.Sprintf(
		"You are a task orchestrator. Break down the following task into subtasks "+
			"that can be assigned to workers. Available workers: [%s].\n\n"+
			"Output a JSON array of objects with \"worker\" and \"task\" fields. "+
			"Output ONLY the JSON array, no other text.\n\nTask: %s",
		strings.Join(workerNames, ", "), input,
	)

	decomposeResult, err := daneel.Run(ctx, boss, decomposePrompt)
	if err != nil {
		return nil, fmt.Errorf("workflow orchestrator decompose: %w", err)
	}

	var subtasks []struct {
		Worker string `json:"worker"`
		Task   string `json:"task"`
	}
	if err := json.Unmarshal([]byte(extractJSON(decomposeResult.Output)), &subtasks); err != nil {
		return nil, fmt.Errorf("workflow orchestrator parse subtasks: %w (output was: %s)", err, decomposeResult.Output)
	}

	if len(subtasks) == 0 {
		return nil, fmt.Errorf("workflow orchestrator: boss produced no subtasks")
	}

	// Build worker lookup.
	workerMap := make(map[string]*daneel.Agent, len(workers))
	for _, w := range workers {
		workerMap[normalizeRoute(w.Name())] = w
	}

	// Phase 2: Execute subtasks in parallel.
	type subtaskResult struct {
		Worker string
		Task   string
		Output string
		Err    error
	}

	results := make([]subtaskResult, len(subtasks))
	var wg sync.WaitGroup
	wg.Add(len(subtasks))

	for i, st := range subtasks {
		go func(idx int, workerName, task string) {
			defer wg.Done()
			w, ok := workerMap[normalizeRoute(workerName)]
			if !ok {
				results[idx] = subtaskResult{
					Worker: workerName,
					Task:   task,
					Err:    fmt.Errorf("unknown worker: %s", workerName),
				}
				return
			}
			r, err := daneel.Run(ctx, w, task)
			sr := subtaskResult{Worker: workerName, Task: task, Err: err}
			if r != nil {
				sr.Output = r.Output
			}
			results[idx] = sr
		}(i, st.Worker, st.Task)
	}
	wg.Wait()

	// Phase 3: Boss synthesizes all results.
	var sb strings.Builder
	sb.WriteString("Here are the results from each worker:\n\n")
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("--- Subtask %d (worker: %s) ---\n", i+1, r.Worker))
		sb.WriteString(fmt.Sprintf("Task: %s\n", r.Task))
		if r.Err != nil {
			sb.WriteString(fmt.Sprintf("Error: %v\n", r.Err))
		} else {
			sb.WriteString(fmt.Sprintf("Result: %s\n", r.Output))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("Synthesize these results into a coherent final answer for the original task:\n")
	sb.WriteString(input)

	return daneel.Run(ctx, boss, sb.String())
}

// extractJSON tries to find a JSON array in the text, handling cases where
// the LLM wraps JSON in markdown code fences.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	// Strip markdown code fences if present.
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 3 {
			// Remove first and last lines (the fences).
			s = strings.Join(lines[1:len(lines)-1], "\n")
			s = strings.TrimSpace(s)
		}
	}

	// Find the JSON array boundaries.
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}
