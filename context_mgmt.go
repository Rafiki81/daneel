package daneel

import (
	"context"
	"fmt"
	"strings"
)

const (
	// defaultMaxContextTokens is the conservative fallback token limit used when
	// the agent has no explicit limit and the provider does not expose model info.
	defaultMaxContextTokens = 100_000

	// summarizeKeepRecent is the number of most-recent messages always preserved
	// verbatim when ContextSummarize is active.
	summarizeKeepRecent = 4
)

// contextWindow returns the effective input-token limit for this call.
// Resolution order:
//  1. Agent-level WithMaxContextTokens override (explicit, takes priority).
//  2. ModelInfoProvider.ModelInfo().ContextWindow — 20 % reserved for output.
//  3. defaultMaxContextTokens (100 000) as a conservative fallback.
func contextWindow(ctx context.Context, p Provider, cfg *agentConfig) int {
	if cfg.maxContextTokens > 0 {
		return cfg.maxContextTokens
	}
	if mip, ok := p.(ModelInfoProvider); ok {
		if info, err := mip.ModelInfo(ctx); err == nil && info.ContextWindow > 0 {
			return info.ContextWindow * 4 / 5 // reserve 20 % for completion
		}
	}
	return defaultMaxContextTokens
}

// countTokens returns the token count for msgs.
// Uses exact counting via TokenCounter if the provider implements it;
// otherwise falls back to the char/4 heuristic.
func countTokens(ctx context.Context, p Provider, msgs []Message) int {
	if tc, ok := p.(TokenCounter); ok {
		if n, err := tc.CountTokens(ctx, msgs); err == nil {
			return n
		}
	}
	return charTokenHeuristic(msgs)
}

// charTokenHeuristic estimates tokens at ~4 chars per token (no API call).
func charTokenHeuristic(msgs []Message) int {
	var n int
	for _, m := range msgs {
		n += len(m.Content)
		for _, tc := range m.ToolCalls {
			n += len(tc.Arguments)
		}
	}
	return n / 4
}

// manageContext applies the configured ContextStrategy to keep the message
// slice within limit tokens. Called once per turn in the runner loop.
//
//   - ContextError: messages are returned unchanged; the provider error signals
//     overflow to the caller.
//   - ContextSlidingWindow: oldest non-system messages are dropped until fit.
//   - ContextSummarize: older messages are condensed via an LLM call; falls
//     back to sliding-window if the summary call fails or still overflows.
func manageContext(ctx context.Context, msgs []Message, strategy ContextStrategy, limit int, p Provider) []Message {
	if strategy == ContextError {
		return msgs
	}

	if countTokens(ctx, p, msgs) <= limit {
		return msgs
	}

	if strategy == ContextSummarize {
		summarized := summarizeOldMessages(ctx, msgs, p)
		if countTokens(ctx, p, summarized) <= limit {
			return summarized
		}
		// Summary still overflows — fall through to sliding window.
	}

	return slidingWindowTrim(ctx, msgs, limit, p)
}

// slidingWindowTrim keeps the system prompt (when present) and the most-recent
// messages that fit within limit tokens, dropping oldest first.
func slidingWindowTrim(ctx context.Context, msgs []Message, limit int, p Provider) []Message {
	if len(msgs) < 2 {
		return msgs
	}

	var system []Message
	body := msgs
	if msgs[0].Role == RoleSystem {
		system = msgs[:1]
		body = msgs[1:]
	}

	sysTokens := charTokenHeuristic(system)
	budget := limit - sysTokens
	if budget <= 0 {
		// Even the system prompt alone exceeds the limit — return it alone.
		if len(system) > 0 {
			return system
		}
		return msgs[:1]
	}

	// Walk backwards from newest to oldest, accumulating what fits.
	keep := make([]Message, 0, len(body))
	for i := len(body) - 1; i >= 0; i-- {
		t := charTokenHeuristic([]Message{body[i]})
		if t > budget && len(keep) > 0 {
			break
		}
		keep = append([]Message{body[i]}, keep...)
		budget -= t
	}

	return append(system, keep...)
}

// summarizeOldMessages condenses older conversation turns into a single
// LLM-generated summary, preserving the system prompt and the most-recent
// summarizeKeepRecent messages verbatim.
//
// Returns the original slice unchanged if:
//   - there are fewer messages than the keep-recent threshold, or
//   - the provider call for summarization fails.
func summarizeOldMessages(ctx context.Context, msgs []Message, p Provider) []Message {
	sysIdx := -1
	if len(msgs) > 0 && msgs[0].Role == RoleSystem {
		sysIdx = 0
	}

	start := sysIdx + 1 // index of first non-system message
	if len(msgs)-start <= summarizeKeepRecent {
		return msgs // not enough history to summarize meaningfully
	}

	toSummarize := msgs[start : len(msgs)-summarizeKeepRecent]
	recent := msgs[len(msgs)-summarizeKeepRecent:]

	var sb strings.Builder
	for _, m := range toSummarize {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", m.Role, m.Content))
	}

	summaryReq := []Message{
		SystemMessage("You are a helpful assistant that summarizes conversations."),
		UserMessage("Summarize the following conversation history in 3-5 concise sentences, preserving key facts, decisions, and context:\n\n" + sb.String()),
	}

	resp, err := p.Chat(ctx, summaryReq, nil)
	if err != nil {
		return msgs // fall back to sliding window in manageContext
	}

	result := make([]Message, 0, 1+1+len(recent))
	if sysIdx >= 0 {
		result = append(result, msgs[sysIdx])
	}
	result = append(result, Message{
		Role:    RoleAssistant,
		Content: "[Earlier conversation summary: " + resp.Content + "]",
	})
	return append(result, recent...)
}
