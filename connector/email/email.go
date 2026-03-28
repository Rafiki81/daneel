package email

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"github.com/Rafiki81/daneel"
)

// Connector implements daneel.Connector for email using IMAP IDLE.
type Connector struct {
	username     string
	password     string
	from         string
	smtpHost     string
	smtpPort     int
	imapHost     string
	imapPort     int
	useTLS       bool
	pollInterval time.Duration
	ch           chan daneel.IncomingMessage
	done         chan struct{}
	mu           sync.Mutex
}

// Option configures the email Connector.
type Option func(*Connector)

// WithSMTP sets SMTP server config.
func WithSMTP(host string, port int) Option {
	return func(c *Connector) { c.smtpHost = host; c.smtpPort = port }
}

// WithIMAP sets IMAP server config.
func WithIMAP(host string, port int) Option {
	return func(c *Connector) { c.imapHost = host; c.imapPort = port }
}

// WithPollInterval sets IMAP poll interval.
func WithPollInterval(d time.Duration) Option {
	return func(c *Connector) { c.pollInterval = d }
}

// Listen creates an email connector.
func Listen(username, password string, opts ...Option) *Connector {
	c := &Connector{
		username:     username,
		password:     password,
		from:         username,
		smtpHost:     "smtp.gmail.com",
		smtpPort:     587,
		imapHost:     "imap.gmail.com",
		imapPort:     993,
		useTLS:       true,
		pollInterval: 30 * time.Second,
		ch:           make(chan daneel.IncomingMessage, 64),
		done:         make(chan struct{}),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Start begins polling IMAP for new messages.
func (c *Connector) Start(ctx context.Context) error {
	conn, err := c.imapConnect()
	if err != nil {
		return fmt.Errorf("email: imap connect: %w", err)
	}
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	// Read greeting
	if scanner.Scan() {
		_ = scanner.Text()
	}

	// Login
	if _, err := fmt.Fprintf(conn, "a001 LOGIN %s %s\r\n", c.username, c.password); err != nil {
		return fmt.Errorf("email: login: %w", err)
	}
	if !readOK(scanner, "a001") {
		return fmt.Errorf("email: login failed")
	}

	// Select INBOX
	if _, err := fmt.Fprintf(conn, "a002 SELECT INBOX\r\n"); err != nil {
		return fmt.Errorf("email: select: %w", err)
	}
	if !readOK(scanner, "a002") {
		return fmt.Errorf("email: select inbox failed")
	}

	// Get initial message count
	lastUID := 0
	tag := 3

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
			tagStr := fmt.Sprintf("a%03d", tag)
			tag++
			cmd := fmt.Sprintf("%s SEARCH UNSEEN\r\n", tagStr)
			if _, err := conn.Write([]byte(cmd)); err != nil {
				continue
			}
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "* SEARCH") {
					uids := strings.Fields(strings.TrimPrefix(line, "* SEARCH"))
					for _, uid := range uids {
						uidNum := 0
						fmt.Sscanf(uid, "%d", &uidNum)
						if uidNum > lastUID {
							lastUID = uidNum
							msg := daneel.IncomingMessage{
								Platform: "email",
								From:     "unknown",
								Content:  fmt.Sprintf("New email (UID %d)", uidNum),
								Channel:  "inbox",
								Metadata: map[string]any{"uid": uidNum},
							}
							select {
							case c.ch <- msg:
							default:
							}
						}
					}
				}
				if strings.HasPrefix(line, tagStr) {
					break
				}
			}
		}
	}
}

// Send sends an email reply.
func (c *Connector) Send(ctx context.Context, to string, content string) error {
	addr := fmt.Sprintf("%s:%d", c.smtpHost, c.smtpPort)
	auth := smtp.PlainAuth("", c.username, c.password, c.smtpHost)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: Re: Agent Reply\r\nContent-Type: text/plain; charset=\"utf-8\"\r\n\r\n%s",
		c.from, to, content)
	return smtp.SendMail(addr, auth, c.from, []string{to}, []byte(msg))
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

func (c *Connector) imapConnect() (net.Conn, error) {
	addr := fmt.Sprintf("%s:%d", c.imapHost, c.imapPort)
	if c.useTLS {
		return tls.Dial("tcp", addr, &tls.Config{ServerName: c.imapHost})
	}
	return net.Dial("tcp", addr)
}

func readOK(scanner *bufio.Scanner, tag string) bool {
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, tag+" OK") {
			return true
		}
		if strings.HasPrefix(line, tag+" NO") || strings.HasPrefix(line, tag+" BAD") {
			return false
		}
	}
	return false
}
