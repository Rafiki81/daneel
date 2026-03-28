package tenant

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Rafiki81/daneel"
)

// tenantEntry holds the mutable state for a single tenant.
type tenantEntry struct {
	id    string
	cfg   Config
	quota Quota
	usage TenantUsage
	mu    sync.Mutex
	// window tracking
	hourStart time.Time
	dayStart  time.Time
	monthKey  string // "YYYY-MM"
}

// Manager manages tenant registration, quota enforcement, and usage tracking.
type Manager struct {
	entries      sync.Map // tenantID → *tenantEntry
	defaultQuota Quota
}

// ManagerOption configures a Manager.
type ManagerOption func(*Manager)

// WithDefaultQuota sets the quota applied to all tenants unless overridden.
func WithDefaultQuota(q Quota) ManagerOption {
	return func(m *Manager) { m.defaultQuota = q }
}

// NewManager creates a new Manager.
func NewManager(opts ...ManagerOption) *Manager {
	m := &Manager{}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Register adds or updates a tenant configuration, optionally overriding the quota.
func (m *Manager) Register(id string, cfg Config, quotaOpts ...Quota) {
	quota := m.defaultQuota
	if len(quotaOpts) > 0 {
		quota = quotaOpts[0]
	}
	now := time.Now()
	e := &tenantEntry{
		id:        id,
		cfg:       cfg,
		quota:     quota,
		hourStart: now,
		dayStart:  truncateToDay(now),
		monthKey:  now.Format("2006-01"),
	}
	m.entries.Store(id, e)
}

// Config returns the configuration for tenant id.
func (m *Manager) Config(id string) (*Config, bool) {
	v, ok := m.entries.Load(id)
	if !ok {
		return nil, false
	}
	cfg := v.(*tenantEntry).cfg
	return &cfg, true
}

// Usage returns the current usage snapshot for a tenant.
func (m *Manager) Usage(_ context.Context, id string) (*TenantUsage, error) {
	v, ok := m.entries.Load(id)
	if !ok {
		return nil, fmt.Errorf("tenant: %q not registered", id)
	}
	e := v.(*tenantEntry)
	e.mu.Lock()
	u := e.usage
	e.mu.Unlock()
	return &u, nil
}

// ListTenants returns all registered tenant IDs.
func (m *Manager) ListTenants() []string {
	var ids []string
	m.entries.Range(func(k, _ any) bool {
		ids = append(ids, k.(string))
		return true
	})
	return ids
}

// checkQuota returns an error if the tenant has exceeded any configured limit.
func (m *Manager) checkQuota(_ context.Context, id string) error {
	v, ok := m.entries.Load(id)
	if !ok {
		return fmt.Errorf("tenant: %q not registered", id)
	}
	e := v.(*tenantEntry)
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	// Reset hourly counter
	if now.Sub(e.hourStart) >= time.Hour {
		e.usage.RunsThisHour = 0
		e.hourStart = now
	}
	// Reset daily counters
	if truncateToDay(now).After(e.dayStart) {
		e.usage.RunsToday = 0
		e.usage.CostToday = 0
		e.dayStart = truncateToDay(now)
	}
	// Reset monthly counters
	if mk := now.Format("2006-01"); mk != e.monthKey {
		e.usage.CostThisMonth = 0
		e.monthKey = mk
	}

	q := e.quota
	if q.MaxRunsPerHour > 0 && e.usage.RunsThisHour >= q.MaxRunsPerHour {
		return fmt.Errorf("tenant %q: hourly run quota exceeded (%d)", id, q.MaxRunsPerHour)
	}
	if q.MaxRunsPerDay > 0 && e.usage.RunsToday >= q.MaxRunsPerDay {
		return fmt.Errorf("tenant %q: daily run quota exceeded (%d)", id, q.MaxRunsPerDay)
	}
	if q.MaxCostPerDay > 0 && e.usage.CostToday >= q.MaxCostPerDay {
		return fmt.Errorf("tenant %q: daily cost quota exceeded ($%.2f)", id, q.MaxCostPerDay)
	}
	if q.MaxCostPerMonth > 0 && e.usage.CostThisMonth >= q.MaxCostPerMonth {
		return fmt.Errorf("tenant %q: monthly cost quota exceeded ($%.2f)", id, q.MaxCostPerMonth)
	}
	return nil
}

// recordUsage updates run counters for the tenant after a successful run.
func (m *Manager) recordUsage(id string, usage daneel.Usage) {
	v, ok := m.entries.Load(id)
	if !ok {
		return
	}
	e := v.(*tenantEntry)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.usage.RunsThisHour++
	e.usage.RunsToday++
	e.usage.TokensUsed += usage.TotalTokens
	e.usage.LastRun = time.Now()
}

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
