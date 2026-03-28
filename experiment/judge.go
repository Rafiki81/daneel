package experiment

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/daneel-ai/daneel"
)

// JudgeResult holds the scores from an LLM judge comparison.
type JudgeResult struct {
	ScoreA float64
	ScoreB float64
	Reason string
}

// judgePrompt asks the judge to score two outputs on a 1–10 scale.
func judgePrompt(input, outputA, outputB string) string {
	return fmt.Sprintf(`You are an impartial judge evaluating two AI responses.

INPUT: %s

RESPONSE A:
%s

RESPONSE B:
%s

Score each response from 1 to 10 for quality, accuracy, and helpfulness.
Reply in exactly this format (nothing else):
SCORE_A: <number>
SCORE_B: <number>
REASON: <one sentence>`, input, outputA, outputB)
}

// judgeCompare runs the judge agent and parses scores.
func judgeCompare(ctx context.Context, judgeAgent *daneel.Agent, input, outputA, outputB string) (*JudgeResult, error) {
	result, err := daneel.Run(ctx, judgeAgent, judgePrompt(input, outputA, outputB))
	if err != nil {
		return nil, fmt.Errorf("judge: run failed: %w", err)
	}
	return parseJudgeOutput(result.Output)
}

func parseJudgeOutput(output string) (*JudgeResult, error) {
	var jr JudgeResult
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "SCORE_A:"):
			v, err := strconv.ParseFloat(strings.TrimSpace(line[8:]), 64)
			if err == nil {
				jr.ScoreA = v
			}
		case strings.HasPrefix(line, "SCORE_B:"):
			v, err := strconv.ParseFloat(strings.TrimSpace(line[8:]), 64)
			if err == nil {
				jr.ScoreB = v
			}
		case strings.HasPrefix(line, "REASON:"):
			jr.Reason = strings.TrimSpace(line[7:])
		}
	}
	return &jr, nil
}
