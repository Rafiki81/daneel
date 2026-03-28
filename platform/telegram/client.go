package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const DefaultBaseURL = "https://api.telegram.org"

type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

type Option func(*Client)

func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.http = hc }
}

func NewClient(token string, opts ...Option) *Client {
	c := &Client{token: token, baseURL: DefaultBaseURL, http: http.DefaultClient}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *Client) endpoint(method string) string {
	return fmt.Sprintf("%s/bot%s/%s", c.baseURL, c.token, method)
}

func (c *Client) call(ctx context.Context, method string, params any, target any) error {
	b, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("telegram: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint(method), bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("telegram: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("telegram: read: %w", err)
	}
	var apiResp struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return fmt.Errorf("telegram: unmarshal: %w", err)
	}
	if !apiResp.OK {
		return fmt.Errorf("telegram: API error: %s", apiResp.Description)
	}
	if target != nil && len(apiResp.Result) > 0 {
		if err := json.Unmarshal(apiResp.Result, target); err != nil {
			return fmt.Errorf("telegram: unmarshal result: %w", err)
		}
	}
	return nil
}
