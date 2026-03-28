# Daneel — Agent Orchestration Library for Go

> *Named after R. Daneel Olivaw, the robot who silently orchestrated humanity for 20,000 years in Asimov's Foundation/Robot saga.*

## Why Daneel

Python tiene LangChain, CrewAI, OpenAI Agents SDK. Go no tiene un equivalente idiomático.

Las opciones actuales en Go son ports incompletos de librerías Python (`langchaingo`), wrappers mínimos de una sola API (`openai-agents-go`), o proyectos experimentales con poca tracción. Ninguno ofrece una solución completa e idiomática para orquestar agentes LLM en Go.

**Daneel llena ese vacío.** Es la librería base que Go necesita: primitivas simples, composables, con cero dependencias en el core, que siguen los patrones del lenguaje (`context.Context`, interfaces pequeñas, functional options, channels). No es un port de Python — está diseñada desde cero para Go.

El objetivo es que cualquier desarrollador Go que quiera construir agentes LLM use Daneel como base, igual que usa `net/http` para servidores web o `database/sql` para bases de datos.

## Philosophy

- **The missing `agent/` package for Go**: Primitivas fundamentales para orquestación de agentes, diseñadas como si fueran parte del stdlib.
- **Few primitives, maximum composability**: 5 conceptos core — `Agent`, `Tool`, `Permission`, `Runner`, `Workflow`. Todo lo demás se compone a partir de estos.
- **3 lines to start, config file to scale**: API key y listo.
- **Idiomatic Go, not ported Python**: `context.Context` para cancelación, channels para concurrencia, interfaces para extensibilidad, functional options para configuración. Sin clases, sin decoradores, sin magia.
- **Permissions as first-class citizens**: allow/deny lists, guardrails, approval workflows.
- **Platforms included**: Twitter, WhatsApp, GitHub, Slack, Email, Telegram — tools + bidirectional connectors.
- **Library, not framework**: No stdout, no `os.Exit`, no stdin. Pure API. A future CLI wraps this.
- **No magic**: No code generation, no unnecessary reflection, no DSLs. Just Go.
- **Minimal dependencies**: stdlib first. `net/http` for REST APIs, `encoding/json` for serialization, `net/smtp` for email. External deps only for complex protocols (WhatsApp WebSocket, IMAP).
- **Accept interfaces, return structs**: Every extension point is an interface. Every concrete type is swappable.
- **Small interfaces**: 1-2 methods per interface (Go proverb). `Provider` has one method. `Memory` has three. `Tool` is a struct, not an interface — see "Tool Struct" section.
- **No global state**: No `init()`, no package-level vars. Everything is explicit and passed as arguments.

## Requirements

- **Go 1.24+** (latest stable; ensures all stdlib enhancements used by Daneel are available)
- **Python 3.10+** (only for `finetune/` package — auto-managed venv)
- No CGO required. Pure Go. Cross-compiles to any platform.
- **Core has zero external dependencies**. Only stdlib.

## Extensibility Model

Every major component is an interface that users can implement:

```go
// Swap LLM providers
type Provider interface {
    Chat(ctx context.Context, messages []Message, tools []ToolDef) (*Response, error)
}

// Swap memory backends
type Memory interface {
    Save(ctx context.Context, sessionID string, msgs []Message) error
    Retrieve(ctx context.Context, sessionID string, query string, limit int) ([]Message, error)
    Clear(ctx context.Context, sessionID string) error
}

// Swap vector stores
type VectorStore interface {
    Store(ctx context.Context, id string, embedding []float32, metadata map[string]string) error
    Search(ctx context.Context, query []float32, topK int) ([]VectorResult, error)
    Delete(ctx context.Context, ids ...string) error
}

// Swap platform connectors
type Connector interface {
    Start(ctx context.Context) error
    Send(ctx context.Context, to string, content string) error
    Messages() <-chan IncomingMessage
    Stop() error
}

// Swap tracing backends
type Tracer interface {
    StartSpan(ctx context.Context, name string, attrs ...Attr) (context.Context, Span)
}

type Span interface {
    SetAttributes(attrs ...Attr)
    RecordError(err error)
    End()
}

// Approval request — passed to Approver when a tool requires human approval
type ApprovalRequest struct {
    Agent     string          // agent requesting approval
    Tool      string          // tool it wants to call
    Args      json.RawMessage // arguments for the tool call
    SessionID string          // conversation session
}

// Swap approval backends
type Approver interface {
    Approve(ctx context.Context, req ApprovalRequest) (bool, error)
}

// ApproverFunc adapts a function to the Approver interface (like http.HandlerFunc).
type ApproverFunc func(ctx context.Context, req ApprovalRequest) (bool, error)
func (f ApproverFunc) Approve(ctx context.Context, req ApprovalRequest) (bool, error) { return f(ctx, req) }

// Swap embedding providers
type Embedder interface {
    Embed(ctx context.Context, text string) ([]float32, error)
}
```

Daneel ships with default implementations for each interface. Users only implement what they need to customize.

The pattern is always the same: **accept the interface in function signatures, return concrete structs**. This makes testing trivial (mock any interface) and makes the library extensible without modifying core code.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                       User Code                         │
├─────────────────────────────────────────────────────────┤
│  daneel.New() / daneel.Quick()                          │
│  ┌──────────┐ ┌───────┐ ┌────────────┐ ┌────────┐      │
│  │  Agent   │ │ Tool  │ │ Permission │ │ Guard  │      │
│  └────┬─────┘ └───┬───┘ └─────┬──────┘ └───┬────┘      │
│       └────────────┼──────────┼─────────────┘           │
│                    ▼                                    │
│             ┌───────────┐                               │
│             │  Runner   │  ← agent loop                 │
│             └─────┬─────┘                               │
│       ┌───────────┼───────────┐                         │
│       ▼           ▼           ▼                         │
│  ┌─────────┐ ┌──────────┐ ┌──────────┐                 │
│  │Provider │ │ Platform │ │ Workflow │                  │
│  │ (LLM)   │ │ (Tools)  │ │          │                  │
│  └─────────┘ └──────────┘ └──────────┘                 │
│       │           │                                     │
│       ▼           ▼                                     │
│  ┌─────────┐ ┌───────────┐ ┌────────┐ ┌─────────────┐  │
│  │ OpenAI  │ │ Twitter   │ │ Memory │ │   Trace     │  │
│  │Anthropic│ │ GitHub    │ │  RAG   │ │   OTEL      │  │
│  │ Google  │ │ Slack     │ │        │ │             │  │
│  │ Ollama  │ │ WhatsApp  │ └────────┘ └─────────────┘  │
│  └─────────┘ │ Email     │                              │
│              │ Telegram  │                              │
│              └─────┬─────┘                              │
│                    ▼                                    │
│             ┌────────────┐                              │
│             │ Connector  │  ← bidirectional             │
│             │  + Bridge  │                              │
│             └────────────┘                              │
├─────────────────────────────────────────────────────────┤
│  finetune/ │ mcp/ │ approval/ │ content/                │
└─────────────────────────────────────────────────────────┘
```

## Package Structure

```
daneel/
├── agent.go              # Agent struct + functional options
├── tool.go               # Tool struct + NewTool[T] with generics
├── permission.go         # AllowTools, DenyTools, AllowHandoffs, MaxTurns
├── runner.go             # Agent loop: LLM → tool calls → results → repeat
├── message.go            # Message, ToolCall, ToolResult
├── result.go             # RunResult with final output + history
├── guard.go              # InputGuard, OutputGuard
├── handoff.go            # Handoff as special tool
├── config.go             # Config struct + LoadConfig (JSON + env vars)
├── registry.go           # Agent/Tool/Platform introspection for future CLI
├── errors.go             # ErrPermissionDenied, ErrMaxTurns, ErrGuardFailed
├── options.go            # RunOptions: MaxTurns, OnToolCall, streaming
├── session.go            # Session ID generation + management
├── context_mgmt.go       # Context window truncation strategies
├── structured.go         # RunStructured[T] + StructuredResult[T]
├── daneel.go             # Package-level convenience: Run(), Quick(), MergeTools()
│
├── provider/             # LLM Providers (interfaces defined in root daneel package)
│   ├── provider.go       # Fallback, RoundRobin, CostRouter, RetryConfig, Pricing
│   ├── openai/           # OpenAI + any compatible (Groq, Together, LocalAI, etc.)
│   ├── anthropic/        # Anthropic Messages API
│   ├── google/           # Gemini
│   └── ollama/           # Via OpenAI-compat or native
│
├── platform/             # Tool Packs (actions on platforms)
│   ├── twitter/          # Tweet, Reply, Search, Follow, Like, GetMentions
│   ├── whatsapp/         # SendMessage, SendMedia, GetMessages
│   ├── github/           # CreateIssue, CreatePR, Review, Comment, MergePR
│   ├── slack/            # SendMessage, ReadChannel, React, CreateChannel
│   ├── email/            # Send, Reply, Read, Search
│   └── telegram/         # SendMessage, SendPhoto, GetUpdates
│
├── connector/            # Bidirectional connectors (receive + send)
│   ├── connector.go      # Listen() helpers, common connector utilities
│   ├── twitter/          # Polling mentions/DMs
│   ├── whatsapp/         # WebSocket via whatsmeow
│   ├── github/           # Webhooks HTTP or polling
│   ├── slack/            # Socket Mode (no public URL needed)
│   ├── email/            # IMAP IDLE
│   └── telegram/         # Long-polling (default) or webhooks
│
├── bridge/               # Connects Connector → Agent automatically
│   └── bridge.go         # Bridge + Multi (multiple connectors, one agent)
│
├── workflow/             # Composition patterns
│   ├── chain.go          # Sequential: A → B → C
│   ├── parallel.go       # Parallel with errgroup
│   ├── router.go         # Triage → specialized agent
│   └── orchestrator.go   # Boss decomposes → workers execute
│
├── memory/               # Conversation memory + RAG (Memory interface defined in root daneel package)
│   ├── memory.go         # Composite constructor, shared helpers
│   ├── sliding.go        # Last N messages
│   ├── summary.go        # Periodic LLM summarization
│   ├── vector.go         # RAG with vector store
│   └── store/
│       ├── memory.go     # Built-in cosine similarity (pure Go, ~50 LOC)
│       ├── qdrant.go     # Qdrant adapter (optional dep)
│       └── weaviate.go   # Weaviate adapter (optional dep)
│
├── finetune/             # Local LLM fine-tuning pipeline
│   ├── collector.go      # Capture conversations → JSONL/ShareGPT/Alpaca
│   ├── dataset.go        # Dataset filtering, quality scoring, augmentation
│   ├── format.go         # Export formats: ShareGPT, Alpaca, OpenAI, chatml
│   ├── trainer.go        # Launch Unsloth/TRL as subprocess (LoRA, QLoRA, full, GRPO)
│   ├── evaluator.go      # Compare new vs old model on test cases
│   ├── ollama.go         # Import GGUF to Ollama, create Modelfile
│   ├── localai.go        # Import to LocalAI (alternative to Ollama)
│   └── scheduler.go      # Auto-retrain when enough new data is collected
│
├── mcp/                  # Model Context Protocol
│   ├── client.go         # Consume MCP servers → Daneel tools
│   └── server.go         # Expose Daneel tools as MCP server
│
├── trace/                # Observability
│   ├── otel.go           # OpenTelemetry spans + metrics
│   └── logger.go         # Structured logging with slog
│
├── approval/             # Human-in-the-loop
│   ├── approval.go       # Func() convenience adapter (ApprovalRequest lives in root daneel package)
│   └── policy.go         # AutoApprove, AlwaysDeny, custom policies
│
├── content/              # Multi-modal content
│   └── content.go        # Text, Image, Audio, File
│
└── examples/
    ├── quickstart/       # 10 lines, one agent
    ├── twitter-bot/      # Bot that replies to mentions
    ├── github-reviewer/  # PR review agent
    ├── slack-assistant/  # Slack support bot
    ├── multi-platform/   # One agent, multiple platforms
    └── permissions/      # Restricted tools + guardrails
```

---

## Internal Dependency Graph

Packages are layered to avoid circular dependencies. Higher layers depend on lower ones, never the reverse.

```
Layer 0 (zero deps):     message.go, errors.go, content/, session.go
Layer 1 (core types):    tool.go, permission.go, guard.go, options.go, context_mgmt.go
Layer 2 (agent):         agent.go, handoff.go, result.go
Layer 3 (runtime):       runner.go, structured.go, config.go, registry.go, daneel.go
Layer 4 (providers):     provider/ (openai, anthropic, google, ollama)
Layer 5 (memory):        memory/ (sliding, summary, vector, store/)
Layer 6 (platforms):     platform/ (twitter, github, slack, ...)
Layer 7 (connectivity):  connector/, bridge/
Layer 8 (advanced):      workflow/, mcp/, trace/, approval/
Layer 9 (training):      finetune/
```

The core `daneel` package (layers 0-3) has **zero external dependencies**. Each subpackage only pulls its own deps when imported.

## Thread Safety

| Type | Thread-safe? | Notes |
|---|---|---|
| `Agent` | ✅ Yes | Immutable after creation. Safe to share across goroutines. `.With*()` methods return a new copy (copy-on-modify). |
| `Tool` | ✅ Yes | Struct is immutable after creation. `NewTool[T]` wraps user func as-is — user is responsible for their function's safety. |
| `Runner` | ✅ Yes | Each `Run()` call creates its own state. Multiple concurrent `Run()` calls are safe. |
| `Connector` | ✅ Yes | `Messages()` returns a channel; multiple consumers need coordination. |
| `Bridge` | ✅ Yes | Handles concurrency internally via goroutines + semaphore. |
| `Memory` | ⚠️ Impl-dependent | `Sliding` uses mutex. `Vector` delegates to store (Qdrant/chromem are safe). |
| `Registry` | ✅ Yes | Read-heavy, uses `sync.RWMutex`. |
| `Collector` (finetune) | ✅ Yes | Writes are mutex-protected. Safe to use from multiple agents. |

---

## Core Primitives

### Agent

Immutable struct created via functional options. Holds everything an agent needs to run.

```go
agent := daneel.New("support",
    daneel.WithInstructions("You handle customer support for Acme Corp"),
    daneel.WithModel("gpt-4o"),  // convenience: creates OpenAI provider with this model
    daneel.WithTools(searchTool, replyTool),
    daneel.WithHandoffs(escalationAgent),
    daneel.WithPermissions(
        daneel.AllowTools("search", "reply"),
        daneel.DenyTools("delete", "ban"),
    ),
    daneel.WithMaxTurns(15),
    daneel.WithInputGuard(validateInput),
    daneel.WithOutputGuard(validateOutput),
    daneel.WithMemory(memory.Sliding(20)),
)
```

**Convenience provider shortcuts** (used with `New()` and `Quick()`):

```go
daneel.WithOpenAI(apiKey)            // reads OPENAI_API_KEY from env if apiKey is ""
daneel.WithModel("gpt-4o")          // uses OpenAI-compat client, reads OPENAI_API_KEY from env
daneel.WithOllama("llama3.3:70b")   // base URL defaults to http://localhost:11434/v1
daneel.WithLocalAI("my-model")      // base URL defaults to http://localhost:8080/v1
```

These functions live in `daneel.go` and use a **built-in minimal OpenAI-compatible HTTP client** (~80 LOC, no import of `provider/` subpackages — avoids circular imports). Since OpenAI, Ollama, and LocalAI all speak the same `/v1/chat/completions` format, one client handles all three — only the base URL and auth differ.

For advanced features (retry, streaming, rate limiting, provider fallback), use `WithProvider()` with the full provider packages:

```go
daneel.WithProvider(openai.New(openai.WithAPIKey(key), openai.WithModel("gpt-4o")))   // full-featured
daneel.WithProvider(ollama.New(ollama.WithModel("llama3.3:70b")))                      // full-featured
daneel.WithProvider(openai.New(openai.WithBaseURL("http://localhost:8080/v1")))         // LocalAI via openai-compat
```

If `WithProvider(p)` is used explicitly, it takes precedence over convenience shortcuts.

### Tool (with generics — auto JSON Schema from struct)

Tools use Go generics to auto-generate JSON Schema from parameter structs. No manual schema writing.

```go
type SearchParams struct {
    Query      string `json:"query"       desc:"Search query"`
    MaxResults int    `json:"max_results" desc:"Max results to return"`
}

