// Package provider provides utilities for combining and routing between
// multiple LLM providers.
//
// Available strategies:
//   - Fallback — try providers in order, use first that succeeds
//   - RoundRobin — distribute requests across providers
//   - CostRouter — use cheapest provider first, escalate on failure
package provider

import (
	"context"
	"fmt"
	"sync/atomic"

	daneel "github.com/daneel-ai/daneel"
)

// Fallback returns a Provider that tries each provider in order and returns
// the first successful response. If all providers fail, the last error is
// returned.
//
//	p := provider.Fallback(primaryProvider, backupProvider)
func Fallback(providers ...daneel.Provider) daneel.Provider {
	if len(providers) == 1 {
		return providers[0]
	}
	return &fallbackProvider{providers: providers}
}

type fallbackProvider struct {
	providers []daneel.Provider
}

func (f *fallbackProvider) Chat(ctx context.Context, messages []daneel.Message, tools []daneel.ToolDef) (*daneel.Response, error) {
	var lastErr error

	for i, p := range f.providers {
		resp, err := p.Chat(ctx, messages, tools)
		if err == nil {
			return resp, nil
		}
		lastErr = fmt.Errorf("provider %d: %w", i, err)

		// Don't try next provider if context is cancelled.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Only fallback on retryable errors.
		if pe, ok := err.(*daneel.ProviderError); ok && !pe.Retryable {
			return nil, lastErr
		}
	}

	return nil, fmt.Errorf("all providers failed: %w", lastErr)
}

// RoundRobin returns a Provider that distributes requests across providers
// using round-robin selection. Useful for load balancing across multiple
// API keys or instances.
//
//	p := provider.RoundRobin(instance1, instance2, instance3)
func RoundRobin(providers ...daneel.Provider) daneel.Provider {
	if len(providers) == 1 {
		return providers[0]
	}
	return &roundRobinProvider{providers: providers}
}

type roundRobinProvider struct {
	providers []daneel.Provider
	counter   atomic.Uint64
}

func (r *roundRobinProvider) Chat(ctx context.Context, messages []daneel.Message, tools []daneel.ToolDef) (*daneel.Response, error) {
	idx := r.counter.Add(1) - 1
	p := r.providers[idx%uint64(len(r.providers))]
	return p.Chat(ctx, messages, tools)
}

// Tier defines a provider with a cost ceiling for cost-based routing.
type Tier struct {
	Provider     daneel.Provider
	MaxCostPer1K float64 // maximum cost per 1K tokens (0 = free/local)
}

// CostRouter returns a Provider that tries providers from cheapest to most
// expensive. Each tier is attempted in order; if it fails with a retryable
// error, the next tier is tried.
//
//	p := provider.CostRouter(
//	    provider.Tier{Provider: localModel, MaxCostPer1K: 0},
//	    provider.Tier{Provider: cheapModel, MaxCostPer1K: 0.15},
//	    provider.Tier{Provider: premiumModel, MaxCostPer1K: 2.50},
//	)
func CostRouter(tiers ...Tier) daneel.Provider {
	if len(tiers) == 1 {
		return tiers[0].Provider
	}
	return &costRouter{tiers: tiers}
}

type costRouter struct {
	tiers []Tier
}

func (c *costRouter) Chat(ctx context.Context, messages []daneel.Message, tools []daneel.ToolDef) (*daneel.Response, error) {
	var lastErr error

	for i, tier := range c.tiers {
		resp, err := tier.Provider.Chat(ctx, messages, tools)
		if err == nil {
			return resp, nil
		}
		lastErr = fmt.Errorf("tier %d (max $%.4f/1K): %w", i, tier.MaxCostPer1K, err)

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Only escalate on retryable errors.
		if pe, ok := err.(*daneel.ProviderError); ok && !pe.Retryable {
			return nil, lastErr
		}
	}

	return nil, fmt.Errorf("all tiers failed: %w", lastErr)
}
