// Package approval provides convenience Approver implementations for
// human-in-the-loop approval workflows.
//
// The core Approver interface is defined in the daneel root package.
// This package provides ready-to-use implementations:
//
//   - AutoApprove: approve all tool calls (testing)
//   - AlwaysDeny: deny all tool calls (dry-run, audit)
//   - Console: interactive terminal approval
//   - Webhook: HTTP POST for external approval services
//   - Policy: rule-based approval with allow/deny lists
//   - WithLogging: decorator that logs decisions
//   - WithTimeout: decorator that adds timeouts
package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/daneel-ai/daneel"
)

// AutoApprove returns an Approver that automatically approves every tool call.
// Useful for testing and trusted execution environments.
func AutoApprove() daneel.Approver {
	return daneel.ApproverFunc(func(ctx context.Context, req daneel.ApprovalRequest) (bool, error) {
		return true, nil
	})
}

// AlwaysDeny returns an Approver that denies every tool call.
// Useful for dry-run and audit scenarios.
func AlwaysDeny() daneel.Approver {
	return daneel.ApproverFunc(func(ctx context.Context, req daneel.ApprovalRequest) (bool, error) {
		return false, nil
	})
}

// DenyWithReason returns an Approver that denies all calls with an error reason.
func DenyWithReason(reason string) daneel.Approver {
	return daneel.ApproverFunc(func(ctx context.Context, req daneel.ApprovalRequest) (bool, error) {
		return false, fmt.Errorf("approval denied: %s", reason)
	})
}

// Console returns an Approver that prompts the user in the terminal.
func Console() daneel.Approver {
	return ConsoleWithWriter(os.Stdout, os.Stdin)
}

// ConsoleWithWriter returns a Console approver using custom I/O.
func ConsoleWithWriter(w io.Writer, r io.Reader) daneel.Approver {
	return daneel.ApproverFunc(func(ctx context.Context, req daneel.ApprovalRequest) (bool, error) {
		var prettyArgs string
		var buf map[string]any
		if err := json.Unmarshal(req.Args, &buf); err == nil {
			b, _ := json.MarshalIndent(buf, "  ", "  ")
			prettyArgs = string(b)
		} else {
			prettyArgs = string(req.Args)
		}

		fmt.Fprintf(w, "\n--- Approval Required ---\n")
		fmt.Fprintf(w, "Agent:   %s\n", req.Agent)
		fmt.Fprintf(w, "Tool:    %s\n", req.Tool)
		fmt.Fprintf(w, "Session: %s\n", req.SessionID)
		fmt.Fprintf(w, "Args:\n")
		for _, line := range strings.Split(prettyArgs, "\n") {
			fmt.Fprintf(w, "  %s\n", line)
		}
		fmt.Fprintf(w, "-------------------------\n")
		fmt.Fprintf(w, "Approve? [y/n]: ")

		var answer string
		if _, err := fmt.Fscan(r, &answer); err != nil {
			return false, fmt.Errorf("reading approval input: %w", err)
		}

		answer = strings.TrimSpace(strings.ToLower(answer))
		return answer == "y" || answer == "yes", nil
	})
}

// CallbackFunc is a simple approval callback function.
type CallbackFunc func(agent, tool string, args map[string]any) bool

// Callback returns an Approver that delegates to a simple callback.
func Callback(fn CallbackFunc) daneel.Approver {
	return daneel.ApproverFunc(func(ctx context.Context, req daneel.ApprovalRequest) (bool, error) {
		var args map[string]any
		if err := json.Unmarshal(req.Args, &args); err != nil {
			args = map[string]any{"raw": string(req.Args)}
		}
		return fn(req.Agent, req.Tool, args), nil
	})
}

// --- Webhook Approver ---

// WebhookConfig configures the webhook-based approval flow.
type WebhookConfig struct {
	URL     string            // POST endpoint
	Headers map[string]string // custom headers
	Timeout time.Duration     // HTTP timeout (default 30s)
}

