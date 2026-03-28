// Package mock provides a mock Connector for testing.
//
// Usage:
//
//	c := mock.NewConnector()
//	c.SimulateMessage(daneel.IncomingMessage{Platform: "test", Content: "hello"})
//	reply := <-c.SentMessages()
package mock

import (
	"context"
	"sync"

	"github.com/Rafiki81/daneel"
)

// Connector is a mock implementation of daneel.Connector for testing.
// It allows simulating incoming messages and capturing sent replies.
type Connector struct {
	mu       sync.Mutex
	incoming chan daneel.IncomingMessage
	sent     chan SentMessage
	sentLog  []SentMessage
	started  bool
	stopped  bool
}

// SentMessage records a message sent via the connector.
type SentMessage struct {
	To      string
	Content string
}

// NewConnector creates a new mock Connector.
func NewConnector() *Connector {
	return &Connector{
		incoming: make(chan daneel.IncomingMessage, 100),
		sent:     make(chan SentMessage, 100),
	}
}

// Start implements daneel.Connector.
func (c *Connector) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.started = true
	return nil
}

// Send implements daneel.Connector. Records the sent message.
func (c *Connector) Send(ctx context.Context, to string, content string) error {
	msg := SentMessage{To: to, Content: content}
	c.mu.Lock()
	c.sentLog = append(c.sentLog, msg)
	c.mu.Unlock()
	c.sent <- msg
	return nil
}

// Messages implements daneel.Connector.
func (c *Connector) Messages() <-chan daneel.IncomingMessage {
	return c.incoming
}

// Stop implements daneel.Connector.
func (c *Connector) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped = true
	return nil
}

// --- Test helper methods ---

// SimulateMessage injects a message as if it came from an external platform.
func (c *Connector) SimulateMessage(msg daneel.IncomingMessage) {
	c.incoming <- msg
}

// SimulateText is a convenience method that creates an IncomingMessage with
// the given platform, from, and content fields.
func (c *Connector) SimulateText(platform, from, content string) {
	c.SimulateMessage(daneel.IncomingMessage{
		Platform: platform,
		From:     from,
		Content:  content,
	})
}

// SentMessages returns a channel that receives all sent messages.
// Use this in tests to assert on replies.
func (c *Connector) SentMessages() <-chan SentMessage {
	return c.sent
}

// AllSent returns all messages sent so far as a slice.
func (c *Connector) AllSent() []SentMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]SentMessage(nil), c.sentLog...)
}

// SentCount returns the number of messages sent.
func (c *Connector) SentCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sentLog)
}

// IsStarted returns whether Start() was called.
func (c *Connector) IsStarted() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.started
}

// IsStopped returns whether Stop() was called.
func (c *Connector) IsStopped() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopped
}

// Reset clears all state (sent messages, started/stopped flags).
func (c *Connector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sentLog = nil
	c.started = false
	c.stopped = false
	// Drain channels
	for len(c.incoming) > 0 {
		<-c.incoming
	}
	for len(c.sent) > 0 {
		<-c.sent
	}
}
