package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/enowdev/enowx-rag/pkg/core"
	"github.com/enowdev/enowx-rag/pkg/rag"
	"github.com/go-chi/chi/v5"
)

// Handlers holds the service layer reference shared by all HTTP handlers.
type Handlers struct {
	svc *core.Service
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr writes a JSON error response with the given status code and message.
func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// NotFound handles unknown /api/ routes with a JSON 404 response.
func (h *Handlers) NotFound(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusNotFound, "API endpoint not found")
}

// ListProjects handles GET /api/projects.
// Returns a JSON array of projects with chunk counts. Empty state returns [].
func (h *Handlers) ListProjects(w http.ResponseWriter, r *http.Request) {
	stats, err := h.svc.ListProjects(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if stats == nil {
		stats = []core.ProjectStat{}
	}
	writeJSON(w, http.StatusOK, stats)
}

// GetProject handles GET /api/projects/{id}.
// Returns 200 with project detail for existing, 404 JSON for missing.
func (h *Handlers) GetProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if projectID == "" {
		writeErr(w, http.StatusBadRequest, "project id is required")
		return
	}

	// List all projects and find the one matching the ID.
	stats, err := h.svc.ListProjects(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	for _, s := range stats {
		if s.ProjectID == projectID {
			writeJSON(w, http.StatusOK, s)
			return
		}
	}

	// If not found in list, try to list points to see if the project exists.
	points, err := h.svc.ListPoints(r.Context(), projectID, nil)
	if err != nil {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}
	if points == nil {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, core.ProjectStat{
		ProjectID:  projectID,
		ChunkCount: len(points),
	})
}

// ListPoints handles GET /api/projects/{id}/points.
// Returns chunks with metadata. Supports ?source_file filter, ?offset, ?limit.
func (h *Handlers) ListPoints(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if projectID == "" {
		writeErr(w, http.StatusBadRequest, "project id is required")
		return
	}

	metaFilter := map[string]string{}
	if sf := r.URL.Query().Get("source_file"); sf != "" {
		metaFilter["source_file"] = sf
	}

	points, err := h.svc.ListPoints(r.Context(), projectID, metaFilter)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Apply offset/limit pagination if provided.
	offset := 0
	limit := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if o, err := strconv.Atoi(v); err == nil && o >= 0 {
			offset = o
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l > 0 {
			limit = l
		}
	}

	if offset > 0 && offset < len(points) {
		points = points[offset:]
	} else if offset >= len(points) {
		points = nil
	}
	if limit > 0 && limit < len(points) {
		points = points[:limit]
	}

	if points == nil {
		points = []rag.PointInfo{}
	}
	writeJSON(w, http.StatusOK, points)
}

// DeletePoint handles DELETE /api/projects/{id}/points/{pointId}.
// Deletes a single chunk from the project collection.
func (h *Handlers) DeletePoint(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if projectID == "" {
		writeErr(w, http.StatusBadRequest, "project id is required")
		return
	}
	pointID := chi.URLParam(r, "pointId")
	if pointID == "" {
		writeErr(w, http.StatusBadRequest, "point id is required")
		return
	}

	if err := h.svc.DeletePoints(r.Context(), projectID, []string{pointID}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "project_id": projectID, "point_id": pointID})
}

// ReindexProject handles POST /api/projects/{id}/reindex.
// Triggers re-index and returns sync statistics.
// Expects a JSON body with "directory" field.
func (h *Handlers) ReindexProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if projectID == "" {
		writeErr(w, http.StatusBadRequest, "project id is required")
		return
	}

	var req struct {
		Directory string `json:"directory"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Directory == "" {
		writeErr(w, http.StatusBadRequest, "directory is required")
		return
	}

	result, err := h.svc.IndexProject(r.Context(), projectID, req.Directory)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "synced",
		"project_id":     projectID,
		"chunks_indexed": result.Indexed,
		"points_deleted": result.Deleted,
		"files_scanned":  result.FilesScanned,
		"skipped":        result.Skipped,
		"stale_error":    result.StaleError,
	})
}

// DeleteProject handles DELETE /api/projects/{id}.
// Deletes the project collection.
func (h *Handlers) DeleteProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if projectID == "" {
		writeErr(w, http.StatusBadRequest, "project id is required")
		return
	}

	if err := h.svc.DeleteProject(r.Context(), projectID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "project_id": projectID})
}

// Search handles POST /api/search.
// Accepts project_id, query, k, recall, hybrid, rerank.
// Returns 400 for missing fields, 404 for bad project.
func (h *Handlers) Search(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID string `json:"project_id"`
		Query     string `json:"query"`
		K         int    `json:"k"`
		Recall    int    `json:"recall"`
		Hybrid    bool   `json:"hybrid"`
		Rerank    bool   `json:"rerank"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.ProjectID == "" || req.Query == "" {
		writeErr(w, http.StatusBadRequest, "project_id and query are required")
		return
	}

	// Check that the project exists before executing the search. A
	// non-existent project in pgvector silently returns 0 rows (no error),
	// which would produce a misleading HTTP 200 with empty results. We
	// verify existence via ListProjectIDs or ListPoints so we can return a
	// proper 404.
	if !h.svc.ProjectExists(r.Context(), req.ProjectID) {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}

	results, err := h.svc.Search(r.Context(), req.ProjectID, req.Query, core.SearchOpts{
		K:      req.K,
		Recall: req.Recall,
		Hybrid: req.Hybrid,
		Rerank: req.Rerank,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	if results == nil {
		results = []rag.Result{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// Stats handles GET /api/stats.
// Returns aggregate statistics across all projects.
func (h *Handlers) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.svc.ListProjects(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	totalProjects := len(stats)
	totalChunks := 0
	for _, s := range stats {
		totalChunks += s.ChunkCount
	}

	// Get embedding model info from the service.
	embedModel := h.svc.EmbedModel()

	writeJSON(w, http.StatusOK, map[string]any{
		"total_projects": totalProjects,
		"total_chunks":   totalChunks,
		"embed_model":    embedModel,
		"projects":       stats,
	})
}
