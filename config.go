package daneel

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

var envVarRe = regexp.MustCompile(`\$\{([^}]+)\}`)

// Config is the top-level agent configuration that can be loaded from a JSON
// file.  Environment variable references of the form ${VAR_NAME} in string
// values are expanded automatically.
//
//	cfg, err := daneel.LoadConfig("daneel.json")
//	agents, err := cfg.BuildAgents(tools, nil)
type Config struct {
	Provider  ProviderConfig            `json:"provider"`
	Platforms map[string]PlatformConfig `json:"platforms,omitempty"`
	Agents    []AgentSpec               `json:"agents"`
}

// ProviderConfig describes the LLM provider to use.
type ProviderConfig struct {
	Type    string `json:"type"`              // openai, anthropic, google, ollama
	APIKey  string `json:"api_key,omitempty"` // may use ${ENV_VAR}
	Model   string `json:"model,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
}

// PlatformConfig holds raw key/value settings for a platform integration.
// Values may reference environment variables using the ${VAR_NAME} syntax
// which is expanded by LoadConfig.
//
//	token := cfg.BuildPlatforms()["slack"].Get("bot_token")
type PlatformConfig struct {
	Settings map[string]string
}

func (p *PlatformConfig) UnmarshalJSON(b []byte) error {
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	p.Settings = m
	return nil
}

func (p PlatformConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.Settings)
}

// Get returns a platform setting value by key.
func (p PlatformConfig) Get(key string) string { return p.Settings[key] }

// AgentSpec describes an agent declared in a config file.
type AgentSpec struct {
	Name         string      `json:"name"`
	Instructions string      `json:"instructions,omitempty"`
	Model        string      `json:"model,omitempty"`
	Tools        []string    `json:"tools,omitempty"`
	AllowTools   []string    `json:"allow_tools,omitempty"`
	DenyTools    []string    `json:"deny_tools,omitempty"`
	MaxTurns     int         `json:"max_turns,omitempty"`
	Memory       *MemorySpec `json:"memory,omitempty"`
}

// MemorySpec describes which memory backend to use for an agent.
type MemorySpec struct {
	Type string `json:"type"` // "sliding", "summary", "vector"
	Size int    `json:"size,omitempty"`
}

// MemoryFactory creates a Memory implementation from a MemorySpec.
// Providing one allows BuildAgents to configure memory without creating a
// circular import dependency between daneel and the memory package.
//
//	cfg.BuildAgents(tools, func(spec daneel.MemorySpec) daneel.Memory {
//	    switch spec.Type {
//	    case "sliding":
//	        return memory.Sliding(spec.Size)
//	    default:
//	        return nil
//	    }
//	})
type MemoryFactory func(spec MemorySpec) Memory

// LoadConfig reads and parses a JSON configuration file.
// Environment variable references of the form ${VAR_NAME} in any string value
// are expanded using os.Getenv before the JSON is decoded.
func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("daneel: config: read %q: %w", path, err)
	}

	// First pass: decode into generic value so we can expand env vars
	// inside string values without risking JSON structural corruption.
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, fmt.Errorf("daneel: config: parse %q: %w", path, err)
	}
	generic = expandEnvInValue(generic)

	expanded, err := json.Marshal(generic)
	if err != nil {
		return nil, fmt.Errorf("daneel: config: re-marshal: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(expanded, &cfg); err != nil {
		return nil, fmt.Errorf("daneel: config: decode: %w", err)
	}
	return &cfg, nil
}

// expandEnvInValue recursively replaces ${VAR} patterns in string leaves.
func expandEnvInValue(v any) any {
	switch vt := v.(type) {
	case string:
		return envVarRe.ReplaceAllStringFunc(vt, func(m string) string {
			return os.Getenv(m[2 : len(m)-1]) // strip ${ and }
		})
	case map[string]any:
		for k, val := range vt {
			vt[k] = expandEnvInValue(val)
		}
		return vt
	case []any:
		for i, val := range vt {
			vt[i] = expandEnvInValue(val)
		}
		return vt
	}
	return v
}

// BuildplanPlatforms returns the raw platform configurations by platform name.
// Use the returned PlatformConfig values to initialize platform clients:
//
//	platforms := cfg.BuildPlatforms()
//	tools := slack.Tools(platforms["slack"].Get("bot_token"))
func (c *Config) BuildPlatforms() map[string]PlatformConfig {
	result := make(map[string]PlatformConfig, len(c.Platforms))
	for k, v := range c.Platforms {
		result[k] = v
	}
	return result
}

// BuildAgents creates Agent instances from the config.
//
// tools is a map of "tool_name" → Tool for tools referenced by name in agent
// specs.  Entries for unknown tool names are silently ignored.
//
// memFactory is called for every agent that declares a memory spec; pass nil
// to skip memory setup.
func (c *Config) BuildAgents(tools map[string]Tool, memFactory MemoryFactory) ([]*Agent, error) {
	agents := make([]*Agent, 0, len(c.Agents))

	for _, spec := range c.Agents {
		if spec.Name == "" {
			return nil, fmt.Errorf("daneel: config: agent with empty name")
		}

		opts := make([]AgentOption, 0)

		if spec.Instructions != "" {
			opts = append(opts, WithInstructions(spec.Instructions))
		}

		// Resolve model — per-agent overrides global provider model.
		model := spec.Model
		if model == "" {
			model = c.Provider.Model
		}

		// Resolve base URL and api key.
		baseURL := c.Provider.BaseURL
		apiKey := c.Provider.APIKey
		switch c.Provider.Type {
		case "ollama":
			if baseURL == "" {
				baseURL = "http://localhost:11434/v1"
			}
		default: // openai and compatible
			if baseURL == "" {
				baseURL = "https://api.openai.com/v1"
			}
		}

		// Capture loop-local copies for the closure.
		m, u, k := model, baseURL, apiKey
		opts = append(opts, func(ac *agentConfig) {
			ac.model = m
			ac.provider = &miniClient{baseURL: u, apiKey: k, model: m}
		})

		if spec.MaxTurns > 0 {
			opts = append(opts, WithMaxTurns(spec.MaxTurns))
		}

		if len(spec.AllowTools) > 0 {
			opts = append(opts, WithPermissions(AllowTools(spec.AllowTools...)))
		}
		if len(spec.DenyTools) > 0 {
			opts = append(opts, WithPermissions(DenyTools(spec.DenyTools...)))
		}

		var agentTools []Tool
		for _, name := range spec.Tools {
			if t, ok := tools[name]; ok {
				agentTools = append(agentTools, t)
			}
		}
		if len(agentTools) > 0 {
			opts = append(opts, WithTools(agentTools...))
		}

		if spec.Memory != nil && memFactory != nil {
			if mem := memFactory(*spec.Memory); mem != nil {
				opts = append(opts, WithMemory(mem))
			}
		}

		agents = append(agents, New(spec.Name, opts...))
	}

	return agents, nil
}
