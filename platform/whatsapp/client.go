package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DefaultBaseURL is the WhatsApp Business Cloud API base.
const DefaultBaseURL = "https://graph.facebook.com/v19.0"

// Client wraps the WhatsApp Business Cloud API.
type Client struct {
	phoneID string
	token   string
	baseURL string
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API base URL.
func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

// WithHTTPClient sets a custom http.Client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.http = hc }
}

// NewClient creates a WhatsApp Business API client.
// phoneID is the Phone Number ID from Meta dashboard.
func NewClient(phoneID, token string, opts ...Option) *Client {
	c := &Client{phoneID: phoneID, token: token, baseURL: DefaultBaseURL, http: http.DefaultClient}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *Client) do(ctx context.Context, method, path string, body any, target any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("whatsapp: marshal: %w", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, r)
	if err != nil {
		return fmt.Errorf("whatsapp: request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("whatsapp: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("whatsapp: read: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("whatsapp: status %d: %s", resp.StatusCode, raw)
	}
	if target != nil {
		if err := json.Unmarshal(raw, target); err != nil {
			return fmt.Errorf("whatsapp: unmarshal: %w", err)
		}
	}
	return nil
}
