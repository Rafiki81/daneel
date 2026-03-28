package telegram

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Rafiki81/daneel"
)

// Tools returns all Telegram tools for an agent.
func Tools(token string, opts ...Option) []daneel.Tool {
	c := NewClient(token, opts...)
	return []daneel.Tool{
		c.sendMessageTool(),
		c.sendPhotoTool(),
		c.replyTool(),
		c.getUpdatesTool(),
	}
}

type SendMessageParams struct {
	ChatID string `json:"chat_id" description:"Chat ID to send to"`
	Text   string `json:"text" description:"Message text"`
}

type SendPhotoParams struct {
	ChatID  string `json:"chat_id" description:"Chat ID to send to"`
	Photo   string `json:"photo" description:"Photo URL or file_id"`
	Caption string `json:"caption,omitempty" description:"Photo caption"`
}

type ReplyParams struct {
	ChatID    string `json:"chat_id" description:"Chat ID"`
	MessageID int    `json:"message_id" description:"Message to reply to"`
	Text      string `json:"text" description:"Reply text"`
}

type GetUpdatesParams struct {
	Offset int `json:"offset,omitempty" description:"Update offset for pagination"`
	Limit  int `json:"limit,omitempty" description:"Max updates to fetch"`
}

func (c *Client) sendMessageTool() daneel.Tool {
	return daneel.NewTool("telegram.send_message", "Send a message via Telegram",
		func(ctx context.Context, p SendMessageParams) (string, error) {
			body := map[string]any{"chat_id": p.ChatID, "text": p.Text}
			var r struct {
				MessageID int `json:"message_id"`
			}
			if err := c.call(ctx, "sendMessage", body, &r); err != nil {
				return "", err
			}
			return fmt.Sprintf("Message sent (id: %d)", r.MessageID), nil
		})
}

func (c *Client) sendPhotoTool() daneel.Tool {
	return daneel.NewTool("telegram.send_photo", "Send a photo via Telegram",
		func(ctx context.Context, p SendPhotoParams) (string, error) {
			body := map[string]any{"chat_id": p.ChatID, "photo": p.Photo}
			if p.Caption != "" {
				body["caption"] = p.Caption
			}
			var r struct {
				MessageID int `json:"message_id"`
			}
			if err := c.call(ctx, "sendPhoto", body, &r); err != nil {
				return "", err
			}
			return fmt.Sprintf("Photo sent (id: %d)", r.MessageID), nil
		})
}

func (c *Client) replyTool() daneel.Tool {
	return daneel.NewTool("telegram.reply", "Reply to a Telegram message",
		func(ctx context.Context, p ReplyParams) (string, error) {
			body := map[string]any{
				"chat_id":             p.ChatID,
				"text":                p.Text,
				"reply_to_message_id": p.MessageID,
			}
			var r struct {
				MessageID int `json:"message_id"`
			}
			if err := c.call(ctx, "sendMessage", body, &r); err != nil {
				return "", err
			}
			return fmt.Sprintf("Reply sent (id: %d)", r.MessageID), nil
		})
}

func (c *Client) getUpdatesTool() daneel.Tool {
	return daneel.NewTool("telegram.get_updates", "Get recent Telegram updates",
		func(ctx context.Context, p GetUpdatesParams) (string, error) {
			body := map[string]any{}
			if p.Offset > 0 {
				body["offset"] = p.Offset
			}
			if p.Limit > 0 {
				body["limit"] = p.Limit
			}
			var updates []map[string]any
			if err := c.call(ctx, "getUpdates", body, &updates); err != nil {
				return "", err
			}
			if len(updates) == 0 {
				return "No new updates.", nil
			}
			out, _ := json.Marshal(updates)
			return string(out), nil
		})
}
