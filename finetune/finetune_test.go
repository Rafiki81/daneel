package finetune_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daneel-ai/daneel"
	"github.com/daneel-ai/daneel/finetune"
)

// helper to build a RunResult with messages
func makeResult(turns int, msgs ...daneel.Message) daneel.RunResult {
	return daneel.RunResult{
		Turns:    turns,
		Messages: msgs,
	}
}

// ---------- Collector ----------

func TestCollectorDefaults(t *testing.T) {
	c := finetune.NewCollector()
	if c.Count() != 0 {
		t.Fatalf("count = %d, want 0", c.Count())
	}
}

func TestCollectorCaptureShareGPT(t *testing.T) {
	dir := t.TempDir()
	c := finetune.NewCollector(
		finetune.WithFormat(finetune.ShareGPT),
		finetune.WithStorage(dir),
	)
	result := makeResult(2,
		daneel.Message{Role: daneel.RoleSystem, Content: "You are helpful"},
		daneel.Message{Role: daneel.RoleUser, Content: "Hello"},
		daneel.Message{Role: daneel.RoleAssistant, Content: "Hi!"},
	)
	c.Capture(context.Background(), result)
	if c.Count() != 1 {
		t.Fatalf("count = %d, want 1", c.Count())
	}
	// Check file was created
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("files = %d, want 1", len(entries))
	}
	// Validate JSON structure
	data, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	var conv struct {
		Conversations []struct {
			From  string `json:"from"`
			Value string `json:"value"`
		} `json:"conversations"`
	}
	if err := json.Unmarshal(data, &conv); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(conv.Conversations) != 3 {
		t.Fatalf("conversations = %d, want 3", len(conv.Conversations))
	}
	if conv.Conversations[0].From != "system" {
		t.Fatalf("from[0] = %q", conv.Conversations[0].From)
	}
	if conv.Conversations[1].From != "human" {
		t.Fatalf("from[1] = %q", conv.Conversations[1].From)
	}
	if conv.Conversations[2].From != "gpt" {
		t.Fatalf("from[2] = %q", conv.Conversations[2].From)
	}
}

func TestCollectorCaptureAlpaca(t *testing.T) {
	dir := t.TempDir()
	c := finetune.NewCollector(
		finetune.WithFormat(finetune.Alpaca),
		finetune.WithStorage(dir),
	)
	result := makeResult(1,
		daneel.Message{Role: daneel.RoleSystem, Content: "Translate"},
		daneel.Message{Role: daneel.RoleUser, Content: "Hello"},
		daneel.Message{Role: daneel.RoleAssistant, Content: "Hola"},
	)
	c.Capture(context.Background(), result)
	entries, _ := os.ReadDir(dir)
	data, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	var sample struct {
		Instruction string `json:"instruction"`
		Input       string `json:"input"`
		Output      string `json:"output"`
	}
	json.Unmarshal(data, &sample)
	if sample.Instruction != "Translate" {
		t.Fatalf("instruction = %q", sample.Instruction)
	}
	if sample.Input != "Hello" {
		t.Fatalf("input = %q", sample.Input)
	}
	if sample.Output != "Hola" {
		t.Fatalf("output = %q", sample.Output)
	}
}

func TestCollectorCaptureOpenAI(t *testing.T) {
	dir := t.TempDir()
	c := finetune.NewCollector(
		finetune.WithFormat(finetune.OpenAIFmt),
		finetune.WithStorage(dir),
	)
	result := makeResult(1,
		daneel.Message{Role: daneel.RoleUser, Content: "Hi"},
		daneel.Message{Role: daneel.RoleAssistant, Content: "Hello"},
	)
	c.Capture(context.Background(), result)
	entries, _ := os.ReadDir(dir)
	data, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	var sample struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	json.Unmarshal(data, &sample)
	if len(sample.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(sample.Messages))
	}
	if sample.Messages[0].Role != "user" {
		t.Fatalf("role[0] = %q", sample.Messages[0].Role)
	}
}

func TestCollectorCaptureChatML(t *testing.T) {
	dir := t.TempDir()
	c := finetune.NewCollector(
		finetune.WithFormat(finetune.ChatML),
		finetune.WithStorage(dir),
	)
	result := makeResult(1,
		daneel.Message{Role: daneel.RoleUser, Content: "Hi"},
		daneel.Message{Role: daneel.RoleAssistant, Content: "Hello"},
	)
	c.Capture(context.Background(), result)
	entries, _ := os.ReadDir(dir)
	data, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	// ChatML is a string, so it is JSON-encoded string
	var chatml string
	json.Unmarshal(data, &chatml)
	if !strings.Contains(chatml, "<|im_start|>user") {
		t.Fatalf("missing user tag in chatML: %q", chatml)
	}
	if !strings.Contains(chatml, "<|im_end|>") {
		t.Fatalf("missing im_end tag in chatML: %q", chatml)
	}
}

