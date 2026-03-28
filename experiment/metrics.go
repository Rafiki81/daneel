// Package experiment provides tools for evaluating and comparing agent configurations.
package experiment

import (
	"time"

	"github.com/Rafiki81/daneel"
)

// Metric names for ABTest and Evaluate options.
type Metric string

const (
	Latency    Metric = "latency"
	TokenCount Metric = "token_count"
	ToolCalls  Metric = "tool_calls"
	Turns      Metric = "turns"
)

// MetricSnapshot captures performance stats for a single run.
type MetricSnapshot struct {
	Latency   time.Duration
	Tokens    int
	ToolCalls int
	Turns     int
}

func collectMetrics(result *daneel.RunResult, elapsed time.Duration) MetricSnapshot {
	return MetricSnapshot{
		Latency:   elapsed,
		Tokens:    result.Usage.TotalTokens,
		ToolCalls: len(result.ToolCalls),
		Turns:     result.Turns,
	}
}
