package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/daneel-ai/daneel"
)

// Tools returns all GitHub tools ready to register with an agent.
func Tools(token string, opts ...Option) []daneel.Tool {
	c := NewClient(token, opts...)
	return []daneel.Tool{
		c.createIssueTool(),
		c.closeIssueTool(),
		c.listIssuesTool(),
		c.commentTool(),
		c.createPRTool(),
		c.mergePRTool(),
		c.reviewPRTool(),
		c.searchCodeTool(),
	}
}

type CreateIssueParams struct {
	Owner string `json:"owner" description:"Repository owner"`
	Repo  string `json:"repo" description:"Repository name"`
	Title string `json:"title" description:"Issue title"`
	Body  string `json:"body,omitempty" description:"Issue body in markdown"`
}

type CloseIssueParams struct {
	Owner  string `json:"owner" description:"Repository owner"`
	Repo   string `json:"repo" description:"Repository name"`
	Number int    `json:"number" description:"Issue number"`
}

type ListIssuesParams struct {
	Owner string `json:"owner" description:"Repository owner"`
	Repo  string `json:"repo" description:"Repository name"`
	State string `json:"state,omitempty" description:"open closed or all"`
}

type CommentParams struct {
	Owner  string `json:"owner" description:"Repository owner"`
	Repo   string `json:"repo" description:"Repository name"`
	Number int    `json:"number" description:"Issue or PR number"`
	Body   string `json:"body" description:"Comment body in markdown"`
}

type CreatePRParams struct {
	Owner string `json:"owner" description:"Repository owner"`
	Repo  string `json:"repo" description:"Repository name"`
	Title string `json:"title" description:"PR title"`
	Head  string `json:"head" description:"Branch with changes"`
	Base  string `json:"base" description:"Target branch"`
	Body  string `json:"body,omitempty" description:"PR description"`
}

type MergePRParams struct {
	Owner  string `json:"owner" description:"Repository owner"`
	Repo   string `json:"repo" description:"Repository name"`
	Number int    `json:"number" description:"PR number"`
	Method string `json:"method,omitempty" description:"merge squash or rebase"`
}

type ReviewPRParams struct {
	Owner  string `json:"owner" description:"Repository owner"`
	Repo   string `json:"repo" description:"Repository name"`
	Number int    `json:"number" description:"PR number"`
	Event  string `json:"event" description:"APPROVE REQUEST_CHANGES or COMMENT"`
	Body   string `json:"body" description:"Review comment"`
}

type SearchCodeParams struct {
	Query string `json:"query" description:"GitHub code search query"`
}

func (c *Client) createIssueTool() daneel.Tool {
	return daneel.NewTool("github.create_issue", "Create a new GitHub issue",
		func(ctx context.Context, p CreateIssueParams) (string, error) {
			b := map[string]string{"title": p.Title, "body": p.Body}
			var r map[string]any
			if err := c.do(ctx, "POST", fmt.Sprintf("/repos/%s/%s/issues", p.Owner, p.Repo), b, &r); err != nil {
				return "", err
			}
			return fmt.Sprintf("Created issue #%.0f: %s", r["number"], r["html_url"]), nil
		})
}

func (c *Client) closeIssueTool() daneel.Tool {
	return daneel.NewTool("github.close_issue", "Close a GitHub issue",
		func(ctx context.Context, p CloseIssueParams) (string, error) {
			b := map[string]string{"state": "closed"}
			var r map[string]any
			if err := c.do(ctx, "PATCH", fmt.Sprintf("/repos/%s/%s/issues/%d", p.Owner, p.Repo, p.Number), b, &r); err != nil {
				return "", err
			}
			return fmt.Sprintf("Closed issue #%d", p.Number), nil
		})
}

