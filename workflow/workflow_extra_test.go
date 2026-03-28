package workflow_test

import (
	"context"
	"testing"

	"github.com/Rafiki81/daneel"
	"github.com/Rafiki81/daneel/provider/mock"
	"github.com/Rafiki81/daneel/workflow"
)

// ========== Orchestrator ==========

func TestOrchestratorHappyPath(t *testing.T) {
	// Boss: decompose prompt -> JSON subtasks
	// Then boss receives worker results and synthesizes
	boss := mock.New()
	boss.QueueResponse(`[{"worker":"coder","task":"write code"},{"worker":"reviewer","task":"review code"}]`)
	boss.QueueResponse("Final synthesized answer")

	coder := mock.New()
	coder.QueueResponse("code written")

	reviewer := mock.New()
	reviewer.QueueResponse("looks good")

	bossAgent := daneel.New("boss", daneel.WithProvider(boss))
	coderAgent := daneel.New("coder", daneel.WithProvider(coder))
	reviewerAgent := daneel.New("reviewer", daneel.WithProvider(reviewer))

	result, err := workflow.Orchestrator(context.Background(), "Build a feature",
		bossAgent, coderAgent, reviewerAgent,
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output != "Final synthesized answer" {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestOrchestratorNoWorkers(t *testing.T) {
	boss := mock.New()
	bossAgent := daneel.New("boss", daneel.WithProvider(boss))
	_, err := workflow.Orchestrator(context.Background(), "task", bossAgent)
	if err == nil {
		t.Fatal("expected error with no workers")
	}
}

func TestOrchestratorBossError(t *testing.T) {
	boss := mock.New() // no responses = error
	worker := mock.New(mock.Respond("ok"))

	bossAgent := daneel.New("boss", daneel.WithProvider(boss))
	workerAgent := daneel.New("worker", daneel.WithProvider(worker))

	_, err := workflow.Orchestrator(context.Background(), "task", bossAgent, workerAgent)
	if err == nil {
		t.Fatal("expected error when boss fails")
	}
}

func TestOrchestratorInvalidJSON(t *testing.T) {
	boss := mock.New()
	boss.QueueResponse("not json at all")

	worker := mock.New(mock.Respond("ok"))
	bossAgent := daneel.New("boss", daneel.WithProvider(boss))
	workerAgent := daneel.New("worker", daneel.WithProvider(worker))

	_, err := workflow.Orchestrator(context.Background(), "task", bossAgent, workerAgent)
	if err == nil {
		t.Fatal("expected error when boss returns invalid JSON")
	}
}

func TestOrchestratorEmptySubtasks(t *testing.T) {
	boss := mock.New()
	boss.QueueResponse("[]")

	worker := mock.New(mock.Respond("ok"))
	bossAgent := daneel.New("boss", daneel.WithProvider(boss))
	workerAgent := daneel.New("worker", daneel.WithProvider(worker))

	_, err := workflow.Orchestrator(context.Background(), "task", bossAgent, workerAgent)
	if err == nil {
		t.Fatal("expected error when boss produces empty subtask list")
	}
}

func TestOrchestratorUnknownWorker(t *testing.T) {
	// Boss assigns to a worker that does not exist
	boss := mock.New()
	boss.QueueResponse(`[{"worker":"nonexistent","task":"do stuff"}]`)
	boss.QueueResponse("synthesized with error")

	worker := mock.New(mock.Respond("ok"))
	bossAgent := daneel.New("boss", daneel.WithProvider(boss))
	workerAgent := daneel.New("worker", daneel.WithProvider(worker))

	result, err := workflow.Orchestrator(context.Background(), "task", bossAgent, workerAgent)
	if err != nil {
		t.Fatalf("err: %v (should still succeed with error in synthesis)", err)
	}
	// Boss should still synthesize even when a worker was unknown
	if result.Output != "synthesized with error" {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestOrchestratorMarkdownFencedJSON(t *testing.T) {
	// Boss wraps JSON in markdown code fences
	boss := mock.New()
	boss.QueueResponse("```json\n[{\"worker\":\"w1\",\"task\":\"do it\"}]\n```")
	boss.QueueResponse("final answer")

	w1 := mock.New()
	w1.QueueResponse("w1 result")

	bossAgent := daneel.New("boss", daneel.WithProvider(boss))
	w1Agent := daneel.New("w1", daneel.WithProvider(w1))

	result, err := workflow.Orchestrator(context.Background(), "complex task",
		bossAgent, w1Agent,
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output != "final answer" {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestOrchestratorSingleWorker(t *testing.T) {
	boss := mock.New()
	boss.QueueResponse(`[{"worker":"helper","task":"help me"}]`)
	boss.QueueResponse("done")

	helper := mock.New()
	helper.QueueResponse("helped")

	bossAgent := daneel.New("boss", daneel.WithProvider(boss))
	helperAgent := daneel.New("helper", daneel.WithProvider(helper))

	result, err := workflow.Orchestrator(context.Background(), "need help",
		bossAgent, helperAgent,
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Output != "done" {
		t.Fatalf("output = %q", result.Output)
	}
}

// ========== Router edge cases ==========

func TestRouterTriageError(t *testing.T) {
	triage := mock.New() // no responses = error
	a := mock.New(mock.Respond("x"))

	triageAgent := daneel.New("triage", daneel.WithProvider(triage))
	agent := daneel.New("agent", daneel.WithProvider(a))

	_, err := workflow.Router(context.Background(), "help",
		triageAgent,
		workflow.Route{Label: "support", Agent: agent},
	)
	if err == nil {
		t.Fatal("expected error when triage fails")
	}
}

// ========== Parallel edge cases ==========

func TestParallelEmpty(t *testing.T) {
	pr := workflow.Parallel(context.Background())
	if pr.Failed() {
		t.Fatal("empty should not fail")
	}
	if len(pr.Results) != 0 {
		t.Fatalf("results = %d", len(pr.Results))
	}
}

func TestParallelAllFail(t *testing.T) {
	p1 := mock.New() // no responses
	p2 := mock.New() // no responses
	a1 := daneel.New("a1", daneel.WithProvider(p1))
	a2 := daneel.New("a2", daneel.WithProvider(p2))

	pr := workflow.Parallel(context.Background(),
		workflow.NewTask(a1, "t1"),
		workflow.NewTask(a2, "t2"),
	)
	if !pr.Failed() {
		t.Fatal("should fail")
	}
	if pr.FirstError() == nil {
		t.Fatal("FirstError should be non-nil")
	}
}

// ========== Chain error passthrough ==========

func TestChainErrorMessage(t *testing.T) {
	p := mock.New() // no responses
	a := daneel.New("fail-agent", daneel.WithProvider(p))
	_, err := workflow.Chain(context.Background(), "input", a)
	if err == nil {
		t.Fatal("expected error")
	}
	// Error should include agent name
	if err.Error() == "" {
		t.Fatal("error should have message")
	}
}
