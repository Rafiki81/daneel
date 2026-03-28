package finetune

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Rafiki81/daneel"
)

// MetricType defines evaluation metrics.
type MetricType int

const (
	Accuracy         MetricType = iota // correct tool selection
	ToolCallAccuracy                   // correct tool arguments
	ResponseQuality                    // LLM-judged quality
	Latency                            // response time
	TokenEfficiency                    // tokens used per task
)

// EvalConfig configures an evaluation run.
type EvalConfig struct {
	models   []EvalModel
	metrics  []MetricType
	judge    daneel.Provider
	parallel int
}

// EvalModel pairs a name with a provider for evaluation.
type EvalModel struct {
	Name     string
	Provider daneel.Provider
}

// EvalOption configures evaluation.
type EvalOption func(*EvalConfig)

// Models sets the models to evaluate.
func Models(models ...EvalModel) EvalOption {
	return func(c *EvalConfig) { c.models = models }
}

// Model creates a named model for evaluation.
func Model(name string, p daneel.Provider) EvalModel {
	return EvalModel{Name: name, Provider: p}
}

// Metrics sets which metrics to compute.
func Metrics(metrics ...MetricType) EvalOption {
	return func(c *EvalConfig) { c.metrics = metrics }
}

// JudgeModel sets the LLM used for quality judging.
func JudgeModel(p daneel.Provider) EvalOption {
	return func(c *EvalConfig) { c.judge = p }
}

// Parallel sets evaluation concurrency.
func Parallel(n int) EvalOption {
	return func(c *EvalConfig) { c.parallel = n }
}

// EvalResult holds evaluation results for all models.
type EvalResult struct {
	Models []ModelResult `json:"models"`
}

// ModelResult holds evaluation results for a single model.
type ModelResult struct {
	Name         string  `json:"name"`
	Accuracy     float64 `json:"accuracy"`
	ToolAccuracy float64 `json:"tool_accuracy"`
	Quality      float64 `json:"quality"`
	AvgLatencyMs int64   `json:"avg_latency_ms"`
	AvgTokens    int     `json:"avg_tokens"`
}

// ExportJSON writes results to a JSON file.
func (r *EvalResult) ExportJSON(path string) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// ExportMarkdown writes a comparison table to a markdown file.
func (r *EvalResult) ExportMarkdown(path string) error {
	md := "# Evaluation Results\n\n"
	md += "| Model | Accuracy | Tool Accuracy | Quality | Latency (ms) | Avg Tokens |\n"
	md += "|---|---|---|---|---|---|\n"
	for _, m := range r.Models {
		md += fmt.Sprintf("| %s | %.1f%% | %.1f%% | %.1f/10 | %d | %d |\n",
			m.Name, m.Accuracy*100, m.ToolAccuracy*100, m.Quality, m.AvgLatencyMs, m.AvgTokens)
	}
	return os.WriteFile(path, []byte(md), 0o644)
}

// Evaluate runs evaluation against a test dataset.
func Evaluate(ctx context.Context, testPath string, opts ...EvalOption) (*EvalResult, error) {
	cfg := EvalConfig{parallel: 1}
	for _, o := range opts {
		o(&cfg)
	}

	testData, err := os.ReadFile(testPath)
	if err != nil {
		return nil, fmt.Errorf("finetune: read test data: %w", err)
	}

	var samples []json.RawMessage
	for _, line := range splitLines(testData) {
		if len(line) > 0 {
			samples = append(samples, json.RawMessage(line))
		}
	}

	result := &EvalResult{}
	for _, model := range cfg.models {
		mr := evaluateModel(ctx, model, samples, cfg)
		result.Models = append(result.Models, mr)
	}
	return result, nil
}

func evaluateModel(ctx context.Context, model EvalModel, samples []json.RawMessage, cfg EvalConfig) ModelResult {
	mr := ModelResult{Name: model.Name}
	if len(samples) == 0 {
		return mr
	}

	var totalLatency time.Duration
	var correct, toolCorrect, totalTokens int

	for _, sample := range samples {
		var conv struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if json.Unmarshal(sample, &conv) != nil || len(conv.Messages) < 2 {
			continue
		}

		// Extract user message and expected response
		var userMsg string
		var expectedOutput string
		for _, m := range conv.Messages {
			if m.Role == "user" {
				userMsg = m.Content
			}
			if m.Role == "assistant" {
				expectedOutput = m.Content
			}
		}
		if userMsg == "" {
			continue
		}

		start := time.Now()
		msgs := []daneel.Message{daneel.UserMessage(userMsg)}
		resp, err := model.Provider.Chat(ctx, msgs, nil)
		elapsed := time.Since(start)

		if err != nil {
			continue
		}

		totalLatency += elapsed
		totalTokens += resp.Usage.TotalTokens

		// Simple accuracy: check if output is similar
		if expectedOutput != "" && resp.Content == expectedOutput {
			correct++
		}
		// Tool accuracy: placeholder
		toolCorrect++
	}

	n := len(samples)
	if n > 0 {
		mr.Accuracy = float64(correct) / float64(n)
		mr.ToolAccuracy = float64(toolCorrect) / float64(n)
		mr.AvgLatencyMs = totalLatency.Milliseconds() / int64(n)
		mr.AvgTokens = totalTokens / n
	}
	mr.Quality = 5.0 // placeholder; would use LLM judge

	return mr
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			line := data[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
