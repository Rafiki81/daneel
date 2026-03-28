package twitter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/daneel-ai/daneel"
)

func Tools(bearerToken string, opts ...Option) []daneel.Tool {
	c := NewClient(bearerToken, opts...)
	return []daneel.Tool{
		c.tweetTool(), c.replyTool(), c.searchTool(),
		c.getMentionsTool(), c.followTool(), c.likeTool(), c.getUserTool(),
	}
}

type TweetParams struct {
	Text string `json:"text" description:"Tweet text (max 280 chars)"`
}

type ReplyParams struct {
	Text    string `json:"text" description:"Reply text"`
	TweetID string `json:"tweet_id" description:"ID of tweet to reply to"`
}

type SearchParams struct {
	Query      string `json:"query" description:"Search query"`
	MaxResults int    `json:"max_results,omitempty" description:"Max results (10-100, default 10)"`
}

type GetMentionsParams struct {
	UserID string `json:"user_id" description:"User ID to get mentions for"`
}

type FollowParams struct {
	TargetUserID string `json:"target_user_id" description:"User ID to follow"`
	UserID       string `json:"user_id" description:"Your user ID"`
}

type LikeParams struct {
	TweetID string `json:"tweet_id" description:"Tweet ID to like"`
	UserID  string `json:"user_id" description:"Your user ID"`
}

type GetUserParams struct {
	Username string `json:"username" description:"Twitter username (without @)"`
}

func (c *Client) tweetTool() daneel.Tool {
	return daneel.NewTool("twitter.tweet", "Post a new tweet",
		func(ctx context.Context, p TweetParams) (string, error) {
			body := map[string]string{"text": p.Text}
			var r struct {
				Data struct {
					ID   string `json:"id"`
					Text string `json:"text"`
				} `json:"data"`
			}
			if err := c.do(ctx, "POST", "/tweets", body, &r); err != nil {
				return "", err
			}
			return fmt.Sprintf("Tweet posted (id: %s)", r.Data.ID), nil
		})
}

func (c *Client) replyTool() daneel.Tool {
	return daneel.NewTool("twitter.reply", "Reply to a tweet",
		func(ctx context.Context, p ReplyParams) (string, error) {
			body := map[string]any{
				"text":  p.Text,
				"reply": map[string]string{"in_reply_to_tweet_id": p.TweetID},
			}
			var r struct {
				Data struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			if err := c.do(ctx, "POST", "/tweets", body, &r); err != nil {
				return "", err
			}
			return fmt.Sprintf("Reply posted (id: %s)", r.Data.ID), nil
		})
}

func (c *Client) searchTool() daneel.Tool {
	return daneel.NewTool("twitter.search_tweets", "Search recent tweets",
		func(ctx context.Context, p SearchParams) (string, error) {
			max := p.MaxResults
			if max <= 0 {
				max = 10
			}
			path := fmt.Sprintf("/tweets/search/recent?query=%s&max_results=%d", url.QueryEscape(p.Query), max)
			var r struct {
				Data []struct {
					ID   string `json:"id"`
					Text string `json:"text"`
				} `json:"data"`
			}
			if err := c.do(ctx, "GET", path, nil, &r); err != nil {
				return "", err
			}
			if len(r.Data) == 0 {
				return "No tweets found.", nil
			}
			var sb strings.Builder
			for _, t := range r.Data {
				fmt.Fprintf(&sb, "[%s] %s\n", t.ID, t.Text)
			}
			return sb.String(), nil
		})
}

func (c *Client) getMentionsTool() daneel.Tool {
	return daneel.NewTool("twitter.get_mentions", "Get recent mentions for a user",
		func(ctx context.Context, p GetMentionsParams) (string, error) {
			var r struct {
				Data []struct {
					ID   string `json:"id"`
					Text string `json:"text"`
				} `json:"data"`
			}
			if err := c.do(ctx, "GET", fmt.Sprintf("/users/%s/mentions", p.UserID), nil, &r); err != nil {
				return "", err
			}
			if len(r.Data) == 0 {
				return "No mentions found.", nil
			}
			b, _ := json.Marshal(r.Data)
			return string(b), nil
		})
}

func (c *Client) followTool() daneel.Tool {
	return daneel.NewTool("twitter.follow", "Follow a user",
		func(ctx context.Context, p FollowParams) (string, error) {
			body := map[string]string{"target_user_id": p.TargetUserID}
			if err := c.do(ctx, "POST", fmt.Sprintf("/users/%s/following", p.UserID), body, nil); err != nil {
				return "", err
			}
			return fmt.Sprintf("Followed user %s", p.TargetUserID), nil
		})
}

func (c *Client) likeTool() daneel.Tool {
	return daneel.NewTool("twitter.like", "Like a tweet",
		func(ctx context.Context, p LikeParams) (string, error) {
			body := map[string]string{"tweet_id": p.TweetID}
			if err := c.do(ctx, "POST", fmt.Sprintf("/users/%s/likes", p.UserID), body, nil); err != nil {
				return "", err
			}
			return fmt.Sprintf("Liked tweet %s", p.TweetID), nil
		})
}

func (c *Client) getUserTool() daneel.Tool {
	return daneel.NewTool("twitter.get_user", "Get user profile by username",
		func(ctx context.Context, p GetUserParams) (string, error) {
			var r struct {
				Data struct {
					ID       string `json:"id"`
					Name     string `json:"name"`
					Username string `json:"username"`
					Bio      string `json:"description"`
				} `json:"data"`
			}
			if err := c.do(ctx, "GET", fmt.Sprintf("/users/by/username/%s?user.fields=description", p.Username), nil, &r); err != nil {
				return "", err
			}
			b, _ := json.Marshal(r.Data)
			return string(b), nil
		})
}
