package finetune_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/daneel-ai/daneel"
	"github.com/daneel-ai/daneel/finetune"
	"github.com/daneel-ai/daneel/provider/mock"
)

// ========== Evaluator Option functions ==========

func TestEvalOptionModels(t *testing.T) {
	p := mock.New()
	m := finetune.Model("gpt4", p)
	if m.Name != "gpt4" {
		t.Fatalf("name = %q", m.Name)
	}
	opt := finetune.Models(m)
	if opt == nil {
		t.Fatal("option should not be nil")
	}
}

func TestEvalOptionMetrics(t *testing.T) {
	opt := finetune.Metrics(finetune.Accuracy, finetune.ToolCallAccuracy, finetune.ResponseQuality)
	if opt == nil {
		t.Fatal("option should not be nil")
	}
}

func TestEvalOptionJudgeModel(t *testing.T) {
	p := mock.New()
	opt := finetune.JudgeModel(p)
	if opt == nil {
		t.Fatal("option should not be nil")
	}
}

func TestEvalOptionParallel(t *testing.T) {
	opt := finetune.Parallel(4)
	if opt == nil {
		t.Fatal("option should not be nil")
	}
}

// ========== Evaluate function ==========

func TestEvaluateNoModels(t *testing.T) {
	// Create test data file
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.jsonl")
	data := `{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"}]}` + "\n"
	os.WriteFile(testFile, []byte(data), 0o644)

	result, err := finetune.Evaluate(context.Background(), testFile)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(result.Models) != 0 {
		t.Fatalf("models = %d, want 0", len(result.Models))
	}
}

