package daneel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Rafiki81/daneel/content"
)

// --- Provider interface (defined in root package, not provider/) ---

// Provider is the single-method interface for LLM backends.
// Implementations live in provider/openai, provider/anthropic, etc.
type Provider interface {
	Chat(ctx context.Context, messages []Message, tools []ToolDef) (*Response, error)
}

// StreamProvider extends Provider with streaming support.
type StreamProvider interface {
	Provider
	ChatStream(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamChunk, error)
}

// ModelInfoProvider extends Provider with model metadata.
type ModelInfoProvider interface {
	Provider
	ModelInfo(ctx context.Context) (ModelInfo, error)
}

// TokenCounter can count tokens for a set of messages. Used for
// accurate context window management.
type TokenCounter interface {
	CountTokens(ctx context.Context, messages []Message) (int, error)
}

// Response is the unified LLM response from any provider.
type Response struct {
	Content   string
	ToolCalls []ToolCall
	Usage     Usage
}

// Usage tracks token consumption for a single LLM call.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// ModelInfo describes a model's capabilities and limits.
type ModelInfo struct {
	ContextWindow  int  // max total tokens (input + output)
	MaxOutput      int  // max output tokens
	SupportsVision bool // can handle image inputs
	SupportsTools  bool // can handle tool calling
	SupportsJSON   bool // can handle structured output / response_format
}

// --- Memory interface ---

// Memory provides conversation persistence scoped by session ID.
// The Runner passes the current session ID automatically.
// Implementations live in the memory/ package.
type Memory interface {
	Save(ctx context.Context, sessionID string, msgs []Message) error
	Retrieve(ctx context.Context, sessionID string, query string, limit int) ([]Message, error)
	Clear(ctx context.Context, sessionID string) error
}

// SessionStore provides durable storage for full session histories.
// Unlike Memory (which may truncate or summarize), SessionStore always
// persists the raw []Message slice — enabling full conversation recovery
// after a process restart. Implementations live in the session/ package.
//
// SessionStore is complementary to Memory:
//   - SessionStore: raw history for persistence across restarts.
//   - Memory: semantic retrieval (sliding window, RAG) for LLM context.
type SessionStore interface {
	// LoadMessages retrieves the full message history for a session.
	// Returns nil, nil if the session does not exist yet.
	LoadMessages(ctx context.Context, sessionID string) ([]Message, error)
	// SaveMessages persists the full message history for a session.
	SaveMessages(ctx context.Context, sessionID string, msgs []Message) error
}

// --- VectorStore and Embedder interfaces ---

// VectorStore is the interface for vector similarity search backends.
type VectorStore interface {
	Store(ctx context.Context, id string, embedding []float32, metadata map[string]string) error
	Search(ctx context.Context, query []float32, topK int) ([]VectorResult, error)
	Delete(ctx context.Context, ids ...string) error
}

// VectorResult is a single result from a vector similarity search.
type VectorResult struct {
	ID       string
	Score    float32
	Metadata map[string]string
}

// Embedder converts text to vector embeddings.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// --- Connector interface ---

// Connector receives messages from external platforms (Telegram, Slack, etc.).
// Implementations live in connector/ subpackages.
type Connector interface {
	Start(ctx context.Context) error
	Send(ctx context.Context, to string, content string) error
	Messages() <-chan IncomingMessage
	Stop() error
}

// IncomingMessage is a message received from an external platform.
// Pure data struct (no closures) — can be serialized for persistence.
type IncomingMessage struct {
	Platform string
	From     string
	Content  string
	Channel  string
	Metadata map[string]any
}

// --- Tracer interface ---

// Tracer provides distributed tracing. The default implementation
// uses OpenTelemetry (see trace/ package).
type Tracer interface {
	StartSpan(ctx context.Context, name string, attrs ...Attr) (context.Context, Span)
}

// Span represents a single unit of work in a trace.
type Span interface {
	SetAttributes(attrs ...Attr)
	RecordError(err error)
	End()
}

// Attr is a key-value attribute for tracing.
type Attr struct {
	Key   string
	Value any
}

// defaultTracer is a no-op tracer used when WithTracing() is called
// without an explicit Tracer. The trace/ package provides the real impl.
type defaultTracer struct{}

func (defaultTracer) StartSpan(ctx context.Context, name string, attrs ...Attr) (context.Context, Span) {
	return ctx, nopSpan{}
}

type nopSpan struct{}

