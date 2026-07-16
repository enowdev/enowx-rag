package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/enowdev/enowx-rag/pkg/core"
	"github.com/enowdev/enowx-rag/pkg/migrate"
	"github.com/enowdev/enowx-rag/pkg/migrate/cloud"
	"github.com/enowdev/enowx-rag/pkg/rag"
	"github.com/enowdev/enowx-rag/pkg/ragbuild"
)

// migrateRequest describes a re-embed / move migration. The source is a project
// in the current (running) provider; the destination is built from an explicit
// spec so it can differ in store, embedder, model, dimension, or pgvector table.
type migrateRequest struct {
	SourceProject string `json:"source_project"`
	DestProject   string `json:"dest_project"`

	// Optional external cloud source. When set, data is read from this vendor
	// instead of the running provider. Only Qdrant is verified; others are
	// experimental (see pkg/migrate/cloud).
	CloudSource *struct {
		Provider  string `json:"provider"` // qdrant | pinecone | weaviate | chroma
		URL       string `json:"url"`
		APIKey    string `json:"api_key"`
		Index     string `json:"index"`
		TextField string `json:"text_field"`
	} `json:"cloud_source"`

	VectorStore   string `json:"vector_store"`
	Embedder      string `json:"embedder"`
	QdrantURL     string `json:"qdrant_url"`
	QdrantAPIKey  string `json:"qdrant_api_key"`
	ChromaURL     string `json:"chroma_url"`
	PGVectorDSN   string `json:"pgvector_dsn"`
	PGVectorTable string `json:"pgvector_table"`
	VoyageAPIKey  string `json:"voyage_api_key"`
	VoyageModel   string `json:"voyage_model"`
	VoyageDim     int    `json:"voyage_dim"`
	OpenAIAPIKey  string `json:"openai_api_key"`
	OpenAIModel   string `json:"openai_model"`
	OpenAIBaseURL string `json:"openai_base_url"`
	OpenAIDim     int    `json:"openai_dim"`
	TEIURL        string `json:"tei_url"`
}

// Migrate handles POST /api/migrate. It starts the migration asynchronously and
// returns 202 immediately; progress is streamed over SSE as migration_started /
// migration_progress / migration_completed / migration_failed events.
func (h *Handlers) Migrate(w http.ResponseWriter, r *http.Request) {
	var req migrateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.SourceProject == "" || req.DestProject == "" {
		writeErr(w, http.StatusBadRequest, "source_project and dest_project are required")
		return
	}
	if req.CloudSource == nil && req.SourceProject == req.DestProject && sameStore(req) {
		writeErr(w, http.StatusBadRequest, "destination must differ from source (project name or store)")
		return
	}

	// Build the destination provider from the request spec (may differ from the
	// running provider in store/model/dimension/table).
	dst, err := ragbuild.BuildProvider(r.Context(), ragbuild.Spec{
		VectorStore:   req.VectorStore,
		Embedder:      req.Embedder,
		QdrantURL:     req.QdrantURL,
		QdrantAPIKey:  req.QdrantAPIKey,
		ChromaURL:     req.ChromaURL,
		PGVectorDSN:   req.PGVectorDSN,
		PGVectorTable: req.PGVectorTable,
		VoyageAPIKey:  req.VoyageAPIKey,
		VoyageModel:   req.VoyageModel,
		VoyageDim:     req.VoyageDim,
		OpenAIAPIKey:  req.OpenAIAPIKey,
		OpenAIModel:   req.OpenAIModel,
		OpenAIBaseURL: req.OpenAIBaseURL,
		OpenAIDim:     req.OpenAIDim,
		TEIURL:        req.TEIURL,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "destination config: "+err.Error())
		return
	}

	// Source: an external cloud connector when cloud_source is set, otherwise
	// the running provider (which must support export).
	var src rag.Exporter
	if req.CloudSource != nil {
		cs, cerr := cloud.NewExporter(r.Context(), cloud.Source{
			Provider:  req.CloudSource.Provider,
			URL:       req.CloudSource.URL,
			APIKey:    req.CloudSource.APIKey,
			Index:     req.CloudSource.Index,
			TextField: req.CloudSource.TextField,
		})
		if cerr != nil {
			writeErr(w, http.StatusBadRequest, "cloud source: "+cerr.Error())
			return
		}
		src = cs
	} else {
		p, ok := h.svc.Provider().(rag.Exporter)
		if !ok {
			writeErr(w, http.StatusBadRequest, "the current vector store does not support export")
			return
		}
		src = p
	}

	events := h.svc.Events()
	m := &migrate.Migrator{Src: src, Dst: dst, BatchSize: 64}

	events.Publish(core.Event{
		Type:      "migration_started",
		Timestamp: time.Now(),
		Data:      map[string]any{"source": req.SourceProject, "dest": req.DestProject},
	})

	// Run asynchronously; progress via SSE. Use a background context so the
	// migration is not cancelled when this HTTP request returns.
	go func() {
		ctx := context.Background()
		lastPct := -1
		_, err := m.Run(ctx, req.SourceProject, req.DestProject, func(p migrate.Progress) {
			// Throttle: publish only when the integer percent changes, so the
			// bounded event bus is not flooded on large migrations.
			pct := 0
			if p.Total > 0 {
				pct = p.Done * 100 / p.Total
			}
			if pct == lastPct {
				return
			}
			lastPct = pct
			events.Publish(core.Event{
				Type:      "migration_progress",
				Timestamp: time.Now(),
				Data:      map[string]any{"done": p.Done, "total": p.Total, "percent": pct, "dest": req.DestProject},
			})
		})
		if err != nil {
			events.Publish(core.Event{
				Type:      "migration_failed",
				Timestamp: time.Now(),
				Data:      map[string]any{"source": req.SourceProject, "dest": req.DestProject, "error": err.Error()},
			})
			_ = dst.Close()
			return
		}
		events.Publish(core.Event{
			Type:      "migration_completed",
			Timestamp: time.Now(),
			Data:      map[string]any{"source": req.SourceProject, "dest": req.DestProject},
		})
		_ = dst.Close()
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status": "started",
		"source": req.SourceProject,
		"dest":   req.DestProject,
	})
}

// sameStore reports whether the request's destination store config matches the
// running provider closely enough that a same-name migration would be a no-op
// target. We keep this conservative: only block when store type is empty
// (interpreted as "same store") — a different store or project is always allowed.
func sameStore(req migrateRequest) bool {
	return req.VectorStore == ""
}
