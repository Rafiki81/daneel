// Package billing provides cost tracking and budget enforcement for daneel agents.
package billing

import "time"

// Period specifies the time window for cost queries.
type Period int

const (
	Today     Period = iota // current calendar day (UTC)
	ThisMonth               // current calendar month (UTC)
	LastMonth               // previous calendar month (UTC)
	AllTime                 // no time filter
)

// CostRecord is a single cost entry recorded after one agent run.
type CostRecord struct {
	Tenant           string
	SessionID        string
	Model            string
	PromptTokens     int
	CompletionTokens int
	PromptCost       float64
	CompletionCost   float64
	Total            float64
	Timestamp        time.Time
}

// CostSummary aggregates multiple CostRecords for reporting.
type CostSummary struct {
	Total      float64
	Prompt     float64
	Completion float64
	Runs       int
	Period     Period
	From       time.Time
	To         time.Time
}

func inPeriod(t time.Time, p Period) bool {
	now := time.Now().UTC()
	switch p {
	case Today:
		y, m, d := now.Date()
		ty, tm, td := t.UTC().Date()
		return ty == y && tm == m && td == d
	case ThisMonth:
		y, m, _ := now.Date()
		ty, tm, _ := t.UTC().Date()
		return ty == y && tm == m
	case LastMonth:
		last := now.AddDate(0, -1, 0)
		y, m, _ := last.Date()
		ty, tm, _ := t.UTC().Date()
		return ty == y && tm == m
	default: // AllTime
		return true
	}
}
