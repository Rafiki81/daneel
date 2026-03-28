package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// check represents a single diagnostic result.
type check struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "ok", "warn", "fail"
	Message string `json:"message,omitempty"`
}

// cmdDoctor runs environment diagnostics and prints a human-readable report.
// Pass "--json" as the first argument to get machine-readable output.
func cmdDoctor(args []string) {
	jsonOutput := len(args) > 0 && args[0] == "--json"

	checks := []check{
		checkOllama(),
		checkOpenAI(),
		checkOllamaModels(),
		checkGoVersion(),
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(checks)
		return
	}

	anyFail := false
	for _, c := range checks {
		icon := "✓"
		switch c.Status {
		case "warn":
			icon = "⚠"
		case "fail":
			icon = "✗"
			anyFail = true
		}
		if c.Message != "" {
			fmt.Printf("  %s  %-30s  %s\n", icon, c.Name, c.Message)
		} else {
			fmt.Printf("  %s  %s\n", icon, c.Name)
		}
	}

	if anyFail {
		fmt.Fprintln(os.Stderr, "\ndoctor: one or more checks failed")
		os.Exit(1)
	}
}

func checkOllama() check {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get("http://localhost:11434/api/version")
	if err != nil {
		return check{
			Name:    "Ollama reachable",
			Status:  "warn",
			Message: "not running on localhost:11434 (start with: ollama serve)",
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return check{
			Name:    "Ollama reachable",
			Status:  "fail",
			Message: fmt.Sprintf("unexpected status %d", resp.StatusCode),
		}
	}
	return check{Name: "Ollama reachable", Status: "ok"}
}

func checkOpenAI() check {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		return check{
			Name:    "OPENAI_API_KEY",
			Status:  "warn",
			Message: "not set (ok if using local Ollama)",
		}
	}
	if !strings.HasPrefix(key, "sk-") {
		return check{
			Name:    "OPENAI_API_KEY",
			Status:  "warn",
			Message: "set but does not look like an OpenAI key (sk-...)",
		}
	}
	return check{Name: "OPENAI_API_KEY", Status: "ok", Message: "set"}
}

func checkOllamaModels() check {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil {
		return check{
			Name:    "Ollama models",
			Status:  "warn",
			Message: "cannot reach /api/tags",
		}
	}
	defer resp.Body.Close()

	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return check{Name: "Ollama models", Status: "warn", Message: "could not parse model list"}
	}
	if len(payload.Models) == 0 {
		return check{
			Name:    "Ollama models",
			Status:  "warn",
			Message: "no models pulled yet (run: ollama pull llama3.2)",
		}
	}
	names := make([]string, 0, len(payload.Models))
	for _, m := range payload.Models {
		names = append(names, m.Name)
	}
	return check{
		Name:    "Ollama models",
		Status:  "ok",
		Message: strings.Join(names, ", "),
	}
}

func checkGoVersion() check {
	out, err := exec.Command("go", "version").Output()
	if err != nil {
		return check{Name: "Go toolchain", Status: "fail", Message: "go not found in PATH"}
	}
	return check{Name: "Go toolchain", Status: "ok", Message: strings.TrimSpace(string(out))}
}
