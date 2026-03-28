package workflow_test

import (
	"context"
	"testing"

	"github.com/daneel-ai/daneel"
	"github.com/daneel-ai/daneel/provider/mock"
	"github.com/daneel-ai/daneel/workflow"
)

// ---------- Chain ----------

func TestChainSequential(t *testing.T) {
	p1 := mock.New(mock.Respond("step1-output"))
	p2 := mock.New(mock.Respond("step2-output"))
	p3 := mock.New(mock.Respond("final"))
	a1 := daneel.New("s1", daneel.WithProvider(p1))
	a2 := daneel.New("s2", daneel.WithProvider(p2))
	a3 := daneel.New("s3", daneel.WithProvider(p3))

	result, err := workflow.Chain(context.Background(), "start", a1, a2, a3)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output != "final" {
		t.Fatalf("output = %q, want final", result.Output)
	}
	// p2 should have received p1 output as input
	calls := p2.Calls()
	if len(calls) == 0 {
		t.Fatal("p2 was not called")
	}
	found := false
	for _, msg := range calls[0].Messages {
		if msg.Role == daneel.RoleUser && msg.Content == "step1-output" {
			found = true
		}
	}
	if !found {
		t.Fatal("p2 did not receive p1 output as input")
	}
}

func TestChainNoAgents(t *testing.T) {
	_, err := workflow.Chain(context.Background(), "input")
	if err == nil {
		t.Fatal("expected error with no agents")
	}
}

func TestChainSingleAgent(t *testing.T) {
	p := mock.New(mock.Respond("only"))
	a := daneel.New("a", daneel.WithProvider(p))
	result, err := workflow.Chain(context.Background(), "go", a)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output != "only" {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestChainStopsOnError(t *testing.T) {
	p1 := mock.New() // no responses = error
	p2 := mock.New(mock.Respond("should not reach"))
	a1 := daneel.New("fail", daneel.WithProvider(p1))
	a2 := daneel.New("ok", daneel.WithProvider(p2))
	_, err := workflow.Chain(context.Background(), "go", a1, a2)
	if err == nil {
		t.Fatal("expected error")
	}
	if p2.CallCount() != 0 {
		t.Fatal("p2 should not have been called")
	}
}

// ---------- Parallel ----------

func TestParallelAllSucceed(t *testing.T) {
	p1 := mock.New(mock.Respond("r1"))
	p2 := mock.New(mock.Respond("r2"))
	p3 := mock.New(mock.Respond("r3"))
	a1 := daneel.New("a1", daneel.WithProvider(p1))
	a2 := daneel.New("a2", daneel.WithProvider(p2))
	a3 := daneel.New("a3", daneel.WithProvider(p3))

	pr := workflow.Parallel(context.Background(),
		workflow.NewTask(a1, "t1"),
		workflow.NewTask(a2, "t2"),
		workflow.NewTask(a3, "t3"),
	)

	if pr.Failed() {
		t.Fatalf("failed: %v", pr.FirstError())
	}
	if len(pr.Results) != 3 {
		t.Fatalf("got %d results, want 3", len(pr.Results))
	}
	// Results should be in order
	if pr.Results[0].Output != "r1" {
		t.Fatalf("result[0] = %q", pr.Results[0].Output)
	}
	if pr.Results[2].Output != "r3" {
		t.Fatalf("result[2] = %q", pr.Results[2].Output)
	}
}

func TestParallelWithError(t *testing.T) {
	p1 := mock.New(mock.Respond("ok"))
	p2 := mock.New() // no responses = error
	a1 := daneel.New("a1", daneel.WithProvider(p1))
	a2 := daneel.New("a2", daneel.WithProvider(p2))

	pr := workflow.Parallel(context.Background(),
		workflow.NewTask(a1, "t1"),
		workflow.NewTask(a2, "t2"),
	)

	if !pr.Failed() {
		t.Fatal("expected failure")
	}
	if pr.FirstError() == nil {
		t.Fatal("FirstError should be non-nil")
	}
	// Task 0 should still succeed
	if pr.Results[0] == nil || pr.Results[0].Output != "ok" {
		t.Fatal("task 0 should have succeeded")
	}
}

func TestParallelNotFailed(t *testing.T) {
	p := mock.New(mock.Respond("ok"))
	a := daneel.New("a", daneel.WithProvider(p))
	pr := workflow.Parallel(context.Background(), workflow.NewTask(a, "go"))
	if pr.Failed() {
		t.Fatal("should not fail")
	}
	if pr.FirstError() != nil {
		t.Fatal("FirstError should be nil")
	}
}

// ---------- Router ----------

func TestRouterDispatch(t *testing.T) {
	triage := mock.New(mock.Respond("billing"))
	billing := mock.New(mock.Respond("billing reply"))
	support := mock.New(mock.Respond("support reply"))

	triageAgent := daneel.New("triage", daneel.WithProvider(triage))
	billingAgent := daneel.New("billing", daneel.WithProvider(billing))
	supportAgent := daneel.New("support", daneel.WithProvider(support))

	result, err := workflow.Router(context.Background(), "I have a billing issue",
		triageAgent,
		workflow.Route{Label: "billing", Agent: billingAgent},
		workflow.Route{Label: "support", Agent: supportAgent},
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output != "billing reply" {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestRouterCaseInsensitive(t *testing.T) {
	triage := mock.New(mock.Respond("  BILLING  "))
	billing := mock.New(mock.Respond("ok"))

	triageAgent := daneel.New("triage", daneel.WithProvider(triage))
	billingAgent := daneel.New("billing", daneel.WithProvider(billing))

	result, err := workflow.Router(context.Background(), "input",
		triageAgent,
		workflow.Route{Label: "billing", Agent: billingAgent},
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output != "ok" {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestRouterFuzzyMatch(t *testing.T) {
	// Triage returns "I think this is about billing" which contains "billing"
	triage := mock.New(mock.Respond("I think this is about billing"))
	billing := mock.New(mock.Respond("billing answer"))

	triageAgent := daneel.New("triage", daneel.WithProvider(triage))
	billingAgent := daneel.New("billing", daneel.WithProvider(billing))

	result, err := workflow.Router(context.Background(), "help",
		triageAgent,
		workflow.Route{Label: "billing", Agent: billingAgent},
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output != "billing answer" {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestRouterNoMatch(t *testing.T) {
	triage := mock.New(mock.Respond("unknown_category"))
	a := mock.New(mock.Respond("x"))

	triageAgent := daneel.New("triage", daneel.WithProvider(triage))
	agent := daneel.New("agent", daneel.WithProvider(a))

	_, err := workflow.Router(context.Background(), "help",
		triageAgent,
		workflow.Route{Label: "billing", Agent: agent},
	)
	if err == nil {
		t.Fatal("expected error for no matching route")
	}
}

func TestRouterNoRoutes(t *testing.T) {
	triage := mock.New(mock.Respond("x"))
	triageAgent := daneel.New("triage", daneel.WithProvider(triage))
	_, err := workflow.Router(context.Background(), "help", triageAgent)
	if err == nil {
		t.Fatal("expected error with no routes")
	}
}
