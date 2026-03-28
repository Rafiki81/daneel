package knowledge

// Document represents a piece of text that can be ingested into a KnowledgeBase.
type Document struct {
	Content  string
	Source   string
	Metadata map[string]string
}

// Embedding is a stored vector for a single chunk of a Document.
type Embedding struct {
	ID             string
	Vector         []float32
	Text           string
	DocumentSource string
	ChunkIndex     int
	Metadata       map[string]string
}
