package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Rafiki81/daneel"
)

func cmdRun(args []string, reg *daneel.Registry, cfg *cliConfig) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	model := fs.String("model", cfg.DefaultModel, "LLM model override")
	maxTurns := fs.Int("max-turns", cfg.MaxTurns, "maximum agent turns (0 = default)")
	sessionID := fs.String("session", "", "session ID for multi-turn conversations")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: daneel run <agent> [prompt] [flags]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}

	agentName := fs.Arg(0)
	info := reg.FindAgent(agentName)
	if info == nil {
		fmt.Fprintf(os.Stderr, "agent %q not found\n", agentName)
		os.Exit(1)
	}

	// Build run options
	var runOpts []daneel.RunOption
	if *maxTurns > 0 {
		runOpts = append(runOpts, daneel.WithRunMaxTurns(*maxTurns))
	}
	if *sessionID != "" {
		runOpts = append(runOpts, daneel.WithSessionID(*sessionID))
	}
	_ = model // model is applied when loading the agent via config; noted here for future extension

	ctx := context.Background()

	// If a prompt is given on the command line, run once and exit
	if fs.NArg() > 1 {
		prompt := strings.Join(fs.Args()[1:], " ")
		runOnce(ctx, info, prompt, runOpts)
		return
	}

	// Interactive REPL
	runREPL(ctx, info, runOpts)
}

func runOnce(ctx context.Context, info *daneel.AgentInfo, prompt string, opts []daneel.RunOption) {
	// Since we only have AgentInfo (no *Agent), we demonstrate the introspection.
	// In a real setup the config file would instantiate the agent.
	fmt.Printf("[%s] %s\n", info.Name, prompt)
	fmt.Println("(configure providers in daneel.json to run agents interactively)")
}

func runREPL(ctx context.Context, info *daneel.AgentInfo, opts []daneel.RunOption) {
	fmt.Printf("daneel › %s (type 'exit' to quit)\n", info.Name)
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			break
		}
		fmt.Printf("[%s] <- %s\n", info.Name, line)
		fmt.Println("(configure providers in daneel.json to execute runs)")
	}
}
