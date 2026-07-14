// Package migrate moves a project's data from a source rag.Provider to a
// destination provider, re-embedding through the destination's embedder. This
// is how enowx-rag changes embedding dimension/model or moves between vector
// stores: the source's stored text is exported (never the raw vectors, which
// are model-specific and non-portable) and re-embedded at the destination.
package migrate

import (
	"context"
	"fmt"

	"github.com/enowdev/enowx-rag/pkg/rag"
)

// Progress reports migration advancement. Total is the number of documents to
// migrate; Done is how many have been written so far.
type Progress struct {
	Done  int
	Total int
}

// Migrator re-embeds a project from Src into Dst.
type Migrator struct {
	Src rag.Provider
	Dst rag.Provider
	// BatchSize controls how many documents are written per Index call.
	BatchSize int
}

// Run exports every document from the source project, creates the destination
// collection, and writes the documents in batches (re-embedded by Dst). onProgress
// is called after each batch (throttled to batch granularity, so the event bus
// is not flooded per-document). It returns the number of documents migrated.
func (m *Migrator) Run(ctx context.Context, srcProject, dstProject string, onProgress func(Progress)) (int, error) {
	exporter, ok := m.Src.(rag.Exporter)
	if !ok {
		return 0, fmt.Errorf("source vector store does not support export")
	}
	docs, err := exporter.ExportPoints(ctx, srcProject)
	if err != nil {
		return 0, fmt.Errorf("export source: %w", err)
	}
	total := len(docs)

	if err := m.Dst.CreateCollection(ctx, dstProject); err != nil {
		return 0, fmt.Errorf("create destination collection: %w", err)
	}

	batch := m.BatchSize
	if batch <= 0 {
		batch = 64
	}
	done := 0
	if onProgress != nil {
		onProgress(Progress{Done: 0, Total: total})
	}
	for i := 0; i < total; i += batch {
		end := i + batch
		if end > total {
			end = total
		}
		if err := ctx.Err(); err != nil {
			return done, err
		}
		if err := m.Dst.Index(ctx, dstProject, docs[i:end]); err != nil {
			return done, fmt.Errorf("write destination batch: %w", err)
		}
		done = end
		if onProgress != nil {
			onProgress(Progress{Done: done, Total: total})
		}
	}
	return done, nil
}
