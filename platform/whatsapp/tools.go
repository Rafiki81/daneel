package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/daneel-ai/daneel"
)

// Tools returns WhatsApp Business API tools for an agent.
func (c *Client) Tools() []daneel.Tool {
	return []daneel.Tool{
		daneel.NewTool("send_message",
			"Send a WhatsApp text message to a phone number",
			func(ctx context.Context, p struct {
				To   string `json:"to" description:"Recipient phone number in E.164 format"`
				Text string `json:"text" description:"Message text"`
			}) (string, error) {
				body := map[string]any{
					"messaging_product": "whatsapp",
					"to":                p.To,
					"type":              "text",
					"text":              map[string]string{"body": p.Text},
				}
				var res map[string]any
				err := c.do(ctx, "POST", "/"+c.phoneID+"/messages", body, &res)
				if err != nil {
					return "", err
				}
				b, _ := json.Marshal(res)
				return string(b), nil
			},
		),

		daneel.NewTool("send_media",
			"Send a media message (image, document, video, audio) via WhatsApp",
			func(ctx context.Context, p struct {
				To        string `json:"to" description:"Recipient phone number in E.164 format"`
				MediaType string `json:"media_type" description:"Type: image, document, video, or audio"`
				URL       string `json:"url" description:"Public URL of the media file"`
				Caption   string `json:"caption,omitempty" description:"Optional caption"`
			}) (string, error) {
				media := map[string]string{"link": p.URL}
				if p.Caption != "" {
					media["caption"] = p.Caption
				}
				body := map[string]any{
					"messaging_product": "whatsapp",
					"to":                p.To,
					"type":              p.MediaType,
					p.MediaType:         media,
				}
				var res map[string]any
				err := c.do(ctx, "POST", "/"+c.phoneID+"/messages", body, &res)
				if err != nil {
					return "", err
				}
				b, _ := json.Marshal(res)
				return string(b), nil
			},
		),

		daneel.NewTool("get_messages",
			"Retrieve recent messages from WhatsApp Business API (requires webhook data or a message store)",
			func(ctx context.Context, p struct {
				Phone string `json:"phone,omitempty" description:"Filter by phone number (optional)"`
				Limit int    `json:"limit,omitempty" description:"Max messages to return"`
			}) (string, error) {
				// WhatsApp Business API does not have a "list messages" endpoint.
				// Messages are received via webhooks. This tool serves as a placeholder
				// that can be backed by a local store populated by the connector.
				return `{"note":"Messages are received via webhooks. Use the connector to listen for incoming messages."}`, nil
			},
		),

		daneel.NewTool("get_contacts",
			"Get the WhatsApp Business profile information",
			func(ctx context.Context, p struct{}) (string, error) {
				path := fmt.Sprintf("/%s/whatsapp_business_profile?fields=about,address,description,email,profile_picture_url,websites,vertical", c.phoneID)
				var res map[string]any
				err := c.do(ctx, "GET", path, nil, &res)
				if err != nil {
					return "", err
				}
				b, _ := json.Marshal(res)
				return string(b), nil
			},
		),
	}
}
