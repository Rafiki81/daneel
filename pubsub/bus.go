package pubsub

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Rafiki81/daneel"
)

type envelope struct {
	msg   Message
	agent *daneel.Agent
}

// Bus routes messages between agents via named topics.
type Bus struct {
	mu   sync.RWMutex
	subs map[string][]*daneel.Agent // topic → subscribers
	ch   chan envelope
	done chan struct{}
}

// New creates a new Bus. Call Start before publishing.
func New() *Bus {
	return &Bus{
		subs: make(map[string][]*daneel.Agent),
		ch:   make(chan envelope, 256),
		done: make(chan struct{}),
	}
}

// Subscribe registers agent to receive messages on topic.
func (b *Bus) Subscribe(topic string, agent *daneel.Agent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[topic] = append(b.subs[topic], agent)
}

// Unsubscribe removes agent from topic.
func (b *Bus) Unsubscribe(topic string, agent *daneel.Agent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	slice := b.subs[topic]
	for i, a := range slice {
		if a == agent {
			b.subs[topic] = append(slice[:i], slice[i+1:]...)
			return
		}
	}
}

// Publish sends content to all agents subscribed to topic.
// It is non-blocking: delivery happens asynchronously.
func (b *Bus) Publish(ctx context.Context, topic, content string) error {
	msg := Message{
		Topic:     topic,
		Content:   content,
		Timestamp: time.Now(),
	}
	b.mu.RLock()
	subs := append([]*daneel.Agent(nil), b.subs[topic]...)
	b.mu.RUnlock()

	if len(subs) == 0 {
		return fmt.Errorf("pubsub: no subscribers for topic %q", topic)
	}
	for _, agent := range subs {
		select {
		case b.ch <- envelope{msg: msg, agent: agent}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// Start begins routing messages to subscribers. Blocks until ctx is cancelled.
func (b *Bus) Start(ctx context.Context) {
	for {
		select {
		case env := <-b.ch:
			go func(e envelope) {
				_, _ = daneel.Run(ctx, e.agent, e.msg.Content)
			}(env)
		case <-ctx.Done():
			return
		case <-b.done:
			return
		}
	}
}

// Stop signals the bus to stop routing.
func (b *Bus) Stop() { close(b.done) }

// Topics returns all topics that have at least one subscriber.
func (b *Bus) Topics() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var topics []string
	for t, subs := range b.subs {
		if len(subs) > 0 {
			topics = append(topics, t)
		}
	}
	return topics
}
