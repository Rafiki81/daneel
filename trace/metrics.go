package trace

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics collects observable counters, histograms, and gauges for agent operations.
// Thread-safe for concurrent use.
type Metrics struct {
	counters   sync.Map // map[string]*atomic.Int64
	histograms sync.Map // map[string]*histogram
	gauges     sync.Map // map[string]*atomic.Int64
}

// NewMetrics creates a new Metrics collector.
func NewMetrics() *Metrics {
	return &Metrics{}
}

// --- Standard metric names ---

const (
	MetricAgentRuns            = "daneel.agent.runs"
	MetricAgentTurns           = "daneel.agent.turns"
	MetricAgentDurationMs      = "daneel.agent.duration_ms"
	MetricLLMRequests          = "daneel.llm.requests"
	MetricLLMPromptTokens      = "daneel.llm.tokens.prompt"
	MetricLLMCompletionTokens  = "daneel.llm.tokens.completion"
	MetricLLMLatencyMs         = "daneel.llm.latency_ms"
	MetricLLMErrors            = "daneel.llm.errors"
	MetricToolExecutions       = "daneel.tool.executions"
	MetricToolDurationMs       = "daneel.tool.duration_ms"
	MetricPermissionDenials    = "daneel.permission.denials"
	MetricGuardFailures        = "daneel.guard.failures"
	MetricHandoffCount         = "daneel.handoff.count"
	MetricCostUSD              = "daneel.cost.usd"
	MetricBridgeMessages       = "daneel.bridge.messages"
	MetricBridgeActiveConvos   = "daneel.bridge.active_conversations"
)

// --- Counters ---

// Inc increments a counter by 1.
func (m *Metrics) Inc(name string) {
	m.Add(name, 1)
}

// Add adds delta to a counter.
func (m *Metrics) Add(name string, delta int64) {
	v, _ := m.counters.LoadOrStore(name, &atomic.Int64{})
	v.(*atomic.Int64).Add(delta)
}

// Counter returns the current value of a counter.
func (m *Metrics) Counter(name string) int64 {
	v, ok := m.counters.Load(name)
	if !ok {
		return 0
	}
	return v.(*atomic.Int64).Load()
}

// --- Gauges ---

// SetGauge sets a gauge to a specific value.
func (m *Metrics) SetGauge(name string, value int64) {
	v, _ := m.gauges.LoadOrStore(name, &atomic.Int64{})
	v.(*atomic.Int64).Store(value)
}

// IncGauge increments a gauge by 1.
func (m *Metrics) IncGauge(name string) {
	v, _ := m.gauges.LoadOrStore(name, &atomic.Int64{})
	v.(*atomic.Int64).Add(1)
}

// DecGauge decrements a gauge by 1.
func (m *Metrics) DecGauge(name string) {
	v, _ := m.gauges.LoadOrStore(name, &atomic.Int64{})
	v.(*atomic.Int64).Add(-1)
}

// Gauge returns the current value of a gauge.
func (m *Metrics) Gauge(name string) int64 {
	v, ok := m.gauges.Load(name)
	if !ok {
		return 0
	}
	return v.(*atomic.Int64).Load()
}

// --- Histograms ---

type histogram struct {
	mu     sync.Mutex
	values []float64
	count  int64
	sum    float64
	min    float64
	max    float64
}

// Record records a value to a histogram.
func (m *Metrics) Record(name string, value float64) {
	v, _ := m.histograms.LoadOrStore(name, &histogram{min: value, max: value})
	h := v.(*histogram)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.values = append(h.values, value)
	h.count++
	h.sum += value
	if value < h.min {
		h.min = value
	}
	if value > h.max {
		h.max = value
	}
}

// RecordDuration records a duration as milliseconds to a histogram.
func (m *Metrics) RecordDuration(name string, d time.Duration) {
	m.Record(name, float64(d.Milliseconds()))
}

// HistogramStats holds aggregated histogram data.
type HistogramStats struct {
	Count int64
	Sum   float64
	Min   float64
	Max   float64
	Avg   float64
}

// Histogram returns summary statistics for a histogram.
func (m *Metrics) Histogram(name string) HistogramStats {
	v, ok := m.histograms.Load(name)
	if !ok {
		return HistogramStats{}
	}
	h := v.(*histogram)
	h.mu.Lock()
	defer h.mu.Unlock()
	stats := HistogramStats{
		Count: h.count,
		Sum:   h.sum,
		Min:   h.min,
		Max:   h.max,
	}
	if h.count > 0 {
		stats.Avg = h.sum / float64(h.count)
	}
	return stats
}

// Snapshot returns all current metric values.
func (m *Metrics) Snapshot() map[string]any {
	snap := make(map[string]any)
	m.counters.Range(func(key, value any) bool {
		snap[key.(string)] = value.(*atomic.Int64).Load()
		return true
	})
	m.gauges.Range(func(key, value any) bool {
		snap[key.(string)+".gauge"] = value.(*atomic.Int64).Load()
		return true
	})
	m.histograms.Range(func(key, value any) bool {
		h := value.(*histogram)
		h.mu.Lock()
		stats := HistogramStats{Count: h.count, Sum: h.sum, Min: h.min, Max: h.max}
		if h.count > 0 {
			stats.Avg = h.sum / float64(h.count)
		}
		h.mu.Unlock()
		snap[key.(string)+".stats"] = stats
		return true
	})
	return snap
}