type webhookRequest struct {
	Agent     string          `json:"agent"`
	Tool      string          `json:"tool"`
	Args      json.RawMessage `json:"args"`
	SessionID string          `json:"session_id"`
}

type webhookResponse struct {
	Approved bool   `json:"approved"`
	Reason   string `json:"reason,omitempty"`
}

// Webhook returns an Approver that sends HTTP POST to an external approval service.
func Webhook(cfg WebhookConfig) daneel.Approver {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	return daneel.ApproverFunc(func(ctx context.Context, req daneel.ApprovalRequest) (bool, error) {
		body, err := json.Marshal(webhookRequest{
			Agent:     req.Agent,
			Tool:      req.Tool,
			Args:      req.Args,
			SessionID: req.SessionID,
		})
		if err != nil {
			return false, fmt.Errorf("marshal webhook request: %w", err)
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, strings.NewReader(string(body)))
		if err != nil {
			return false, fmt.Errorf("create webhook request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		for k, v := range cfg.Headers {
			httpReq.Header.Set(k, v)
		}

		resp, err := client.Do(httpReq)
		if err != nil {
			return false, fmt.Errorf("webhook request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return false, fmt.Errorf("webhook returned status %d", resp.StatusCode)
		}

		var result webhookResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return false, fmt.Errorf("decode webhook response: %w", err)
		}
		return result.Approved, nil
	})
}

// --- Policy Approver ---

// Policy is a rule-based approver with ordered allow/deny lists.
type Policy struct {
	rules []policyRule
}

type policyAction int

const (
	actionAllow policyAction = iota
	actionDeny
)

type policyRule struct {
	action policyAction
	tools  []string
}

// NewPolicy creates a new Policy approver.
func NewPolicy() *Policy {
	return &Policy{}
}

// Allow adds tools that should be auto-approved.
func (p *Policy) Allow(tools ...string) *Policy {
	p.rules = append(p.rules, policyRule{action: actionAllow, tools: tools})
	return p
}

// Deny adds tools that should always be denied.
func (p *Policy) Deny(tools ...string) *Policy {
	p.rules = append(p.rules, policyRule{action: actionDeny, tools: tools})
	return p
}

// Approve implements daneel.Approver. First match wins; unmatched = deny.
func (p *Policy) Approve(ctx context.Context, req daneel.ApprovalRequest) (bool, error) {
	for _, rule := range p.rules {
		for _, t := range rule.tools {
			if t == req.Tool {
				return rule.action == actionAllow, nil
			}
		}
	}
	return false, nil
}

// --- Decorators ---

// Logger wraps another Approver and logs each decision.
type Logger struct {
	inner daneel.Approver
	logFn func(tool string, approved bool)
}

// WithLogging wraps an approver with logging.
func WithLogging(inner daneel.Approver, logFn func(tool string, approved bool)) daneel.Approver {
	return &Logger{inner: inner, logFn: logFn}
}

// Approve implements daneel.Approver.
func (l *Logger) Approve(ctx context.Context, req daneel.ApprovalRequest) (bool, error) {
	approved, err := l.inner.Approve(ctx, req)
	if err == nil && l.logFn != nil {
		l.logFn(req.Tool, approved)
	}
	return approved, err
}

// WithTimeout wraps an approver with a context timeout.
func WithTimeout(inner daneel.Approver, d time.Duration) daneel.Approver {
	return daneel.ApproverFunc(func(ctx context.Context, req daneel.ApprovalRequest) (bool, error) {
		ctx, cancel := context.WithTimeout(ctx, d)
		defer cancel()

		type result struct {
			approved bool
			err      error
		}
		ch := make(chan result, 1)
		go func() {
			a, e := inner.Approve(ctx, req)
			ch <- result{a, e}
		}()

		select {
		case r := <-ch:
			return r.approved, r.err
		case <-ctx.Done():
			return false, fmt.Errorf("approval timed out after %v", d)
		}
	})
}
