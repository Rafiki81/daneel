package email

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

type Client struct {
	smtpHost string
	smtpPort int
	imapHost string
	imapPort int
	username string
	password string
	from     string
	useTLS   bool
}

type Option func(*Client)

func WithSMTP(host string, port int) Option {
	return func(c *Client) { c.smtpHost = host; c.smtpPort = port }
}

func WithIMAP(host string, port int) Option {
	return func(c *Client) { c.imapHost = host; c.imapPort = port }
}

func WithTLS(enabled bool) Option {
	return func(c *Client) { c.useTLS = enabled }
}

func WithFrom(from string) Option {
	return func(c *Client) { c.from = from }
}

func NewClient(username, password string, opts ...Option) *Client {
	c := &Client{
		username: username,
		password: password,
		from:     username,
		smtpHost: "smtp.gmail.com",
		smtpPort: 587,
		imapHost: "imap.gmail.com",
		imapPort: 993,
		useTLS:   true,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *Client) SendEmail(to []string, subject, body string) error {
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=\"utf-8\"\r\n\r\n%s",
		c.from, strings.Join(to, ", "), subject, body)
	addr := fmt.Sprintf("%s:%d", c.smtpHost, c.smtpPort)
	auth := smtp.PlainAuth("", c.username, c.password, c.smtpHost)
	if c.smtpPort == 465 {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: c.smtpHost})
		if err != nil {
			return fmt.Errorf("email: tls dial: %w", err)
		}
		client, err := smtp.NewClient(conn, c.smtpHost)
		if err != nil {
			return fmt.Errorf("email: smtp client: %w", err)
		}
		defer client.Close()
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("email: auth: %w", err)
		}
		if err := client.Mail(c.from); err != nil {
			return fmt.Errorf("email: mail: %w", err)
		}
		for _, r := range to {
			if err := client.Rcpt(r); err != nil {
				return fmt.Errorf("email: rcpt: %w", err)
			}
		}
		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("email: data: %w", err)
		}
		w.Write([]byte(msg))
		w.Close()
		return client.Quit()
	}
	return smtp.SendMail(addr, auth, c.from, to, []byte(msg))
}

func (c *Client) IMAPConnect() (net.Conn, error) {
	addr := fmt.Sprintf("%s:%d", c.imapHost, c.imapPort)
	if c.useTLS {
		return tls.Dial("tcp", addr, &tls.Config{ServerName: c.imapHost})
	}
	return net.Dial("tcp", addr)
}

func (c *Client) IMAPCredentials() (string, string) {
	return c.username, c.password
}
