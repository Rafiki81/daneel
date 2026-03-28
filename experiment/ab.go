package experiment

import (
	"context"
	"fmt"
	"time"

	"github.com/Rafiki81/daneel"
)

// RunPair holds results from a single A/B run.
type RunPair struct {
	ResultA  *daneel.RunResult
	ResultB  *daneel.RunResult
	MetricsA MetricSnapshot
	MetricsB MetricSnapshot
	Judge    *JudgeResult
}

// ABResult is the aggregate outcome of an ABTest.
type ABResult struct {
	NameA    string
	NameB    string
	ScoreA   float64
	ScoreB   float64
	Winner   string
	MetricsA MetricSnapshot // averages across runs
	MetricsB MetricSnapshot
	Runs     []RunPair
}

// abConfig holds ABTest options.
type abConfig struct {
	runs    int
	judge   *daneel.Agent
	metrics []Metric
}

// ABOption configures ABTest.
type ABOption func(*abConfig)

// WithJudge sets an LLM judge agent to score each run pair.
func WithJudge(agent *daneel.Agent) ABOption {
	return func(c *abConfig) { c.judge = agent }
}

// WithMetrics specifies which metrics to collect.
func WithMetrics(ms ...Metric) ABOption {
	return func(c *abConfig) { c.metrics = ms }
}

// WithRuns sets the number of runs per candidate (default: 1).
func WithRuns(n int) ABOption {
	return func(c *abConfig) { c.runs = n }
}

// ABTest runs both agents on the same input and compares results.
func ABTest(ctx context.Context, input string, agentA, agentB *daneel.Agent, opts ...ABOption) (*ABResult, error) {
	cfg := &abConfig{runs: 1}
	for _, o := range opts {
		o(cfg)
	}

	res := &ABResult{
		NameA: agentA.Name(),
		NameB: agentB.Name(),
	}

	var totalA, totalB MetricSnapshot
	for i := 0; i < cfg.runs; i++ {
		pair, err := runPair(ctx, input, agentA, agentB, cfg)
		if err != nil {
			return nil, fmt.Errorf("ab test run %d: %w", i+1, err)
		}
		res.Runs = append(res.Runs, *pair)
		totalA = addMetrics(totalA, pair.MetricsA)
		totalB = addMetrics(totalB, pair.MetricsB)
		if pair.Judge != nil {
			res.ScoreA += pair.Judge.ScoreA
			res.ScoreB += pair.Judge.ScoreB
		}
	}

	n := float64(cfg.runs)
	res.MetricsA = divMetrics(totalA, n)
	res.MetricsB = divMetrics(totalB, n)
	if cfg.judge != nil {
		res.ScoreA /= n
		res.ScoreB /= n
	}

	switch {
	case res.ScoreA > res.ScoreB:
		res.Winner = res.NameA
	case res.ScoreB > res.ScoreA:
		res.Winner = res.NameB
	default:
		res.Winner = "tie"
	}
	return res, nil
}

func runPair(ctx context.Context, input string, agentA, agentB *daneel.Agent, cfg *abConfig) (*RunPair, error) {
	pair := &RunPair{}

	var err error
	startA := time.Now()
	pair.ResultA, err = daneel.Run(ctx, agentA, input)
	if err != nil {
		return nil, fmt.Errorf("agent A: %w", err)
	}
	pair.MetricsA = collectMetrics(pair.ResultA, time.Since(startA))

	startB := time.Now()
	pair.ResultB, err = daneel.Run(ctx, agentB, input)
	if err != nil {
		return nil, fmt.Errorf("agent B: %w", err)
	}
	pair.MetricsB = collectMetrics(pair.ResultB, time.Since(startB))

	if cfg.judge != nil {
		jr, err := judgeCompare(ctx, cfg.judge, input, pair.ResultA.Output, pair.ResultB.Output)
		if err != nil {
			return nil, fmt.Errorf("judge: %w", err)
		}
		pair.Judge = jr
	}
	return pair, nil
}

func addMetrics(a, b MetricSnapshot) MetricSnapshot {
	return MetricSnapshot{
		Latency:   a.Latency + b.Latency,
		Tokens:    a.Tokens + b.Tokens,
		ToolCalls: a.ToolCalls + b.ToolCalls,
		Turns:     a.Turns + b.Turns,
	}
}

func divMetrics(a MetricSnapshot, n float64) MetricSnapshot {
	return MetricSnapshot{
		Latency:   time.Duration(float64(a.Latency) / n),
		Tokens:    int(float64(a.Tokens) / n),
		ToolCalls: int(float64(a.ToolCalls) / n),
		Turns:     int(float64(a.Turns) / n),
	}
}