searchTool := daneel.NewTool("search", "Search the knowledge base",
    func(ctx context.Context, p SearchParams) (string, error) {
        results, err := kb.Search(p.Query, p.MaxResults)
        if err != nil {
            return "", err
        }
        return formatResults(results), nil
    },
)

// Tools can also return Content for multi-modal output:
screenshotTool := daneel.NewToolWithContent("screenshot", "Take a screenshot",
    func(ctx context.Context, p ScreenshotParams) (content.Content, error) {
        img, err := takeScreenshot(p.Region)
        return content.ImageContent(img, "image/png"), err
    },
)

// Or return structured data (auto-serialized to JSON for the LLM):
type AnalysisResult struct {
    Score   float64  `json:"score"`
    Issues  []string `json:"issues"`
}

analyzeTool := daneel.NewToolTyped[AnalyzeParams, AnalysisResult]("analyze", "Analyze code",
    func(ctx context.Context, p AnalyzeParams) (AnalysisResult, error) {
        return AnalysisResult{Score: 8.5, Issues: []string{"missing tests"}}, nil
    },
)

// Tool constructor variants (all accept optional ...ToolOption as last arg):
// - NewTool[T](name, desc, fn, ...ToolOption)           — returns text (most common)
// - NewToolWithContent[T](name, desc, fn, ...ToolOption) — returns multi-modal content
// - NewToolTyped[In, Out](name, desc, fn, ...ToolOption) — returns typed struct (auto-JSON)
// ToolOption examples: WithToolTimeout(d)
```

### Tool Struct (internal)

`Tool` is a concrete struct, not an interface. The generic constructors (`NewTool[T]`, etc.) produce a `Tool` by type-erasing the parameter struct at creation time via a closure. Reflection runs once (at creation), not per call.

```go
type Tool struct {
    Name           string                                                   // unique identifier
    Description    string                                                   // description for the LLM
    Schema         json.RawMessage                                          // auto-generated JSON Schema
    fn             func(ctx context.Context, args json.RawMessage) (string, error) // type-erased handler
    timeout        time.Duration                                            // per-tool timeout (0 = use default)
    returnsContent bool                                                     // true for NewToolWithContent
}

// Run executes the tool with raw JSON arguments. Called by the Runner.
func (t Tool) Run(ctx context.Context, args json.RawMessage) (string, error)

// Def returns the ToolDef sent to the provider (name, description, schema only).
func (t Tool) Def() ToolDef

// How NewTool[T] works internally:
// 1. Reflect on T to generate JSON Schema (once, at creation)
// 2. Capture T in a closure that json.Unmarshal's args into T, calls user func
// 3. Store closure as t.fn — no generics at runtime, just func(ctx, []byte)(string, error)
```

### Utility: MergeTools

When combining tool packs from multiple platforms, use `MergeTools` to flatten them into a single slice:

```go
// MergeTools concatenates multiple []Tool slices into one.
func MergeTools(toolSets ...[]Tool) []Tool

// Usage:
daneel.WithTools(daneel.MergeTools(
    twitter.Tools(twitterToken),
    slack.Tools(slackToken),
    github.Tools(githubToken),
)...)
```

### Tool Timeout

Individual tools can have execution timeouts to prevent hanging:

```go
// Per-tool timeout
slowTool := daneel.NewTool[Params]("slow-api", "Calls a slow external API", myFunc,
    daneel.WithToolTimeout(30 * time.Second),
)

// Global default timeout for all tools (in the Runner)
agent := daneel.New("assistant",
    daneel.WithDefaultToolTimeout(15 * time.Second), // tools without explicit timeout
)
```

When a tool exceeds its timeout, the Runner returns a `ToolResult` with `IsError: true` and the message `"tool execution timed out after 30s"`. The LLM sees this and can decide to retry or inform the user.

### Permission

Declarative allow/deny lists evaluated by the Runner before each tool call.

```go
daneel.WithPermissions(
    daneel.AllowTools("read", "search"),       // whitelist (only these)
    daneel.DenyTools("exec", "delete"),        // blacklist (never these)
    daneel.AllowHandoffs("coder", "researcher"), // which agents can be invoked
)
daneel.WithMaxTurns(10) // max iterations of the agent loop
```

Resolution order:
1. Check tool is NOT in DenyTools
2. If AllowTools defined, check tool IS in AllowTools
3. If denied → inject error message to LLM ("you don't have permission to use this tool"). The loop **continues** — the LLM can try another tool or respond to the user. This is the default behavior.
4. To abort on first denial instead, use `daneel.WithStrictPermissions()` — returns `ErrPermissionDenied` immediately.

Wildcard matching: patterns ending with `*` use prefix matching (`strings.HasPrefix`). Exact match otherwise. No regex.

```go
daneel.AllowTools("mcp.github.*")  // matches mcp.github.create_issue, mcp.github.comment, etc.
daneel.DenyTools("github.merge_pr") // exact match only
```

### Provider

Minimal interface — one function. Makes adding new providers trivial.

```go
type Provider interface {
    Chat(ctx context.Context, messages []Message, tools []ToolDef) (*Response, error)
}

type Response struct {
    Content   string
    ToolCalls []ToolCall
    Usage     Usage // token counts
}

type Usage struct {
    PromptTokens     int
    CompletionTokens int
    TotalTokens      int
}

// ToolDef is the subset of Tool sent to the provider.
// The Runner extracts ToolDef from each Tool via tool.Def().
type ToolDef struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    Schema      json.RawMessage `json:"parameters"` // JSON Schema for parameters
}
```

### Streaming

Stream tokens as they arrive from the LLM. The Runner supports streaming via callback:

```go
result, err := daneel.Run(ctx, agent, "Explain quantum computing",
    daneel.WithStreaming(func(chunk StreamChunk) {
        switch chunk.Type {
        case StreamText:
            fmt.Print(chunk.Text) // print as tokens arrive
        case StreamToolCallStart:
            fmt.Printf("\n[Calling %s...]\n", chunk.ToolCall.Name)
        case StreamToolCallDone:
            fmt.Printf("[Done: %s]\n", chunk.ToolResult.Content)
        case StreamDone:
            fmt.Println("\n--- Complete ---")
        }
    }),
)
```

Not all providers support streaming natively:

| Provider | Streaming | Notes |
|---|---|---|
| OpenAI | ✅ Native | SSE stream |
| Anthropic | ✅ Native | SSE stream |
| Google | ✅ Native | SSE stream |
| Ollama | ✅ Native | NDJSON stream |
| Non-streaming provider | ⚠️ Simulated | Emits entire response as single chunk |

Providers that support streaming implement an optional interface:

```go
type StreamProvider interface {
    Provider
    ChatStream(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamChunk, error)
}
```

### Structured Output

Force the LLM to return a typed Go struct instead of free text. Uses OpenAI's `response_format`, Anthropic's `tool_use` trick, or schema-constrained generation.

```go
type SentimentAnalysis struct {
    Sentiment  string  `json:"sentiment"  desc:"positive, negative, or neutral" enum:"positive,negative,neutral"`
    Confidence float64 `json:"confidence" desc:"Confidence score 0-1"`
    Reasoning  string  `json:"reasoning"  desc:"Brief explanation"`
}

result, err := daneel.RunStructured[SentimentAnalysis](ctx, agent, "Review: The product is amazing!")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("%s (%.0f%%): %s\n", result.Data.Sentiment, result.Data.Confidence*100, result.Data.Reasoning)
// positive (95%): The user expresses strong enthusiasm with "amazing"
```

How it works internally:
1. `RunStructured[T]` generates a JSON Schema from `T` (same reflection as `NewTool[T]`)
2. Sends the schema to the provider:
   - **OpenAI**: Uses `response_format: { type: "json_schema", json_schema: ... }`
   - **Anthropic**: Uses a synthetic tool with `tool_choice: { type: "tool", name: "output" }` + extracts result
   - **Google**: Uses `response_mime_type: "application/json"` + `response_schema`
   - **Ollama**: Uses `format: "json"` + schema in prompt
3. Parses the JSON response into `T` with validation
4. Returns `StructuredResult[T]` which embeds `RunResult` + the parsed `Data T`

```go
type StructuredResult[T any] struct {
    RunResult         // embeds the full run result
    Data      T       // the parsed structured output
    Raw       string  // the raw JSON string before parsing
}
```

Providers that don't support native structured output fall back to a strong system prompt with the JSON Schema + retry on parse failure (up to 2 retries).

Also works with `Run()` for optional structured hints:

```go
// Ask the LLM to respond in a specific format, but still returns string
result, err := daneel.Run(ctx, agent, "Analyze this",
    daneel.WithResponseFormat(daneel.JSON),      // just force JSON
    daneel.WithResponseSchema(MyStruct{}), // force schema (uses reflect internally)
)
```

### RunResult

The complete result of an agent execution:

```go
type RunResult struct {
    Output      string           // final text response
    Messages    []Message        // full conversation history
    ToolCalls   []ToolCallRecord // all tool calls made (name, args, result, duration)
    Turns       int              // number of agent loop iterations
    Usage       Usage            // total token usage across all LLM calls
    Duration    time.Duration    // total wall-clock time
    HandoffFrom string           // if this was a handoff, which agent started it
    AgentName   string           // which agent produced this result
    SessionID   string           // conversation session identifier
}

type ToolCallRecord struct {
    Name      string
    Arguments json.RawMessage
    Result    string
    IsError   bool
    Duration  time.Duration
    Permitted bool // was it allowed by permissions?
}
```

### Provider Details

Each provider maps to the unified interface. Configuration per provider:

```go
// OpenAI (also works for Groq, Together, Fireworks, DeepSeek, etc.)
p := openai.New(
    openai.WithAPIKey(apiKey),
    openai.WithModel("gpt-4o"),
    openai.WithBaseURL("https://api.groq.com/openai/v1"),  // for compatible APIs
    openai.WithOrganization("org-xxx"),                     // optional
    openai.WithMaxTokens(4096),
    openai.WithTemperature(0.7),
)

// Anthropic
p := anthropic.New(
    anthropic.WithAPIKey(apiKey),
    anthropic.WithModel("claude-sonnet-4-20250514"),
    anthropic.WithMaxTokens(4096),
    anthropic.WithBetaHeaders("prompt-caching-2024-07-31"), // enable features
)

// Google Gemini
p := google.New(
    google.WithAPIKey(apiKey),
    google.WithModel("gemini-2.0-flash"),
    google.WithSafetySettings(google.BlockNone),
)

// Ollama (local) — uses Ollama's native /api/chat endpoint (not OpenAI-compat).
// For OpenAI-compatible endpoint, use: openai.New(openai.WithBaseURL("http://localhost:11434/v1"))
p := ollama.New(
    ollama.WithModel("llama3.3:70b"),
    ollama.WithBaseURL("http://localhost:11434"),  // default (native Ollama API)
    ollama.WithKeepAlive(30 * time.Minute),
)
```

All providers handle:
- **Tool call format translation**: OpenAI format ↔ Anthropic format ↔ Gemini format
- **System prompt placement**: Some providers use a dedicated system field, others prepend to messages
- **Token counting**: Each provider reports usage in its own format; provider adapter normalizes to `Usage`
- **Model info**: Each provider exposes context window limits for auto-truncation

```go
// Optional interface for providers that expose model metadata
type ModelInfoProvider interface {
    Provider
    ModelInfo(ctx context.Context) (ModelInfo, error)
}

type ModelInfo struct {
    ContextWindow  int  // max total tokens (input + output)
    MaxOutput      int  // max output tokens
    SupportsVision bool // can handle image inputs
    SupportsTools  bool // can handle tool calling
    SupportsJSON   bool // can handle structured output / response_format
}
```

Built-in model info for known models (no API call needed). This is the canonical location for model metadata — Context Window Management (below) references the same table:

```go
// Looked up from built-in table, no network call
info := openai.KnownModels["gpt-4o"]
// {ContextWindow: 128000, MaxOutput: 16384, SupportsVision: true, SupportsTools: true, SupportsJSON: true}
```

### Provider Fallback & Routing

Use multiple providers with automatic fallback:

```go
// Fallback: try providers in order, use first that succeeds
p := provider.Fallback(
    openai.New(openai.WithModel("gpt-4o")),
    anthropic.New(anthropic.WithModel("claude-sonnet-4-20250514")),
    ollama.New(ollama.WithModel("llama3.3:70b")),
)

// Load balancing: round-robin across providers
p := provider.RoundRobin(
    openai.New(openai.WithModel("gpt-4o")),
    openai.New(openai.WithModel("gpt-4o"), openai.WithAPIKey(secondKey)),
)

// Cost-based routing: use cheapest provider first, escalate if quality is low
p := provider.CostRouter(
    provider.Tier{Provider: ollama.New(ollama.WithModel("llama3.3:70b")), MaxCostPer1K: 0},
    provider.Tier{Provider: openai.New(openai.WithModel("gpt-4o-mini")), MaxCostPer1K: 0.15},
    provider.Tier{Provider: openai.New(openai.WithModel("gpt-4o")), MaxCostPer1K: 2.50},
)
```

### Retry & Resilience

All providers include built-in resilience:

```go
p := openai.New(
    openai.WithAPIKey(apiKey),
    openai.WithRetry(provider.RetryConfig{
        MaxRetries:  3,
        InitialWait: 1 * time.Second,
        MaxWait:     30 * time.Second,
        Backoff:     provider.ExponentialBackoff,
        RetryOn:     []int{429, 500, 502, 503},  // HTTP status codes
    }),
    openai.WithTimeout(60 * time.Second),
)
```

Default behavior (no config needed):
- Retry 3 times on 429 (rate limit) and 5xx errors
- Exponential backoff: 1s → 2s → 4s
- Respect `Retry-After` header from provider
- Context cancellation stops retries immediately

### Rate Limiting

Platform APIs have rate limits. Daneel handles them:

```go
// Per-platform rate limiting
tools := twitter.Tools(token,
    twitter.WithRateLimit(twitter.RateLimitConfig{
        RequestsPerMinute: 300,
        RequestsPer15Min:  900,  // Twitter's actual limit
    }),
)