func TestCollectorMinTurnsFilter(t *testing.T) {
	dir := t.TempDir()
	c := finetune.NewCollector(
		finetune.WithMinTurns(3),
		finetune.WithStorage(dir),
	)
	// This result has Turns=2, below minTurns=3 -> should be skipped
	result := makeResult(2,
		daneel.Message{Role: daneel.RoleUser, Content: "x"},
		daneel.Message{Role: daneel.RoleAssistant, Content: "y"},
	)
	c.Capture(context.Background(), result)
	if c.Count() != 0 {
		t.Fatalf("count = %d, should skip below minTurns", c.Count())
	}
	// This result has Turns=3 -> should be captured
	result2 := makeResult(3,
		daneel.Message{Role: daneel.RoleUser, Content: "a"},
		daneel.Message{Role: daneel.RoleAssistant, Content: "b"},
		daneel.Message{Role: daneel.RoleUser, Content: "c"},
	)
	c.Capture(context.Background(), result2)
	if c.Count() != 1 {
		t.Fatalf("count = %d, want 1", c.Count())
	}
}

func TestCollectorToolCalls(t *testing.T) {
	dir := t.TempDir()
	c := finetune.NewCollector(
		finetune.WithIncludeToolCalls(true),
		finetune.WithStorage(dir),
	)
	result := makeResult(1,
		daneel.Message{Role: daneel.RoleUser, Content: "search for X"},
		daneel.Message{
			Role:      daneel.RoleAssistant,
			Content:   "searching",
			ToolCalls: []daneel.ToolCall{{ID: "c1", Name: "search", Arguments: json.RawMessage(`{"q":"X"}`)}},
		},
		daneel.Message{Role: daneel.RoleTool, Content: "found it", Name: "search"},
	)
	c.Capture(context.Background(), result)
	entries, _ := os.ReadDir(dir)
	data, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if !strings.Contains(string(data), "tool_calls") {
		t.Fatal("should include tool calls")
	}
	if !strings.Contains(string(data), "found it") {
		t.Fatal("should include tool response")
	}
}

// ---------- Dataset ----------

func TestDatasetLoadAndLen(t *testing.T) {
	dir := t.TempDir()
	// Create some JSON files
	for i, content := range []string{
		`{"messages":[{"role":"user","content":"a"},{"role":"assistant","content":"b"}]}`,
		`{"messages":[{"role":"user","content":"c"}]}`,
		`{"messages":[{"role":"user","content":"d"},{"role":"assistant","content":"e"},{"role":"user","content":"f"}]}`,
	} {
		os.WriteFile(filepath.Join(dir, strings.Replace("conv_X.json", "X", string(rune('0'+i)), 1)), []byte(content), 0o644)
	}
	// Also create a non-json file that should be ignored
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0o644)

	ds, err := finetune.LoadDataset(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if ds.Len() != 3 {
		t.Fatalf("len = %d, want 3", ds.Len())
	}
}

func TestDatasetFilterMinTurns(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"messages":[{"role":"user","content":"a"}]}`), 0o644)
	os.WriteFile(filepath.Join(dir, "b.json"), []byte(`{"messages":[{"role":"user","content":"a"},{"role":"assistant","content":"b"}]}`), 0o644)
	ds, _ := finetune.LoadDataset(dir)
	filtered := ds.Filter(finetune.MinTurns(2))
	if filtered.Len() != 1 {
		t.Fatalf("filtered len = %d, want 1", filtered.Len())
	}
}

func TestDatasetFilterMaxTurns(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"messages":[{"role":"user","content":"a"}]}`), 0o644)
	os.WriteFile(filepath.Join(dir, "b.json"), []byte(`{"messages":[{"role":"user","content":"a"},{"role":"assistant","content":"b"},{"role":"user","content":"c"}]}`), 0o644)
	ds, _ := finetune.LoadDataset(dir)
	filtered := ds.Filter(finetune.MaxTurns(1))
	if filtered.Len() != 1 {
		t.Fatalf("filtered len = %d, want 1", filtered.Len())
	}
}

func TestDatasetFilterNoErrors(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"messages":[{"role":"user","content":"ok"}]}`), 0o644)
	os.WriteFile(filepath.Join(dir, "b.json"), []byte(`{"messages":[{"role":"user","content":"Error: bad"}]}`), 0o644)
	ds, _ := finetune.LoadDataset(dir)
	filtered := ds.Filter(finetune.NoErrors())
	if filtered.Len() != 1 {
		t.Fatalf("filtered len = %d, want 1", filtered.Len())
	}
}

func TestDatasetFilterContainsTool(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"messages":[{"role":"tool","content":"search result"}]}`), 0o644)
	os.WriteFile(filepath.Join(dir, "b.json"), []byte(`{"messages":[{"role":"user","content":"hello"}]}`), 0o644)
	ds, _ := finetune.LoadDataset(dir)
	filtered := ds.Filter(finetune.ContainsTool("search"))
	if filtered.Len() != 1 {
		t.Fatalf("filtered len = %d, want 1", filtered.Len())
	}
}

