package workflow

import (
	"context"
	"fmt"
	"sync"

	daneel "github.com/daneel-ai/daneel"
)

// Task represents a single unit of parallel work.
type Task struct {
	Agent *daneel.Agent
	Input string
}

// NewTask creates a task for parallel execution.
func NewTask(agent *daneel.Agent, input string) Task {
	return Task{Agent: agent, Input: input}
}

// ParallelResult holds the results from all parallel tasks.
type ParallelResult struct {
	Results []*daneel.RunResult
	Errors  []error
}

// Failed returns true if any task failed.
func (p *ParallelResult) Failed() bool {
	for _, err := range p.Errors {
		if err != nil {
			return true
		}
	}
	return false
}

// FirstError returns the first non-nil error, or nil.
func (p *ParallelResult) FirstError() error {
	for i, err := range p.Errors {
		if err != nil {
			return fmt.Errorf("task %d: %w", i, err)
		}
	}
	return nil
}

// Parallel runs multiple tasks concurrently. Each task gets its own goroutine.
// Results and errors are returned in the same order as the input tasks.
//
//	pr, err := workflow.Parallel(ctx,
//	    workflow.NewTask(analyzer, doc1),
//	    workflow.NewTask(analyzer, doc2),
//	    workflow.NewTask(analyzer, doc3),
//	)
func Parallel(ctx context.Context, tasks ...Task) *ParallelResult {
	pr := &ParallelResult{
		Results: make([]*daneel.RunResult, len(tasks)),
		Errors:  make([]error, len(tasks)),
	}

	var wg sync.WaitGroup
	wg.Add(len(tasks))

	for i, t := range tasks {
		go func(idx int, task Task) {
			defer wg.Done()
			result, err := daneel.Run(ctx, task.Agent, task.Input)
			pr.Results[idx] = result
			pr.Errors[idx] = err
		}(i, t)
	}

	wg.Wait()
	return pr
}
