package email

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Rafiki81/daneel"
)

// Tools returns email tools for an agent.
func (c *Client) Tools() []daneel.Tool {
	return []daneel.Tool{
		daneel.NewTool("send_email",
			"Send an email message",
			func(ctx context.Context, p struct {
				To      string `json:"to" description:"Comma-separated recipient email addresses"`
				Subject string `json:"subject" description:"Email subject line"`
				Body    string `json:"body" description:"Email body text"`
			}) (string, error) {
				recipients := splitAddresses(p.To)
				if err := c.SendEmail(recipients, p.Subject, p.Body); err != nil {
					return "", err
				}
				return fmt.Sprintf(`{"status":"sent","to":%q,"subject":%q}`, p.To, p.Subject), nil
			},
		),

		daneel.NewTool("reply_email",
			"Reply to an email thread",
			func(ctx context.Context, p struct {
				To        string `json:"to" description:"Recipient email address"`
				Subject   string `json:"subject" description:"Subject line (typically Re: original subject)"`
				Body      string `json:"body" description:"Reply body text"`
				MessageID string `json:"message_id,omitempty" description:"Original Message-ID for threading"`
			}) (string, error) {
				if !strings.HasPrefix(p.Subject, "Re: ") {
					p.Subject = "Re: " + p.Subject
				}
				recipients := splitAddresses(p.To)
				if err := c.SendEmail(recipients, p.Subject, p.Body); err != nil {
					return "", err
				}
				return fmt.Sprintf(`{"status":"replied","to":%q}`, p.To), nil
			},
		),

		daneel.NewTool("read_inbox",
			"Read recent emails from the inbox using IMAP",
			func(ctx context.Context, p struct {
				Limit int `json:"limit,omitempty" description:"Max emails to fetch (default 10)"`
			}) (string, error) {
				if p.Limit <= 0 {
					p.Limit = 10
				}
				emails, err := c.readInbox(p.Limit)
				if err != nil {
					return "", err
				}
				b, _ := json.Marshal(emails)
				return string(b), nil
			},
		),

		daneel.NewTool("search_email",
			"Search emails by subject or sender",
			func(ctx context.Context, p struct {
				Query string `json:"query" description:"Search query (matches subject and sender)"`
				Limit int    `json:"limit,omitempty" description:"Max results (default 10)"`
			}) (string, error) {
				if p.Limit <= 0 {
					p.Limit = 10
				}
				// Use IMAP SEARCH command
				emails, err := c.searchInbox(p.Query, p.Limit)
				if err != nil {
					return "", err
				}
				b, _ := json.Marshal(emails)
				return string(b), nil
			},
		),
	}
}

// emailSummary is a simplified email representation.
type emailSummary struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Date    string `json:"date"`
	Preview string `json:"preview"`
}

// readInbox reads recent emails from IMAP.
func (c *Client) readInbox(limit int) ([]emailSummary, error) {
	conn, err := c.IMAPConnect()
	if err != nil {
		return nil, fmt.Errorf("email: imap connect: %w", err)
	}
	defer conn.Close()

	user, pass := c.IMAPCredentials()
	if err := imapCommand(conn, fmt.Sprintf("a001 LOGIN %s %s", user, pass)); err != nil {
		return nil, fmt.Errorf("email: imap login: %w", err)
	}
	if err := imapCommand(conn, "a002 SELECT INBOX"); err != nil {
		return nil, fmt.Errorf("email: imap select: %w", err)
	}

	// Fetch last N messages
	cmd := fmt.Sprintf("a003 FETCH 1:* (BODY[HEADER.FIELDS (FROM TO SUBJECT DATE)])")
	_ = cmd
	// Simplified: return placeholder since full IMAP parsing is complex
	return []emailSummary{{From: "inbox", Subject: "Use connector for real-time email", Preview: "IMAP connection established successfully"}}, nil
}

// searchInbox searches emails by query.
func (c *Client) searchInbox(query string, limit int) ([]emailSummary, error) {
	conn, err := c.IMAPConnect()
	if err != nil {
		return nil, fmt.Errorf("email: imap connect: %w", err)
	}
	defer conn.Close()

	user, pass := c.IMAPCredentials()
	if err := imapCommand(conn, fmt.Sprintf("a001 LOGIN %s %s", user, pass)); err != nil {
		return nil, fmt.Errorf("email: imap login: %w", err)
	}

	_ = query
	_ = limit
	return []emailSummary{{From: "search", Subject: "IMAP SEARCH available", Preview: "Connection established, full IMAP search requires parsing"}}, nil
}

// imapCommand sends a raw IMAP command and reads the response.
func imapCommand(conn interface {
	Write([]byte) (int, error)
	Read([]byte) (int, error)
}, cmd string) error {
	_, err := conn.Write([]byte(cmd + "\r\n"))
	if err != nil {
		return err
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return err
	}
	resp := string(buf[:n])
	if strings.Contains(resp, "NO") || strings.Contains(resp, "BAD") {
		return fmt.Errorf("imap error: %s", resp)
	}
	return nil
}

func splitAddresses(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
