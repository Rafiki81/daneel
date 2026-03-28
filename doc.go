// Package daneel is a Go library for building AI agents and multi-agent
// systems using only the standard library.
//
// # Overview
//
// Daneel provides primitives for composing LLM-powered agents that can use
// tools, persist memory, hand off between each other, run as long-lived
// services, and be governed by fine-grained permission policies — all without
// external dependencies.
//
// # Quick Start
//
//	tool := daneel.NewTool("get_weather", "Get weather for a city",
//	    func(ctx context.Context, p WeatherParams) (string, error) {
//	        return "28°C, sunny", nil
//	    },
//	)
//
//	agent := daneel.New("assistant",
//	    daneel.WithInstructions("You are a helpful weather assistant."),
//	    daneel.WithModel("gpt-4o"),
//	    daneel.WithTools(tool),
//	)
//
//	result, err := daneel.Run(ctx, agent, "What's the weather in Madrid?")
//
// # Key Concepts
//
//   - [Agent]: the central type. Created with [New] and configured with options.
//   - [Tool]: typed function callable by the LLM. Created with [NewTool].
//   - [Run]: the package-level function that executes an agent turn.
//   - [RunResult]: returned by [Run], contains the output, tool call history, and usage.
//   - [Connector]: interface for integrating external message platforms (Slack, Telegram …).
//   - [Registry]: global registry of agents and tools for introspection and the CLI.
//
// # Packages
//
// The daneel module ships with batteries included:
//
//   - [github.com/Rafiki81/daneel/memory] — conversation memory (sliding window, summary).
//   - [github.com/Rafiki81/daneel/workflow] — multi-agent workflow primitives (chain, parallel, router, orchestrator, FSM).
//   - [github.com/Rafiki81/daneel/provider] — LLM provider adapters (OpenAI, Anthropic, Google, Ollama).
//   - [github.com/Rafiki81/daneel/platform] — platform clients (GitHub, Slack, Twitter, Telegram, WhatsApp, Email).
//   - [github.com/Rafiki81/daneel/connector] — message platform connectors backed by platform clients.
//   - [github.com/Rafiki81/daneel/mcp] — Model Context Protocol client and server.
//   - [github.com/Rafiki81/daneel/session] — persistent session store (in-memory, file).
//   - [github.com/Rafiki81/daneel/knowledge] — document ingestion, chunking, and retrieval.
//   - [github.com/Rafiki81/daneel/cron] — cron-style scheduled agent runs.
//   - [github.com/Rafiki81/daneel/ws] — WebSocket server and connector (stdlib only).
//   - [github.com/Rafiki81/daneel/experiment] — A/B testing and LLM-as-judge evaluation.
//   - [github.com/Rafiki81/daneel/pubsub] — publish/subscribe bus with agent tools.
//   - [github.com/Rafiki81/daneel/tenant] — multi-tenant quota and scoped session management.
//   - [github.com/Rafiki81/daneel/billing] — cost tracking, pricing tables, and budget alerts.
//   - [github.com/Rafiki81/daneel/finetune] — fine-tuning dataset collection and evaluation.
//   - [github.com/Rafiki81/daneel/trace] — OpenTelemetry tracing and metrics.
//   - [github.com/Rafiki81/daneel/approval] — human-in-the-loop approval for tool calls.
//   - [github.com/Rafiki81/daneel/bridge] — bridge between two agents.
//   - [github.com/Rafiki81/daneel/cmd/daneel] — CLI for running and managing agents.
package daneel