func (c *Client) listIssuesTool() daneel.Tool {
	return daneel.NewTool("github.list_issues", "List issues in a repository",
		func(ctx context.Context, p ListIssuesParams) (string, error) {
			st := p.State
			if st == "" {
				st = "open"
			}
			var r []map[string]any
			if err := c.do(ctx, "GET", fmt.Sprintf("/repos/%s/%s/issues?state=%s", p.Owner, p.Repo, st), nil, &r); err != nil {
				return "", err
			}
			if len(r) == 0 {
				return "No issues found.", nil
			}
			var sb strings.Builder
			for _, i := range r {
				fmt.Fprintf(&sb, "#%.0f %s (%s)\n", i["number"], i["title"], i["state"])
			}
			return sb.String(), nil
		})
}

func (c *Client) commentTool() daneel.Tool {
	return daneel.NewTool("github.comment", "Comment on a GitHub issue or PR",
		func(ctx context.Context, p CommentParams) (string, error) {
			b := map[string]string{"body": p.Body}
			var r map[string]any
			if err := c.do(ctx, "POST", fmt.Sprintf("/repos/%s/%s/issues/%d/comments", p.Owner, p.Repo, p.Number), b, &r); err != nil {
				return "", err
			}
			return fmt.Sprintf("Comment added: %s", r["html_url"]), nil
		})
}

func (c *Client) createPRTool() daneel.Tool {
	return daneel.NewTool("github.create_pr", "Create a pull request",
		func(ctx context.Context, p CreatePRParams) (string, error) {
			b := map[string]string{"title": p.Title, "head": p.Head, "base": p.Base, "body": p.Body}
			var r map[string]any
			if err := c.do(ctx, "POST", fmt.Sprintf("/repos/%s/%s/pulls", p.Owner, p.Repo), b, &r); err != nil {
				return "", err
			}
			return fmt.Sprintf("Created PR #%.0f: %s", r["number"], r["html_url"]), nil
		})
}

func (c *Client) mergePRTool() daneel.Tool {
	return daneel.NewTool("github.merge_pr", "Merge a pull request",
		func(ctx context.Context, p MergePRParams) (string, error) {
			m := p.Method
			if m == "" {
				m = "merge"
			}
			b := map[string]string{"merge_method": m}
			var r map[string]any
			if err := c.do(ctx, "PUT", fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", p.Owner, p.Repo, p.Number), b, &r); err != nil {
				return "", err
			}
			return fmt.Sprintf("PR #%d merged", p.Number), nil
		})
}

func (c *Client) reviewPRTool() daneel.Tool {
	return daneel.NewTool("github.review_pr", "Review a pull request",
		func(ctx context.Context, p ReviewPRParams) (string, error) {
			b := map[string]string{"event": strings.ToUpper(p.Event), "body": p.Body}
			var r map[string]any
			if err := c.do(ctx, "POST", fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", p.Owner, p.Repo, p.Number), b, &r); err != nil {
				return "", err
			}
			return fmt.Sprintf("Review submitted: %s", p.Event), nil
		})
}

func (c *Client) searchCodeTool() daneel.Tool {
	return daneel.NewTool("github.search_code", "Search code on GitHub",
		func(ctx context.Context, p SearchCodeParams) (string, error) {
			var r struct {
				Total int `json:"total_count"`
				Items []struct {
					Path string `json:"path"`
					URL  string `json:"html_url"`
					Repo struct {
						Full string `json:"full_name"`
					} `json:"repository"`
				} `json:"items"`
			}
			if err := c.do(ctx, "GET", "/search/code?q="+p.Query, nil, &r); err != nil {
				return "", err
			}
			if r.Total == 0 {
				return "No results found.", nil
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "Found %d results:\n", r.Total)
			n := len(r.Items)
			if n > 10 {
				n = 10
			}
			for _, it := range r.Items[:n] {
				fmt.Fprintf(&sb, "- %s/%s: %s\n", it.Repo.Full, it.Path, it.URL)
			}
			return sb.String(), nil
		})
}