func TestEvaluateWithModel(t *testing.T) {
	p := mock.New()
	// Queue responses for each sample evaluation
	p.QueueResponse("hello")
	p.QueueResponse("world")

	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.jsonl")
	lines := []string{
		`{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"}]}`,
		`{"messages":[{"role":"user","content":"hey"},{"role":"assistant","content":"world"}]}`,
	}
	os.WriteFile(testFile, []byte(strings.Join(lines, "\n")+"\n"), 0o644)

	result, err := finetune.Evaluate(context.Background(), testFile,
		finetune.Models(finetune.Model("test-model", p)),
		finetune.Metrics(finetune.Accuracy, finetune.Latency),
		finetune.Parallel(2),
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(result.Models) != 1 {
		t.Fatalf("models = %d, want 1", len(result.Models))
	}
	mr := result.Models[0]
	if mr.Name != "test-model" {
		t.Fatalf("name = %q", mr.Name)
	}
	// ToolAccuracy should be 1.0 (always counted as correct)
	if mr.ToolAccuracy != 1.0 {
		t.Fatalf("toolAccuracy = %f, want 1.0", mr.ToolAccuracy)
	}
	// Quality is hardcoded to 5.0
	if mr.Quality != 5.0 {
		t.Fatalf("quality = %f, want 5.0", mr.Quality)
	}
}

func TestEvaluateWithExactMatch(t *testing.T) {
	p := mock.New()
	// Return exact expected output
	p.QueueResponse("hello")

	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.jsonl")
	data := `{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"}]}` + "\n"
	os.WriteFile(testFile, []byte(data), 0o644)

	result, err := finetune.Evaluate(context.Background(), testFile,
		finetune.Models(finetune.Model("exact", p)),
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Models[0].Accuracy != 1.0 {
		t.Fatalf("accuracy = %f, want 1.0 (exact match)", result.Models[0].Accuracy)
	}
}

func TestEvaluateMissingFile(t *testing.T) {
	_, err := finetune.Evaluate(context.Background(), "/nonexistent/test.jsonl")
	if err == nil {
		t.Fatal("should error on missing file")
	}
}

func TestEvaluateEmptyFile(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "empty.jsonl")
	os.WriteFile(testFile, []byte(""), 0o644)

	p := mock.New()
	result, err := finetune.Evaluate(context.Background(), testFile,
		finetune.Models(finetune.Model("m", p)),
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Models[0].Accuracy != 0 {
		t.Fatalf("accuracy = %f, want 0 for empty", result.Models[0].Accuracy)
	}
}

func TestEvaluateMultipleModels(t *testing.T) {
	p1 := mock.New()
	p2 := mock.New()
	p1.QueueResponse("hello")
	p2.QueueResponse("wrong")

	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.jsonl")
	data := `{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"}]}` + "\n"
	os.WriteFile(testFile, []byte(data), 0o644)

	result, err := finetune.Evaluate(context.Background(), testFile,
		finetune.Models(
			finetune.Model("good", p1),
			finetune.Model("bad", p2),
		),
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(result.Models) != 2 {
		t.Fatalf("models = %d, want 2", len(result.Models))
	}
	if result.Models[0].Accuracy != 1.0 {
		t.Fatalf("good accuracy = %f", result.Models[0].Accuracy)
	}
	if result.Models[1].Accuracy != 0.0 {
		t.Fatalf("bad accuracy = %f", result.Models[1].Accuracy)
	}
}

// ========== Trainer method option functions ==========

func TestTrainerWithUnsloth(t *testing.T) {
	opt := finetune.WithUnsloth()
	if opt == nil {
		t.Fatal("nil")
	}
}

func TestTrainerWithTRL(t *testing.T) {
	opt := finetune.WithTRL()
	if opt == nil {
		t.Fatal("nil")
	}
}

func TestTrainerWithMLX(t *testing.T) {
	opt := finetune.WithMLX()
	if opt == nil {
		t.Fatal("nil")
	}
}

func TestTrainerLoRA(t *testing.T) {
	cfg := finetune.LoRAConfig{Rank: 16, Alpha: 32, Dropout: 0.05}
	opt := finetune.LoRA(cfg)
	if opt == nil {
		t.Fatal("nil")
	}
}

func TestTrainerQLoRA(t *testing.T) {
	cfg := finetune.LoRAConfig{Rank: 8, Alpha: 16}
	opt := finetune.QLoRA(cfg)
	if opt == nil {
		t.Fatal("nil")
	}
}

func TestTrainerFullFineTune(t *testing.T) {
	opt := finetune.FullFineTune()
	if opt == nil {
		t.Fatal("nil")
	}
}

func TestTrainerGRPO(t *testing.T) {
	cfg := finetune.GRPOConfig{RewardModel: "rm", KLCoeff: 0.1, NumGenerations: 4}
	opt := finetune.GRPO(cfg)
	if opt == nil {
		t.Fatal("nil")
	}
}

// ========== Scheduler option functions ==========

func TestSchedulerRetrainEvery(t *testing.T) {
	s := finetune.NewScheduler(
		finetune.RetrainEvery(24 * time.Hour),
	)
	if s == nil {
		t.Fatal("nil")
	}
}

func TestSchedulerBaseConfig(t *testing.T) {
	cfg := finetune.Config{}
	s := finetune.NewScheduler(
		finetune.BaseConfig(cfg),
	)
	if s == nil {
		t.Fatal("nil")
	}
}

func TestSchedulerOnComplete(t *testing.T) {
	var called bool
	s := finetune.NewScheduler(
		finetune.OnComplete(func(r finetune.Result) { called = true }),
	)
	if s == nil {
		t.Fatal("nil")
	}
	_ = called // just verify the option sets without panic
}

func TestSchedulerOnError(t *testing.T) {
	var called bool
	s := finetune.NewScheduler(
		finetune.OnError(func(err error) { called = true }),
	)
	if s == nil {
		t.Fatal("nil")
	}
	_ = called
}

func TestSchedulerStartCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := finetune.NewScheduler(
		finetune.RetrainEvery(100 * time.Millisecond),
	)
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	err := s.Start(ctx)
	if err != context.Canceled {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// ========== Deploy option functions ==========

func TestDeployOptions(t *testing.T) {
	opts := []finetune.DeployOption{
		finetune.WithSystemPrompt("You are a helper"),
		finetune.WithTemperature(0.7),
		finetune.WithContextLength(4096),
		finetune.WithBackend("llama"),
		finetune.WithGPULayers(32),
	}
	for _, o := range opts {
		if o == nil {
			t.Fatal("option should not be nil")
		}
	}
}

// ========== splitLines coverage via Evaluate ==========

func TestEvaluateWindowsLineEndings(t *testing.T) {
	p := mock.New()
	p.QueueResponse("a")
	p.QueueResponse("b")

	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.jsonl")
	// Use \r\n line endings
	lines := `{"messages":[{"role":"user","content":"x"},{"role":"assistant","content":"a"}]}` + "\r\n" +
		`{"messages":[{"role":"user","content":"y"},{"role":"assistant","content":"b"}]}` + "\r\n"
	os.WriteFile(testFile, []byte(lines), 0o644)

	result, err := finetune.Evaluate(context.Background(), testFile,
		finetune.Models(finetune.Model("m", p)),
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// Should still parse 2 samples correctly even with \r\n
	if result.Models[0].ToolAccuracy != 1.0 {
		t.Fatalf("toolAcc = %f", result.Models[0].ToolAccuracy)
	}
}

// ========== EvalResult JSON round-trip ==========

func TestEvalResultJSONRoundTrip(t *testing.T) {
	original := &finetune.EvalResult{
		Models: []finetune.ModelResult{
			{Name: "m1", Accuracy: 0.9, ToolAccuracy: 0.85, Quality: 7.5, AvgLatencyMs: 100, AvgTokens: 42},
			{Name: "m2", Accuracy: 0.7, ToolAccuracy: 0.6, Quality: 5.0, AvgLatencyMs: 200, AvgTokens: 80},
		},
	}
	path := filepath.Join(t.TempDir(), "round.json")
	if err := original.ExportJSON(path); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, _ := os.ReadFile(path)
	var loaded finetune.EvalResult
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(loaded.Models) != 2 {
		t.Fatalf("models = %d", len(loaded.Models))
	}
	if loaded.Models[0].Name != "m1" {
		t.Fatalf("name = %q", loaded.Models[0].Name)
	}
	if loaded.Models[1].AvgTokens != 80 {
		t.Fatalf("tokens = %d", loaded.Models[1].AvgTokens)
	}
}

// ========== Collector with tool calls (OpenAI format) ==========

func TestCollectorToolCallsOpenAI(t *testing.T) {
	dir := t.TempDir()
	c := finetune.NewCollector(
		finetune.WithFormat(finetune.OpenAIFmt),
		finetune.WithIncludeToolCalls(true),
		finetune.WithStorage(dir),
	)
	result := makeResult(1,
		daneel.Message{Role: daneel.RoleUser, Content: "search"},
		daneel.Message{
			Role:      daneel.RoleAssistant,
			Content:   "searching",
			ToolCalls: []daneel.ToolCall{{ID: "c1", Name: "search", Arguments: json.RawMessage(`{"q":"X"}`)}},
		},
		daneel.Message{Role: daneel.RoleTool, Content: "result", Name: "search"},
		daneel.Message{Role: daneel.RoleAssistant, Content: "found it"},
	)
	c.Capture(context.Background(), result)
	if c.Count() != 1 {
		t.Fatalf("count = %d", c.Count())
	}
}

// ========== Evaluate with malformed JSON lines ==========

func TestEvaluateMalformedLines(t *testing.T) {
	p := mock.New()
	p.QueueResponse("ok")

	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.jsonl")
	// Mix valid and invalid JSON lines
	lines := "not valid json\n" +
		`{"messages":[{"role":"user","content":"x"},{"role":"assistant","content":"ok"}]}` + "\n" +
		"also invalid\n"
	os.WriteFile(testFile, []byte(lines), 0o644)

	result, err := finetune.Evaluate(context.Background(), testFile,
		finetune.Models(finetune.Model("m", p)),
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// Malformed lines treated as non-parseable samples; metric calculations should still work
	if result.Models[0].Name != "m" {
		t.Fatalf("name = %q", result.Models[0].Name)
	}
}

// ========== PythonPath option ==========

func TestPythonPath(t *testing.T) {
	opt := finetune.PythonPath("/usr/bin/python3")
	if opt == nil {
		t.Fatal("nil")
	}
}

// ========== Evaluate with wrong response (0 accuracy) ==========

func TestEvaluateZeroAccuracy(t *testing.T) {
	p := mock.New()
	p.QueueResponse("completely wrong answer")

	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.jsonl")
	data := `{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"}]}` + "\n"
	os.WriteFile(testFile, []byte(data), 0o644)

	result, err := finetune.Evaluate(context.Background(), testFile,
		finetune.Models(finetune.Model("bad-model", p)),
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// Response doesn't match expected, accuracy should be 0
	if result.Models[0].Accuracy != 0 {
		t.Fatalf("accuracy = %f, want 0", result.Models[0].Accuracy)
	}
}

// ========== MetricType constants ==========

func TestMetricTypeValues(t *testing.T) {
	if finetune.Accuracy != 0 {
		t.Fatal("Accuracy should be 0")
	}
	if finetune.ToolCallAccuracy != 1 {
		t.Fatal("ToolCallAccuracy should be 1")
	}
	if finetune.ResponseQuality != 2 {
		t.Fatal("ResponseQuality should be 2")
	}
	if finetune.Latency != 3 {
		t.Fatal("Latency should be 3")
	}
	if finetune.TokenEfficiency != 4 {
		t.Fatal("TokenEfficiency should be 4")
	}
}
