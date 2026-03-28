package billing

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Rafiki81/daneel"
)

// Tracker records costs for every agent run and enforces per-tenant budgets.
type Tracker struct {
	mu       sync.RWMutex
	pricing  *PricingTable
	budgets  map[string]Budget   // tenantID → Budget
	alerts   []Alert
	fired    map[string]bool     // "tenantID:threshold" → already fired
	records  []CostRecord
}

// TrackerOption configures a Tracker.
type TrackerOption func(*Tracker)

// WithPricing sets the pricing table used to compute costs.
func WithPricing(pt *PricingTable) TrackerOption {
	return func(t *Tracker) { t.pricing = pt }
}

// WithBudget registers a monthly budget for tenantID.
func WithBudget(tenantID string, limitUSD float64) TrackerOption {
	return func(t *Tracker) {
		t.budgets[tenantID] = Budget{Tenant: tenantID, Limit: limitUSD, Period: ThisMonth}
	}
}

// WithAlert registers a spend alert.
func WithAlert(threshold func(float64, float64) bool, cb func(string, float64, float64)) TrackerOption {
	return func(t *Tracker) {
		t.alerts = append(t.alerts, Alert{Threshold: threshold, Callback: cb})
	}
}

// NewTracker creates a Tracker with the given options.
func NewTracker(opts ...TrackerOption) *Tracker {
	t := &Tracker{
		budgets: make(map[string]Budget),
		fired:   make(map[string]bool),
	}
	for _, o := range opts {
		o(t)
	}
	return t
}

// RecordAs returns a callback compatible with daneel.WithOnConversationEnd that
// records costs attributed to tenantID using model for pricing lookup.
//
//	agent := daneel.New("bot", daneel.WithOnConversationEnd(tracker.RecordAs("acme", "gpt-4o")))
func (t *Tracker) RecordAs(tenantID, model string) func(ctx context.Context, result daneel.RunResult) {
	return func(ctx context.Context, result daneel.RunResult) {
		t.record(tenantID, model, result)
	}
}

// Record is a convenience callback that uses result.AgentName as the tenant key
// and an empty model (zero-cost recording). Suitable for agent-level cost aggregation.
func (t *Tracker) Record(ctx context.Context, result daneel.RunResult) {
	t.record(result.AgentName, "", result)
}

func (t *Tracker) record(tenantID, model string, result daneel.RunResult) {
	var promptCost, completionCost float64
	if t.pricing != nil && model != "" {
		promptCost, completionCost = t.pricing.Cost(model, result.Usage.PromptTokens, result.Usage.CompletionTokens)
	}
	rec := CostRecord{
		Tenant:           tenantID,
		SessionID:        result.SessionID,
		Model:            model,
		PromptTokens:     result.Usage.PromptTokens,
		CompletionTokens: result.Usage.CompletionTokens,
		PromptCost:       promptCost,
		CompletionCost:   completionCost,
		Total:            promptCost + completionCost,
		Timestamp:        time.Now(),
	}

	t.mu.Lock()
	t.records = append(t.records, rec)
	t.mu.Unlock()

	t.checkAlerts(tenantID)
}

func (t *Tracker) checkAlerts(tenantID string) {
	budget, hasBudget := t.budgets[tenantID]
	if !hasBudget || len(t.alerts) == 0 {
		return
	}

	summary, _ := t.Cost(context.Background(), tenantID, budget.Period)
	if summary == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, alert := range t.alerts {
		key := fmt.Sprintf("%s:%d", tenantID, i)
		if !t.fired[key] && alert.Threshold(summary.Total, budget.Limit) {
			t.fired[key] = true
			if alert.Callback != nil {
				go alert.Callback(tenantID, summary.Total, budget.Limit)
			}
		}
	}
}

// Cost returns a CostSummary for tenantID over the given period.
func (t *Tracker) Cost(_ context.Context, tenantID string, period Period) (*CostSummary, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	s := &CostSummary{Period: period}
	for _, r := range t.records {
		if r.Tenant != tenantID {
			continue
		}
		if !inPeriod(r.Timestamp, period) {
			continue
		}
		s.Total += r.Total
		s.Prompt += r.PromptCost
		s.Completion += r.CompletionCost
		s.Runs++
	}
	return s, nil
}

// ExportCSV writes all cost records for the given period to a CSV file.
func (t *Tracker) ExportCSV(_ context.Context, path string, period Period) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("billing: create %q: %w", path, err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	_ = w.Write([]string{"tenant", "session_id", "model", "prompt_tokens", "completion_tokens", "prompt_cost", "completion_cost", "total", "timestamp"})
	for _, r := range t.records {
		if !inPeriod(r.Timestamp, period) {
			continue
		}
		_ = w.Write([]string{
			r.Tenant,
			r.SessionID,
			r.Model,
			fmt.Sprintf("%d", r.PromptTokens),
			fmt.Sprintf("%d", r.CompletionTokens),
			fmt.Sprintf("%.6f", r.PromptCost),
			fmt.Sprintf("%.6f", r.CompletionCost),
			fmt.Sprintf("%.6f", r.Total),
			r.Timestamp.UTC().Format(time.RFC3339),
		})
	}
	w.Flush()
	return w.Error()
}
