package slack

import (
	"context"
	"fmt"
	"strings"

	"github.com/Rafiki81/daneel"
)

func Tools(botToken string, opts ...Option) []daneel.Tool {
	c := NewClient(botToken, opts...)
	return []daneel.Tool{
		c.sendMessageTool(), c.readChannelTool(), c.reactTool(),
		c.createChannelTool(), c.listChannelsTool(), c.uploadFileTool(),
	}
}

type SendMessageParams struct {
	Channel string `json:"channel" description:"Channel ID or name"`
	Text    string `json:"text" description:"Message text"`
}

type ReadChannelParams struct {
	Channel string `json:"channel" description:"Channel ID"`
	Limit   int    `json:"limit,omitempty" description:"Number of messages (default 10)"`
}

type ReactParams struct {
	Channel   string `json:"channel" description:"Channel ID"`
	Timestamp string `json:"timestamp" description:"Message timestamp"`
	Emoji     string `json:"emoji" description:"Emoji name without colons"`
}

type CreateChannelParams struct {
	Name string `json:"name" description:"Channel name"`
}

type ListChannelsParams struct {
	Limit int `json:"limit,omitempty" description:"Max channels to list"`
}

type UploadFileParams struct {
	Channels string `json:"channels" description:"Comma-separated channel IDs"`
	Content  string `json:"content" description:"File content as text"`
	Filename string `json:"filename" description:"Filename"`
	Title    string `json:"title,omitempty" description:"File title"`
}

func (c *Client) sendMessageTool() daneel.Tool {
	return daneel.NewTool("slack.send_message", "Send a message to a Slack channel",
		func(ctx context.Context, p SendMessageParams) (string, error) {
			body := map[string]string{"channel": p.Channel, "text": p.Text}
			var r struct {
				TS      string `json:"ts"`
				Channel string `json:"channel"`
			}
			if err := c.call(ctx, "chat.postMessage", body, &r); err != nil {
				return "", err
			}
			return fmt.Sprintf("Message sent to %s (ts: %s)", r.Channel, r.TS), nil
		})
}

func (c *Client) readChannelTool() daneel.Tool {
	return daneel.NewTool("slack.read_channel", "Read recent messages from a Slack channel",
		func(ctx context.Context, p ReadChannelParams) (string, error) {
			limit := p.Limit
			if limit <= 0 {
				limit = 10
			}
			body := map[string]any{"channel": p.Channel, "limit": limit}
			var r struct {
				Messages []struct {
					User string `json:"user"`
					Text string `json:"text"`
					TS   string `json:"ts"`
				} `json:"messages"`
			}
			if err := c.call(ctx, "conversations.history", body, &r); err != nil {
				return "", err
			}
			if len(r.Messages) == 0 {
				return "No messages found.", nil
			}
			var sb strings.Builder
			for _, m := range r.Messages {
				fmt.Fprintf(&sb, "[%s] %s: %s\n", m.TS, m.User, m.Text)
			}
			return sb.String(), nil
		})
}

func (c *Client) reactTool() daneel.Tool {
	return daneel.NewTool("slack.react", "Add a reaction to a Slack message",
		func(ctx context.Context, p ReactParams) (string, error) {
			body := map[string]string{
				"channel":   p.Channel,
				"timestamp": p.Timestamp,
				"name":      p.Emoji,
			}
			if err := c.call(ctx, "reactions.add", body, nil); err != nil {
				return "", err
			}
			return fmt.Sprintf("Reacted with :%s:", p.Emoji), nil
		})
}

func (c *Client) createChannelTool() daneel.Tool {
	return daneel.NewTool("slack.create_channel", "Create a new Slack channel",
		func(ctx context.Context, p CreateChannelParams) (string, error) {
			body := map[string]string{"name": p.Name}
			var r struct {
				Channel struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"channel"`
			}
			if err := c.call(ctx, "conversations.create", body, &r); err != nil {
				return "", err
			}
			return fmt.Sprintf("Channel created: #%s (%s)", r.Channel.Name, r.Channel.ID), nil
		})
}

func (c *Client) listChannelsTool() daneel.Tool {
	return daneel.NewTool("slack.list_channels", "List Slack channels",
		func(ctx context.Context, p ListChannelsParams) (string, error) {
			limit := p.Limit
			if limit <= 0 {
				limit = 20
			}
			body := map[string]any{"limit": limit}
			var r struct {
				Channels []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"channels"`
			}
			if err := c.call(ctx, "conversations.list", body, &r); err != nil {
				return "", err
			}
			if len(r.Channels) == 0 {
				return "No channels found.", nil
			}
			var sb strings.Builder
			for _, ch := range r.Channels {
				fmt.Fprintf(&sb, "#%s (%s)\n", ch.Name, ch.ID)
			}
			return sb.String(), nil
		})
}

func (c *Client) uploadFileTool() daneel.Tool {
	return daneel.NewTool("slack.upload_file", "Upload a text file to Slack",
		func(ctx context.Context, p UploadFileParams) (string, error) {
			body := map[string]string{
				"channels":        p.Channels,
				"content":         p.Content,
				"filename":        p.Filename,
				"title":           p.Title,
				"initial_comment": "",
			}
			if err := c.call(ctx, "files.upload", body, nil); err != nil {
				return "", err
			}
			return fmt.Sprintf("File %s uploaded", p.Filename), nil
		})
}
