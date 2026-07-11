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
	ID      string            `json:"id"`
	Content string            `json:"content"`
	Score   float64           `json:"score"`
	Meta    map[string]string `json:"meta,omitempty"`
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

	// DeletePoints removes specific points by ID from the project collection.
	DeletePoints(ctx context.Context, projectID string, pointIDs []string) error

	// ListPointIDs returns all point IDs in the project collection, optionally filtered by metadata.
	ListPointIDs(ctx context.Context, projectID string, metaFilter map[string]string) ([]string, error)

	// ListPoints returns all points (ID + source_file payload) in the project
	// collection, optionally filtered by metadata.
	ListPoints(ctx context.Context, projectID string, metaFilter map[string]string) ([]PointInfo, error)

	// Close releases resources.
	Close() error
}

// EmbeddingClient abstracts text embedding models (TEI, OpenAI, local, etc.).
type EmbeddingClient interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	VectorSize() int
}

// QueryEmbedder is an optional interface for embedders that distinguish
// query vs document inputs (e.g. Voyage AI input_type=query). Providers check
// for this via type assertion in SemanticSearch and prefer EmbedQuery when
// available, falling back to Embed otherwise.
type QueryEmbedder interface {
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
}

// ModelNamer is an optional interface for embedders that expose their model
// name. Providers use this in Index() to inject embed_model into document
// metadata before persisting.
type ModelNamer interface {
	ModelName() string
}

// HybridSearcher is an optional interface for providers that support hybrid
// search combining dense vector similarity with lexical full-text search
// using Reciprocal Rank Fusion (RRF). Providers that implement this
// interface are used by core.Service.Search when opts.Hybrid is true.
// Providers that do not implement it fall back to dense-only SemanticSearch.
type HybridSearcher interface {
	SemanticSearchHybrid(ctx context.Context, projectID, query string, limit int) ([]Result, error)
}

// RerankHit is a single reranked result: the Index of the original document
// in the input slice and its relevance Score from the reranker.
type RerankHit struct {
	Index int     `json:"index"`
	Score float64 `json:"relevance_score"`
}

// Reranker is an optional interface for reranking retrieved documents.
// Implementations (e.g. VoyageReranker) call a rerank API to re-order
// candidates by relevance to the query.
type Reranker interface {
	Rerank(ctx context.Context, query string, docs []string, topK int) ([]RerankHit, error)
}

// PointInfo is a stored point's ID together with the payload fields needed to
// reconcile it against the current file set during incremental sync. Content
// and ChunkIndex are populated by ListPoints when the UI needs to display
// chunk previews (e.g. the Chunks page).
type PointInfo struct {
	ID           string `json:"id"`
	SourceFile   string `json:"source_file,omitempty"`
	ContentHash  string `json:"content_hash,omitempty"`
	ChunkVersion string `json:"chunk_version,omitempty"`
	DocID        string `json:"doc_id,omitempty"` // original document ID (Qdrant stores UUID, doc_id preserves the original)
	Content      string `json:"content,omitempty"`
	ChunkIndex   string `json:"chunk_index,omitempty"`
}
