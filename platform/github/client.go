// Package github provides tools for interacting with the GitHub REST API.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// DefaultBaseURL is the default GitHub API endpoint.
const DefaultBaseURL = "https://api.github.com"

// Client holds the configuration for GitHub API calls.
type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

// Option configures the GitHub client.
type Option func(*Client)

// WithBaseURL sets a custom API base URL (for GitHub Enterprise).
func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(url, "/") }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.http = hc }
}

// NewClient creates a new GitHub API client.
func NewClient(token string, opts ...Option) *Client {
	c := &Client{token: token, baseURL: DefaultBaseURL, http: http.DefaultClient}
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
			return fmt.Errorf("github: marshal: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("github: request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("github: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("github: read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("github: API %d: %s", resp.StatusCode, string(raw))
	}
	if target != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, target); err != nil {
			return fmt.Errorf("github: unmarshal: %w", err)
		}
	}
	return nil
}
