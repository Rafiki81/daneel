// Package pubsub provides asynchronous agent-to-agent messaging via a topic-based bus.
package pubsub

import "time"

// Message is a unit of data published to the bus.
type Message struct {
	Topic     string
	Content   string
	From      string
	Timestamp time.Time
	Metadata  map[string]string
}