func (nopSpan) SetAttributes(attrs ...Attr) {}
func (nopSpan) RecordError(err error)       {}
func (nopSpan) End()                        {}

// --- Approval ---

// ApprovalRequest is passed to the Approver when a tool requires
// human approval before execution.
type ApprovalRequest struct {
	Agent     string          // agent requesting approval
	Tool      string          // tool it wants to call
	Args      json.RawMessage // arguments for the tool call
	SessionID string          // conversation session
}

// Approver decides whether a tool call should proceed.
type Approver interface {
	Approve(ctx context.Context, req ApprovalRequest) (bool, error)
}

// ApproverFunc adapts a function to the Approver interface.
type ApproverFunc func(ctx context.Context, req ApprovalRequest) (bool, error)

// Approve implements the Approver interface.
func (f ApproverFunc) Approve(ctx context.Context, req ApprovalRequest) (bool, error) {
	return f(ctx, req)
}

// --- Streaming types ---

// StreamChunkType indicates what kind of data a StreamChunk carries.
type StreamChunkType int

const (
	StreamText          StreamChunkType = iota // text token
	StreamToolCallStart                        // tool call beginning
	StreamToolCallDone                         // tool call completed
	StreamDone                                 // stream finished
	StreamError                                // error during stream
)

// StreamChunk is a single piece of streaming output from the LLM.
type StreamChunk struct {
	Type       StreamChunkType
	Text       string      // for StreamText
	ToolCall   *ToolCall   // for StreamToolCallStart
	ToolResult *ToolResult // for StreamToolCallDone
	Error      error       // for StreamError
}

// --- Context window strategies ---

// ContextStrategy defines how the Runner handles context window overflow.
type ContextStrategy int

const (
	// ContextSlidingWindow keeps system prompt + first user message +
	// last N messages that fit. This is the default.
	ContextSlidingWindow ContextStrategy = iota

	// ContextSummarize summarizes older messages with an LLM call,
	// keeping the summary + recent messages.
	ContextSummarize

	// ContextError returns ErrContextOverflow and lets the caller decide.
	ContextError
)

// --- Tool execution concurrency ---

// ToolExecution controls how multiple tool calls in a single LLM
// response are executed.
type ToolExecution int

const (
	// Sequential executes tools one at a time (default).
	Sequential ToolExecution = iota

	// Parallel executes all tool calls concurrently.
	Parallel
)

// ParallelN returns a ToolExecution that runs at most n tools concurrently.
func ParallelN(n int) ToolExecution {
	if n <= 0 {
		return Sequential
	}
	return ToolExecution(n + 1) // offset by 2 to distinguish from Sequential(0) and Parallel(1)
}

// parallelism returns the concurrency limit for this execution mode.
// Returns 0 for unlimited (Parallel), 1 for Sequential.
func (te ToolExecution) parallelism() int {
	switch te {
	case Sequential:
		return 1
	case Parallel:
		return 0 // unlimited
	default:
		return int(te) - 1 // undo ParallelN offset
	}
}

// --- Handoff history mode ---

// HandoffHistory controls how much conversation history is passed
// to the target agent during a handoff.
type HandoffHistory int

const (
	// FullHistory sends the complete conversation history (default).
	FullHistory HandoffHistory = iota

	// SummaryHistory summarizes the conversation with an LLM call.
	SummaryHistory
)

// LastN creates a HandoffHistory that sends the last n messages.
func LastN(n int) HandoffHistory {
	if n <= 0 {
		return FullHistory
	}
	return HandoffHistory(n + 1) // offset by 2
}

// count returns the number of messages to keep, or 0 for all.
func (hh HandoffHistory) count() int {
	switch hh {
	case FullHistory:
		return 0
	case SummaryHistory:
		return -1 // sentinel for "summarize"
	default:
		return int(hh) - 1
	}
}

// --- Response format ---

// ResponseFormat controls the response format hint sent to the LLM.
type ResponseFormat int

const (
	// ResponseText is the default free-form text response.
	ResponseText ResponseFormat = iota

	// JSON forces the LLM to respond with valid JSON.
	JSON
)

// --- RunOption (functional options for Run) ---

// RunOption configures a single call to Run or RunStructured.
type RunOption func(*runConfig)

