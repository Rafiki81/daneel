package pubsub

import (
	"context"
	"fmt"
	"strings"

	"github.com/daneel-ai/daneel"
)

type publishParams struct {
	Content string `json:"content" desc:"Message content to publish"`
}

// PublishTool returns a daneel.Tool that lets an agent publish messages to topic.
func PublishTool(bus *Bus, topic string) daneel.Tool {
	return daneel.NewTool(
		"publish_"+sanitize(topic),
		fmt.Sprintf("Publish a message to the %q topic", topic),
		func(ctx context.Context, p publishParams) (string, error) {
			if err := bus.Publish(ctx, topic, p.Content); err != nil {
				return "", err
			}
			return fmt.Sprintf("published to %q", topic), nil
		},
	)
}

type subscribeParams struct {
	Topic string `json:"topic" desc:"Topic name to inspect"`
}

// TopicsTool returns a daneel.Tool that lists all active topics on the bus.
func TopicsTool(bus *Bus) daneel.Tool {
	return daneel.NewTool(
		"list_topics",
		"List all active pub/sub topics",
		func(ctx context.Context, _ struct{}) (string, error) {
			topics := bus.Topics()
			if len(topics) == 0 {
				return "no active topics", nil
			}
			return strings.Join(topics, ", "), nil
		},
	)
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			return r
		}
		return '_'
	}, s)
}
