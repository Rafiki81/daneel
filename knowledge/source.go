package knowledge

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
)

type ingestConfig struct {
	source   string
	metadata map[string]string
}

// IngestOption configures an ingest operation.
type IngestOption func(*ingestConfig)

// WithSource sets the source label for ingested documents.
func WithSource(source string) IngestOption {
	return func(c *ingestConfig) { c.source = source }
}

// WithMetadata attaches a key/value pair to ingested documents.
func WithMetadata(key, value string) IngestOption {
	return func(c *ingestConfig) {
		if c.metadata == nil {
			c.metadata = make(map[string]string)
		}
		c.metadata[key] = value
	}
}

func applyIngestOptions(opts []IngestOption) *ingestConfig {
	c := &ingestConfig{metadata: make(map[string]string)}
	for _, o := range opts {
		o(c)
	}
	return c
}

func loadFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("knowledge: read file %q: %w", path, err)
	}
	return string(b), nil
}

func loadURL(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("knowledge: build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("knowledge: fetch %q: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("knowledge: HTTP %d fetching %q", resp.StatusCode, url)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", fmt.Errorf("knowledge: read body: %w", err)
	}
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "html") {
		return stripHTML(string(body)), nil
	}
	return string(body), nil
}

var (
	reTag      = regexp.MustCompile(`<[^>]+>`)
	reSpaces   = regexp.MustCompile(`[ \t]+`)
	reNewlines = regexp.MustCompile(`\n{3,}`)
)

func stripHTML(html string) string {
	s := reTag.ReplaceAllString(html, " ")
	s = reSpaces.ReplaceAllString(s, " ")
	s = reNewlines.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
