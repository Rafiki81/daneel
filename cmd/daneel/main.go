// Command daneel is the CLI for the Daneel AI agent framework.
//
// Usage:
//
//	daneel agents list
//	daneel agents describe <name>
//	daneel tools list
//	daneel tools describe <name>
//	daneel run <agent> [prompt] [--model m] [--max-turns n] [--session id]
//	daneel listen [--slack] [--twitter] [--telegram] [--whatsapp] [--email]
//	daneel finetune --dataset <file> --base <model>
//	daneel doctor [--json]
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Rafiki81/daneel"
)

var version = "1.0.0"

func main() {
	configFile := flag.String("config", "", "path to JSON config file (default: ./daneel.json or ~/.daneel.json)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Printf("daneel %s\n", version)
		return
	}

	cfg, err := loadConfig(*configFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}

	// Registry is built by the user's daneel.json through their own main.
	// The CLI ships with an empty registry for introspection of registered agents.
	reg := daneel.NewRegistry()

	if flag.NArg() == 0 {
		usage()
		os.Exit(1)
	}

	switch flag.Arg(0) {
	case "agents":
		cmdAgents(flag.Args()[1:], reg)
	case "tools":
		cmdTools(flag.Args()[1:], reg)
	case "run":
		cmdRun(flag.Args()[1:], reg, cfg)
	case "listen":
		cmdListen(flag.Args()[1:])
	case "finetune":
		cmdFinetune(flag.Args()[1:])
	case "doctor":
		cmdDoctor(flag.Args()[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", flag.Arg(0))
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `daneel - AI agent framework CLI (v%s)

Usage:
  daneel [--config file] <command> [arguments]

Commands:
  agents list                    List registered agents
  agents describe <name>         Show agent details
  tools list                     List all available tools
  tools describe <name>          Show tool schema
  run <agent> [prompt]           Run an agent (interactive REPL if no prompt)
  listen [--slack] [--twitter]   Start connectors and listen for messages
  finetune --dataset --base      Fine-tune a model on collected conversations
  doctor [--json]                Check environment (Ollama, API keys, Go)

Flags:
`, version)
	flag.PrintDefaults()
}
