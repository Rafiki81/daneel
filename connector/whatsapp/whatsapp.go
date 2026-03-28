package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/Rafiki81/daneel"
)

// Connector implements daneel.Connector for WhatsApp Business API.
// It starts an HTTP webhook server to receive incoming messages.
type Connector struct {
	token       string
	phoneID     string
	verifyToken string
	addr        string
	http        *http.Client
	ch          chan daneel.IncomingMessage
	done        chan struct{}
	mu          sync.Mutex
	server      *http.Server
}

// Option configures a Connector.
type Option func(*Connector)

// WithAddr sets the webhook listen address (default ":8080").
func WithAddr(addr string) Option {
	return func(c *Connector) { c.addr = addr }
}

// WithVerifyToken sets the webhook verification token.
func WithVerifyToken(t string) Option {
	return func(c *Connector) { c.verifyToken = t }
}

// Listen creates a WhatsApp connector.
func Listen(phoneID, token string, opts ...Option) *Connector {
	c := &Connector{
		token:       token,
		phoneID:     phoneID,
		verifyToken: "daneel_verify",
		addr:        ":8080",
		http:        &http.Client{Timeout: 30 * time.Second},
		ch:          make(chan daneel.IncomingMessage, 64),
		done:        make(chan struct{}),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Start starts the webhook HTTP server and blocks until ctx is cancelled.
func (c *Connector) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", c.handleWebhook)

	c.mu.Lock()
	c.server = &http.Server{Addr: c.addr, Handler: mux}
	c.mu.Unlock()

	errCh := make(chan error, 1)
	go func() {
		if err := c.server.ListenAndServe(); err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		c.server.Close()
		close(c.ch)
		return ctx.Err()
	case <-c.done:
		c.server.Close()
		close(c.ch)
		return nil
	case err := <-errCh:
		close(c.ch)
		return fmt.Errorf("whatsapp: server: %w", err)
	}
}

// Send sends a text message to a WhatsApp number.
func (c *Connector) Send(ctx context.Context, to string, content string) error {
	body := map[string]any{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              "text",
		"text":              map[string]string{"body": content},
	}
	b, _ := json.Marshal(body)
	url := fmt.Sprintf("https://graph.facebook.com/v19.0/%s/messages", c.phoneID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, byteRead(b))
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
		return fmt.Errorf("whatsapp: status %d: %s", resp.StatusCode, raw)
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

// handleWebhook processes incoming WhatsApp webhook notifications.
func (c *Connector) handleWebhook(w http.ResponseWriter, r *http.Request) {
	// Verification challenge
	if r.Method == http.MethodGet {
		mode := r.URL.Query().Get("hub.mode")
		token := r.URL.Query().Get("hub.verify_token")
		challenge := r.URL.Query().Get("hub.challenge")
		if mode == "subscribe" && token == c.verifyToken {
			w.WriteHeader(200)
			w.Write([]byte(challenge))
			return
		}
		w.WriteHeader(403)
		return
	}

	// POST - incoming message
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(400)
		return
	}
	w.WriteHeader(200)

	var payload struct {
		Entry []struct {
			Changes []struct {
				Value struct {
					Messages []struct {
						From string `json:"from"`
						Type string `json:"type"`
						Text struct {
							Body string `json:"body"`
						} `json:"text"`
						Timestamp string `json:"timestamp"`
						ID        string `json:"id"`
					} `json:"messages"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return
	}
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			for _, m := range change.Value.Messages {
				if m.Type != "text" {
					continue
				}
				msg := daneel.IncomingMessage{
					Platform: "whatsapp",
					From:     m.From,
					Content:  m.Text.Body,
					Channel:  c.phoneID,
					Metadata: map[string]any{"message_id": m.ID, "timestamp": m.Timestamp},
				}
				select {
				case c.ch <- msg:
				default:
				}
			}
		}
	}
}

type byteReadImpl struct {
	data []byte
	pos  int
}

func (br *byteReadImpl) Read(p []byte) (int, error) {
	if br.pos >= len(br.data) {
		return 0, io.EOF
	}
	n := copy(p, br.data[br.pos:])
	br.pos += n
	return n, nil
}

func byteRead(b []byte) io.Reader {
	return &byteReadImpl{data: b}
}
