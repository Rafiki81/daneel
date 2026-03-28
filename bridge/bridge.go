// Package bridge connects Connectors to Agents automatically.
// It handles concurrency, conversation history, and session management.
package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Rafiki81/daneel"
)

// BridgeMetrics is an optional observer for Bridge runtime events.
// Implement this interface to integrate with Prometheus, OpenTelemetry,
// Datadog, or any other metrics backend.
type BridgeMetrics interface {
	// RecordMessageProcessed is called after each message is handled.
	// platform is e.g. "slack", "telegram". duration is total processing time.
	RecordMessageProcessed(platform string, duration time.Duration, err error)
	// RecordActiveConversations is called whenever the conversation map changes.
	RecordActiveConversations(count int)
	// RecordCleanup is called after each TTL sweep.
	RecordCleanup(deletedSessions int)
}

// Bridge connects one or more Connectors to an Agent.
type Bridge struct {
	agent       *daneel.Agent
	connectors  []daneel.Connector
	concurrency int
	historyTTL  time.Duration
	maxHistory  int
	errHandler  func(error, daneel.IncomingMessage)
	metrics     BridgeMetrics
	mu          sync.RWMutex
	convos      map[string]*conversation
}

type conversation struct {
	messages []daneel.Message
	lastSeen time.Time
}

// Option configures the Bridge.
type Option func(*Bridge)

// WithConnector adds a connector.
func WithConnector(c daneel.Connector) Option {
	return func(b *Bridge) { b.connectors = append(b.connectors, c) }
}

// WithAgent sets the agent.
func WithAgent(a *daneel.Agent) Option {
	return func(b *Bridge) { b.agent = a }
}

// WithConcurrency limits concurrent conversations.
func WithConcurrency(n int) Option {
	return func(b *Bridge) { b.concurrency = n }
}

// WithHistoryTTL sets the TTL for conversation history.
func WithHistoryTTL(d time.Duration) Option {
	return func(b *Bridge) { b.historyTTL = d }
}

// WithMaxHistory caps messages per conversation.
func WithMaxHistory(n int) Option {
	return func(b *Bridge) { b.maxHistory = n }
}

// WithErrorHandler sets a handler for processing errors.
func WithErrorHandler(fn func(error, daneel.IncomingMessage)) Option {
	return func(b *Bridge) { b.errHandler = fn }
}

// WithMetrics attaches a BridgeMetrics observer to the Bridge.
func WithMetrics(m BridgeMetrics) Option {
	return func(b *Bridge) { b.metrics = m }
}

// New creates a new Bridge.
func New(opts ...Option) *Bridge {
	b := &Bridge{
		concurrency: 10,
		historyTTL:  1 * time.Hour,
		maxHistory:  100,
		convos:      make(map[string]*conversation),
		errHandler: func(err error, msg daneel.IncomingMessage) {
			slog.Error("bridge error", "from", msg.From, "platform", msg.Platform, "error", err)
		},
	}
	for _, o := range opts {
		o(b)
	}
	return b
}

// Run starts all connectors and processes messages. Blocks until ctx is cancelled.
func (b *Bridge) Run(ctx context.Context) error {
	if b.agent == nil {
		return fmt.Errorf("bridge: no agent configured")
	}
	if len(b.connectors) == 0 {
		return fmt.Errorf("bridge: no connectors configured")
	}

	// Start cleanup goroutine
	go b.cleanupLoop(ctx)

	// Start all connectors
	errCh := make(chan error, len(b.connectors))
	for _, conn := range b.connectors {
		go func(c daneel.Connector) {
			errCh <- c.Start(ctx)
		}(conn)
	}

	// Semaphore for concurrency control
	sem := make(chan struct{}, b.concurrency)

	// Merge all message channels
	merged := make(chan connMsg, 64)
	var wg sync.WaitGroup
	for _, conn := range b.connectors {
		wg.Add(1)
		go func(c daneel.Connector) {
			defer wg.Done()
			for msg := range c.Messages() {
				merged <- connMsg{conn: c, msg: msg}
			}
		}(conn)
	}
	go func() {
		wg.Wait()
		close(merged)
	}()

	// Process messages
	var procWg sync.WaitGroup
	for cm := range merged {
		sem <- struct{}{}
		procWg.Add(1)
		go func(cm connMsg) {
			defer procWg.Done()
			defer func() { <-sem }()
			b.handleMessage(ctx, cm.conn, cm.msg)
		}(cm)
	}
	procWg.Wait()

	// Stop all connectors
	for _, conn := range b.connectors {
		conn.Stop()
	}

	return nil
}

type connMsg struct {
	conn daneel.Connector
	msg  daneel.IncomingMessage
}

func (b *Bridge) handleMessage(ctx context.Context, conn daneel.Connector, msg daneel.IncomingMessage) {
	start := time.Now()
	sessionID := daneel.DeterministicSessionID(msg.Platform, msg.From, msg.Channel)

	// Get or create conversation
	b.mu.Lock()
	convo, ok := b.convos[sessionID]
	if !ok {
		convo = &conversation{}
		b.convos[sessionID] = convo
	}
	convo.lastSeen = time.Now()
	convo.messages = append(convo.messages, daneel.Message{
		Role:    daneel.RoleUser,
		Content: msg.Content,
	})
	// Trim history
	if len(convo.messages) > b.maxHistory {
		convo.messages = convo.messages[len(convo.messages)-b.maxHistory:]
	}
	// Copy for safe use outside lock
	history := make([]daneel.Message, len(convo.messages))
	copy(history, convo.messages)
	b.mu.Unlock()

	// Run agent
	result, err := daneel.Run(ctx, b.agent, msg.Content,
		daneel.WithSessionID(sessionID),
		daneel.WithHistory(history),
	)
	if b.metrics != nil {
		b.metrics.RecordMessageProcessed(msg.Platform, time.Since(start), err)
	}
	if err != nil {
		b.errHandler(err, msg)
		return
	}

	// Send reply
	// Reply to channel if available, otherwise to sender directly
	replyTo := msg.Channel
	if replyTo == "" {
		replyTo = msg.From
	}
	if err := conn.Send(ctx, replyTo, result.Output); err != nil {
		b.errHandler(fmt.Errorf("send reply: %w", err), msg)
		return
	}

	// Store assistant response
	b.mu.Lock()
	if c, ok := b.convos[sessionID]; ok {
		c.messages = append(c.messages, daneel.Message{
			Role:    daneel.RoleAssistant,
			Content: result.Output,
		})
	}
	b.mu.Unlock()
}

func (b *Bridge) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.cleanup()
		}
	}
}

func (b *Bridge) cleanup() {
	b.mu.Lock()
	defer b.mu.Unlock()
	cutoff := time.Now().Add(-b.historyTTL)
	deleted := 0
	for id, convo := range b.convos {
		if convo.lastSeen.Before(cutoff) {
			delete(b.convos, id)
			deleted++
		}
	}
	if b.metrics != nil {
		b.metrics.RecordCleanup(deleted)
		b.metrics.RecordActiveConversations(len(b.convos))
	}
}

// Multi is a convenience function that runs an agent with multiple connectors.
func Multi(ctx context.Context, agent *daneel.Agent, connectors ...daneel.Connector) error {
	opts := []Option{WithAgent(agent)}
	for _, c := range connectors {
		opts = append(opts, WithConnector(c))
	}
	return New(opts...).Run(ctx)
}