func TestDatasetFilterComposed(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"messages":[{"role":"user","content":"a"},{"role":"assistant","content":"b"}]}`), 0o644)
	os.WriteFile(filepath.Join(dir, "b.json"), []byte(`{"messages":[{"role":"user","content":"x"}]}`), 0o644)
	os.WriteFile(filepath.Join(dir, "c.json"), []byte(`{"messages":[{"role":"user","content":"Error: bad"},{"role":"assistant","content":"oops"}]}`), 0o644)
	ds, _ := finetune.LoadDataset(dir)
	filtered := ds.Filter(finetune.MinTurns(2), finetune.NoErrors())
	if filtered.Len() != 1 {
		t.Fatalf("filtered len = %d, want 1 (only a.json)", filtered.Len())
	}
}

func TestDatasetSplit(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		os.WriteFile(filepath.Join(dir, strings.Replace("s_X.json", "X", string(rune('a'+i)), 1)), []byte(`{"messages":[]}`), 0o644)
	}
	ds, _ := finetune.LoadDataset(dir)
	train, test := ds.Split(0.8)
	if train.Len() != 8 {
		t.Fatalf("train = %d, want 8", train.Len())
	}
	if test.Len() != 2 {
		t.Fatalf("test = %d, want 2", test.Len())
	}
}

func TestDatasetExport(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"messages":[{"role":"user","content":"a"}]}`), 0o644)
	os.WriteFile(filepath.Join(dir, "b.json"), []byte(`{"messages":[{"role":"user","content":"b"}]}`), 0o644)
	ds, _ := finetune.LoadDataset(dir)

	exportPath := filepath.Join(t.TempDir(), "output.jsonl")
	if err := ds.Export(exportPath); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, _ := os.ReadFile(exportPath)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("exported lines = %d, want 2", len(lines))
	}
	// Each line should be valid JSON
	for i, line := range lines {
		var v any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatalf("line %d not valid JSON: %v", i, err)
		}
	}
}

func TestDatasetSamples(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"x":1}`), 0o644)
	ds, _ := finetune.LoadDataset(dir)
	samples := ds.Samples()
	if len(samples) != 1 {
		t.Fatalf("samples = %d", len(samples))
	}
	if string(samples[0]) != `{"x":1}` {
		t.Fatalf("sample = %q", string(samples[0]))
	}
}

func TestLoadDatasetNonExistent(t *testing.T) {
	_, err := finetune.LoadDataset("/nonexistent/path")
	if err == nil {
		t.Fatal("should error on missing dir")
	}
}

// ---------- EvalResult export ----------

func TestEvalResultExportJSON(t *testing.T) {
	r := &finetune.EvalResult{
		Models: []finetune.ModelResult{
			{Name: "gpt4", Accuracy: 0.95, Quality: 8.5},
		},
	}
	path := filepath.Join(t.TempDir(), "eval.json")
	if err := r.ExportJSON(path); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "gpt4") {
		t.Fatal("should contain model name")
	}
}

func TestEvalResultExportMarkdown(t *testing.T) {
	r := &finetune.EvalResult{
		Models: []finetune.ModelResult{
			{Name: "llama", Accuracy: 0.80, ToolAccuracy: 0.75, Quality: 7.0, AvgLatencyMs: 120, AvgTokens: 50},
		},
	}
	path := filepath.Join(t.TempDir(), "eval.md")
	if err := r.ExportMarkdown(path); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, _ := os.ReadFile(path)
	md := string(data)
	if !strings.Contains(md, "llama") {
		t.Fatal("should contain model name")
	}
	if !strings.Contains(md, "80.0%") {
		t.Fatal("should contain accuracy percentage")
	}
	if !strings.Contains(md, "|") {
		t.Fatal("should be a markdown table")
	}
}

// ---------- Trainer Config ----------

func TestTrainConfigDefaults(t *testing.T) {
	// Test that option functions work and do not panic
	opts := []finetune.TrainOption{
		finetune.BaseModel("llama-3"),
		finetune.Epochs(5),
		finetune.BatchSize(8),
		finetune.LearningRate(1e-5),
		finetune.ExportGGUF("q4_k_m"),
		finetune.OutputDir("/tmp/out"),
		finetune.VenvPath("/tmp/venv"),
	}
	// Just verify they do not panic when applied
	for _, o := range opts {
		if o == nil {
			t.Fatal("option should not be nil")
		}
	}
}

func TestSchedulerDefaults(t *testing.T) {
	s := finetune.NewScheduler()
	if s == nil {
		t.Fatal("scheduler should not be nil")
	}
}

func TestSchedulerWithCollector(t *testing.T) {
	c := finetune.NewCollector()
	s := finetune.NewScheduler(
		finetune.CollectFrom(c),
		finetune.RetrainAfter(100),
	)
	if s == nil {
		t.Fatal("scheduler should not be nil")
	}
}
