package tenant

import (
	"context"

	"github.com/Rafiki81/daneel"
)

// WithTenant returns a RunOption that:
//  1. Checks all quotas for tenantID before the run starts.
//  2. Records token usage for tenantID after a successful run.
//
// Example:
//
//	result, err := daneel.Run(ctx, agent, "help",
//	    tenant.WithTenant(mgr, "acme-corp"),
//	)
func WithTenant(mgr *Manager, tenantID string) daneel.RunOption {
	return daneel.WithRunHook(
		// pre: quota check
		func(ctx context.Context) error {
			return mgr.checkQuota(ctx, tenantID)
		},
		// post: usage accounting
		func(ctx context.Context, result *daneel.RunResult) {
			mgr.recordUsage(tenantID, result.Usage)
		},
	)
}
