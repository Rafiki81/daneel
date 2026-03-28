package twitter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const DefaultBaseURL = "https://api.twitter.com/2"

type Client struct {
	bearerToken string
	baseURL     string
	http        *http.Client
}

type Option func(*Client)

func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.http = hc }
}

func NewClient(bearerToken string, opts ...Option) *Client {
	c := &Client{bearerToken: bearerToken, baseURL: DefaultBaseURL, http: http.DefaultClient}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *Client) do(ctx context.Context, method, path string, body any, target any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("twitter: marshal: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("twitter: request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("twitter: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("twitter: read: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("twitter: API %d: %s", resp.StatusCode, string(raw))
	}
	if target != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, target); err != nil {
			return fmt.Errorf("twitter: unmarshal: %w", err)
		}
	}
	return nil
}
