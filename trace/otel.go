package trace

import (
	"context"
	"fmt"
	"sync"

	"github.com/Rafiki81/daneel"
)

// OTELTracer implements daneel.Tracer using the daneel.Span interface.
// It wraps any OTEL-compatible TracerProvider by accepting a SpanStarter.
//
// For zero-dependency usage, use the noop tracer or provide a custom SpanStarter
// that wraps go.opentelemetry.io/otel.
type OTELTracer struct {
	starter SpanStarter
	name    string
}

// SpanStarter is an adapter interface so we don't import OTEL directly.
// Users provide an implementation wrapping otel.Tracer.
type SpanStarter interface {
	Start(ctx context.Context, name string) (context.Context, SpanEnder)
}

// SpanEnder is the minimal span interface for ending and attributing.
type SpanEnder interface {
	SetAttribute(key string, value any)
	RecordError(err error)
	End()
}

// Option configures the tracer.
type Option func(*OTELTracer)

// WithName sets the tracer instrumentation name.
func WithName(name string) Option {
	return func(t *OTELTracer) { t.name = name }
}

// New creates an OTELTracer with the given SpanStarter.
func New(starter SpanStarter, opts ...Option) *OTELTracer {
	t := &OTELTracer{starter: starter, name: "daneel"}
	for _, o := range opts {
		o(t)
	}
	return t
}

// StartSpan implements daneel.Tracer.
func (t *OTELTracer) StartSpan(ctx context.Context, name string, attrs ...daneel.Attr) (context.Context, daneel.Span) {
	spanName := fmt.Sprintf("%s.%s", t.name, name)
	ctx, se := t.starter.Start(ctx, spanName)
	for _, a := range attrs {
		se.SetAttribute(a.Key, a.Value)
	}
	return ctx, &otelSpan{se: se}
}

var _ daneel.Tracer = (*OTELTracer)(nil)

type otelSpan struct {
	se   SpanEnder
	once sync.Once
}

func (s *otelSpan) SetAttributes(attrs ...daneel.Attr) {
	for _, a := range attrs {
		s.se.SetAttribute(a.Key, a.Value)
	}
}

func (s *otelSpan) RecordError(err error) {
	s.se.RecordError(err)
}

func (s *otelSpan) End() {
	s.once.Do(func() { s.se.End() })
}

// --- Noop tracer ---

// Noop returns a no-op tracer. Useful as a default or for testing.
func Noop() *OTELTracer {
	return New(&noopStarter{})
}

type noopStarter struct{}

func (noopStarter) Start(ctx context.Context, name string) (context.Context, SpanEnder) {
	return ctx, noopEnder{}
}

type noopEnder struct{}

func (noopEnder) SetAttribute(key string, value any) {}
func (noopEnder) RecordError(err error)               {}
func (noopEnder) End()                                 {}

// --- Convenience span helpers ---

// AgentSpan starts a span for an agent run.
func AgentSpan(t daneel.Tracer, ctx context.Context, agentName string, attrs ...daneel.Attr) (context.Context, daneel.Span) {
	allAttrs := append([]daneel.Attr{{Key: "agent.name", Value: agentName}}, attrs...)
	return t.StartSpan(ctx, "Agent:"+agentName, allAttrs...)
}

// LLMSpan starts a span for an LLM call.
func LLMSpan(t daneel.Tracer, ctx context.Context, model string, attrs ...daneel.Attr) (context.Context, daneel.Span) {
	allAttrs := append([]daneel.Attr{{Key: "llm.model", Value: model}}, attrs...)
	return t.StartSpan(ctx, "LLM:"+model, allAttrs...)
}

// ToolSpan starts a span for a tool execution.
func ToolSpan(t daneel.Tracer, ctx context.Context, toolName string, attrs ...daneel.Attr) (context.Context, daneel.Span) {
	allAttrs := append([]daneel.Attr{{Key: "tool.name", Value: toolName}}, attrs...)
	return t.StartSpan(ctx, "Tool:"+toolName, allAttrs...)
}

// GuardSpan starts a span for a guard check.
func GuardSpan(t daneel.Tracer, ctx context.Context, guardType string, attrs ...daneel.Attr) (context.Context, daneel.Span) {
	return t.StartSpan(ctx, "Guard:"+guardType, attrs...)
}

// PermissionSpan starts a span for a permission check.
func PermissionSpan(t daneel.Tracer, ctx context.Context, tool string, allowed bool, attrs ...daneel.Attr) (context.Context, daneel.Span) {
	allAttrs := append([]daneel.Attr{
		{Key: "tool.name", Value: tool},
		{Key: "permission.allowed", Value: allowed},
	}, attrs...)
	return t.StartSpan(ctx, "Permission:check", allAttrs...)
}
