# Daneel

[![Go Reference](https://pkg.go.dev/badge/github.com/daneel-ai/daneel.svg)](https://pkg.go.dev/github.com/daneel-ai/daneel)
[![Go Report Card](https://goreportcard.com/badge/github.com/daneel-ai/daneel)](https://goreportcard.com/report/github.com/daneel-ai/daneel)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Build](https://github.com/daneel-ai/daneel/actions/workflows/ci.yml/badge.svg)](https://github.com/daneel-ai/daneel/actions)

> *"The Laws of Robotics are not suggestions." — R. Daneel Olivaw*

**Daneel** is a Go library for building production-grade AI agents and multi-agent systems.  
Zero external dependencies — just the Go standard library.

---

## Features

| Category | Capabilities |
|---|---|
| **Core** | Typed tool definitions, permission policies, handoff between agents, session management |
| **Providers** | OpenAI, Anthropic, Google Gemini, Ollama |
| **Memory** | Sliding-window, summarisation, vector store, file-backed persistence |
| **Multi-agent** | Chain, parallel, router, orchestrator, finite-state-machine workflows |
| **Platforms** | Slack, Telegram, Twitter/X, WhatsApp, GitHub, Email |
| **Protocols** | Model Context Protocol (MCP) client & server, WebSocket connector |
| **Observability** | OpenTelemetry tracing & metrics, fine-tune dataset collection |
| **Operations** | Multi-tenant quotas, billing / cost tracking, budget alerts, cron scheduling |
| **Human-in-loop** | Tool-call approval, A/B testing, LLM-as-judge evaluation |
| **CLI** | `daneel` binary — run agents, listen on platforms, trigger fine-tuning |

---

## Installation

```sh
go get github.com/daneel-ai/daneel
```

Requires **Go 1.24+**.

---

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"
    "strings"

    "github.com/daneel-ai/daneel"
)

type WeatherParams struct {
    City string `json:"city" desc:"City name"`
}

func main() {
    // 1. Define a typed tool
    weatherTool := daneel.NewTool("get_weather", "Get current weather for a city",
        func(ctx context.Context, p WeatherParams) (string, error) {
            temps := map[string]string{"madrid": "28°C ☀️", "tokyo": "22°C ⛅"}
            city := strings.ToLower(p.City)
            if t, ok := temps[city]; ok {
                return t, nil
            }
            return "20°C 🌡️", nil
        },
    )

    // 2. Create an agent
    agent := daneel.New("assistant",
        daneel.WithInstructions("You are a helpful weather assistant."),
        daneel.WithModel("gpt-4o"),
        daneel.WithTools(weatherTool),
        daneel.WithMaxTurns(5),
    )

    // 3. Run it
    result, err := daneel.Run(context.Background(), agent, "Weather in Madrid?")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(result.Output)
}
```

---

## Examples

| Example | Description |
|---|---|
| [`examples/quickstart`](examples/quickstart/) | Single agent with a tool and interactive REPL |
| [`examples/multi-platform`](examples/multi-platform/) | Same agent running on Slack + Telegram simultaneously |
| [`examples/github-reviewer`](examples/github-reviewer/) | PR review bot using the GitHub platform |
| [`examples/slack-assistant`](examples/slack-assistant/) | Slack bot with memory and file upload support |
| [`examples/twitter-bot`](examples/twitter-bot/) | Automated Twitter/X agent |
| [`examples/permissions`](examples/permissions/) | Fine-grained tool permission policies |

---

## Package Index

### Core
| Package | Description |
|---|---|
| `github.com/daneel-ai/daneel` | Agent, Tool, Run, RunResult, Registry, Connector |
| `github.com/daneel-ai/daneel/content` | Multi-modal content types (text, image, file) |
| `github.com/daneel-ai/daneel/approval` | Human-in-the-loop approval for tool calls |
| `github.com/daneel-ai/daneel/bridge` | Point-to-point agent bridging |

### Providers
| Package | Description |
|---|---|
| `github.com/daneel-ai/daneel/provider/openai` | OpenAI (GPT-4o, o1, …) |
| `github.com/daneel-ai/daneel/provider/anthropic` | Anthropic (Claude 3.x, …) |
| `github.com/daneel-ai/daneel/provider/google` | Google Gemini |
| `github.com/daneel-ai/daneel/provider/ollama` | Ollama (local models) |

### Memory
| Package | Description |
|---|---|
| `github.com/daneel-ai/daneel/memory` | Sliding-window + summary memory |
| `github.com/daneel-ai/daneel/memory/store` | In-memory vector store |
| `github.com/daneel-ai/daneel/session` | Persistent session store (memory, file) |

### Workflows
| Package | Description |
|---|---|
| `github.com/daneel-ai/daneel/workflow` | Chain, parallel, router, orchestrator, FSM |

### Platforms & Connectors
| Package | Description |
|---|---|
| `github.com/daneel-ai/daneel/platform/slack` | Slack client & tools |
| `github.com/daneel-ai/daneel/platform/telegram` | Telegram client & tools |
| `github.com/daneel-ai/daneel/platform/twitter` | Twitter/X client & tools |
| `github.com/daneel-ai/daneel/platform/whatsapp` | WhatsApp client & tools |
| `github.com/daneel-ai/daneel/platform/github` | GitHub client & tools |
| `github.com/daneel-ai/daneel/platform/email` | Email client & tools |
| `github.com/daneel-ai/daneel/connector/*` | High-level connectors for each platform |
| `github.com/daneel-ai/daneel/ws` | WebSocket server and connector (stdlib only) |

### Protocols
| Package | Description |
|---|---|
| `github.com/daneel-ai/daneel/mcp` | Model Context Protocol client & server |

### Knowledge & Scheduling
| Package | Description |
|---|---|
| `github.com/daneel-ai/daneel/knowledge` | Document ingestion, chunking, RAG retrieval |
| `github.com/daneel-ai/daneel/cron` | Cron-style scheduled agent runs |

### Experimentation
| Package | Description |
|---|---|
| `github.com/daneel-ai/daneel/experiment` | A/B testing, LLM-as-judge, metrics |
| `github.com/daneel-ai/daneel/finetune` | Fine-tune dataset collection & evaluation |

### Operations
| Package | Description |
|---|---|
| `github.com/daneel-ai/daneel/tenant` | Multi-tenant quota management & scoped sessions |
| `github.com/daneel-ai/daneel/billing` | Cost tracking, pricing tables, budget alerts |
| `github.com/daneel-ai/daneel/pubsub` | Publish/subscribe message bus with agent tools |
| `github.com/daneel-ai/daneel/trace` | OpenTelemetry tracing and metrics |

### CLI
| Package | Description |
|---|---|
| `github.com/daneel-ai/daneel/cmd/daneel` | CLI — `agents`, `tools`, `run`, `listen`, `finetune` |

---

## CLI

Install the `daneel` binary:

```sh
go install github.com/daneel-ai/daneel/cmd/daneel@latest
```

```
daneel agents list              # list registered agents
daneel agents describe <name>   # show agent details
daneel tools list               # list registered tools
daneel run <agent> "prompt"     # run an agent once
daneel run <agent>              # interactive REPL
daneel listen --slack           # start Slack listener
daneel finetune --dataset data.jsonl --base gpt-4o
```

---

## Multi-Agent Workflow Example

```go
import "github.com/daneel-ai/daneel/workflow"

result, err := workflow.Chain(ctx, input,
    researchAgent,
    writerAgent,
    editorAgent,
)
```

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). All contributions welcome.

---

## License

[MIT](LICENSE) © 2026 The Daneel Authors