// runConfig holds all configurable options for a Run() call.
type runConfig struct {
	sessionID      string
	sessionPrefix  string // prepended to auto-generated session IDs
	streamFn       func(StreamChunk)
	approver       Approver
	maxTurns       int // 0 = use agent default or 25
	responseFormat ResponseFormat
	responseSchema any // struct value for WithResponseSchema
	history        []Message
	images         []content.Content // multi-modal image inputs
	preRunHook     func(ctx context.Context) error
	postRunHook    func(ctx context.Context, result *RunResult)
}

func defaultRunConfig() runConfig {
	return runConfig{
		maxTurns: 0, // will default to agent's or 25
	}
}

func applyRunOptions(opts []RunOption, cfg *runConfig) {
	for _, opt := range opts {
		opt(cfg)
	}
}

// WithSession sets an explicit session ID for multi-turn persistence.
// If not set, a random UUID v4 is generated automatically.
func WithSession(id string) RunOption {
	return func(c *runConfig) {
		c.sessionID = id
	}
}

// WithSessionID is an alias for WithSession.
func WithSessionID(id string) RunOption {
	return WithSession(id)
}

// WithHistory pre-seeds the conversation with existing messages.
// Useful for Bridge and multi-turn scenarios where history is maintained externally.
func WithHistory(msgs []Message) RunOption {
	return func(c *runConfig) {
		c.history = msgs
	}
}

// WithStreaming enables token-by-token streaming. The callback is invoked
// for each chunk as it arrives from the LLM.
func WithStreaming(fn func(StreamChunk)) RunOption {
	return func(c *runConfig) {
		c.streamFn = fn
	}
}

// WithApprover sets the human-in-the-loop approver for tool calls that
// have WithApprovalRequired() set.
func WithApprover(a Approver) RunOption {
	return func(c *runConfig) {
		c.approver = a
	}
}

// WithRunMaxTurns overrides the agent's MaxTurns for this specific run.
func WithRunMaxTurns(n int) RunOption {
	return func(c *runConfig) {
		c.maxTurns = n
	}
}

// WithResponseFormat forces the LLM to respond in a specific format.
func WithResponseFormat(f ResponseFormat) RunOption {
	return func(c *runConfig) {
		c.responseFormat = f
	}
}

// WithResponseSchema forces the LLM to respond with JSON matching the
// given struct's schema. Pass a zero-value struct, e.g. WithResponseSchema(MyStruct{}).
func WithResponseSchema(v any) RunOption {
	return func(c *runConfig) {
		c.responseFormat = JSON
		c.responseSchema = v
	}
}

// WithImage adds an image from a local file to the user message.
// The image is read at call time and included as multi-modal content.
// ErrImageRead is returned when WithImage fails to read the file.
var ErrImageRead error

func WithImage(path string) RunOption {
	return func(c *runConfig) {
		data, err := os.ReadFile(path)
		if err != nil {
			ErrImageRead = fmt.Errorf("daneel: failed to read image %q: %w", path, err)
			return
		}
		ErrImageRead = nil
		mime := detectImageMime(path)
		c.images = append(c.images, content.ImageContent(data, mime))
	}
}

// WithImageURL adds an image from a URL to the user message.
func WithImageURL(url string) RunOption {
	return func(c *runConfig) {
		c.images = append(c.images, content.ImageURLContent(url))
	}
}

// WithImageData adds raw image bytes to the user message.
func WithImageData(data []byte, mimeType string) RunOption {
	return func(c *runConfig) {
		c.images = append(c.images, content.ImageContent(data, mimeType))
	}
}

// WithRunHook registers pre- and post-run callbacks for a single Run() call.
// pre is called after options are resolved but before the agent loop starts;
// return a non-nil error to abort the run. post is called with the RunResult
// on success. Either argument may be nil.
//
// Multiple WithRunHook calls compose: each pre-hook runs in registration order
// (stopping on the first error), and each post-hook runs in registration order.
func WithRunHook(pre func(ctx context.Context) error, post func(ctx context.Context, result *RunResult)) RunOption {
	return func(c *runConfig) {
		if pre != nil {
			prev := c.preRunHook
			if prev == nil {
				c.preRunHook = pre
			} else {
				c.preRunHook = func(ctx context.Context) error {
					if err := prev(ctx); err != nil {
						return err
					}
					return pre(ctx)
				}
			}
		}
		if post != nil {
			prev := c.postRunHook
			if prev == nil {
				c.postRunHook = post
			} else {
				c.postRunHook = func(ctx context.Context, result *RunResult) {
					prev(ctx, result)
					post(ctx, result)
				}
			}
		}
	}
}

