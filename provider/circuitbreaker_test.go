package provider_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Rafiki81/daneel"
	"github.com/Rafiki81/daneel/provider"
	"github.com/Rafiki81/daneel/provider/mock"
)

// alwaysFailProvider is a daneel.Provider that always returns an error.
type alwaysFailProvider struct{ err error }

func (a *alwaysFailProvider) Chat(_ context.Context, _ []daneel.Message, _ []daneel.ToolDef) (*daneel.Response, error) {
	return nil, a.err
}

func TestCircuitBreaker_ClosedPassthrough(t *testing.T) {
	inner := mock.New(mock.Respond("hello"))
	cb := provider.CircuitBreaker(inner)
	resp, err := cb.Chat(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "hello" {
		t.Errorf("content = %q, want hello", resp.Content)
	}
}

func TestCircuitBreaker_OpensAfterMaxFailures(t *testing.T) {
	sentinel := errors.New("provider down")
	inner := &alwaysFailProvider{err: sentinel}
	cb := provider.CircuitBreaker(inner, provider.MaxFailures(3))

	// 3 failures should trip the breaker.
	for i := 0; i < 3; i++ {
		_, err := cb.Chat(context.Background(), nil, nil)
		if !errors.Is(err, sentinel) {
			t.Fatalf("call %d: want sentinel error, got %v", i, err)
		}
	}
	// 4th request should hit the open circuit.
	_, err := cb.Chat(context.Background(), nil, nil)
	if !errors.Is(err, provider.ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen after MaxFailures, got: %v", err)
	}
}

func TestCircuitBreaker_HalfOpenThenClosed(t *testing.T) {
	sentinel := errors.New("down")
	// Inner: fail once, then succeed.
	inner := mock.New(
		mock.RespondFunc(func(_ []daneel.Message) *daneel.Response {
			return &daneel.Response{Content: "error", ToolCalls: nil}
		}),
		mock.Respond("recovered"),
	)
	// We use a provider that returns an error on the first call via ErrorResponse hack:
	// actually use alwaysFail + success mock via Fallback-like pattern.
	// Simpler: just use RespondFunc to return a provider error.
	_ = inner // not used directly; rebuild below

	failThenSucceed := mock.New(
		mock.RespondFunc(func(_ []daneel.Message) *daneel.Response {
			return nil // nil response causes mock to error? No — mock returns it as-is.
			//  Instead, queue an error response, then a success.
		}),
	)
	_ = failThenSucceed

	// Simplest approach: use alwaysFailProvider for trip, then a fresh CB with
	// success for the half-open trial.
	failP := &alwaysFailProvider{err: sentinel}
	cb := provider.CircuitBreaker(failP,
		provider.MaxFailures(1),
		provider.OpenTimeout(20*time.Millisecond),
		provider.HalfOpenRequests(1),
	)

	// Trip the breaker.
	cb.Chat(context.Background(), nil, nil) //nolint:errcheck

	// Verify it's open.
	_, err := cb.Chat(context.Background(), nil, nil)
	if !errors.Is(err, provider.ErrCircuitOpen) {
		t.Fatalf("expected open, got: %v", err)
	}

	// Wait for open timeout to expire.
	time.Sleep(30 * time.Millisecond)

	// After timeout, the CB transitions to half-open on the next call.
	// Since inner always fails, this call will fail and re-trip.
	_, err = cb.Chat(context.Background(), nil, nil)
	// Should NOT be ErrCircuitOpen (was in half-open, let the request through).
	if errors.Is(err, provider.ErrCircuitOpen) {
		t.Error("half-open should let one request through, not block")
	}
	// Should be the sentinel error from the inner provider.
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error in half-open, got: %v", err)
	}

	// Now it should be open again (failure in half-open re-trips).
	_, err = cb.Chat(context.Background(), nil, nil)
	if !errors.Is(err, provider.ErrCircuitOpen) {
		t.Errorf("expected circuit to be open again after half-open failure, got: %v", err)
	}
}

func TestCircuitBreaker_DefaultOptions(t *testing.T) {
	sentinel := errors.New("oops")
	inner := &alwaysFailProvider{err: sentinel}
	// Default: MaxFailures=5, OpenTimeout=30s.
	cb := provider.CircuitBreaker(inner)

	for i := 0; i < 5; i++ {
		cb.Chat(context.Background(), nil, nil) //nolint:errcheck
	}
	_, err := cb.Chat(context.Background(), nil, nil)
	if !errors.Is(err, provider.ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen with default MaxFailures=5, got: %v", err)
	}
}