// Or global rate limiter for all tool calls
agent := daneel.New("assistant",
    daneel.WithRateLimit(10), // max 10 tool calls per minute
)
```

Default: each platform tool pack comes with conservative defaults matching the platform's known rate limits. The rate limiter uses `golang.org/x/time/rate` (token bucket, pseudo-stdlib).

### Cost Tracking

Track LLM costs per agent, per conversation, per tool call:

```go
result, err := daneel.Run(ctx, agent, "Deploy the release")

fmt.Printf("Cost: $%.4f\n", result.Usage.EstimatedCost(provider.OpenAIPricing))
fmt.Printf("Tokens: %d prompt + %d completion = %d total\n",
    result.Usage.PromptTokens, result.Usage.CompletionTokens, result.Usage.TotalTokens)
```

Pricing tables built-in for major providers (updated via constants, not API calls):

```go
// Built-in pricing (per 1M tokens)
var OpenAIPricing = Pricing{
    "gpt-4o":      {Input: 2.50, Output: 10.00},
    "gpt-4o-mini": {Input: 0.15, Output: 0.60},
    "o3-mini":     {Input: 1.10, Output: 4.40},
}

var AnthropicPricing = Pricing{
    "claude-sonnet-4-20250514": {Input: 3.00, Output: 15.00},
    "claude-haiku-3-5":         {Input: 0.80, Output: 4.00},
}

// Custom pricing for self-hosted
var MyPricing = Pricing{
    "llama3.3:70b": {Input: 0, Output: 0}, // free, it's local!
}
```

### Session & Conversation ID

Every agent run can be associated with a session for multi-turn persistence and tracing:

```go
// Explicit session — resume a conversation
result, err := daneel.Run(ctx, agent, "Follow up on my last question",
    daneel.WithSession("session-abc-123"),
)
// result.SessionID == "session-abc-123"

// Auto-generated session (default) — UUID v4
result, err := daneel.Run(ctx, agent, "Hello")
// result.SessionID == "550e8400-e29b-41d4-a716-446655440000" (auto)

// Session enables:
// - Memory retrieval scoped to this conversation
// - Bridge maps platform user/channel to session automatically
// - OTEL traces grouped by session
// - Cost tracking per session
```

The Bridge uses sessions internally — each `(platform, from, channel)` tuple maps to a stable session ID:

```go
// Bridge auto-generates deterministic session IDs:
// sha256("telegram:user123:group456") -> session ID
// Same user in same channel always resumes their conversation
```

### Dynamic Context Injection

Static instructions via `WithInstructions()` aren't enough when context depends on runtime state (current user, database values, time of day):

```go
agent := daneel.New("support",
    daneel.WithInstructions("You are a support agent for Acme Corp"),
    daneel.WithContextFunc(func(ctx context.Context) (string, error) {
        user, err := auth.UserFromContext(ctx)
        if err != nil {
            return "", fmt.Errorf("failed to get user context: %w", err)
        }
        return fmt.Sprintf(
            "Current user: %s (plan: %s, balance: $%.2f).\nTime: %s",
            user.Name, user.Plan, user.Balance, time.Now().Format(time.RFC3339),
        ), nil
    }),
)
```

`WithContextFunc` is called at the start of each `Run()`. Its output is appended to the system prompt after `WithInstructions`. If the function returns an error, `Run()` returns that error immediately. Multiple context functions are supported and concatenated in order:

```go
daneel.WithContextFunc(userContext),      // user info
daneel.WithContextFunc(inventoryContext), // current stock levels
daneel.WithContextFunc(timeContext),      // current time + timezone
```

### Context Window Management

Long-running agents can exceed the model's context window. The Runner handles this automatically:

```go
// Model limits come from the provider's KnownModels table (see Provider Details section).
// Each provider exposes its own KnownModels map. The Runner looks up the current
// model's limits automatically. Example entries:
//   openai.KnownModels["gpt-4o"]       → {ContextWindow: 128_000, MaxOutput: 16_384, ...}
//   anthropic.KnownModels["claude-..."] → {ContextWindow: 200_000, MaxOutput: 8_192, ...}
```

When the conversation history approaches the limit, the Runner applies a truncation strategy:

```go
agent := daneel.New("assistant",
    daneel.WithContextStrategy(daneel.ContextSlidingWindow), // default
)
```

| Strategy | Behavior |
|---|---|
| `ContextSlidingWindow` (default) | Keep system prompt + first user message + last N messages that fit |
| `ContextSummarize` | Summarize older messages with an LLM call, keep summary + recent |
| `ContextError` | Return `ErrContextOverflow` — let the caller decide |

The Runner estimates token count via a fast heuristic (`len(text)/4` for English, `len(text)/2` for CJK). For exact counts, providers can implement an optional interface:

```go
type TokenCounter interface {
    CountTokens(ctx context.Context, messages []Message) (int, error)
}
```

### Runner (Agent Loop)

The heart of the library. Simple loop:

1. Build messages (system prompt + context funcs + history + user input)
2. Apply context window management (truncate/summarize if needed)
3. Run input guards — if fail, return `ErrGuardFailed`
4. Call provider LLM (with streaming if configured)
5. If response has ToolCalls:
   a. Check permissions for each tool (allow/deny lists) — if denied, inject error to LLM and continue loop (default) or return `ErrPermissionDenied` (with `WithStrictPermissions()`)
   b. Check if any tool requires approval (`WithApprovalRequired`) — if yes, call `Approver`; if denied, return `ErrApprovalDenied`
   c. Execute permitted+approved tools (parallel or sequential, configurable)
   d. If tool is a Handoff — transfer control to target agent, new loop
   e. Handle tool errors gracefully (inject error message to LLM, don't crash)
   f. Append results to history
   g. Fire `OnToolCall` callback if configured
   h. Go to step 2
6. Run output guards — if fail, return `ErrGuardFailed`
7. If response is final text — save to memory (scoped by session) — return `RunResult`
8. If MaxTurns reached — return `ErrMaxTurns` with partial result

Tool execution concurrency:

```go
// Sequential (default): tools run one at a time
agent := daneel.New("assistant",
    daneel.WithToolExecution(daneel.Sequential),
)

// Parallel: all tool calls in a single LLM response run concurrently
agent := daneel.New("assistant",
    daneel.WithToolExecution(daneel.Parallel),
)

// Parallel with concurrency limit
agent := daneel.New("assistant",
    daneel.WithToolExecution(daneel.ParallelN(3)), // max 3 concurrent
)
```

### Connector (Bidirectional)

Unified interface for receiving messages from any platform.

```go
type Connector interface {
    Start(ctx context.Context) error
    Send(ctx context.Context, to string, content string) error
    Messages() <-chan IncomingMessage
    Stop() error
}

type IncomingMessage struct {
    Platform  string
    From      string
    Content   string
    Channel   string
    Metadata  map[string]any
}
```

Note: `IncomingMessage` is pure data (no functions/closures) so it can be serialized for persistence. The Bridge maps each message back to its source connector internally for replies — the message itself doesn't need to know how to reply.

### Memory

All memory operations are scoped by session ID. The Runner passes the current session ID automatically.

```go
type Memory interface {
    Save(ctx context.Context, sessionID string, msgs []Message) error
    Retrieve(ctx context.Context, sessionID string, query string, limit int) ([]Message, error)
    Clear(ctx context.Context, sessionID string) error
}
```

Implementations:
- `memory.Sliding(n)` — keep last N messages per session
- `memory.Summary(provider)` — periodically summarize with LLM
- `memory.Vector(store, embedder)` — RAG: search relevant context before each turn (requires an `Embedder` to convert text → vectors)
- `memory.Composite(sliding, vector)` — combine: recent messages + relevant old ones

### Content (Multi-modal types)

The `content` package defines types for multi-modal data:

```go
type Content struct {
    Type     ContentType  // Text, Image, Audio, File
    Text     string       // for text content
    Data     []byte       // raw bytes for binary content
    MimeType string       // "image/png", "audio/wav", etc.
    URL      string       // remote URL (alternative to Data)
    Filename string       // optional filename
}

type ContentType string
const (
    ContentText  ContentType = "text"
    ContentImage ContentType = "image"
    ContentAudio ContentType = "audio"
    ContentFile  ContentType = "file"
)

// Convenience constructors
func TextContent(text string) Content
func ImageContent(data []byte, mimeType string) Content
func ImageURLContent(url string) Content
func AudioContent(data []byte, mimeType string) Content
func FileContent(data []byte, filename string, mimeType string) Content
```

Providers map `Content` to their native format:
- OpenAI: `content` array with `text` and `image_url` parts
- Anthropic: `content` array with `text` and `image` (base64) parts  
- Gemini: `parts` array with `text` and `inlineData`

---

## Error Handling Patterns

Daneel uses Go's standard error patterns: sentinel errors, typed errors, and `errors.Is`/`errors.As`.

### Sentinel Errors

```go
var (
    ErrPermissionDenied  = errors.New("daneel: permission denied")
    ErrMaxTurns          = errors.New("daneel: maximum turns exceeded")
    ErrGuardFailed       = errors.New("daneel: guard validation failed")
    ErrHandoff           = errors.New("daneel: handoff")              // internal
    ErrNoProvider        = errors.New("daneel: no provider configured")
    ErrApprovalRequired  = errors.New("daneel: approval required")
    ErrApprovalDenied    = errors.New("daneel: approval denied")
    ErrToolTimeout       = errors.New("daneel: tool execution timed out")
    ErrContextOverflow   = errors.New("daneel: context window exceeded") // only with ContextError strategy
)
```

### Typed Errors (with context)

```go
type PermissionError struct {
    Agent  string
    Tool   string
    Reason string // "tool in deny list", "tool not in allow list"
}

type GuardError struct {
    Agent   string
    Guard   string // "input" or "output"
    Message string
}

type MaxTurnsError struct {
    Agent    string
    MaxTurns int
    Partial  *RunResult // partial result up to the limit
}

type ProviderError struct {
    Provider   string
    StatusCode int
    Message    string
    Retryable  bool
}
```

### Error handling in user code

```go
result, err := daneel.Run(ctx, agent, input)
if err != nil {
    var permErr *daneel.PermissionError
    var maxErr  *daneel.MaxTurnsError

    switch {
    case errors.As(err, &permErr):
        log.Printf("Agent %s denied tool %s: %s", permErr.Agent, permErr.Tool, permErr.Reason)
    case errors.As(err, &maxErr):
        log.Printf("Agent %s hit %d turns, partial: %s", maxErr.Agent, maxErr.MaxTurns, maxErr.Partial.Output)
    case errors.Is(err, context.Canceled):
        log.Println("Request cancelled")
    case errors.Is(err, context.DeadlineExceeded):
        log.Println("Request timed out")
    default:
        log.Printf("Unexpected error: %v", err)
    }
}
```

### Error recovery in the agent loop

When a tool execution fails, the Runner does NOT crash. Instead:
1. The error is captured as a `ToolResult` with `IsError: true`
2. The error message is sent back to the LLM as context
3. The LLM can decide to retry, use a different tool, or give up
4. Tool panics are recovered via `recover()` and converted to error results

```go
// The LLM sees:
// Tool "github.merge_pr" returned error: "merge conflict: branch has 3 conflicting files"
// And can respond: "I couldn't merge the PR because there are merge conflicts. Please resolve them first."
```

---

## Security Considerations

### API Key Handling
- API keys are never logged, never serialized, never included in error messages
- Config file uses `${ENV_VAR}` syntax — keys stay in env vars
- Provider structs implement `fmt.Stringer` to redact keys: `OpenAI{model: gpt-4o, key: sk-...xxxx}`

### Prompt Injection Defense
- **Input guards** run before sending user input to the LLM. Use them to detect/block injection attempts:

```go
daneel.WithInputGuard(func(ctx context.Context, input string) error {
    if strings.Contains(input, "ignore previous instructions") {
        return fmt.Errorf("potential prompt injection detected")
    }
    return nil
})
```

- **Output guards** validate LLM responses before returning to the user
- **Tool permissions** prevent the LLM from executing dangerous operations even if injected
- **MaxTurns** prevents infinite loops from adversarial inputs

### Platform Token Scoping
- Document minimum required scopes per platform (e.g., GitHub: `repo`, `issues:write`)
- Recommend read-only tokens where possible
- WhatsApp via whatsmeow: document ToS risk and recommend WhatsApp Business API for production

### Sandbox Recommendations
- For agents with `exec`-type tools, recommend running in Docker/container
- The library doesn't enforce sandboxing (it's a library, not a runtime) but documents best practices
- The future CLI could add `--sandbox` flag using Docker

---

## Tool Schema Generation (Internals)

`NewTool[T]` uses reflection to auto-generate JSON Schema from the parameter struct. Supported features:

### Struct Tags

```go
type MyParams struct {
    // Basic types
    Name    string  `json:"name"    desc:"User name"                   required:"true"`
    Age     int     `json:"age"     desc:"User age"                    required:"false"`
    Score   float64 `json:"score"   desc:"Quality score 0-10"`
    Active  bool    `json:"active"  desc:"Whether user is active"`

    // Enums
    Status  string  `json:"status"  desc:"Current status" enum:"active,inactive,pending"`

    // Optional with default
    Limit   int     `json:"limit"   desc:"Max results" default:"10"`

    // Nested structs (mapped to JSON Schema objects)
    Address Address `json:"address" desc:"User address"`

    // Slices (mapped to JSON Schema arrays)
    Tags    []string `json:"tags"   desc:"Labels"`
}
```

### Type Mapping

| Go Type | JSON Schema Type | Notes |
|---|---|---|
| `string` | `string` | |
| `int`, `int64`, `int32` | `integer` | |
| `float64`, `float32` | `number` | |
| `bool` | `boolean` | |
| `[]T` | `array` with `items` | |
| `struct` | `object` with `properties` | recursive |
| `map[string]T` | `object` with `additionalProperties` | |
| `*T` | same as T | pointer means optional |
| `json.RawMessage` | `object` | pass-through |

Fields without `json` tag are ignored. Fields with `json:"-"` are skipped. The `required` tag defaults to `true` for non-pointer fields.

### Handoff Internals

When you pass `WithHandoffs(agentA, agentB)`, the Runner:

1. Creates a synthetic tool for each handoff target:
   - Name: `handoff_to_{agent_name}`
   - Description: auto-generated from agent's instructions (first 200 chars)
   - Schema: `{"reason": "string"}` — why the handoff is happening

2. When LLM calls `handoff_to_spanish`:
   - Runner detects it's a handoff tool (not a real tool)
   - Creates a new Runner with the target agent
   - **Provider inheritance**: If the target agent has no provider, it inherits the current agent's provider. If it has its own, that one is used.
   - Transfers conversation history (configurable: full, last N, or summary)
   - Runs the target agent's loop
   - Returns the target agent's result as the final result

3. History transfer is configured at the agent level (applies to all outgoing handoffs):
   ```go
   daneel.WithHandoffs(spanishAgent, techAgent),       // which agents
   daneel.WithHandoffHistory(daneel.FullHistory),       // send all history (default)
   // alternatives:
   daneel.WithHandoffHistory(daneel.LastN(5)),          // last 5 messages
   daneel.WithHandoffHistory(daneel.SummaryHistory),    // summarize with LLM
   ```

---

## Configuration

### Quick start (code only, zero files)

`Quick()` is a convenience constructor that returns a `QuickAgent` — a wrapper around `Agent` that also holds pre-configured connectors. The `Agent` itself stays immutable and connector-free. `QuickAgent` embeds `*Agent` (usable anywhere an Agent is expected) and exposes `.Connectors()` for Bridge.

```go
// Quick() returns QuickAgent which embeds *Agent + stores connectors
qa := daneel.Quick("assistant",
    daneel.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
    daneel.WithTwitter(os.Getenv("TWITTER_TOKEN")),  // adds tools to Agent + registers connector
)

