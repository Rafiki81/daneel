package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Rafiki81/daneel"
)

// Connector implements daneel.Connector for GitHub using webhook HTTP server or polling.
type Connector struct {
	token        string
	baseURL      string
	http         *http.Client
	ch           chan daneel.IncomingMessage
	mode         string // "poll" or "webhook"
	webhookPort  int
	pollInterval time.Duration
	repos        []string // owner/repo list for polling
	done         chan struct{}
	lastPoll     map[string]time.Time
}

// Option configures the GitHub connector.
type Option func(*Connector)

// WebhookPort sets a port to start an HTTP server for webhooks.
func WebhookPort(port int) Option {
	return func(c *Connector) { c.mode = "webhook"; c.webhookPort = port }
}

// WithRepos sets the repos to poll for events (owner/repo format).
func WithRepos(repos ...string) Option {
	return func(c *Connector) { c.repos = repos }
}

// WithPollInterval sets how often to poll for events (default 60s).
func WithPollInterval(d time.Duration) Option {
	return func(c *Connector) { c.pollInterval = d }
}

// Listen creates a new GitHub connector (defaults to polling mode).
func Listen(token string, opts ...Option) *Connector {
	c := &Connector{
		token:        token,
		baseURL:      "https://api.github.com",
		http:         &http.Client{Timeout: 30 * time.Second},
		ch:           make(chan daneel.IncomingMessage, 64),
		mode:         "poll",
		pollInterval: 60 * time.Second,
		done:         make(chan struct{}),
		lastPoll:     make(map[string]time.Time),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Start begins listening for GitHub events.
func (c *Connector) Start(ctx context.Context) error {
	if c.mode == "webhook" {
		return c.startWebhook(ctx)
	}
	return c.startPolling(ctx)
}

// Send posts a comment on an issue/PR. The "to" field should be "owner/repo/number".
func (c *Connector) Send(ctx context.Context, to string, content string) error {
	parts := strings.SplitN(to, "/", 3)
	if len(parts) < 3 {
		return fmt.Errorf("github connector: invalid target %q (expected owner/repo/number)", to)
	}
	owner, repo, numStr := parts[0], parts[1], parts[2]
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%s/comments", c.baseURL, owner, repo, numStr)
	body := fmt.Sprintf(`{"body":%q}`, content)
	req, err := http.NewRequestWithContext(ctx, "POST", url, stringReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github: API %d: %s", resp.StatusCode, string(raw))
	}
	return nil
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

var _ daneel.Connector = (*Connector)(nil)

// --- polling ---

func (c *Connector) startPolling(ctx context.Context) error {
	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()
	// Do an immediate poll
	c.pollAll(ctx)
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

func (c *Connector) pollAll(ctx context.Context) {
	for _, repo := range c.repos {
		since := c.lastPoll[repo]
		if since.IsZero() {
			since = time.Now().Add(-5 * time.Minute)
		}
		events := c.fetchEvents(ctx, repo, since)
		for _, ev := range events {
			select {
			case c.ch <- ev:
			case <-ctx.Done():
				return
			}
		}
		c.lastPoll[repo] = time.Now()
	}
}

func (c *Connector) fetchEvents(ctx context.Context, repo string, since time.Time) []daneel.IncomingMessage {
	url := fmt.Sprintf("%s/repos/%s/events?per_page=30", c.baseURL, repo)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	var events []struct {
		Type    string                 `json:"type"`
		Actor   struct{ Login string } `json:"actor"`
		Payload json.RawMessage        `json:"payload"`
		Created string                 `json:"created_at"`
	}
	if err := json.Unmarshal(raw, &events); err != nil {
		return nil
	}
	var msgs []daneel.IncomingMessage
	for _, ev := range events {
		t, _ := time.Parse(time.RFC3339, ev.Created)
		if !t.After(since) {
			continue
		}
		msgs = append(msgs, daneel.IncomingMessage{
			Platform: "github",
			From:     ev.Actor.Login,
			Content:  string(ev.Payload),
			Channel:  repo,
			Metadata: map[string]any{
				"event_type": ev.Type,
				"created_at": ev.Created,
			},
		})
	}
	return msgs
}

// --- webhook ---

func (c *Connector) startWebhook(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}
		evType := r.Header.Get("X-GitHub-Event")
		msg := daneel.IncomingMessage{
			Platform: "github",
			From:     "",
			Content:  string(raw),
			Channel:  evType,
			Metadata: map[string]any{
				"event_type": evType,
				"delivery":   r.Header.Get("X-GitHub-Delivery"),
			},
		}
		// Try to extract sender
		var payload struct {
			Sender struct{ Login string } `json:"sender"`
			Repo   struct {
				FullName string `json:"full_name"`
			} `json:"repository"`
		}
		if json.Unmarshal(raw, &payload) == nil {
			msg.From = payload.Sender.Login
			msg.Channel = payload.Repo.FullName
		}
		select {
		case c.ch <- msg:
		default:
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := &http.Server{
		Addr:    ":" + strconv.Itoa(c.webhookPort),
		Handler: mux,
	}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	err := srv.ListenAndServe()
	close(c.ch)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func stringReader(s string) io.Reader {
	return strings.NewReader(s)
}
