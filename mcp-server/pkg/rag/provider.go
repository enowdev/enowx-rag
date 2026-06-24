// Package rag abstracts vector stores and embedding backends used by the MCP server.
// The Provider interface is intentionally small: one project = one collection/index.
package rag

import "context"

// Document is a unit of content stored in a RAG collection.
type Document struct {
	ID      string
	Content string
	Meta    map[string]string
}

// Result is a retrieved chunk with its similarity score.
type Result struct {
	ID      string
	Content string
	Score   float64
	Meta    map[string]string
}

// Provider describes a RAG backend capable of per-project collections.
type Provider interface {
	// CreateCollection creates the project collection if it doesn't exist.
	CreateCollection(ctx context.Context, projectID string) error

	// DeleteCollection removes the project collection.
	DeleteCollection(ctx context.Context, projectID string) error

	// Index inserts documents into the project collection, embedding them.
	Index(ctx context.Context, projectID string, docs []Document) error

	// SemanticSearch returns the k most relevant chunks for a query.
	SemanticSearch(ctx context.Context, projectID, query string, limit int) ([]Result, error)

	// Embed returns the vector for a single text (useful for debugging).
	Embed(ctx context.Context, text string) ([]float32, error)

	// Close releases resources.
	Close() error
}

// EmbeddingClient abstracts text embedding models (TEI, OpenAI, local, etc.).
type EmbeddingClient interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	VectorSize() int
}