// Use as regular agent (QuickAgent embeds *Agent):
result, err := daneel.Run(ctx, qa.Agent, "What's trending?")
fmt.Println(result.Output)

// Use with Bridge (connectors auto-extracted):
bridge.Multi(ctx, qa.Agent, qa.Connectors()...)

// Equivalent using New() (explicit control):
agent := daneel.New("assistant",
    daneel.WithProvider(openai.New(openai.WithAPIKey(os.Getenv("OPENAI_API_KEY")))),
    daneel.WithTools(twitter.Tools(os.Getenv("TWITTER_TOKEN"))...),
)
```

```go
// QuickAgent wraps Agent + pre-configured connectors
type QuickAgent struct {
    Agent      *Agent
    connectors []Connector
}

func (qa *QuickAgent) Connectors() []Connector { return qa.connectors }
```

### Auto-config (env vars)

```go
// Reads DANEEL_OPENAI_KEY, DANEEL_TWITTER_TOKEN, etc.
agent := daneel.Quick("assistant", daneel.AutoConfig())
```

### Config file (daneel.json)

```json
{
  "provider": {
    "type": "openai",
    "api_key": "${OPENAI_API_KEY}",
    "model": "gpt-4o"
  },
  "platforms": {
    "twitter": {
      "bearer_token": "${TWITTER_BEARER_TOKEN}",
      "poll_interval": "30s"
    },
    "github": {
      "token": "${GITHUB_TOKEN}",
      "webhook_secret": "${GITHUB_WEBHOOK_SECRET}"
    },
    "slack": {
      "bot_token": "${SLACK_BOT_TOKEN}",
      "socket_mode": true
    },
    "whatsapp": {
      "data_dir": "./whatsapp-data"
    },
    "email": {
      "smtp": {
        "host": "smtp.gmail.com",
        "port": 587,
        "username": "${EMAIL_USER}",
        "password": "${EMAIL_PASS}"
      },
      "imap": {
        "host": "imap.gmail.com",
        "port": 993,
        "username": "${EMAIL_USER}",
        "password": "${EMAIL_PASS}"
      }
    },
    "telegram": {
      "bot_token": "${TELEGRAM_BOT_TOKEN}"
    }
  },
  "agents": [
    {
      "name": "support",
      "instructions": "Handle customer support for Acme Corp",
      "model": "gpt-4o",
      "tools": ["twitter.reply", "twitter.search", "slack.send_message", "github.comment"],
      "deny_tools": ["twitter.follow", "github.merge_pr"],
      "max_turns": 15,
      "memory": {
        "type": "sliding",
        "size": 20
      }
    },
    {
      "name": "reviewer",
      "instructions": "Review pull requests for code quality",
      "model": "claude-sonnet-4",
      "tools": ["github.review_pr", "github.comment"],
      "max_turns": 5
    }
  ]
}
```

Load from Go:

```go
cfg, err := daneel.LoadConfig("daneel.json")

// Step 1: Build platform clients (resolves tokens from env vars)
platforms, err := cfg.BuildPlatforms()
// platforms["twitter"] has tools + connector ready

// Step 2: Build agents (resolves tool references like "twitter.reply" against platforms)
agents, err := cfg.BuildAgents(platforms)
// agents["support"] has the resolved tools, provider, permissions, etc.
```

> **Why two steps?** Tool references in the config (`"twitter.reply"`) need to be resolved against actual platform tool packs. Platforms need tokens. Tokens come from the config. This two-step flow avoids a chicken-and-egg problem.

---

## Workflows

### Chain (Sequential Pipeline)

```go
result, err := workflow.Chain(ctx, input,
    researcherAgent,
    writerAgent,
    editorAgent,
)
// Each agent uses its own provider. researcher output → writer input → editor input → final result
```

### Parallel (Concurrent Execution)

```go
results, err := workflow.Parallel(ctx,
    workflow.Task(analystAgent, "Analyze financials"),
    workflow.Task(techLeadAgent, "Review architecture"),
    workflow.Task(legalAgent, "Check compliance"),
)
// All run concurrently via goroutines + errgroup. Each agent uses its own provider.
// results[0], results[1], results[2]
```

### Router (Triage → Specialized Agent)

```go
result, err := workflow.Router(ctx, input,
    triageAgent,
    map[string]*daneel.Agent{
        "billing":   billingAgent,
        "technical": techAgent,
        "general":   generalAgent,
    },
)
// triageAgent classifies input → routes to the right agent. Each agent uses its own provider.
```

### Orchestrator (Dynamic Task Decomposition)

```go
result, err := workflow.Orchestrator(ctx, input,
    bossAgent,     // decomposes the task
    coderAgent,    // worker
    researchAgent, // worker
    writerAgent,   // worker
)
// boss breaks task into subtasks → assigns to workers → synthesizes results
```

### Handoffs (Agent-to-Agent Transfer)

```go
triageAgent := daneel.New("triage",
    daneel.WithInstructions("Route to the right specialist"),
    daneel.WithHandoffs(spanishAgent, techAgent, billingAgent),
)
// LLM decides when to handoff. Internally, each handoff is a Tool:
// "handoff_to_spanish" → transfers control + conversation history
```

---

## Platform Integration

### Tool Packs

Each platform exposes a `Tools()` function returning ready-to-use tools.

```go
// Twitter
tools := twitter.Tools(bearerToken)
// Returns: tweet, reply, search_tweets, get_mentions, follow, like, get_user

// GitHub
tools := github.Tools(token)
// Returns: create_issue, close_issue, create_pr, merge_pr, review_pr, comment, list_issues, search_code

// Slack
tools := slack.Tools(botToken)
// Returns: send_message, read_channel, react, create_channel, list_channels, upload_file

// WhatsApp
tools := whatsapp.Tools(client)
// Returns: send_message, send_media, get_messages, get_contacts

// Email
tools := email.Tools(smtpCfg, imapCfg)
// Returns: send_email, reply_email, read_inbox, search_email

// Telegram
tools := telegram.Tools(botToken)
// Returns: send_message, send_photo, reply, get_updates
```

### Connectors (Bidirectional)

| Platform | Mechanism          | Requirements                         |
|----------|--------------------|--------------------------------------|
| Twitter  | Polling (30s)      | Bearer token, no public URL          |
| WhatsApp | WebSocket          | QR scan on first run (whatsmeow)     |
| GitHub   | Webhooks or Polling | Token; webhook needs public URL      |
| Slack    | Socket Mode        | Bot token, no public URL             |
| Email    | IMAP IDLE          | IMAP credentials                     |
| Telegram | Long-polling       | Bot token, no public URL             |

### Bridge (Connector → Agent)

```go
// Single platform
b := bridge.New(
    bridge.WithConnector(telegramConnector),
    bridge.WithAgent(supportAgent),   // provider is already configured in the agent
)
b.Run(ctx) // blocks, processes messages

// Multi-platform (provider lives in the agent, not in bridge)
bridge.Multi(ctx, supportAgent,
    twitter.Listen(twitterToken),
    slack.Listen(slackToken),
    telegram.Listen(telegramToken),
    github.Listen(githubToken, github.WebhookPort(8080)),
)
// Each incoming message → agent.Run() → reply via same channel
```

Bridge handles:
- **Concurrency**: Each message in its own goroutine (configurable semaphore)
- **Conversation history**: `map[string][]Message` per user/channel for context
- **Permissions**: Inherited from agent. Denied tools are rejected even from connectors.
- **History TTL**: Conversations expire after configurable duration (default: 1 hour)
- **Max history size**: Cap messages per conversation to prevent memory leaks
- **Persistence**: Optional persistence to disk/DB for conversation recovery after restart

```go
b := bridge.New(
    bridge.WithConnector(telegramConnector),
    bridge.WithAgent(supportAgent),   // provider is configured in the agent
    bridge.WithConcurrency(10),                    // max 10 concurrent conversations
    bridge.WithHistoryTTL(1 * time.Hour),          // expire after 1h
    bridge.WithMaxHistory(100),                    // max 100 messages per conversation
    bridge.WithPersistence(bridge.FileStore("./conversations")), // optional
    bridge.WithErrorHandler(func(err error, msg IncomingMessage) {
        slog.Error("bridge error", "from", msg.From, "error", err)
    }),
)
```

### Graceful Shutdown

All long-running components (Bridge, Connectors, Scheduler) support graceful shutdown via `context.Context`:

```go
ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer cancel()

// Bridge.Run blocks until ctx is cancelled
err := bridge.Multi(ctx, agent,  // provider is configured in the agent
    telegram.Listen(token),
    slack.Listen(slackToken),
)
// On SIGINT/SIGTERM:
// 1. Stop accepting new messages
// 2. Wait for in-flight conversations to complete (with timeout)
// 3. Close connectors
// 4. Return nil

if err != nil && !errors.Is(err, context.Canceled) {
    log.Fatal(err)
}
```

---

## Memory & RAG

### Sliding Window (simplest)

```go
agent := daneel.New("assistant",
    daneel.WithMemory(memory.Sliding(20)), // keep last 20 messages
)
```

### Summary Memory

```go
agent := daneel.New("assistant",
    daneel.WithMemory(memory.Summary(provider, memory.SummarizeEvery(10))),
    // Every 10 messages, summarize older ones with LLM
)
```

### Vector Memory (RAG)

```go
// Built-in vector store (zero external deps, cosine similarity in ~50 LOC)
store := store.NewLocal("./memory.db")     // persisted via encoding/gob

// Or external vector DB (import adapter subpackage: memory/store/qdrant)
store := qdrant.New("localhost:6334", "conversations")

// Vector memory requires an Embedder to convert text to vectors.
// Use a provider's embedding API or a local model.
embedder := openai.NewEmbedder(apiKey, "text-embedding-3-small")

agent := daneel.New("assistant",
    daneel.WithMemory(memory.Vector(store, embedder, memory.TopK(5))),
    // Before each turn, embed the query and search for 5 most relevant past messages
)
```

---

## Fine-tuning Pipeline

Full pipeline to capture agent conversations, curate datasets, fine-tune local LLMs, evaluate, and deploy — all orchestrated from Go.

### How it works

```
Agent runs → Collector captures conversations → Dataset filters by quality
    → Trainer fine-tunes via Unsloth (Python subprocess)
    → Exports GGUF → Imports to Ollama/LocalAI
    → Evaluator compares new vs old → Scheduler auto-retrains
```

There is no native LLM fine-tuning in Go. Daneel orchestrates the full pipeline: Go captures data and manages the lifecycle, Python (Unsloth/TRL) does the actual training as a subprocess. The user never touches Python directly.

### Step 1: Capture Training Data

The `Collector` hooks into agent conversations via `WithOnConversationEnd`. Every completed conversation is captured with full context: system prompt, user messages, tool calls, tool results, and assistant responses.

```go
collector := finetune.NewCollector(
    finetune.WithFormat(finetune.ShareGPT), // or Alpaca, OpenAI, ChatML
    finetune.WithStorage("./training_data"),
    finetune.WithMinTurns(3),               // skip trivial conversations
    finetune.WithIncludeToolCalls(true),     // include tool usage in training
)

agent := daneel.New("assistant",
    daneel.WithOnConversationEnd(collector.Capture),
)
// Agent runs normally. Conversations are captured in the background.
// OnConversationEnd fires at the end of each Run() call (not at session expiry).
// In Bridge mode, each incoming message triggers a Run(), so each exchange is captured.
// If the process crashes, all completed Run() calls have already been captured.
```

Supported formats:
- **ShareGPT** — Standard for Unsloth/Axolotl. Multi-turn with roles.
- **Alpaca** — instruction/input/output triplets. Good for single-turn.
- **OpenAI** — OpenAI fine-tuning format (messages array).
- **ChatML** — Raw ChatML template format.

### Step 2: Dataset Curation

Raw conversations need filtering. Not all conversations are good training data.

```go
ds, err := finetune.LoadDataset("./training_data")

// Filter by quality
ds = ds.Filter(
    finetune.MinTurns(3),                    // at least 3 turns
    finetune.MaxTurns(50),                   // not too long
    finetune.NoErrors(),                     // exclude conversations with errors
    finetune.ContainsTool("github.create_pr"), // only convos that used this tool
)

// Score quality using an LLM judge
ds, err = ds.Score(ctx, provider, finetune.LLMJudge{
    Criteria: "helpfulness, accuracy, tool usage",
    MinScore: 7, // keep only 7+/10
})

// Augment: generate variations of good conversations
ds, err = ds.Augment(ctx, provider, finetune.AugmentOptions{
    Paraphrase:  true,  // rephrase user messages
    Multilingual: []string{"es", "fr", "de"}, // translate
    Variations:  3,     // 3 variations per conversation
})

// Split into train/test
train, test := ds.Split(0.9) // 90% train, 10% test
train.Export("train.jsonl")
test.Export("test.jsonl")

fmt.Printf("Dataset: %d train, %d test\n", train.Len(), test.Len())
```

### Step 3: Fine-tune

Daneel generates and launches a Python training script using Unsloth (52k ★) or HuggingFace TRL. The user never writes Python.

```go
// LoRA fine-tuning (fast, low VRAM)
job, err := finetune.Run(ctx, "train.jsonl",
    finetune.WithUnsloth(),
    finetune.LoRA(finetune.LoRAConfig{
        Rank:      16,
        Alpha:     32,
        Dropout:   0.05,
        TargetModules: []string{"q_proj", "v_proj", "k_proj", "o_proj"},
    }),
    finetune.BaseModel("unsloth/Llama-3.3-70B-Instruct"),
    finetune.Epochs(3),
    finetune.BatchSize(4),
    finetune.LearningRate(2e-4),
    finetune.ExportGGUF("Q4_K_M"),
    finetune.OutputDir("./models/my-agent-v1"),
)
```

Training methods:

```go
// QLoRA — 4-bit quantized LoRA (even less VRAM, ~6GB for 7B model)
finetune.QLoRA(finetune.QLoRAConfig{
    Rank:       16,
    BitsAndBytes: 4,
})

