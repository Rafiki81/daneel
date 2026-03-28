package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/daneel-ai/daneel"
)

// Connector implements daneel.Connector for Telegram using long-polling.
type Connector struct {
	token    string
	baseURL  string
	http     *http.Client
	pollTime int
	ch       chan daneel.IncomingMessage
	offset   int
	done     chan struct{}
}

// Option configures the Telegram connector.
type Option func(*Connector)

// WithBaseURL sets a custom API URL.
func WithBaseURL(url string) Option {
	return func(c *Connector) { c.baseURL = url }
}

// WithPollTimeout sets the long-polling timeout in seconds (default 30).
func WithPollTimeout(seconds int) Option {
	return func(c *Connector) { c.pollTime = seconds }
}

// Listen creates a new Telegram connector (implements daneel.Connector).
func Listen(token string, opts ...Option) *Connector {
	c := &Connector{
		token:    token,
		baseURL:  "https://api.telegram.org",
		http:     &http.Client{Timeout: 60 * time.Second},
		pollTime: 30,
		ch:       make(chan daneel.IncomingMessage, 64),
		done:     make(chan struct{}),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Start begins long-polling for updates. Blocks until ctx is cancelled or Stop is called.
func (c *Connector) Start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			close(c.ch)
			return ctx.Err()
		case <-c.done:
			close(c.ch)
			return nil
		default:
		}
		updates, err := c.getUpdates(ctx)
		if err != nil {
			if ctx.Err() != nil {
				close(c.ch)
				return ctx.Err()
			}
			time.Sleep(2 * time.Second)
			continue
		}
		for _, u := range updates {
			if u.Message == nil {
				c.offset = u.UpdateID + 1
				continue
			}
			msg := daneel.IncomingMessage{
				Platform: "telegram",
				From:     strconv.FormatInt(u.Message.Chat.ID, 10),
				Content:  u.Message.Text,
				Channel:  strconv.FormatInt(u.Message.Chat.ID, 10),
				Metadata: map[string]any{
					"message_id": u.Message.MessageID,
					"from_user":  u.Message.From.Username,
					"chat_type":  u.Message.Chat.Type,
				},
			}
			select {
			case c.ch <- msg:
			case <-ctx.Done():
				close(c.ch)
				return ctx.Err()
			}
			c.offset = u.UpdateID + 1
		}
	}
}

// Send sends a text message to the given chat ID.
func (c *Connector) Send(ctx context.Context, to string, content string) error {
	body := map[string]any{"chat_id": to, "text": content}
	return c.apiCall(ctx, "sendMessage", body)
}

// Messages returns the channel of incoming messages.
func (c *Connector) Messages() <-chan daneel.IncomingMessage { return c.ch }

// Stop stops the connector.
func (c *Connector) Stop() error {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
	return nil
}

// Compile-time interface check.
var _ daneel.Connector = (*Connector)(nil)

// --- internal types ---

type tgUpdate struct {
	UpdateID int        `json:"update_id"`
	Message  *tgMessage `json:"message"`
}

type tgMessage struct {
	MessageID int    `json:"message_id"`
	Text      string `json:"text"`
	Chat      tgChat `json:"chat"`
	From      tgUser `json:"from"`
}

type tgChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type tgUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

func (c *Connector) endpoint(method string) string {
	return fmt.Sprintf("%s/bot%s/%s", c.baseURL, c.token, method)
}

func (c *Connector) getUpdates(ctx context.Context) ([]tgUpdate, error) {
	body := map[string]any{
		"offset":  c.offset,
		"timeout": c.pollTime,
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint("getUpdates"), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var apiResp struct {
		OK     bool       `json:"ok"`
		Result []tgUpdate `json:"result"`
	}
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, err
	}
	if !apiResp.OK {
		return nil, fmt.Errorf("telegram: getUpdates failed")
	}
	return apiResp.Result, nil
}

func (c *Connector) apiCall(ctx context.Context, method string, params map[string]any) error {
	b, _ := json.Marshal(params)
	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint(method), bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var apiResp struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return err
	}
	if !apiResp.OK {
		return fmt.Errorf("telegram: %s: %s", method, apiResp.Description)
	}
	return nil
}
