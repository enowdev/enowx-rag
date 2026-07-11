// Package httpapi provides the HTTP API layer for enowx-rag. It exposes
// REST endpoints and an SSE event stream over a chi router, serving both
// the API and an embedded React SPA from a single binary.
package httpapi

import (
	"io/fs"
	"net/http"

	"github.com/enowdev/enowx-rag/pkg/core"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter creates a chi router with all API routes and SPA fallback.
// svc is the core service layer shared with MCP stdio mode.
// ui is the embedded filesystem containing the SPA dist (index.html + assets).
func NewRouter(svc *core.Service, ui fs.FS) http.Handler {
	h := &Handlers{svc: svc}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.Get("/projects", h.ListProjects)
		r.Get("/projects/{id}", h.GetProject)
		r.Get("/projects/{id}/points", h.ListPoints)
		r.Post("/projects/{id}/reindex", h.ReindexProject)
		r.Delete("/projects/{id}", h.DeleteProject)
		r.Post("/search", h.Search)
		r.Get("/stats", h.Stats)
		r.Get("/events", h.SSE)

		// Setup wizard endpoints
		r.Post("/setup/test", h.SetupTest)
		r.Post("/setup/apply", h.SetupApply)
		r.Get("/setup/status", h.SetupStatus)

		// Unknown /api/ routes return 404 JSON (not SPA fallback)
		r.NotFound(h.NotFound)
	})

	// SPA fallback: serve embedded dist files, fall back to index.html
	// for client-side routes (e.g., /playground, /chunks, /setup).
	if ui != nil {
		spa := &SPAHandler{root: ui}
		r.Handle("/*", spa)
	}

	return r
}

// SPAHandler serves static files from an embedded filesystem. For paths
// that don't match a real file, it falls back to index.html to enable
// client-side routing (SPA mode).
type SPAHandler struct {
	root fs.FS
}

func (s *SPAHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "" || path == "/" {
		path = "/index.html"
	}

	// Try to serve the file directly from the embedded FS.
	data, err := fs.ReadFile(s.root, path[1:]) // strip leading "/"
	if err == nil {
		w.Header().Set("Content-Type", contentTypeFor(path))
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
		return
	}

	// Fall back to index.html for client-side routes.
	indexData, err := fs.ReadFile(s.root, "index.html")
	if err != nil {
		http.Error(w, "index.html not found in embedded dist", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	w.Write(indexData)
}

// contentTypeFor returns the Content-Type header value for common asset types.
func contentTypeFor(path string) string {
	switch {
	case endsWith(path, ".html"):
		return "text/html; charset=utf-8"
	case endsWith(path, ".js"):
		return "application/javascript"
	case endsWith(path, ".mjs"):
		return "application/javascript"
	case endsWith(path, ".css"):
		return "text/css"
	case endsWith(path, ".json"):
		return "application/json"
	case endsWith(path, ".svg"):
		return "image/svg+xml"
	case endsWith(path, ".png"):
		return "image/png"
	case endsWith(path, ".jpg"), endsWith(path, ".jpeg"):
		return "image/jpeg"
	case endsWith(path, ".gif"):
		return "image/gif"
	case endsWith(path, ".ico"):
		return "image/x-icon"
	case endsWith(path, ".woff"):
		return "font/woff"
	case endsWith(path, ".woff2"):
		return "font/woff2"
	case endsWith(path, ".ttf"):
		return "font/ttf"
	case endsWith(path, ".map"):
		return "application/json"
	case endsWith(path, ".txt"):
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

func endsWith(s, suffix string) bool {
	if len(suffix) > len(s) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}
