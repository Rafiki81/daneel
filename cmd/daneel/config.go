// Package main is the daneel CLI tool.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// cliConfig is the structure of the JSON config file (~/.daneel.json or ./daneel.json).
type cliConfig struct {
	DefaultModel    string            `json:"default_model"`
	DefaultProvider string            `json:"default_provider"`
	MaxTurns        int               `json:"max_turns"`
	Env             map[string]string `json:"env"` // additional env overrides
}

func loadConfig(path string) (*cliConfig, error) {
	if path == "" {
		// Try common locations
		for _, candidate := range []string{"./daneel.json", os.ExpandEnv("$HOME/.daneel.json")} {
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
				break
			}
		}
	}
	if path == "" {
		return &cliConfig{}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %q: %w", path, err)
	}
	// Expand ${ENV_VAR} references in the raw JSON
	expanded := os.ExpandEnv(string(data))

	var cfg cliConfig
	if err := json.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %q: %w", path, err)
	}
	// Apply env overrides
	for k, v := range cfg.Env {
		if !strings.Contains(v, "${") {
			_ = os.Setenv(k, v)
		}
	}
	return &cfg, nil
}
