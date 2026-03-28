package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/daneel-ai/daneel"
)

// Connector implements daneel.Connector for Slack using Socket Mode polling fallback.
// For full Socket Mode WebSocket, use the slack-go library.
// This implementation polls conversations.history as a simple approach.
type Connector struct {
	botToken     string
	baseURL      string
	http         *http.Client
	channels     []string
	pollInterval time.Duration
	ch           chan daneel.IncomingMessage
	done         chan struct{}
	lastTS       map[string]string
}

type Option func(*Connector)

func WithChannels(channels ...string) Option {
	return func(c *Connector) { c.channels = channels }
}

func WithPollInterval(d time.Duration) Option {
	return func(c *Connector) { c.pollInterval = d }
}

// Listen creates a Slack connector.
func Listen(botToken string, opts ...Option) *Connector {
	c := &Connector{
		botToken:     botToken,
		baseURL:      "https://slack.com/api",
		http:         &http.Client{Timeout: 30 * time.Second},
		pollInterval: 5 * time.Second,
		ch:           make(chan daneel.IncomingMessage, 64),
		done:         make(chan struct{}),
		lastTS:       make(map[string]string),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *Connector) Start(ctx context.Context) error {
	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			close(c.ch)
			return ctx.Err()
		case <-c.done:
			close(c.ch)
			return nil
		case <-ticker.C:
			c.pollAll(ctx)
		}
	}
}

func (c *Connector) Send(ctx context.Context, to string, content string) error {
	body := map[string]string{"channel": to, "text": content}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat.postMessage", byteRead(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.botToken)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var r struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return err
	}
	if !r.OK {
		return fmt.Errorf("slack: %s", r.Error)
	}
	return nil
}

func (c *Connector) Messages() <-chan daneel.IncomingMessage { return c.ch }

func (c *Connector) Stop() error {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
	return nil
}

var _ daneel.Connector = (*Connector)(nil)

func (c *Connector) pollAll(ctx context.Context) {
	for _, ch := range c.channels {
		c.pollChannel(ctx, ch)
	}
}

func (c *Connector) pollChannel(ctx context.Context, channel string) {
	url := fmt.Sprintf("%s/conversations.history?channel=%s&limit=10", c.baseURL, channel)
	if ts, ok := c.lastTS[channel]; ok {
		url += "&oldest=" + ts
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.botToken)
	resp, err := c.http.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var r struct {
		OK       bool `json:"ok"`
		Messages []struct {
			User string `json:"user"`
			Text string `json:"text"`
			TS   string `json:"ts"`
		} `json:"messages"`
	}
	if json.Unmarshal(raw, &r) != nil || !r.OK || len(r.Messages) == 0 {
		return
	}
	for _, m := range r.Messages {
		if m.TS == c.lastTS[channel] {
			continue
		}
		msg := daneel.IncomingMessage{
			Platform: "slack",
			From:     m.User,
			Content:  m.Text,
			Channel:  channel,
			Metadata: map[string]any{"ts": m.TS},
		}
		select {
		case c.ch <- msg:
		case <-ctx.Done():
			return
		}
	}
	if len(r.Messages) > 0 {
		c.lastTS[channel] = r.Messages[0].TS
	}
}

func byteRead(b []byte) io.Reader {
	return bytes.NewReader(b)
}
