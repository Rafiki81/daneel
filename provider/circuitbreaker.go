package provider

import (
	"context"
	"errors"
	"sync"
	"time"

	daneel "github.com/Rafiki81/daneel"
)

// ErrCircuitOpen is returned when a request is rejected because the circuit
// breaker is in the open (tripped) state.
var ErrCircuitOpen = errors.New("provider: circuit breaker is open")

// CircuitBreakerOption configures a CircuitBreaker.
type CircuitBreakerOption func(*cbConfig)

type cbConfig struct {
	maxFailures      int
	openTimeout      time.Duration
	halfOpenRequests int
}

// MaxFailures sets the number of consecutive failures required to trip the
// circuit breaker from closed to open. Default: 5.
func MaxFailures(n int) CircuitBreakerOption {
	return func(c *cbConfig) { c.maxFailures = n }
}

// OpenTimeout sets how long the circuit stays open before transitioning to
// half-open for a trial request. Default: 30s.
func OpenTimeout(d time.Duration) CircuitBreakerOption {
	return func(c *cbConfig) { c.openTimeout = d }
}

// HalfOpenRequests sets the number of successful requests in half-open state
// required to close the circuit again. Default: 1.
func HalfOpenRequests(n int) CircuitBreakerOption {
	return func(c *cbConfig) { c.halfOpenRequests = n }
}

type breakerState int

const (
	breakerClosed   breakerState = iota // normal operation
	breakerOpen                         // rejecting all requests
	breakerHalfOpen                     // trial mode after open timeout
)

type circuitBreaker struct {
	inner daneel.Provider
	cfg   cbConfig

	mu                sync.Mutex
	state             breakerState
	consecutiveFails  int
	openedAt          time.Time
	halfOpenSuccesses int
}

// CircuitBreaker wraps p with a circuit breaker that trips after consecutive
// failures and recovers automatically.
//
//	p := provider.CircuitBreaker(openai.New(...),
//	    provider.MaxFailures(5),
//	    provider.OpenTimeout(30*time.Second),
//	)
func CircuitBreaker(p daneel.Provider, opts ...CircuitBreakerOption) daneel.Provider {
	cfg := cbConfig{
		maxFailures:      5,
		openTimeout:      30 * time.Second,
		halfOpenRequests: 1,
	}
	for _, o := range opts {
		o(&cfg)
	}
	return &circuitBreaker{inner: p, cfg: cfg}
}

// Chat implements daneel.Provider.
func (cb *circuitBreaker) Chat(ctx context.Context, messages []daneel.Message, tools []daneel.ToolDef) (*daneel.Response, error) {
	if err := cb.allow(); err != nil {
		return nil, err
	}
	resp, err := cb.inner.Chat(ctx, messages, tools)
	cb.record(err)
	return resp, err
}

// State returns the current circuit breaker state as a string.
// Useful for health checks and monitoring.
func (cb *circuitBreaker) State() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case breakerOpen:
		return "open"
	case breakerHalfOpen:
		return "half-open"
	default:
		return "closed"
	}
}

// allow checks if a request should be allowed through.
func (cb *circuitBreaker) allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case breakerOpen:
		if time.Since(cb.openedAt) >= cb.cfg.openTimeout {
			// Transition to half-open — let one trial request through.
			cb.state = breakerHalfOpen
			cb.halfOpenSuccesses = 0
			return nil
		}
		return ErrCircuitOpen
	default:
		return nil // closed or half-open: let through
	}
}

// record updates the breaker state based on whether the last call succeeded.
func (cb *circuitBreaker) record(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if err != nil {
		cb.consecutiveFails++
		if cb.state == breakerHalfOpen || cb.consecutiveFails >= cb.cfg.maxFailures {
			// Trip the breaker.
			cb.state = breakerOpen
			cb.openedAt = time.Now()
			cb.consecutiveFails = 0
		}
	} else {
		switch cb.state {
		case breakerHalfOpen:
			cb.halfOpenSuccesses++
			if cb.halfOpenSuccesses >= cb.cfg.halfOpenRequests {
				cb.state = breakerClosed
				cb.consecutiveFails = 0
			}
		default:
			cb.consecutiveFails = 0
		}
	}
}
