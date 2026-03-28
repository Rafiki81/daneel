package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const DefaultBaseURL = "https://slack.com/api"

type Client struct {
	botToken string
	baseURL  string
	http     *http.Client
}

type Option func(*Client)

func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.http = hc }
}

func NewClient(botToken string, opts ...Option) *Client {
	c := &Client{botToken: botToken, baseURL: DefaultBaseURL, http: http.DefaultClient}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *Client) call(ctx context.Context, method string, body any, target any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("slack: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/"+method, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("slack: request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.botToken)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("slack: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("slack: read: %w", err)
	}
	var slackResp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &slackResp); err != nil {
		return fmt.Errorf("slack: unmarshal: %w", err)
	}
	if !slackResp.OK {
		return fmt.Errorf("slack: %s", slackResp.Error)
	}
	if target != nil {
		if err := json.Unmarshal(raw, target); err != nil {
			return fmt.Errorf("slack: unmarshal target: %w", err)
		}
	}
	return nil
}
