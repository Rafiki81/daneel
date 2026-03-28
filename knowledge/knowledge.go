package knowledge

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/Rafiki81/daneel"
)

type contextKey struct{}

// KnowledgeBase ingests documents and retrieves relevant passages for agents.
type KnowledgeBase struct {
	embedder daneel.Embedder
	store    daneel.VectorStore
	chunker  Chunker
}

// Option configures a KnowledgeBase.
type Option func(*KnowledgeBase)

// WithEmbedder sets the embedding model.
func WithEmbedder(e daneel.Embedder) Option {
	return func(kb *KnowledgeBase) { kb.embedder = e }
}

// WithStore sets the vector store backend.
func WithStore(vs daneel.VectorStore) Option {
	return func(kb *KnowledgeBase) { kb.store = vs }
}

// WithChunker overrides the default chunking strategy.
func WithChunker(c Chunker) Option {
	return func(kb *KnowledgeBase) { kb.chunker = c }
}

// New creates a KnowledgeBase. Embedder and VectorStore must be provided via options.
func New(opts ...Option) *KnowledgeBase {
	kb := &KnowledgeBase{
		chunker: Recursive(500, 50),
	}
	for _, o := range opts {
		o(kb)
	}
	return kb
}

// IngestText ingests raw text content.
func (kb *KnowledgeBase) IngestText(ctx context.Context, text string, opts ...IngestOption) error {
	cfg := applyIngestOptions(opts)
	source := cfg.source
	if source == "" {
		source = "text"
	}
	return kb.ingestDocument(ctx, Document{Content: text, Source: source, Metadata: cfg.metadata})
}

// IngestFile reads a file from disk and ingests its content.
func (kb *KnowledgeBase) IngestFile(ctx context.Context, path string, opts ...IngestOption) error {
	cfg := applyIngestOptions(opts)
	source := cfg.source
	if source == "" {
		source = path
	}
	text, err := loadFile(path)
	if err != nil {
		return err
	}
	return kb.ingestDocument(ctx, Document{Content: text, Source: source, Metadata: cfg.metadata})
}

// IngestURL fetches a URL and ingests its content.
func (kb *KnowledgeBase) IngestURL(ctx context.Context, url string, opts ...IngestOption) error {
	cfg := applyIngestOptions(opts)
	source := cfg.source
	if source == "" {
		source = url
	}
	text, err := loadURL(ctx, url)
	if err != nil {
		return err
	}
	return kb.ingestDocument(ctx, Document{Content: text, Source: source, Metadata: cfg.metadata})
}

func (kb *KnowledgeBase) ingestDocument(ctx context.Context, doc Document) error {
	if kb.embedder == nil {
		return fmt.Errorf("knowledge: no embedder configured")
	}
	if kb.store == nil {
		return fmt.Errorf("knowledge: no vector store configured")
	}
	chunks := kb.chunker.Chunk(doc.Content)
	for i, ch := range chunks {
		vec, err := kb.embedder.Embed(ctx, ch.Text)
		if err != nil {
			return fmt.Errorf("knowledge: embed chunk %d: %w", i, err)
		}
		id := chunkID(doc.Source, i, ch.Text)
		meta := map[string]string{
			"text":   ch.Text,
			"source": doc.Source,
		}
		for k, v := range doc.Metadata {
			meta[k] = v
		}
		if err := kb.store.Store(ctx, id, vec, meta); err != nil {
			return fmt.Errorf("knowledge: store chunk %d: %w", i, err)
		}
	}
	return nil
}

// Search embeds the query and returns the topK most relevant text passages.
func (kb *KnowledgeBase) Search(ctx context.Context, query string, topK int) ([]string, error) {
	if kb.embedder == nil {
		return nil, fmt.Errorf("knowledge: no embedder configured")
	}
	if kb.store == nil {
		return nil, fmt.Errorf("knowledge: no vector store configured")
	}
	vec, err := kb.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("knowledge: embed query: %w", err)
	}
	results, err := kb.store.Search(ctx, vec, topK)
	if err != nil {
		return nil, fmt.Errorf("knowledge: search: %w", err)
	}
	texts := make([]string, 0, len(results))
	for _, r := range results {
		if t, ok := r.Metadata["text"]; ok && t != "" {
			texts = append(texts, t)
		}
	}
	return texts, nil
}

// WithRetrieverQuery attaches a retrieval query to a context for use by Retriever.
func WithRetrieverQuery(ctx context.Context, query string) context.Context {
	return context.WithValue(ctx, contextKey{}, query)
}

// Retriever returns a context function suitable for use with daneel.WithContextFunc.
// The returned function reads the query from the context (set via WithRetrieverQuery)
// or falls back to the supplied defaultQuery.
func (kb *KnowledgeBase) Retriever(topK int, defaultQuery string) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		query := defaultQuery
		if q, ok := ctx.Value(contextKey{}).(string); ok && q != "" {
			query = q
		}
		if query == "" {
			return "", nil
		}
		passages, err := kb.Search(ctx, query, topK)
		if err != nil {
			return "", err
		}
		return strings.Join(passages, "\n\n---\n\n"), nil
	}
}

// searchParams is the typed parameter struct for SearchTool.
type searchParams struct {
	Query string `json:"query" desc:"Search query to find relevant passages"`
}

// SearchTool exposes knowledge-base search as a daneel.Tool.
func (kb *KnowledgeBase) SearchTool(name, description string, topK int) daneel.Tool {
	return daneel.NewTool(name, description, func(ctx context.Context, p searchParams) (string, error) {
		results, err := kb.Search(ctx, p.Query, topK)
		if err != nil {
			return "", err
		}
		return strings.Join(results, "\n\n---\n\n"), nil
	})
}

func chunkID(source string, idx int, text string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s:%d:%s", source, idx, text)
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}