// Full fine-tuning (maximum quality, needs lots of VRAM)
finetune.FullFineTune()

// GRPO — Group Relative Policy Optimization (reinforcement learning)
// Train the model to prefer certain behaviors using reward signals
finetune.GRPO(finetune.GRPOConfig{
    RewardModel: "quality_scorer",
    KLCoeff:     0.1,
    NumGenerations: 4,
})

// SFT with TRL instead of Unsloth
finetune.WithTRL() // uses HuggingFace TRL library
```

Monitor progress:

```go
// Progress channel — stream training metrics
for update := range job.Progress() {
    fmt.Printf("Epoch %d/%d | Step %d | Loss: %.4f | LR: %.2e\n",
        update.Epoch, update.TotalEpochs, update.Step, update.Loss, update.LR)
}

// Or just wait
result, err := job.Wait()
fmt.Printf("Training complete in %s. Model at: %s\n", result.Duration, result.OutputPath)
```

### Step 4: Deploy to Local Runtime

Import the trained model to Ollama or LocalAI for serving.

```go
// Option A: Ollama
err = finetune.ImportToOllama(ctx, result.OutputPath, "my-agent-v1",
    finetune.WithSystemPrompt("You are a specialized support agent for Acme Corp"),
    finetune.WithTemperature(0.7),
    finetune.WithContextLength(8192),
)
// Creates Modelfile + runs `ollama create my-agent-v1`

// Option B: LocalAI (supports more backends: llama.cpp, MLX, vLLM, transformers)
err = finetune.ImportToLocalAI(ctx, result.OutputPath, "my-agent-v1",
    finetune.WithBackend("llama-cpp"),   // or "mlx", "vllm", "transformers"
    finetune.WithGPULayers(35),
)
// Copies model + creates config for LocalAI

// Use in Daneel
customAgent := daneel.New("custom",
    daneel.WithOllama("my-agent-v1"),      // or daneel.WithLocalAI("my-agent-v1")
    daneel.WithInstructions("You are a specialized support agent"),
    daneel.WithTools(github.Tools(token)...),
)
```

### Step 5: Evaluate

Compare your fine-tuned model against the original or against a cloud model.

```go
eval, err := finetune.Evaluate(ctx, "test.jsonl",
    finetune.Models(
        finetune.Model("custom", ollama.New(ollama.WithModel("my-agent-v1"))),
        finetune.Model("base",   ollama.New(ollama.WithModel("llama3.3:70b"))),
        finetune.Model("gpt4o",  openai.New(openai.WithAPIKey(apiKey), openai.WithModel("gpt-4o"))),
    ),
    finetune.Metrics(
        finetune.Accuracy,         // correct tool selection
        finetune.ToolCallAccuracy, // correct tool arguments
        finetune.ResponseQuality,  // LLM-judged quality (uses a judge model)
        finetune.Latency,          // response time
        finetune.TokenEfficiency,  // tokens used per task
    ),
    finetune.JudgeModel(openai.New(openai.WithAPIKey(apiKey), openai.WithModel("gpt-4o"))), // LLM judge for quality
    finetune.Parallel(4), // run 4 evaluations concurrently
)

// Print results
for _, m := range eval.Models {
    fmt.Printf("%s: accuracy=%.1f%% quality=%.1f/10 latency=%dms tokens=%d\n",
        m.Name, m.Accuracy*100, m.Quality, m.AvgLatencyMs, m.AvgTokens)
}
// Output:
// custom:  accuracy=92.3% quality=8.2/10 latency=45ms  tokens=380
// base:    accuracy=78.1% quality=7.1/10 latency=42ms  tokens=520
// gpt4o:   accuracy=95.7% quality=9.1/10 latency=890ms tokens=410

// Export evaluation report
eval.ExportJSON("eval_report.json")
eval.ExportMarkdown("eval_report.md") // human-readable comparison table
```

### Step 6: Auto-retrain (Scheduler)

Automatically trigger retraining when enough new data is collected.

```go
scheduler := finetune.NewScheduler(
    finetune.CollectFrom(collector),
    finetune.RetrainAfter(1000),          // retrain after 1000 new conversations
    finetune.RetrainEvery(7 * 24 * time.Hour), // or weekly, whichever comes first
    finetune.BaseConfig(finetune.Config{
        Method:    finetune.MethodLoRA,
        BaseModel: "my-agent-v1",         // incremental: fine-tune on top of previous
        ExportGGUF: "Q4_K_M",
    }),
    finetune.OnComplete(func(result finetune.Result) {
        // Auto-deploy if quality improved
        if result.Eval.Accuracy > 0.90 {
            finetune.ImportToOllama(ctx, result.OutputPath, "my-agent-latest")
            slog.Info("deployed new model", "version", result.Version, "accuracy", result.Eval.Accuracy)
        }
    }),
    finetune.OnError(func(err error) {
        slog.Error("training failed", "error", err)
    }),
)

go scheduler.Start(ctx) // runs in background
```

### Hardware Requirements

| Method | 7B Model | 13B Model | 70B Model |
|---|---|---|---|
| QLoRA (4-bit) | ~6 GB VRAM | ~10 GB VRAM | ~48 GB VRAM |
| LoRA (16-bit) | ~16 GB VRAM | ~28 GB VRAM | ~140 GB VRAM |
| Full fine-tune | ~28 GB VRAM | ~52 GB VRAM | ~280 GB VRAM |
| GRPO | ~32 GB VRAM | ~56 GB VRAM | ~300 GB VRAM |

> **Apple Silicon note**: Unsloth supports MLX backend for M1/M2/M3/M4 Macs. Use `finetune.WithMLX()` to use unified memory instead of discrete VRAM. A MacBook Pro with 36GB can fine-tune 7B-13B models comfortably.

### Python Dependency Management

Daneel handles the Python environment automatically:

```go
// Auto-setup: creates a venv, installs unsloth + dependencies
err := finetune.Setup(ctx,
    finetune.PythonPath("/usr/bin/python3"), // optional, auto-detected
    finetune.VenvPath("./.daneel-venv"),      // where to create venv
)
// First run takes ~2-5 min to install. Subsequent runs reuse the venv.

// Check if everything is ready
ready, missing := finetune.Check(ctx)
if !ready {
    fmt.Printf("Missing: %v\n", missing) // ["torch", "unsloth"]
}
```

---

## MCP (Model Context Protocol)

MCP is the open standard for connecting AI agents to external systems — "USB-C for AI". Daneel integrates via `mark3labs/mcp-go` (8.2k ★).

### Consume MCP Servers

Any existing MCP server becomes Daneel tools automatically. There are 1000+ MCP servers available.

```go
// Connect to stdio MCP server
mcpTools, err := mcp.Connect(ctx, "npx @modelcontext/server-github")

// Connect to HTTP MCP server
mcpTools, err := mcp.ConnectHTTP(ctx, "http://localhost:3001")

// Connect to multiple MCP servers at once
mcpTools, err := mcp.ConnectAll(ctx,
    mcp.Stdio("npx @modelcontext/server-github"),
    mcp.Stdio("npx @modelcontext/server-filesystem", "--root", "/data"),
    mcp.HTTP("http://localhost:3001"),
)

agent := daneel.New("assistant",
    daneel.WithTools(mcpTools...), // all MCP tools available to the agent
)
```

MCP tools respect Daneel permissions:
```go
daneel.WithPermissions(
    daneel.AllowTools("mcp.github.*"),    // allow all GitHub MCP tools
    daneel.DenyTools("mcp.filesystem.delete"), // but no deleting
)
```

### Expose Daneel Tools as MCP Server

Let other MCP clients (Claude Desktop, Cursor, etc.) use Daneel tools:

```go
server := mcp.NewServer(
    mcp.WithName("daneel-tools"),
    mcp.WithTools(searchTool, calcTool, twitter.Tools(token)...),
    mcp.WithResources(mcp.FileResource("./data")),
)

// Stdio (for Claude Desktop, Cursor, etc.)
server.ListenStdio()

// HTTP (for remote clients)
server.ListenHTTP(":8080",
    mcp.WithAuth(mcp.BearerToken("secret")),
)
```

---

## Observability

### OpenTelemetry Integration

```go
agent := daneel.New("assistant",
    daneel.WithTracing(), // enables OTEL spans
)
// Produces hierarchical spans:
//   Agent:assistant (run_id=abc123)
//   ├── LLM:gpt-4o (340ms, prompt=800tok, completion=400tok)
//   ├── Permission:check (0.1ms, tool=github.create_issue, allowed=true)
//   ├── Tool:github.create_issue (120ms, success=true)
//   ├── LLM:gpt-4o (280ms, prompt=1200tok, completion=200tok)
//   └── Guard:output (2ms, passed=true)
```

Span attributes include: `agent.name`, `agent.model`, `tool.name`, `tool.duration_ms`, `llm.prompt_tokens`, `llm.completion_tokens`, `llm.model`, `llm.provider`, `error.type`.

Export to any OTEL-compatible backend:

```go
import "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"

// Export to Jaeger, Grafana Tempo, Datadog, etc.
exporter, _ := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint("localhost:4318"))
tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
otel.SetTracerProvider(tp)

// Now all daneel.Run() calls produce traces automatically
```

Also compatible with **LangFuse** via its OTEL bridge (LangFuse accepts OTEL traces).

### Structured Logging

Uses `log/slog` (stdlib since Go 1.21). No imposed logger.

```go
// The library logs via slog. Configure your handler:
slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
    Level: slog.LevelDebug,
})))

// Daneel logs with structured fields:
// {"level":"INFO","msg":"tool executed","agent":"support","tool":"github.comment","duration_ms":120}
// {"level":"WARN","msg":"permission denied","agent":"support","tool":"github.merge_pr"}
// {"level":"DEBUG","msg":"llm response","agent":"support","model":"gpt-4o","tokens":1200}
```

### Metrics

OTEL metrics (counters, histograms, gauges):

| Metric | Type | Description |
|---|---|---|
| `daneel.agent.runs` | Counter | Total agent runs |
| `daneel.agent.turns` | Histogram | Turns per run |
| `daneel.agent.duration_ms` | Histogram | Total run duration |
| `daneel.llm.requests` | Counter | LLM API calls (by provider, model) |
| `daneel.llm.tokens.prompt` | Counter | Prompt tokens used |
| `daneel.llm.tokens.completion` | Counter | Completion tokens used |
| `daneel.llm.latency_ms` | Histogram | LLM response time |
| `daneel.llm.errors` | Counter | LLM API errors (by type) |
| `daneel.tool.executions` | Counter | Tool calls (by name, success/failure) |
| `daneel.tool.duration_ms` | Histogram | Tool execution time |
| `daneel.permission.denials` | Counter | Permission denials (by agent, tool) |
| `daneel.guard.failures` | Counter | Guard validation failures |
| `daneel.handoff.count` | Counter | Handoffs (by source→target) |
| `daneel.cost.usd` | Counter | Estimated cost in USD |
| `daneel.bridge.messages` | Counter | Incoming messages (by platform) |
| `daneel.bridge.active_conversations` | Gauge | Active conversations |

---

## Human-in-the-Loop (Approval)

```go
agent := daneel.New("assistant",
    daneel.WithTools(searchTool, deleteTool, mergeTool),
    daneel.WithApprovalRequired("delete", "github.merge_pr"), // these tools need human OK
)

// Approval is handled via the Approver interface, passed to Run() (not on the Agent).
// This keeps Agent immutable (no internal channels/state).
// The `approval` package provides convenience implementations.
result, err := daneel.Run(ctx, agent, "merge the PR",
    daneel.WithApprover(daneel.ApproverFunc(func(ctx context.Context, req daneel.ApprovalRequest) (bool, error) {
        fmt.Printf("Agent wants to call %s with %s\n", req.Tool, req.Args)
        fmt.Print("Approve? [y/n]: ")
        var answer string
        fmt.Scanln(&answer)
        return answer == "y", nil
    })),
)

// For testing:
result, err := daneel.Run(ctx, agent, "delete all",
    daneel.WithApprover(approval.AutoApprove()), // approve everything
)

// For dry-run / audit:
result, err := daneel.Run(ctx, agent, "delete all",
    daneel.WithApprover(approval.AlwaysDeny()), // deny everything, log what would happen
)
```

---

## Multi-Modal Content

```go
// Send image to agent
result, err := daneel.Run(ctx, agent, "What's in this image?",
    daneel.WithImage("photo.jpg"),              // from file
    daneel.WithImageURL("https://example.com/img.png"), // from URL
)

// Tools can return multi-modal content
screenshotTool := daneel.NewToolWithContent("screenshot", "Take a screenshot",
    func(ctx context.Context, p ScreenshotParams) (content.Content, error) {
        img, err := takeScreenshot(p.Region)
        return content.ImageContent(img, "image/png"), err
    },
)
```

---

## Testing

Daneel is designed to be testable without real API keys or external services.

### Mock Provider

A built-in mock provider for deterministic testing:

```go
import "github.com/daneel-ai/daneel/provider/mock"

// Static responses
p := mock.New(
    mock.Respond("Hello! How can I help?"),
    mock.RespondWithToolCall("search", `{"query": "golang"}`),
    mock.Respond("Here are the results..."),
)

// Agent is immutable — WithProvider returns a new Agent (copy-on-modify pattern)
result, err := daneel.Run(ctx, agent.WithProvider(p), "help me")
assert.Equal(t, "Here are the results...", result.Output)
assert.Equal(t, 2, result.Turns)

// Dynamic responses
p := mock.New(mock.RespondFunc(func(msgs []daneel.Message) *daneel.Response {
    lastMsg := msgs[len(msgs)-1]
    if strings.Contains(lastMsg.Content, "error") {
        return mock.ErrorResponse("something went wrong")
    }
    return mock.TextResponse("all good")
}))
```

### Mock Connector

Simulate incoming messages from platforms:

```go
import "github.com/daneel-ai/daneel/connector/mock"

c := mock.NewConnector()

// In test:
c.SimulateMessage(daneel.IncomingMessage{
    Platform: "telegram",
    From:     "user123",
    Content:  "What's the status of my order?",
})

// Assert reply
reply := <-c.SentMessages()
assert.Contains(t, reply.Content, "order")
```

### Test Helpers

```go
import "github.com/daneel-ai/daneel/testing/daneeltest"

// Quick agent for testing (uses mock provider internally)
agent, mock := daneeltest.QuickAgent(t, "test-agent",
    daneel.WithTools(myTool),
    daneel.WithPermissions(daneel.DenyTools("dangerous")),
    daneel.WithStrictPermissions(), // abort on denial (default is soft inject)
)

