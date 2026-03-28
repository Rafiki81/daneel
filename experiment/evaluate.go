package experiment

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Rafiki81/daneel"
)

// Candidate pairs a name with an agent for batch evaluation.
type Candidate struct {
	Name  string
	Agent *daneel.Agent
}

// EvalRun is the result of running one candidate on one input.
type EvalRun struct {
	CandidateName string
	Input         string
	Result        *daneel.RunResult
	Metrics       MetricSnapshot
	JudgeScore    float64
	JudgeReason   string
}

// EvalResults holds all evaluation runs.
type EvalResults struct {
	Runs []EvalRun
}

// AverageScore returns the average judge score per candidate.
func (r *EvalResults) AverageScore() map[string]float64 {
	totals := make(map[string]float64)
	counts := make(map[string]int)
	for _, run := range r.Runs {
		totals[run.CandidateName] += run.JudgeScore
		counts[run.CandidateName]++
	}
	out := make(map[string]float64, len(totals))
	for name, total := range totals {
		out[name] = total / float64(counts[name])
	}
	return out
}

// ExportCSV writes results to a CSV file.
func (r *EvalResults) ExportCSV(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("experiment: create csv %q: %w", path, err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	_ = w.Write([]string{"candidate", "input", "output", "latency_ms", "tokens", "tool_calls", "turns", "judge_score", "judge_reason"})
	for _, run := range r.Runs {
		_ = w.Write([]string{
			run.CandidateName,
			run.Input,
			run.Result.Output,
			fmt.Sprintf("%d", run.Metrics.Latency.Milliseconds()),
			fmt.Sprintf("%d", run.Metrics.Tokens),
			fmt.Sprintf("%d", run.Metrics.ToolCalls),
			fmt.Sprintf("%d", run.Metrics.Turns),
			fmt.Sprintf("%.2f", run.JudgeScore),
			run.JudgeReason,
		})
	}
	w.Flush()
	return w.Error()
}

// ExportJSON writes results to a JSON file.
func (r *EvalResults) ExportJSON(path string) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("experiment: marshal json: %w", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("experiment: write json %q: %w", path, err)
	}
	return nil
}

// EvalOption configures Evaluate.
type EvalOption func(*evalConfig)

type evalConfig struct {
	judge       *daneel.Agent
	concurrency int
}

// WithEvalJudge sets the judge agent for scoring outputs.
func WithEvalJudge(agent *daneel.Agent) EvalOption {
	return func(c *evalConfig) { c.judge = agent }
}

// WithConcurrency sets how many (candidate, input) pairs run in parallel.
func WithConcurrency(n int) EvalOption {
	return func(c *evalConfig) { c.concurrency = n }
}

// Evaluate runs each candidate against each input in the dataset, optionally
// scoring outputs with a judge agent. Returns all results.
func Evaluate(ctx context.Context, dataset []string, candidates []Candidate, opts ...EvalOption) (*EvalResults, error) {
	cfg := &evalConfig{concurrency: 1}
	for _, o := range opts {
		o(cfg)
	}
	if cfg.concurrency < 1 {
		cfg.concurrency = 1
	}

	type work struct {
		c     Candidate
		input string
	}

	var works []work
	for _, c := range candidates {
		for _, inp := range dataset {
			works = append(works, work{c, inp})
		}
	}

	sem := make(chan struct{}, cfg.concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var runs []EvalRun
	var firstErr error

	for _, w := range works {
		sem <- struct{}{}
		wg.Add(1)
		go func(w work) {
			defer func() { <-sem; wg.Done() }()

			start := time.Now()
			result, err := daneel.Run(ctx, w.c.Agent, w.input)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("candidate %q on input %q: %w", w.c.Name, w.input, err)
				}
				mu.Unlock()
				return
			}
			run := EvalRun{
				CandidateName: w.c.Name,
				Input:         w.input,
				Result:        result,
				Metrics:       collectMetrics(result, time.Since(start)),
			}
			if cfg.judge != nil {
				jr, err := judgeCompare(ctx, cfg.judge, w.input, result.Output, "")
				if err == nil {
					run.JudgeScore = jr.ScoreA
					run.JudgeReason = jr.Reason
				}
			}
			mu.Lock()
			runs = append(runs, run)
			mu.Unlock()
		}(w)
	}
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	return &EvalResults{Runs: runs}, nil
}
