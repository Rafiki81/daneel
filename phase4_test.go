package daneel_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Rafiki81/daneel"
	"github.com/Rafiki81/daneel/provider/mock"
	"github.com/Rafiki81/daneel/tenant"
)

// ---- Tenant: session prefix ---------------------------------------------------

func TestWithTenant_SessionPrefixed(t *testing.T) {
	mgr := tenant.NewManager()
	mgr.Register("acme", tenant.Config{})

	p := mock.New(mock.Respond("ok"))
	agent := daneel.New("a", daneel.WithProvider(p))

	result, err := daneel.Run(context.Background(), agent, "hi",
		tenant.WithTenant(mgr, "acme"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(result.SessionID, "acme:") {
		t.Errorf("session ID = %q, want prefix acme:", result.SessionID)
	}
}

// ---- Tenant: quota enforcement ------------------------------------------------

func TestWithTenant_QuotaEnforced(t *testing.T) {
	mgr := tenant.NewManager()
	mgr.Register("cheap", tenant.Config{},
		tenant.Quota{MaxRunsPerHour: 1},
	)

	p := mock.New(mock.Respond("ok"), mock.Respond("ok"))
	agent := daneel.New("a", daneel.WithProvider(p))

	// First run should succeed.
	_, err := daneel.Run(context.Background(), agent, "first",
		tenant.WithTenant(mgr, "cheap"),
	)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Second run in the same hour should be rejected.
	_, err = daneel.Run(context.Background(), agent, "second",
		tenant.WithTenant(mgr, "cheap"),
	)
	if err == nil {
		t.Fatal("expected quota error on second run, got nil")
	}
	if !strings.Contains(err.Error(), "hourly run quota") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---- Tenant: usage recorded ---------------------------------------------------

func TestWithTenant_UsageRecorded(t *testing.T) {
	mgr := tenant.NewManager()
	mgr.Register("biz", tenant.Config{})

	p := mock.New(mock.Respond("hello"))
	agent := daneel.New("a", daneel.WithProvider(p))

	_, err := daneel.Run(context.Background(), agent, "hey",
		tenant.WithTenant(mgr, "biz"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	usage, err := mgr.Usage(context.Background(), "biz")
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if usage.RunsToday != 1 {
		t.Errorf("RunsToday = %d, want 1", usage.RunsToday)
	}
}

// ---- Max handoff depth --------------------------------------------------------

func TestWithMaxHandoffDepth_Exceeded(t *testing.T) {
	// Chain: A(maxDepth=1) → B → C.
	// A→B is allowed at depth 0 (< 1). B→C at depth 1 (≥ 1) should fail.
	agentC := daneel.New("c",
		daneel.WithProvider(mock.New(mock.Respond("done"))),
		daneel.WithMaxHandoffDepth(1),
	)
	agentB := daneel.New("b",
		daneel.WithProvider(mock.New(
			mock.RespondWithToolCall("handoff_to_c", `{"reason":"go to c"}`),
		)),
		daneel.WithHandoffs(agentC),
		daneel.WithMaxHandoffDepth(1),
	)
	agentA := daneel.New("a",
		daneel.WithProvider(mock.New(
			mock.RespondWithToolCall("handoff_to_b", `{"reason":"go to b"}`),
		)),
		daneel.WithHandoffs(agentB),
		daneel.WithMaxHandoffDepth(1),
	)

	_, err := daneel.Run(context.Background(), agentA, "start")
	if !errors.Is(err, daneel.ErrMaxHandoffDepth) {
		t.Errorf("expected ErrMaxHandoffDepth, got: %v", err)
	}
}

// ---- FailFast -----------------------------------------------------------------

func TestWithFailFast_CancelsOnError(t *testing.T) {
	// "fail" tool always returns an error immediately.
	errorTool := daneel.NewTool("fail", "always errors",
		func(ctx context.Context, p struct{}) (string, error) {
			return "", errors.New("boom")
		},
	)
	// "slow" tool blocks for 5s unless its context is cancelled.
	slowTool := daneel.NewTool("slow", "blocks until cancelled",
		func(ctx context.Context, p struct{}) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(5 * time.Second):
				return "late", nil
			}
		},
	)

	// Provider: first call returns two tool calls; second call returns text.
	twoTools := mock.RespondFunc(func(_ []daneel.Message) *daneel.Response {
		return &daneel.Response{
			ToolCalls: []daneel.ToolCall{
				{ID: "1", Name: "fail", Arguments: []byte(`{}`)},
				{ID: "2", Name: "slow", Arguments: []byte(`{}`)},
			},
		}
	})
	providerMock := mock.New(twoTools, mock.Respond("all done"))

	agent := daneel.New("a",
		daneel.WithProvider(providerMock),
		daneel.WithTools(errorTool, slowTool),
		daneel.WithToolExecution(daneel.Parallel),
		daneel.WithFailFast(),
	)

	done := make(chan struct{})
	go func() {
		daneel.Run(context.Background(), agent, "run tools") //nolint:errcheck
		close(done)
	}()

	select {
	case <-done:
		// Good — run completed quickly because slowTool was cancelled.
	case <-time.After(3 * time.Second):
		t.Fatal("FailFast did not cancel slowTool; Run blocked for too long")
	}
}

// ---- CombineRunOptions -------------------------------------------------------

func TestCombineRunOptions(t *testing.T) {
	var calls []string

	opt1 := daneel.WithRunHook(func(ctx context.Context) error {
		calls = append(calls, "pre1")
		return nil
	}, nil)
	opt2 := daneel.WithRunHook(func(ctx context.Context) error {
		calls = append(calls, "pre2")
		return nil
	}, nil)
	combined := daneel.CombineRunOptions(opt1, opt2)

	p := mock.New(mock.Respond("ok"))
	agent := daneel.New("a", daneel.WithProvider(p))

	_, err := daneel.Run(context.Background(), agent, "hi", combined)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 2 || calls[0] != "pre1" || calls[1] != "pre2" {
		t.Errorf("expected [pre1 pre2], got %v", calls)
	}
}