// Assert permission denied (only fires with WithStrictPermissions)
mock.QueueToolCall("dangerous", `{}`)
result, err := daneel.Run(ctx, agent, "do something dangerous")
assert.ErrorIs(t, err, daneel.ErrPermissionDenied)

// Assert tool was called
mock.QueueToolCall("safe_tool", `{"key": "value"}`)
mock.QueueResponse("done")
result, err = daneel.Run(ctx, agent, "do something safe")
assert.NoError(t, err)
assert.True(t, mock.ToolWasCalled("safe_tool"))
assert.JSONEq(t, `{"key": "value"}`, mock.ToolCallArgs("safe_tool"))
```

### Integration Tests

Platform integration tests are gated behind build tags:

```go
//go:build integration

func TestTwitterLive(t *testing.T) {
    token := os.Getenv("TWITTER_BEARER_TOKEN")
    if token == "" {
        t.Skip("TWITTER_BEARER_TOKEN not set")
    }
    tools := twitter.Tools(token)
    // ... test real API ...
}
```

Run: `go test -tags=integration ./platform/twitter/`

### Benchmarks

```go
func BenchmarkRunnerLoop(b *testing.B) {
    p := mock.New(mock.Respond("ok"))
    agent := daneel.New("bench", daneel.WithProvider(p))
    for i := 0; i < b.N; i++ {
        daneel.Run(context.Background(), agent, "test")
    }
}
// Target: <100µs overhead per run (excluding LLM latency)
```

---

## Comparison with Alternatives

| Feature | **Daneel** | langchaingo | openai-agents-go | AgenticGoKit | Cogito |
|---|---|---|---|---|---|
| **Agent loop** | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Handoffs** | ✅ | ❌ | ✅ | ❌ | ❌ |
| **Permissions (allow/deny)** | ✅ First-class | ❌ | ❌ | ❌ | ❌ |
| **Guardrails** | ✅ Input + Output | ❌ | ✅ | ❌ | ✅ |
| **Human-in-the-loop** | ✅ Approval system | ❌ | ❌ | ❌ | ✅ |
| **Platform tool packs** | ✅ 6 platforms | ❌ | ❌ | ❌ | ❌ |
| **Bidirectional connectors** | ✅ 6 platforms | ❌ | ❌ | ❌ | ❌ |
| **Workflows** | ✅ Chain/Parallel/Router/Orchestrator | ✅ Chains | ❌ | ✅ DAG/Loop | ❌ |
| **Memory/RAG** | ✅ Sliding/Summary/Vector | ✅ | ❌ | ✅ | Partial |
| **MCP** | ✅ Client + Server | ❌ | ❌ | ✅ | ✅ |
| **Fine-tuning pipeline** | ✅ Full pipeline | ❌ | ❌ | ❌ | ❌ |
| **Multi-provider** | ✅ 4+ providers | ✅ | ❌ OpenAI only | ✅ | ✅ |
| **Provider fallback** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **Streaming** | ✅ | ✅ | ✅ | ✅ | ❌ |
| **Cost tracking** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **OTEL tracing** | ✅ | ❌ | ❌ | ✅ | ❌ |
| **Config file** | ✅ JSON (stdlib) | ❌ | ❌ | ❌ | ❌ |
| **External deps (core)** | 0 | Many | 1 | Several | Few |
| **Stars** | New | 8.7k | 236 | 97 | 36 |

**Daneel's unique value**: The foundational agent orchestration library that Go is missing. Combines agent loop + declarative permissions + platform integrations + fine-tuning pipeline with zero core dependencies, all designed from scratch for Go idioms — not ported from Python.

### Why not just use langchaingo?

`langchaingo` es un port de LangChain (Python) a Go. Hereda la arquitectura de LangChain — abstracciones pesadas, muchas dependencias, patrones que no son idiomáticos en Go (`Chain` como clase con métodos heredados, `CallbackHandler` con 15+ métodos). Funciona, pero se siente como Python escrito en Go.

Daneel está diseñado **desde cero para Go**: interfaces de 1-2 métodos, functional options, `context.Context`, channels, zero deps en core. Es la diferencia entre usar un ORM portado de Java y usar `database/sql` nativo.

---

## Contributing — How to Add New Components

### Adding a New Provider

1. Create `provider/myprovider/myprovider.go`
2. Implement `daneel.Provider` interface (single `Chat()` method)
3. Optionally implement `daneel.StreamProvider` for streaming
4. Map your provider's tool call format to/from `daneel.ToolCall`
5. Map your provider's message format to/from `daneel.Message`
6. Add tests with `provider/mock` for unit tests + build-tagged integration tests
7. Add pricing constants to `provider/pricing.go`

### Adding a New Platform (Tool Pack)

1. Create `platform/myplatform/myplatform.go`
2. Define a `Tools(config) []daneel.Tool` function
3. Each tool uses `daneel.NewTool[T]` with a typed params struct
4. Keep tools focused: one action per tool, clear descriptions for the LLM
5. Include sensible rate limit defaults
6. Add examples to `examples/`

### Adding a New Connector

1. Create `connector/myplatform/connector.go`
2. Implement `daneel.Connector` interface (Start, Send, Messages, Stop)
3. Add a `Listen(config) daneel.Connector` convenience function
4. Handle reconnection internally (the Bridge shouldn't need to retry)
5. Respect `context.Context` for graceful shutdown

### Adding a New Memory Backend

1. Create `memory/store/mystore.go`
2. Implement the vector store interface: `Store(vectors)`, `Search(query, topK)`, `Delete(ids)`
3. Add connection pooling if applicable
4. Add tests with both unit tests and build-tagged integration tests

### Adding a New Workflow Pattern

1. Create `workflow/mypattern.go`
2. Implement as a pure function: `func MyPattern(ctx, input, agents...) (*RunResult, error)`
3. Use `daneel.Run()` internally — compose, don't reimplement the loop
4. Handle errors, context cancellation, and partial results

---

## The "Wow Moment" — Full Example

A multi-platform support bot in ~25 lines:

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/daneel-ai/daneel"
    "github.com/daneel-ai/daneel/bridge"
    "github.com/daneel-ai/daneel/platform/github"
    "github.com/daneel-ai/daneel/platform/slack"
    "github.com/daneel-ai/daneel/platform/twitter"
    "github.com/daneel-ai/daneel/memory"
)

func main() {
    ctx := context.Background()

    support := daneel.New("support",
        daneel.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
        daneel.WithInstructions("You're a friendly support bot for Acme Corp"),
        daneel.WithTools(daneel.MergeTools(
            twitter.Tools(os.Getenv("TWITTER_TOKEN")),
            slack.Tools(os.Getenv("SLACK_TOKEN")),
            github.Tools(os.Getenv("GITHUB_TOKEN")),
        )...),
        daneel.WithPermissions(
            daneel.AllowTools("twitter.reply", "twitter.search", "slack.send_message", "github.comment"),
            daneel.DenyTools("twitter.follow", "github.merge_pr"),
        ),
        daneel.WithMaxTurns(10),
        daneel.WithMemory(memory.Sliding(20)),
    )

    if err := bridge.Multi(ctx, support,
        twitter.Listen(os.Getenv("TWITTER_TOKEN")),
        slack.Listen(os.Getenv("SLACK_TOKEN")),
        github.Listen(os.Getenv("GITHUB_TOKEN"), github.WebhookPort(8080)),
    ); err != nil {
        log.Fatal(err)
    }
}
```

---

## Dependencies

### Dependency Philosophy

> **"A little copying is better than a little dependency."** — Go Proverbs

Daneel follows a strict dependency policy:

1. **stdlib first**: If Go's stdlib can do it, use it. `net/http` for REST APIs, `encoding/json` for serialization, `net/smtp` for SMTP, `crypto/hmac` for webhook signatures.
2. **Only for complex protocols**: External deps only when reimplementing would be unreasonable (WhatsApp's multi-device WebSocket protocol, IMAP state machine, MCP spec compliance).
3. **Interfaces for optional deps**: Heavy deps (OTEL, vector DBs) are behind interfaces. Users import the adapter they need. The core compiles with zero deps.
4. **Vendored micro-helpers over big SDKs**: Instead of importing `google/go-github` (11k★, massive dep tree), we use `net/http` + `encoding/json` against GitHub's REST API directly. Same for Twitter, Slack, Telegram — they're all simple REST APIs.

### What we use and why

| Layer | Dependency | Why not stdlib? | Imported by |
|---|---|---|---|
| **Core** | — (zero deps) | stdlib only: `encoding/json`, `reflect`, `context`, `sync`, `log/slog`, `errors`, `fmt` | `daneel` |
| **Config** | `encoding/json` (stdlib) | JSON config instead of YAML — one less dep, Go has native support | `daneel` |
| **Providers** | `net/http` (stdlib) | All LLM APIs are HTTP+JSON. No SDK needed. | `provider/*` |
| **Twitter** | `net/http` (stdlib) | Twitter API v2 is simple REST+JSON. OAuth via `crypto/hmac` + stdlib. | `platform/twitter`, `connector/twitter` |
| **GitHub** | `net/http` (stdlib) | GitHub REST API is well-documented JSON. Webhook validation via `crypto/hmac`. | `platform/github`, `connector/github` |
| **Slack** | `net/http` + `nhooyr.io/websocket` | REST for actions. WebSocket needed for Socket Mode (real-time, no public URL). stdlib has no WebSocket. | `platform/slack`, `connector/slack` |
| **Telegram** | `net/http` (stdlib) | Telegram Bot API is pure REST+JSON. Long-polling is just HTTP with timeout. | `platform/telegram`, `connector/telegram` |
| **Email (SMTP)** | `net/smtp` (stdlib) | stdlib handles SMTP natively. | `platform/email` |
| **Email (IMAP)** | `emersion/go-imap` | IMAP is a complex stateful protocol with IDLE, flags, MIME parsing. Can't reasonably reimplement. | `connector/email` |
| **WhatsApp** | `tulir/whatsmeow` | Multi-device WhatsApp uses a proprietary binary WebSocket protocol with Signal encryption. Impossible to reimplement. | `platform/whatsapp`, `connector/whatsapp` |
| **MCP** | `mark3labs/mcp-go` | MCP spec has stdio/SSE/HTTP transports, JSON-RPC, capability negotiation. The reference Go impl (8.2k★). | `mcp/` |
| **WebSocket** | `nhooyr.io/websocket` | Used only by Slack Socket Mode. Minimal, well-maintained (4.5k★), no CGO. Only imported if you use Slack. | `connector/slack` |
| **Vector search** | — (built-in) | Cosine similarity in ~50 lines of Go. No external dep for basic vector memory. | `memory/store` |
| **Rate limiting** | `golang.org/x/time/rate` (pseudo-stdlib) | Token bucket rate limiter. Part of Go extended stdlib (`x/`). Tiny, no transitive deps. | `platform/*` |
| **OTEL** | `go.opentelemetry.io/otel` (optional) | Industry standard. Behind `trace.Tracer` interface — users import OTEL adapter only if they want tracing. | `trace/otel` (optional adapter) |

### Deps eliminated vs. original plan

| Original dep | Stars | Replaced with | Why |
|---|---|---|---|
| `gopkg.in/yaml.v3` | — | `encoding/json` (stdlib) | JSON config works fine. One less dep. Go has native JSON. |
| `michimani/gotwi` | 155 | `net/http` | Twitter v2 is ~10 endpoints of REST+JSON. Not worth a dep. |
| `google/go-github` | 11.1k | `net/http` | GitHub REST API is well-documented. The SDK pulls a huge dep tree. |
| `slack-go/slack` | 4.9k | `net/http` + `nhooyr.io/websocket` | REST part is trivial. Only WebSocket for Socket Mode needs a dep. |
| `tucnak/telebot` | 4.6k | `net/http` | Telegram Bot API is the simplest REST API imaginable. |
| `wneessen/go-mail` | 1.3k | `net/smtp` (stdlib) | stdlib handles SMTP. For advanced features, users can wrap their own. |
| `philippgille/chromem-go` | 862 | Built-in (~50 LOC) | Cosine similarity is trivial. Persistence via `encoding/gob`. |
| `qdrant/go-client` | 305 | Interface only | Adapter package. Users import if they need Qdrant. Not a core dep. |
| `open-telemetry/opentelemetry-go` | 6.3k | Interface only | `trace/` defines interface. `trace/otel/` adapter imports OTEL. Core doesn't. |

### Final dependency count

| Component | External deps |
|---|---|
| **Core** (`daneel` package) | **0** |
| **All providers** (`provider/*`) | **0** (stdlib `net/http`) |
| **Twitter, GitHub, Telegram, Email-SMTP** | **0** (stdlib) |
| **Slack** (Socket Mode) | **1** (`nhooyr.io/websocket`) |
| **WhatsApp** | **1** (`tulir/whatsmeow` — complex protocol, no alternative) |
| **Email-IMAP** | **1** (`emersion/go-imap` — complex protocol) |
| **MCP** | **1** (`mark3labs/mcp-go` — spec compliance) |
| **OTEL tracing** (optional) | **1** (`go.opentelemetry.io/otel`) |

**Total: 5 required deps** (if you use all platforms) + 1 optional (OTEL). The core library and all providers have **zero external dependencies**.

> **Note**: `golang.org/x/time/rate` is pseudo-stdlib (maintained by the Go team, same review process). Platform dependencies are only pulled when you import their subpackage. If you only use `daneel` + `provider/openai`, your binary has zero external deps.

---

## Implementation Phases

| Phase | Modules | Goal | Status |
|---|---|---|---|
| **v0.1** | Core (agent, tool, permission, runner, message, errors, session, context mgmt) + OpenAI provider + Structured Output | Working agent loop with permissions | ✅ Done |
| **v0.2** | Memory (sliding + built-in vector) + Workflows (chain, parallel, router, orchestrator) | Agents with memory and composition | ✅ Done |
| **v0.3** | Rest of providers (Anthropic, Google, Ollama) + Provider fallback/routing | Universal provider support | ✅ Done |
| **v0.4** | GitHub tool pack + Telegram connector + Bridge | First platform integration end-to-end | ✅ Done |
| **v0.5** | Rest of platforms (Twitter, Slack, WhatsApp, Email) + MCP client | Full platform + MCP coverage | ✅ Done |
| **v0.6** | Fine-tune pipeline + Observability (OTEL) | Train custom models, debug agents | ✅ Done |
| **v0.7** | Approval + Multi-modal + Registry + MCP server + Mock provider/connector | Human-in-the-loop, testing, future CLI support | ✅ Done |
| **v0.8** | Persistent sessions + Knowledge base (RAG pipeline) + Cron triggers | Memory persistence, document knowledge, scheduled runs | ✅ |
| **v0.9** | WebSocket server + A/B testing + Agent-to-agent messaging (pub/sub) | Real-time UIs, experimentation, async agent comms | ✅ |
| **v1.0** | Multi-tenant + Billing/cost tracking + State machines + CLI tool | Production-ready with tenancy, budgets, and tooling | ✅ |

> **Rationale for phase order**: v0.1–v0.3 build a rock-solid core (agent loop, memory, providers) before touching any platform. v0.4–v0.7 add platform integrations, fine-tuning, observability, human-in-the-loop, and testing infrastructure. v0.8–v1.0 focus on production readiness: persistence, knowledge management, real-time capabilities, multi-tenancy, and operational tooling.

---

## Design Decisions

| # | Decision | Rationale |
|---|---|---|
| 1 | **Generics for tools** (`NewTool[T]`) | Auto JSON Schema from Go structs via `reflect`. Requires Go 1.24+. Massive DX improvement over manual schema. Uses reflection only at tool creation time, not per call. |
| 2 | **Permissions as data, not policy engine** | Declarative allow/deny lists per agent. No OPA/Rego. Covers 90% of cases. A `PermissionFunc` escape hatch covers the rest. |
| 3 | **Handoffs as special Tools** | The LLM decides when to handoff naturally. Runner intercepts the tool call. No separate mechanism. Follows the composition-over-abstraction principle. |
| 4 | **Providers via minimal interface** | Single `Chat()` method. Adding a new provider = implementing one method. Accept interfaces, return structs. |
| 5 | **Workflows as functions, not DSL** | `workflow.Chain()` etc. are normal Go functions. More idiomatic and debuggable than graph DSLs. Compose with standard Go control flow. |
| 6 | **JSON config, not YAML** | `encoding/json` is stdlib. YAML requires `gopkg.in/yaml.v3`. JSON is one less dependency and Go has native support. Config files use `.json` with `${ENV_VAR}` expansion. |
| 7 | **`log/slog` for logging** | stdlib since Go 1.21. Zero dependency. Users configure their own handler. No logger interface to define — slog IS the interface. |
| 8 | **`context.Context` everywhere** | Cancellation, timeouts, tracing — all via standard Go context. No custom cancellation mechanism. |
| 9 | **Tool Packs as functions** | `twitter.Tools(token)` returns `[]Tool`. Composable, no coupling. Pick what you need. |
| 10 | **`net/http` for REST platforms** | Twitter, GitHub, Slack, Telegram are all REST+JSON. Using `net/http` directly eliminates 4 large SDK dependencies (~22k combined stars but huge dep trees). ~200 LOC per platform vs. importing thousands of files. |
| 11 | **Connectors with channels** | `Messages() <-chan IncomingMessage` — idiomatic Go. Composes with `select` for multi-platform. |
| 12 | **WhatsApp via whatsmeow** | No official Go SDK. Proprietary binary protocol with Signal encryption. Can't reimplement. 5.4k stars, basis for many bridges. ToS risk documented. |
| 13 | **MCP via mcp-go** | MCP spec has JSON-RPC, multiple transports, capability negotiation. The reference Go implementation. Worth the dep for spec compliance. |
| 14 | **Library, not framework** | No stdout, no os.Exit, no stdin. Pure API returning structs and errors. Future CLI is a separate project. |
| 15 | **Built-in vector search** | Cosine similarity in ~50 LOC of Go. `encoding/gob` for persistence. No external vector DB needed for basic RAG. Qdrant/Weaviate adapters exist but behind interface. |
| 16 | **OTEL behind interface** | `trace/` defines a `Tracer` interface. `trace/otel/` is an adapter that imports OTEL. Users who don't want tracing pay zero dep cost. |
| 17 | **Functional options, not builders** | `daneel.New("name", WithX(), WithY())` — the standard Go pattern. Extensible without breaking changes. Each option is an independent function. |
| 18 | **Fine-tuning via Python subprocess** | No native Go fine-tuning exists. Unsloth (52k ★) is the standard. Go orchestrates, Python trains. The user never touches Python. |
| 19 | **No init(), no global state** | Everything is explicit. No package-level `Register()` calls. No hidden initialization. Tests are deterministic. |
| 20 | **Table-driven tests** | All test files use Go's table-driven test pattern. Subtests with `t.Run()`. Parallel where safe. Benchmarks for hot paths. |
| 21 | **Structured output via generics** | `RunStructured[T]` reuses the same JSON Schema generation as `NewTool[T]`. Providers map to their native structured output mechanism. Fallback to prompt+retry. |
| 22 | **Session as first-class concept** | Every `Run()` has a session ID (auto or explicit). Enables memory scoping, tracing correlation, cost tracking per conversation. |
| 23 | **Context window auto-management** | Runner estimates token count and truncates/summarizes history before sending to provider. User never hits unexpected context overflow errors. |
| 24 | **Dynamic context injection** | `WithContextFunc` evaluated at runtime, not creation time. Enables user-specific, time-specific, state-specific system prompts. |

### Go Best Practices Enforced

| Practice | How Daneel applies it |
|---|---|
| **Accept interfaces, return structs** | `Provider`, `Memory`, `Connector` are interfaces. `Agent`, `Tool`, `RunResult`, `Config` are structs. |
| **Small interfaces** | `Provider`: 1 method. `Memory`: 3 methods. `Connector`: 4 methods. |
| **Errors are values** | Sentinel errors (`ErrPermissionDenied`), typed errors (`PermissionError`), `errors.Is`/`errors.As`. Never strings. |
| **Don't panic** | `recover()` in Runner for tool panics. Library never panics. Returns errors. |
| **Make the zero value useful** | `RunOptions{}` uses sensible defaults (MaxTurns=25, Sequential tool execution). |
| **Concurrency via goroutines + channels** | Bridge uses goroutines per message. Connectors use channels. `errgroup` for parallel workflows. |
| **Context for cancellation** | Every public function takes `context.Context` as first arg. No custom timeout mechanisms. |
| **Package naming** | Short, lowercase, no underscores: `daneel`, `provider`, `platform`, `workflow`, `memory`. |
| **Godoc as documentation** | Every exported type/func has a godoc comment. Examples in `_test.go` files with `Example` prefix. |
| **Testable by design** | Interfaces everywhere = mock anything. Built-in mock provider. No file I/O in core. |

---

## Future CLI (Separate Project)

The library exposes a `Registry` for introspection that a future CLI will consume:

```go
reg := daneel.NewRegistry()
reg.RegisterAgent(supportAgent)
reg.RegisterPlatform("twitter", twitter.Tools(token))

reg.Agents()     // []AgentInfo — name, tools, permissions
reg.Platforms()  // []PlatformInfo — name, available tools
reg.Tools()      // []ToolInfo — name, description, JSON schema
```

This enables CLI commands like:
- `daneel agents list`
- `daneel tools describe twitter.tweet`
- `daneel run support "handle this ticket"`
- `daneel listen --twitter --slack --telegram`
- `daneel finetune --dataset conversations.jsonl --base llama3`

---

---

## v0.8 — Persistent Sessions + Knowledge Base + Cron Triggers

### Persistent Sessions

Resume agent conversations across process restarts. Sessions serialize `[]Message` + metadata to durable storage.

```go
import "github.com/daneel-ai/daneel/session"

// File-based persistence (one JSON file per session)
store := session.NewFileStore("/var/daneel/sessions")

// Attach to agent
agent := daneel.New("support",
    daneel.WithSessionStore(store),
)

// Run with a named session — automatically loads/saves history
result, err := daneel.Run(ctx, agent, "what was my last question?",
    daneel.WithSession("user-123"),
)

// List and manage sessions
sessions, _ := store.List(ctx)
store.Delete(ctx, "user-123")
```

#### Package: `session/`

| File | Description | ~LOC |
|---|---|---|
| `session/store.go` | `SessionStore` interface: `Save(ctx, id, []Message, Metadata)`, `Load(ctx, id)`, `List(ctx)`, `Delete(ctx, id)`. `Metadata` struct (CreatedAt, UpdatedAt, AgentName, TurnCount). `SessionData` struct. | ~80 |
| `session/file.go` | `FileStore` implementation: directory-based, one `.json` file per session, atomic writes via temp+rename, optional TTL cleanup. | ~120 |
| `session/memory.go` | `MemoryStore` implementation: in-memory `sync.Map`-based, for testing and short-lived processes. | ~60 |

Core changes:
- Add `WithSessionStore(s SessionStore)` as `AgentOption` in `options.go`
- Runner checks `SessionStore` before `Memory` — if store exists, load history from it and save after run
- `SessionStore` is complementary to `Memory`: store persists raw history, memory provides semantic retrieval

### Knowledge Base (RAG Pipeline)

Built-in document ingestion, chunking, embedding, and retrieval. Connects to the existing `VectorStore` + `Embedder` interfaces.

```go
import "github.com/daneel-ai/daneel/knowledge"

// Create a knowledge base backed by the built-in vector store
kb := knowledge.New(
    knowledge.WithEmbedder(openaiEmbedder),
    knowledge.WithStore(vectorStore),
    knowledge.WithChunker(knowledge.Recursive(500, 50)), // 500 tokens, 50 overlap
)

// Ingest documents
kb.IngestFile(ctx, "docs/manual.pdf")
kb.IngestURL(ctx, "https://docs.example.com/api")
kb.IngestText(ctx, "Company policy: ...", knowledge.WithSource("policy-v2"))

// Attach to agent as dynamic context
agent := daneel.New("assistant",
    daneel.WithContextFunc(kb.Retriever(5)), // top-5 relevant chunks
)

// Or use as a standalone tool
agent := daneel.New("assistant",
    daneel.WithTools(kb.SearchTool("search_docs", "Search internal documentation", 5)),
)
```

#### Package: `knowledge/`

| File | Description | ~LOC |
|---|---|---|
| `knowledge/knowledge.go` | `KnowledgeBase` struct, `New()`, `Option` funcs (`WithEmbedder`, `WithStore`, `WithChunker`). `Ingest*` methods. `Retriever()` returns `func(ctx) (string, error)` for `WithContextFunc`. `SearchTool()` returns `daneel.Tool`. | ~150 |
| `knowledge/chunker.go` | `Chunker` interface: `Chunk(text string) []Chunk`. Implementations: `FixedSize(maxTokens)`, `Paragraph()` (split on `\n\n`), `Recursive(maxTokens, overlap)` (split on separators hierarchy: `\n\n` → `\n` → `. ` → ` `). `Chunk` struct (Text, Start, End, Source). | ~120 |
| `knowledge/source.go` | `Source` types and document loading. `LoadFile(path)` (reads text, future: PDF/markdown extraction). `LoadURL(url)` (HTTP GET + HTML-to-text). `IngestOption` funcs (`WithSource`, `WithMetadata`). | ~80 |
| `knowledge/document.go` | `Document` struct (Content, Source, Metadata). `Embedding` struct (ID, Vector, DocumentID, ChunkIndex, Text). Batch embedding with rate limiting. | ~60 |

Dependencies: zero new deps (HTTP for URLs uses stdlib, text extraction is basic — advanced PDF parsing is behind interface for users to plug in).

### Cron Triggers

Schedule agent runs with cron expressions. Pure Go cron parser (no external dep).

```go
import "github.com/daneel-ai/daneel/cron"

scheduler := cron.New()

// Daily report at 9am
scheduler.Schedule("0 9 * * *", agent, "Generate the daily KPI report",
    cron.WithSession("daily-report"),
    cron.WithCallback(func(result *daneel.RunResult, err error) {
        if err != nil {
            slog.Error("cron failed", "error", err)
            return
        }
        // Send report via Slack, email, etc.
        slackClient.Send(ctx, "#reports", result.Output)
    }),
)

// Every 30 minutes: check for new support tickets
scheduler.Every(30*time.Minute, triageAgent, "Check for new unassigned tickets")

// Start scheduler (blocks until ctx cancelled)
scheduler.Start(ctx)
```

#### Package: `cron/`

| File | Description | ~LOC |
|---|---|---|
| `cron/cron.go` | `Scheduler` struct, `New()`, `Schedule(expr, agent, input, opts...)`, `Every(duration, agent, input, opts...)`, `Start(ctx)`, `Stop()`. Uses goroutine per job with ticker. | ~120 |
| `cron/parser.go` | Cron expression parser (standard 5-field: minute, hour, day, month, weekday). `Parse(expr) (Schedule, error)`. `Schedule.Next(time.Time) time.Time`. Supports `*`, ranges (`1-5`), steps (`*/15`), lists (`1,3,5`). No external deps. | ~150 |
| `cron/job.go` | `Job` struct (ID, Expression, Agent, Input, Options, LastRun, NextRun, RunCount, Errors). `CronOption` funcs (`WithSession`, `WithCallback`, `WithMaxRetries`, `WithTimeout`). | ~60 |

---

## v0.9 — WebSocket Server + A/B Testing + Agent-to-Agent Messaging

### WebSocket Server

Built-in WebSocket endpoint for real-time chat UIs. Uses `nhooyr.io/websocket` (already a dependency via Slack connector).

```go
import "github.com/daneel-ai/daneel/ws"

// Create a WebSocket server for an agent
server := ws.NewServer(agent,
    ws.WithPath("/chat"),
    ws.WithAuth(func(r *http.Request) bool {
        return r.Header.Get("Authorization") != ""
    }),
    ws.WithOnConnect(func(sessionID string) {
        slog.Info("client connected", "session", sessionID)
    }),
)

// Mount on existing HTTP server
mux.Handle("/chat", server)

// Or standalone
server.ListenAndServe(ctx, ":8080")
```

Protocol (JSON over WebSocket):

```json
// Client → Server
{"type": "message", "content": "Hello", "session_id": "abc"}

// Server → Client (streaming)
{"type": "token", "content": "Hi"}
{"type": "token", "content": " there!"}
{"type": "tool_call", "tool": "search", "args": {"q": "..."}}
{"type": "tool_result", "tool": "search", "content": "..."}
{"type": "done", "content": "Hi there! Here's what I found...", "session_id": "abc"}

// Server → Client (error)
{"type": "error", "content": "rate limit exceeded"}
```

#### Package: `ws/`

| File | Description | ~LOC |
|---|---|---|
| `ws/server.go` | `Server` struct, `NewServer(agent, opts...)`, `ListenAndServe(ctx, addr)`, `Handler() http.Handler`. Manages WebSocket connections. Each connection gets a session. Streams tokens via `WithStreaming`. | ~180 |
| `ws/conn.go` | `connection` struct wrapping a single WebSocket conn. Read/write JSON messages. Heartbeat/ping. Graceful close. | ~100 |
| `ws/protocol.go` | Message types: `ClientMessage`, `ServerMessage`. Type constants. Serialization helpers. | ~40 |
| `ws/connector.go` | `WSConnector` implementing `daneel.Connector` so Bridge can use WebSocket as another channel alongside Telegram, Slack, etc. | ~80 |

Dependency: `nhooyr.io/websocket` (already in go.mod via Slack).

### A/B Testing

Run two agent configurations against the same input and compare outcomes.

```go
import "github.com/daneel-ai/daneel/experiment"

// Compare two agent configurations
result, err := experiment.ABTest(ctx,
    "Summarize this article: ...",
    agentGPT4,
    agentClaude,
    experiment.WithJudge(judgeAgent), // LLM-as-judge
    experiment.WithMetrics(experiment.Latency, experiment.TokenCount, experiment.Cost),
    experiment.WithRuns(10), // run each config 10 times
)

fmt.Printf("Winner: %s (score: %.2f vs %.2f)\n",
    result.Winner, result.ScoreA, result.ScoreB)
result.ExportCSV("ab_results.csv")

// Batch evaluation over a dataset
results, err := experiment.Evaluate(ctx, dataset,
    experiment.Candidate("gpt4o", agentGPT4),
    experiment.Candidate("claude", agentClaude),
    experiment.WithJudge(judgeAgent),
    experiment.WithConcurrency(5),
)
```

#### Package: `experiment/`

| File | Description | ~LOC |
|---|---|---|
| `experiment/ab.go` | `ABTest(ctx, input, agentA, agentB, opts...)`. Runs both agents, collects metrics. Optional LLM-as-judge comparison. Returns `ABResult{ScoreA, ScoreB, Winner, Metrics, Runs}`. | ~120 |
| `experiment/evaluate.go` | `Evaluate(ctx, dataset, candidates, opts...)`. Batch evaluation: run multiple agents over a dataset, score each. `Candidate(name, agent)`. `EvalResults` with `ExportCSV`, `ExportJSON`. | ~130 |
| `experiment/metrics.go` | Metric types: `Latency`, `TokenCount`, `Cost`, `ToolCalls`, `Turns`. `MetricCollector` that wraps Run and captures stats. | ~60 |
| `experiment/judge.go` | `WithJudge(agent)`: uses an LLM agent to score/compare outputs. Generates structured prompt: "Given input X, compare output A vs B. Which is better? Score 1-10." | ~80 |

### Agent-to-Agent Messaging

Direct pub/sub between agents — asynchronous fan-out, unlike handoffs (which are synchronous 1-to-1 transfers).

```go
import "github.com/daneel-ai/daneel/pubsub"

bus := pubsub.New()

// Agent A publishes when it finds something interesting
monitorAgent := daneel.New("monitor",
    daneel.WithTools(
        pubsub.PublishTool(bus, "alerts"), // tool: publish to "alerts" topic
    ),
)

// Agent B subscribes and reacts
alertAgent := daneel.New("alerter", daneel.WithInstructions("Handle alerts"))
bus.Subscribe("alerts", alertAgent) // auto-runs agent when message arrives

// Start the bus (routes messages to subscribers)
bus.Start(ctx)
```

#### Package: `pubsub/`

| File | Description | ~LOC |
|---|---|---|
| `pubsub/bus.go` | `Bus` struct, `New()`, `Publish(ctx, topic, msg)`, `Subscribe(topic, agent)`, `Unsubscribe(topic, agent)`, `Start(ctx)`, `Stop()`. Channel-based routing. Fan-out to all subscribers. | ~120 |
| `pubsub/tools.go` | `PublishTool(bus, topic)` returns a `daneel.Tool` that agents can call to publish messages. `SubscribeTool(bus, topics...)` returns a tool for listing available topics. | ~60 |
| `pubsub/message.go` | `PubSubMessage{Topic, Content, From, Timestamp, Metadata}`. Serializable. | ~30 |

---

## v1.0 — Multi-Tenant + Billing + State Machines + CLI

### Multi-Tenant Isolation

Isolation between different users/organizations. All resources (sessions, memory, metrics, costs) are scoped by tenant.

```go
import "github.com/daneel-ai/daneel/tenant"

// Create a tenant manager
tm := tenant.NewManager(
    tenant.WithSessionStore(fileStore),
    tenant.WithQuota(tenant.Quota{MaxRunsPerHour: 100, MaxCostPerDay: 10.0}),
)

// Register tenants
tm.Register("acme-corp", tenant.Config{Model: "gpt-4o", MaxTurns: 15})
tm.Register("startup-inc", tenant.Config{Model: "gpt-4o-mini", MaxTurns: 25})

// Run with tenant context — auto-scopes sessions, memory, metrics
result, err := daneel.Run(ctx, agent, "help me",
    tenant.WithTenant(tm, "acme-corp"),
)

// Tenant usage
usage, _ := tm.Usage(ctx, "acme-corp")
fmt.Printf("Runs today: %d, Cost: $%.2f\n", usage.RunsToday, usage.CostToday)
```

#### Package: `tenant/`

| File | Description | ~LOC |
|---|---|---|
| `tenant/manager.go` | `Manager` struct, `NewManager(opts...)`, `Register(id, Config)`, `Get(id)`, `Usage(ctx, id)`, `ListTenants()`. Thread-safe with `sync.RWMutex`. | ~120 |
| `tenant/config.go` | `Config{Model, MaxTurns, MaxTokens, AllowedTools, DeniedTools}`. `Quota{MaxRunsPerHour, MaxRunsPerDay, MaxCostPerDay, MaxCostPerMonth}`. `Usage{RunsToday, RunsThisHour, CostToday, CostThisMonth, TokensUsed}`. | ~60 |
| `tenant/middleware.go` | `WithTenant(manager, tenantID)` as `RunOption`. Injects tenant config into the run, scopes session IDs with tenant prefix, checks quotas before execution, records usage after. | ~100 |
| `tenant/scope.go` | `ScopedMemory` and `ScopedSessionStore` wrappers that prefix all keys with tenant ID for isolation. | ~60 |

### Billing & Cost Tracking

Track costs per tenant/session with per-model pricing tables. Set budgets and alerts.

```go
import "github.com/daneel-ai/daneel/billing"

tracker := billing.NewTracker(
    billing.WithPricing(billing.OpenAIPricing()),   // built-in pricing tables
    billing.WithBudget("acme-corp", 100.0),         // $100/month
    billing.WithAlert(billing.AtPercent(80), func(tenant string, spent, budget float64) {
        slog.Warn("budget alert", "tenant", tenant, "spent", spent, "budget", budget)
    }),
)

// Attach to agent
agent := daneel.New("assistant",
    daneel.WithOnConversationEnd(tracker.Record), // auto-records costs
)

// Query costs
cost, _ := tracker.Cost(ctx, "acme-corp", billing.ThisMonth)
fmt.Printf("Cost: $%.4f (prompt: $%.4f, completion: $%.4f)\n",
    cost.Total, cost.Prompt, cost.Completion)

// Export
tracker.ExportCSV(ctx, "costs.csv", billing.ThisMonth)
```

#### Package: `billing/`

| File | Description | ~LOC |
|---|---|---|
| `billing/tracker.go` | `Tracker` struct, `NewTracker(opts...)`, `Record(ctx, RunResult)` (implements the `OnConversationEnd` callback), `Cost(ctx, tenant, period)`, `ExportCSV()`. | ~130 |
| `billing/pricing.go` | `PricingTable` map of model → price per token (prompt/completion). Built-in tables: `OpenAIPricing()`, `AnthropicPricing()`, `GooglePricing()`. `CustomPricing(model, promptPrice, completionPrice)`. | ~80 |
| `billing/budget.go` | `Budget{Tenant, Limit, Period}`. `Alert{Threshold, Callback}`. Budget checking in `Record()`. `AtPercent(pct)`, `AtAmount(usd)` threshold helpers. | ~80 |
| `billing/cost.go` | `CostRecord{Tenant, SessionID, Model, PromptTokens, CompletionTokens, PromptCost, CompletionCost, Total, Timestamp}`. `CostSummary{Total, Prompt, Completion, Runs, Period}`. `Period` type (ThisMonth, LastMonth, Today, Custom). | ~60 |

### State Machines

Define complex agent behavior as finite state machines with agent-per-state.

```go
import "github.com/daneel-ai/daneel/workflow"

// Define a support ticket FSM
fsm := workflow.NewFSM("ticket-handler",
    workflow.State("triage", triageAgent,
        workflow.On("escalate", "escalation"),
        workflow.On("resolve", "resolution"),
        workflow.On("need_info", "info_gathering"),
    ),
    workflow.State("info_gathering", infoAgent,
        workflow.On("info_received", "triage"),
    ),
    workflow.State("escalation", escalationAgent,
        workflow.On("resolved", "resolution"),
    ),
    workflow.State("resolution", resolutionAgent), // terminal state
    workflow.WithInitialState("triage"),
    workflow.WithMaxTransitions(20),
)

// Run the FSM
result, err := fsm.Run(ctx, "Customer can't log in")
fmt.Printf("Final state: %s, Path: %v\n", result.FinalState, result.Path)
```

Transitions are triggered by the agent's output matching a keyword or by a classifier function.

#### Implementation in `workflow/`

| File | Description | ~LOC |
|---|---|---|
| `workflow/fsm.go` | `FSM` struct, `NewFSM(name, states...)`, `Run(ctx, input)`. State loop: run current state's agent → match output to transitions → move to next state. `FSMResult{FinalState, Path, Messages, Duration}`. | ~150 |
| `workflow/state.go` | `StateDef` struct, `State(name, agent, transitions...)` constructor. `Transition{Event, Target}`. `On(event, targetState)`. `TransitionFunc(fn)` for custom logic. `WithInitialState()`, `WithMaxTransitions()`. | ~80 |

### CLI Tool (Separate Binary)

A CLI that consumes the `Registry` for agent introspection and management.

```bash
# List registered agents
$ daneel agents list
  NAME        TOOLS   HANDOFFS   MAX_TURNS
  support     8       2          25
  reviewer    5       0          15

# Describe a tool
$ daneel tools describe twitter.tweet
  Name:   twitter.tweet
  Desc:   Post a tweet
  Schema: {"type":"object","properties":{"text":{"type":"string"}}}

# Run an agent interactively
$ daneel run support
  > What's the status of ticket #1234?
  Agent: The ticket is currently open and assigned to...

# Run with a single prompt
$ daneel run support "Summarize open tickets" --model gpt-4o

# Listen on multiple platforms
$ daneel listen --twitter --slack --telegram

# Fine-tune from collected data
$ daneel finetune --dataset conversations.jsonl --base llama3
```

#### Package: `cmd/daneel/`

Separate Go module (`cmd/daneel/`) — not part of the library. Uses `daneel.Registry` for discovery.

| File | Description | ~LOC |
|---|---|---|
| `cmd/daneel/main.go` | Entry point, root command, config loading (JSON config file with `${ENV_VAR}` expansion). | ~80 |
| `cmd/daneel/agents.go` | `daneel agents list`, `daneel agents describe <name>`. Table output via `text/tabwriter`. | ~60 |
| `cmd/daneel/tools.go` | `daneel tools list`, `daneel tools describe <name>`. | ~50 |
| `cmd/daneel/run.go` | `daneel run <agent> [prompt]`. Interactive REPL mode if no prompt given. Flags: `--model`, `--max-turns`, `--session`. | ~100 |
| `cmd/daneel/listen.go` | `daneel listen --twitter --slack ...`. Starts Bridge with selected connectors. | ~80 |
| `cmd/daneel/finetune.go` | `daneel finetune --dataset --base --method`. Wraps `finetune.Trainer`. | ~60 |
| `cmd/daneel/config.go` | JSON config file loading. Agent definitions, platform tokens, model preferences. `${ENV_VAR}` expansion. | ~70 |

Dependency: Uses stdlib `flag` package (no cobra/urfave) to stay consistent with the zero-unnecessary-deps philosophy.

---

## Roadmap (Post-v1)

Ideas for future exploration beyond v1.0:

| Idea | Description |
|---|---|
| **Dashboard** | Web UI for monitoring agents, conversations, metrics (separate project) |
| **Wasm plugins** | Load tools as Wasm modules via wazero (sandboxed execution) |
| **go-plugin support** | HashiCorp go-plugin for out-of-process tools |
| **Guardrails DSL** | Declarative YAML/JSON guardrail definitions instead of code |
| **Agent marketplace** | Registry of community-contributed agents and tool packs |
| **Distributed agents** | Run agents across multiple nodes with work distribution |
| **Conversation branching** | Fork conversations, explore alternative paths, merge results |

---

## Release Plan — Publicación v1.0.0

Objetivo: dejar el módulo publicado en pkg.go.dev con documentación, licencia y listo para `go get`.

### Fase R1 — Archivos de Publicación

| # | Tarea | Archivo(s) | Descripción | Status |
|---|---|---|---|---|
| R1.1 | Licencia MIT | `LICENSE` | Año 2026, titular "The Daneel Authors". Texto MIT estándar. | ✅ |
| R1.2 | README.md | `README.md` | Logo/título, badges (Go Reference, Go Report Card, License), descripción corta, features, quickstart con código, tabla de paquetes (39), links a docs. | ✅ |
| R1.3 | doc.go raíz | `doc.go` | Package-level godoc para `package daneel` con Overview, ejemplo mínimo y links. Aparece en pkg.go.dev como descripción principal. | ✅ |
| R1.4 | CONTRIBUTING.md | `CONTRIBUTING.md` | Guía breve: fork, branch, tests, PR. | ✅ |
| R1.5 | .gitignore | `.gitignore` | Ignorar binarios, `_*.py`, `.env`, `*.test`, etc. | ✅ |

### Fase R2 — Git & GitHub

| # | Tarea | Descripción | Status |
|---|---|---|---|
| R2.1 | Crear repo en GitHub | `github.com/daneel-ai/daneel` (público). | ⏳ |
| R2.2 | git init + commit inicial | `git init && git add . && git commit -m "v1.0.0: initial release"` | ⏳ |
| R2.3 | Añadir remote + push | `git remote add origin git@github.com:daneel-ai/daneel.git && git push -u origin main` | ⏳ |
| R2.4 | Tag semver | `git tag v1.0.0 && git push origin v1.0.0` | ⏳ |

### Fase R3 — Go Module Proxy

| # | Tarea | Descripción | Status |
|---|---|---|---|
| R3.1 | Solicitar indexación | `GOPROXY=https://proxy.golang.org go list -m github.com/daneel-ai/daneel@v1.0.0` — fuerza la caché en el proxy. | ⏳ |
| R3.2 | Verificar pkg.go.dev | Abrir `https://pkg.go.dev/github.com/daneel-ai/daneel` y confirmar que aparece el doc, README, y todos los sub-paquetes. | ⏳ |
| R3.3 | Test de consumo | Crear un módulo temporal, `go get github.com/daneel-ai/daneel@v1.0.0`, compilar un main mínimo. | ⏳ |

### Notas

- El módulo tiene **cero dependencias externas** — solo stdlib. Esto simplifica la publicación (no hay `go.sum` complejo).
- `cmd/daneel/` es un binario que se instalará con `go install github.com/daneel-ai/daneel/cmd/daneel@v1.0.0`.
- Los badges del README apuntarán a:
  - `pkg.go.dev/github.com/daneel-ai/daneel` (Go Reference)
  - `goreportcard.com/report/github.com/daneel-ai/daneel` (Go Report Card)
  - El archivo `LICENSE` (MIT badge)

---

## License

MIT

---

*"The Laws of Robotics are not suggestions." — R. Daneel Olivaw*