// WithSessionPrefix sets a prefix that is prepended to auto-generated session
// IDs. Useful for multi-tenant scenarios where you want session IDs scoped to
// a tenant or namespace, e.g. "acme-corp:" produces "acme-corp:<uuid>".
//
// If WithSession is also provided the explicit ID is used as-is (no prefix).
func WithSessionPrefix(prefix string) RunOption {
	return func(c *runConfig) {
		c.sessionPrefix = prefix
	}
}

// CombineRunOptions merges multiple RunOptions into a single RunOption that
// applies each in order. Useful for building composite options in sub-packages
// that cannot directly construct runConfig literals.
func CombineRunOptions(opts ...RunOption) RunOption {
	return func(c *runConfig) {
		for _, o := range opts {
			o(c)
		}
	}
}

// detectImageMime returns a MIME type based on file extension.
func detectImageMime(path string) string {
	switch {
	case hasSuffix(path, ".png"):
		return "image/png"
	case hasSuffix(path, ".gif"):
		return "image/gif"
	case hasSuffix(path, ".webp"):
		return "image/webp"
	case hasSuffix(path, ".svg"):
		return "image/svg+xml"
	default:
		return "image/jpeg"
	}
}

func hasSuffix(s, suffix string) bool {
	return strings.HasSuffix(s, suffix)
}

// --- AgentOption (functional options for New) ---

// AgentOption configures an Agent at creation time.
type AgentOption func(*agentConfig)

// agentConfig holds all agent configuration. After New() returns,
// the config is frozen inside the Agent (immutable).
type agentConfig struct {
	instructions       string
	provider           Provider
	model              string // for convenience shortcuts
	tools              []Tool
	handoffs           []*Agent
	permissions        []PermissionRule
	maxTurns           int
	inputGuards        []InputGuard
	outputGuards       []OutputGuard
	memory             Memory
	sessionStore       SessionStore
	contextFuncs       []func(ctx context.Context) (string, error)
	contextStrategy    ContextStrategy
	maxContextTokens   int // 0 means auto-detect from ModelInfoProvider or use default
	toolExecution      ToolExecution
	failFast           bool // cancel remaining parallel tools if one fails
	defaultToolTimeout time.Duration
	strictPermissions  bool
	handoffHistory     HandoffHistory
	maxHandoffDepth    int // 0 = unlimited (default)
	rateLimit          int // max tool calls per minute (0 = no limit)
	tracer             Tracer
	onConversationEnd  []func(ctx context.Context, result RunResult)
	embedder           Embedder // optional embedder (e.g. for local AI stacks)
}

// WithInstructions sets the agent's system prompt.
func WithInstructions(instructions string) AgentOption {
	return func(c *agentConfig) {
		c.instructions = instructions
	}
}

// WithProvider sets the LLM provider for this agent. Takes precedence
// over convenience shortcuts (WithModel, WithOpenAI, etc.).
func WithProvider(p Provider) AgentOption {
	return func(c *agentConfig) {
		c.provider = p
	}
}

// WithEmbedder sets the embedding engine for this agent. Useful when pairing
// an LLM provider with a local embedding model (e.g. via WithLocalStack).
func WithEmbedder(e Embedder) AgentOption {
	return func(c *agentConfig) {
		c.embedder = e
	}
}

// WithTools adds tools available to the agent.
func WithTools(tools ...Tool) AgentOption {
	return func(c *agentConfig) {
		c.tools = append(c.tools, tools...)
	}
}

// WithHandoffs registers agents that this agent can hand off to.
func WithHandoffs(agents ...*Agent) AgentOption {
	return func(c *agentConfig) {
		c.handoffs = append(c.handoffs, agents...)
	}
}

// WithPermissions sets the tool and handoff permission rules.
func WithPermissions(rules ...[]PermissionRule) AgentOption {
	return func(c *agentConfig) {
		for _, r := range rules {
			c.permissions = append(c.permissions, r...)
		}
	}
}

// WithMaxTurns sets the maximum number of agent loop iterations.
// Default is 25.
func WithMaxTurns(n int) AgentOption {
	return func(c *agentConfig) {
		c.maxTurns = n
	}
}

// WithInputGuard adds an input guard that validates user input
// before sending to the LLM.
func WithInputGuard(g InputGuard) AgentOption {
	return func(c *agentConfig) {
		c.inputGuards = append(c.inputGuards, g)
	}
}

