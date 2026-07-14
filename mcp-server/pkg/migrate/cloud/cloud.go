// Package cloud provides connectors that read documents (text stored in
// metadata) from external cloud vector databases, exposing them as rag.Exporter
// so the migration engine can re-embed them into enowx-rag.
//
// Verification status:
//   - Qdrant Cloud: uses the same rag.QdrantProvider pointed at a remote URL,
//     so it shares the fully-tested Qdrant code path. VERIFIED.
//   - Pinecone / Weaviate / Chroma Cloud: implemented from each vendor's API
//     docs and covered by mock-based tests only. They have NOT been verified
//     against a live vendor account and should be treated as EXPERIMENTAL —
//     the same honesty policy applied to the Chroma provider.
package cloud

import (
	"context"
	"fmt"
	"strings"

	"github.com/enowdev/enowx-rag/pkg/rag"
)

// Source identifies an external vector DB and where to read text from.
type Source struct {
	Provider string // "qdrant" | "pinecone" | "weaviate" | "chroma"
	URL      string // endpoint / host
	APIKey   string
	Index    string // index / collection / class name
	// TextField is the metadata key that holds the chunk text. Defaults to
	// "content" (what enowx-rag itself stores); vendors differ, so it's tunable.
	TextField string
}

// Experimental reports whether this source connector is unverified against a
// live vendor (everything except Qdrant, which reuses the tested provider).
func (s Source) Experimental() bool {
	return strings.ToLower(s.Provider) != "qdrant"
}

// NewExporter returns a rag.Exporter for the given cloud source.
func NewExporter(ctx context.Context, s Source) (rag.Exporter, error) {
	tf := s.TextField
	if tf == "" {
		tf = "content"
	}
	switch strings.ToLower(s.Provider) {
	case "qdrant":
		// Reuse the fully-tested Qdrant provider against the remote URL. Its
		// ExportPoints reads payload["content"] and doc_id, which matches how
		// enowx-rag itself stores data; for foreign Qdrant collections with a
		// different text field, PineconeExporter-style mapping would be needed,
		// but the common case (Qdrant Cloud holding enowx-rag data) works.
		p, err := rag.NewQdrantProvider(ctx, s.URL, s.APIKey, noopEmbedder{})
		if err != nil {
			return nil, err
		}
		return &qdrantCloud{p: p, index: s.Index}, nil
	case "pinecone":
		return &pineconeExporter{url: s.URL, apiKey: s.APIKey, index: s.Index, textField: tf}, nil
	case "weaviate":
		return &weaviateExporter{url: s.URL, apiKey: s.APIKey, class: s.Index, textField: tf}, nil
	case "chroma":
		return &chromaCloudExporter{url: s.URL, apiKey: s.APIKey, collection: s.Index, textField: tf}, nil
	default:
		return nil, fmt.Errorf("unsupported cloud source: %s", s.Provider)
	}
}

// qdrantCloud adapts a QdrantProvider so ExportPoints uses the source Index as
// the "project" (Qdrant collections are named project_<id>; a raw collection
// name is passed through by the provider's collectionName sanitization).
type qdrantCloud struct {
	p     *rag.QdrantProvider
	index string
}

func (q *qdrantCloud) ExportPoints(ctx context.Context, _ string) ([]rag.Document, error) {
	return q.p.ExportPoints(ctx, q.index)
}

// noopEmbedder satisfies rag.EmbeddingClient for read-only export (no embedding
// happens on the source side — the destination re-embeds).
type noopEmbedder struct{}

func (noopEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return make([][]float32, len(texts)), nil
}
func (noopEmbedder) VectorSize() int { return 1 }
