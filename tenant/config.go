// Package tenant provides multi-tenant isolation for daneel agents.
// All resources (quotas, usage, cost) are scoped by a tenant identifier.
package tenant

import "time"

// Config holds per-tenant agent settings.
type Config struct {
	Model        string   // override LLM model for this tenant
	MaxTurns     int      // override max turns (0 = global default)
	MaxTokens    int      // soft token limit per run (0 = no limit)
	AllowedTools []string // allowlist of tool names (empty = all allowed)
	DeniedTools  []string // denylist of tool names
}

// Quota limits resource consumption per tenant.
type Quota struct {
	MaxRunsPerHour  int     // 0 = unlimited
	MaxRunsPerDay   int     // 0 = unlimited
	MaxCostPerDay   float64 // USD; 0 = unlimited
	MaxCostPerMonth float64 // USD; 0 = unlimited
}

// TenantUsage tracks accumulated resource consumption for a tenant.
type TenantUsage struct {
	RunsThisHour  int
	RunsToday     int
	CostToday     float64
	CostThisMonth float64
	TokensUsed    int
	LastRun       time.Time
}
