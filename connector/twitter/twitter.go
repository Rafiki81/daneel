package twitter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/daneel-ai/daneel"
)

// Connector implements daneel.Connector for Twitter using polling.
type Connector struct {
	bearerToken  string
	userID       string
	baseURL      string
	http         *http.Client
	pollInterval time.Duration
	ch           chan daneel.IncomingMessage
	done         chan struct{}
	sinceID      string
}

type Option func(*Connector)

func WithPollInterval(d time.Duration) Option {
	return func(c *Connector) { c.pollInterval = d }
}

func WithUserID(id string) Option {
	return func(c *Connector) { c.userID = id }
}

// Listen creates a Twitter connector that polls for mentions.
func Listen(bearerToken string, opts ...Option) *Connector {
	c := &Connector{
		bearerToken:  bearerToken,
		baseURL:      "https://api.twitter.com/2",
		http:         &http.Client{Timeout: 30 * time.Second},
		pollInterval: 30 * time.Second,
		ch:           make(chan daneel.IncomingMessage, 64),
		done:         make(chan struct{}),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *Connector) Start(ctx context.Context) error {
	if c.userID == "" {
		id, err := c.fetchMyUserID(ctx)
		if err != nil {
			return fmt.Errorf("twitter connector: cannot get user ID: %w", err)
		}
		c.userID = id
	}
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
			c.poll(ctx)
		}
	}
}

func (c *Connector) Send(ctx context.Context, to string, content string) error {
	body := map[string]any{
		"text":  content,
		"reply": map[string]string{"in_reply_to_tweet_id": to},
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/tweets", bytesReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("twitter: send error %d: %s", resp.StatusCode, string(raw))
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

func (c *Connector) fetchMyUserID(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/users/me", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var r struct {
		Data struct{ ID string `json:"id"` } `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	return r.Data.ID, nil
}

func (c *Connector) poll(ctx context.Context) {
	path := fmt.Sprintf("/users/%s/mentions?tweet.fields=author_id,text", c.userID)
	if c.sinceID != "" {
		path += "&since_id=" + c.sinceID
	}
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	resp, err := c.http.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var r struct {
		Data []struct {
			ID       string `json:"id"`
			Text     string `json:"text"`
			AuthorID string `json:"author_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil || len(r.Data) == 0 {
		return
	}
	for _, tweet := range r.Data {
		msg := daneel.IncomingMessage{
			Platform: "twitter",
			From:     tweet.AuthorID,
			Content:  tweet.Text,
			Channel:  tweet.ID,
			Metadata: map[string]any{"tweet_id": tweet.ID},
		}
		select {
		case c.ch <- msg:
		case <-ctx.Done():
			return
		}
		c.sinceID = tweet.ID
	}
}

type bytesReaderWrapper struct {
	data []byte
	pos  int
}

func (r *bytesReaderWrapper) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func bytesReader(b []byte) io.Reader {
	return &bytesReaderWrapper{data: b}
}