// WithOutputGuard adds an output guard that validates LLM responses
// before returning to the caller.
func WithOutputGuard(g OutputGuard) AgentOption {
	return func(c *agentConfig) {
		c.outputGuards = append(c.outputGuards, g)
	}
}

// WithMemory sets the conversation memory backend.
func WithMemory(m Memory) AgentOption {
	return func(c *agentConfig) {
		c.memory = m
	}
}

// WithSessionStore sets the persistent session store. The Runner will load
// conversation history from the store at the start of each Run() (if the
// session has prior history) and save it after each Run() completes.
//
// WithSessionStore complements WithMemory: the store provides full-history
// persistence across restarts, while memory provides semantic retrieval.
func WithSessionStore(s SessionStore) AgentOption {
	return func(c *agentConfig) {
		c.sessionStore = s
	}
}

// WithContextFunc adds a dynamic context function called at the start
// of each Run(). Its output is appended to the system prompt. If it
// returns an error, Run() returns immediately.
func WithContextFunc(fn func(ctx context.Context) (string, error)) AgentOption {
	return func(c *agentConfig) {
		c.contextFuncs = append(c.contextFuncs, fn)
	}
}

// WithContextStrategy sets how the Runner handles context window overflow.
func WithContextStrategy(s ContextStrategy) AgentOption {
	return func(c *agentConfig) {
		c.contextStrategy = s
	}
}

// WithMaxContextTokens sets an explicit input-token limit for this agent.
// When n > 0 it overrides any limit that could be inferred from the provider's
// model info. Use this to cap context for cost or latency reasons.
// A zero value (default) lets the framework auto-detect the limit.
func WithMaxContextTokens(n int) AgentOption {
	return func(c *agentConfig) {
		if n > 0 {
			c.maxContextTokens = n
		}
	}
}

// WithToolExecution sets tool call concurrency mode.
func WithToolExecution(te ToolExecution) AgentOption {
	return func(c *agentConfig) {
		c.toolExecution = te
	}
}

// WithDefaultToolTimeout sets the default timeout for tools that don't
// have their own timeout configured.
func WithDefaultToolTimeout(d time.Duration) AgentOption {
	return func(c *agentConfig) {
		c.defaultToolTimeout = d
	}
}

// WithStrictPermissions makes permission denials abort Run() immediately
// with ErrPermissionDenied, instead of injecting an error to the LLM.
func WithStrictPermissions() AgentOption {
	return func(c *agentConfig) {
		c.strictPermissions = true
	}
}

// WithHandoffHistory controls how much conversation history is passed
// to the target agent during handoffs.
func WithHandoffHistory(hh HandoffHistory) AgentOption {
	return func(c *agentConfig) {
		c.handoffHistory = hh
	}
}

// WithRateLimit sets the maximum tool calls per minute for this agent.
func WithRateLimit(callsPerMinute int) AgentOption {
	return func(c *agentConfig) {
		c.rateLimit = callsPerMinute
	}
}

// WithTracing sets a Tracer for distributed tracing (e.g. OpenTelemetry).
func WithTracing(t ...Tracer) AgentOption {
	return func(c *agentConfig) {
		if len(t) > 0 {
			c.tracer = t[0]
		} else {
			c.tracer = defaultTracer{}
		}
	}
}

// WithOnConversationEnd registers a callback fired at the end of each Run().
// Used by finetune.Collector to capture training data.
func WithOnConversationEnd(fn func(ctx context.Context, result RunResult)) AgentOption {
	return func(c *agentConfig) {
		c.onConversationEnd = append(c.onConversationEnd, fn)
	}
}

// WithMaxHandoffDepth limits how many consecutive handoffs can happen in a
// single Run() call. When the depth is exceeded the run returns
// ErrMaxHandoffDepth instead of delegating further.
//
// Default is 0 (unlimited). Set to a small positive integer (e.g. 5) in
// production to prevent infinite handoff chains.
func WithMaxHandoffDepth(n int) AgentOption {
	return func(c *agentConfig) {
		if n > 0 {
			c.maxHandoffDepth = n
		}
	}
}

// WithFailFast makes parallel tool execution abort remaining tools as soon as
// one tool returns an error. The cancelled tools receive a context-cancellation
// and their results are discarded; the run continues with the results of tools
// that completed before the failure.
func WithFailFast() AgentOption {
	return func(c *agentConfig) {
		c.failFast = true
	}
}
