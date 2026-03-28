package daneel_test

import (
	"context"
	"os"
	"testing"

	"github.com/Rafiki81/daneel"
	"github.com/Rafiki81/daneel/provider/mock"
)

// ---- LoadConfig ----------------------------------------------------------------

func TestLoadConfig_Parses(t *testing.T) {
	cfg, err := daneel.LoadConfig("testdata/config.json")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(cfg.Agents))
	}
	if cfg.Agents[0].Name != "support" {
		t.Errorf("agent[0].Name = %q, want support", cfg.Agents[0].Name)
	}
	if cfg.Agents[0].MaxTurns != 15 {
		t.Errorf("agent[0].MaxTurns = %d, want 15", cfg.Agents[0].MaxTurns)
	}
	if cfg.Provider.Type != "openai" {
		t.Errorf("provider.Type = %q, want openai", cfg.Provider.Type)
	}
}

func TestLoadConfig_ExpandsEnvVars(t *testing.T) {
	os.Setenv("TEST_OPENAI_KEY", "sk-test-key-123")
	defer os.Unsetenv("TEST_OPENAI_KEY")

	cfg, err := daneel.LoadConfig("testdata/config.json")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Provider.APIKey != "sk-test-key-123" {
		t.Errorf("api_key = %q, want sk-test-key-123", cfg.Provider.APIKey)
	}
}

func TestLoadConfig_PlatformConfig(t *testing.T) {
	os.Setenv("TEST_SLACK_TOKEN", "xoxb-test")
	defer os.Unsetenv("TEST_SLACK_TOKEN")

	cfg, err := daneel.LoadConfig("testdata/config.json")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	plats := cfg.BuildPlatforms()
	if plats["slack"].Get("bot_token") != "xoxb-test" {
		t.Errorf("slack.bot_token = %q, want xoxb-test", plats["slack"].Get("bot_token"))
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := daneel.LoadConfig("testdata/does_not_exist.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// ---- BuildAgents ---------------------------------------------------------------

func TestBuildAgents_CreatesAgents(t *testing.T) {
	cfg, err := daneel.LoadConfig("testdata/config.json")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	agents, err := cfg.BuildAgents(nil, nil)
	if err != nil {
		t.Fatalf("BuildAgents: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}
	if agents[0].Name() != "support" {
		t.Errorf("agents[0].Name = %q, want support", agents[0].Name())
	}
	if agents[1].Name() != "coder" {
		t.Errorf("agents[1].Name = %q, want coder", agents[1].Name())
	}
}

func TestBuildAgents_WithTool(t *testing.T) {
	cfg, err := daneel.LoadConfig("testdata/config.json")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	// Provide a mock for the "github.comment" tool referenced in the coder agent.
	commentTool := daneel.NewTool("github.comment", "Post a comment",
		func(ctx context.Context, p struct {
			Body string `json:"body"`
		}) (string, error) {
			return "commented", nil
		},
	)
	tools := map[string]daneel.Tool{"github.comment": commentTool}
	agents, err := cfg.BuildAgents(tools, nil)
	if err != nil {
		t.Fatalf("BuildAgents: %v", err)
	}
	// Replace provider on the coder agent (index 1) so we can run it.
	coder := agents[1]
	p := mock.New(mock.Respond("looks good"))
	coder = daneel.New(coder.Name(), daneel.WithProvider(p), daneel.WithInstructions("write code"))
	result, err := daneel.Run(context.Background(), coder, "review this")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Output != "looks good" {
		t.Errorf("output = %q, want 'looks good'", result.Output)
	}
}

func TestBuildAgents_MemoryFactory(t *testing.T) {
	cfg := &daneel.Config{
		Provider: daneel.ProviderConfig{Type: "openai", Model: "gpt-4o"},
		Agents: []daneel.AgentSpec{
			{
				Name:   "mem-agent",
				Memory: &daneel.MemorySpec{Type: "custom", Size: 5},
			},
		},
	}
	called := false
	_, err := cfg.BuildAgents(nil, func(spec daneel.MemorySpec) daneel.Memory {
		called = true
		if spec.Type != "custom" || spec.Size != 5 {
			t.Errorf("got spec %+v, want {custom 5}", spec)
		}
		return nil // skip actual memory setup
	})
	if err != nil {
		t.Fatalf("BuildAgents: %v", err)
	}
	if !called {
		t.Error("MemoryFactory was not called")
	}
}

// ---- RunStructured validation --------------------------------------------------

func TestRunStructured_ValidResponse(t *testing.T) {
	type Sentiment struct {
		Label string  `json:"label" enum:"positive,negative,neutral"`
		Score float64 `json:"score"`
	}
	p := mock.New(mock.Respond(`{"label":"positive","score":0.9}`))
	agent := daneel.New("a", daneel.WithProvider(p))

	result, err := daneel.RunStructured[Sentiment](context.Background(), agent, "awesome!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Data.Label != "positive" {
		t.Errorf("label = %q, want positive", result.Data.Label)
	}
}

func TestRunStructured_EnumViolation_Retries(t *testing.T) {
	type Sentiment struct {
		Label string  `json:"label" enum:"positive,negative,neutral"`
		Score float64 `json:"score"`
	}
	// First response has invalid enum. Second response is corrected.
	p := mock.New(
		mock.Respond(`{"label":"POSITIVE","score":0.9}`), // invalid enum value
		mock.Respond(`{"label":"positive","score":0.9}`), // fixed
	)
	agent := daneel.New("a", daneel.WithProvider(p))

	result, err := daneel.RunStructured[Sentiment](context.Background(), agent, "awesome!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Data.Label != "positive" {
		t.Errorf("label after retry = %q, want positive", result.Data.Label)
	}
	if p.CallCount() != 2 {
		t.Errorf("expected 2 provider calls (initial + retry), got %d", p.CallCount())
	}
}

func TestRunStructured_MissingRequired_Retries(t *testing.T) {
	type Info struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	p := mock.New(
		mock.Respond(`{"name":"Alice"}`),               // missing required "email"
		mock.Respond(`{"name":"Alice","email":"a@b"}`), // fixed
	)
	agent := daneel.New("a", daneel.WithProvider(p))

	result, err := daneel.RunStructured[Info](context.Background(), agent, "extract info")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Data.Email != "a@b" {
		t.Errorf("email after retry = %q, want a@b", result.Data.Email)
	}
}
