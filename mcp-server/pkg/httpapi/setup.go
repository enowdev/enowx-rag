package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/enowdev/enowx-rag/pkg/config"
)

// --- Request / response types for setup wizard endpoints ---

// setupConfigRequest is the JSON body sent by the wizard for both /test and
// /apply. It mirrors config.Config but uses flat JSON field names that the
// React wizard sends.
type setupConfigRequest struct {
	VectorStore  string `json:"vector_store"`
	Embedder     string `json:"embedder"`
	VoyageAPIKey string `json:"voyage_api_key"`
	VoyageModel  string `json:"voyage_model"`
	VoyageDim    int    `json:"voyage_dim"`
	PGVectorDSN  string `json:"pgvector_dsn"`
	QdrantURL    string `json:"qdrant_url"`
	QdrantAPIKey string `json:"qdrant_api_key"`
	ChromaURL    string `json:"chroma_url"`
	TEIURL       string `json:"tei_url"`
}

// toConfig converts the flat wizard request into a config.Config struct.
func (r *setupConfigRequest) toConfig() *config.Config {
	dim := r.VoyageDim
	if dim == 0 {
		dim = 1024
	}
	model := r.VoyageModel
	if model == "" {
		model = "voyage-4"
	}
	return &config.Config{
		VectorStore: r.VectorStore,
		Embedder:    r.Embedder,
		Voyage: config.VoyageConfig{
			APIKey: r.VoyageAPIKey,
			Model:  model,
			Dim:    dim,
		},
		PGVectorDSN:   r.PGVectorDSN,
		QdrantURL:     r.QdrantURL,
		QdrantAPIKey:  r.QdrantAPIKey,
		ChromaURL:     r.ChromaURL,
		TEIURL:        r.TEIURL,
		RerankerModel: "rerank-2.5",
	}
}

// componentResult is the per-component health check result returned by
// POST /api/setup/test.
type componentResult struct {
	OK        bool   `json:"ok"`
	Message   string `json:"message"`
	LatencyMs int64  `json:"latency_ms"`
}

// setupTestResponse is the JSON body returned by POST /api/setup/test.
type setupTestResponse struct {
	VectorStore componentResult `json:"vector_store"`
	Embedder    componentResult `json:"embedder"`
}

// setupStatusResponse is the JSON body returned by GET /api/setup/status.
type setupStatusResponse struct {
	Configured bool `json:"configured"`
}

// --- Handlers ---

// SetupTest handles POST /api/setup/test.
// It tests connectivity to the configured vector store and embedder,
// returning per-component ok/message/latency_ms.
func (h *Handlers) SetupTest(w http.ResponseWriter, r *http.Request) {
	var req setupConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.VectorStore == "" {
		writeErr(w, http.StatusBadRequest, "vector_store is required")
		return
	}
	if req.Embedder == "" {
		writeErr(w, http.StatusBadRequest, "embedder is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	vsResult := testVectorStore(ctx, &req)
	embResult := testEmbedder(ctx, &req)

	writeJSON(w, http.StatusOK, setupTestResponse{
		VectorStore: vsResult,
		Embedder:    embResult,
	})
}

// SetupApply handles POST /api/setup/apply.
// It saves the submitted config to ~/.enowx-rag/config.yaml with chmod 0600.
func (h *Handlers) SetupApply(w http.ResponseWriter, r *http.Request) {
	var req setupConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.VectorStore == "" {
		writeErr(w, http.StatusBadRequest, "vector_store is required")
		return
	}
	if req.Embedder == "" {
		writeErr(w, http.StatusBadRequest, "embedder is required")
		return
	}

	cfg := req.toConfig()
	if err := config.Save(cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to save config: %v", err))
		return
	}

	// Do not echo the config back — it contains secrets (e.g. Voyage API key).
	// Return only the non-sensitive outcome.
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "saved",
		"path":   config.Path(),
	})
}

// SetupStatus handles GET /api/setup/status.
// It returns whether a config file exists at ~/.enowx-rag/config.yaml.
func (h *Handlers) SetupStatus(w http.ResponseWriter, r *http.Request) {
	_, err := os.Stat(config.Path())
	configured := err == nil
	writeJSON(w, http.StatusOK, setupStatusResponse{Configured: configured})
}

// --- Connectivity test helpers ---

// testVectorStore checks connectivity to the configured vector store.
// It returns a componentResult with ok, message, and latency_ms.
func testVectorStore(ctx context.Context, req *setupConfigRequest) componentResult {
	start := time.Now()

	switch strings.ToLower(req.VectorStore) {
	case "pgvector":
		// For pgvector we do a lightweight TCP dial to the Postgres host.
		// We don't import pgx here to keep the setup handler free of
		// database driver dependencies; a successful TCP connection is
		// sufficient for a wizard connectivity check.
		dsn := req.PGVectorDSN
		if dsn == "" {
			return componentResult{OK: false, Message: "pgvector_dsn is required", LatencyMs: 0}
		}
		host, port := parsePGDSN(dsn)
		if host == "" {
			return componentResult{OK: false, Message: "could not parse host from pgvector_dsn", LatencyMs: 0}
		}
		if err := tcpDial(ctx, host, port); err != nil {
			latency := time.Since(start).Milliseconds()
			return componentResult{
				OK:        false,
				Message:   fmt.Sprintf("cannot connect to PostgreSQL at %s:%s — %s", host, port, err),
				LatencyMs: latency,
			}
		}
		return componentResult{
			OK:        true,
			Message:   fmt.Sprintf("connected to PostgreSQL at %s:%s", host, port),
			LatencyMs: time.Since(start).Milliseconds(),
		}

	case "qdrant":
		url := req.QdrantURL
		if url == "" {
			return componentResult{OK: false, Message: "qdrant_url is required", LatencyMs: 0}
		}
		ok, msg := httpHealthCheck(ctx, strings.TrimRight(url, "/")+"/healthz")
		return componentResult{OK: ok, Message: msg, LatencyMs: time.Since(start).Milliseconds()}

	case "chroma":
		url := req.ChromaURL
		if url == "" {
			return componentResult{OK: false, Message: "chroma_url is required", LatencyMs: 0}
		}
		ok, msg := httpHealthCheck(ctx, strings.TrimRight(url, "/")+"/api/v1/heartbeat")
		return componentResult{OK: ok, Message: msg, LatencyMs: time.Since(start).Milliseconds()}

	default:
		return componentResult{OK: false, Message: fmt.Sprintf("unsupported vector store: %s", req.VectorStore), LatencyMs: 0}
	}
}

// testEmbedder checks connectivity to the configured embedder by embedding
// a single test word. It returns a componentResult with ok, message, and
// latency_ms.
func testEmbedder(ctx context.Context, req *setupConfigRequest) componentResult {
	start := time.Now()

	switch strings.ToLower(req.Embedder) {
	case "voyage":
		apiKey := req.VoyageAPIKey
		if apiKey == "" {
			return componentResult{OK: false, Message: "voyage_api_key is required", LatencyMs: 0}
		}
		model := req.VoyageModel
		if model == "" {
			model = "voyage-4"
		}
		ok, msg := testVoyageEmbed(ctx, apiKey, model)
		return componentResult{OK: ok, Message: msg, LatencyMs: time.Since(start).Milliseconds()}

	case "tei":
		teiURL := req.TEIURL
		if teiURL == "" {
			return componentResult{OK: false, Message: "tei_url is required", LatencyMs: 0}
		}
		ok, msg := testTEIEmbed(ctx, teiURL)
		return componentResult{OK: ok, Message: msg, LatencyMs: time.Since(start).Milliseconds()}

	default:
		return componentResult{OK: false, Message: fmt.Sprintf("unsupported embedder: %s", req.Embedder), LatencyMs: 0}
	}
}

// httpHealthCheck sends a GET request to the given URL and returns
// (true, "connected to <url>") on HTTP 200, or (false, "<error>") on failure.
func httpHealthCheck(ctx context.Context, url string) (bool, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Sprintf("invalid URL %s: %s", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Sprintf("cannot connect to %s — %s", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Sprintf("%s returned HTTP %d", url, resp.StatusCode)
	}
	return true, fmt.Sprintf("connected to %s", url)
}

// testVoyageEmbed sends a minimal embedding request to the Voyage AI API
// to verify that the API key is valid and the service is reachable.
func testVoyageEmbed(ctx context.Context, apiKey, model string) (bool, string) {
	url := "https://api.voyageai.com/v1/embeddings"
	body := fmt.Sprintf(`{"input":["test"],"model":"%s"}`, model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return false, fmt.Sprintf("failed to create request: %s", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Sprintf("cannot connect to Voyage AI API — %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return false, "Voyage AI API key is invalid or expired"
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Sprintf("Voyage AI API returned HTTP %d", resp.StatusCode)
	}
	return true, fmt.Sprintf("Voyage AI API connected (model: %s)", model)
}

// testTEIEmbed sends a minimal embedding request to the TEI server to
// verify that the service is reachable and responding.
func testTEIEmbed(ctx context.Context, teiURL string) (bool, string) {
	url := strings.TrimRight(teiURL, "/") + "/embed"
	body := `{"inputs":["test"],"options":{"wait_for_model":true}}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return false, fmt.Sprintf("failed to create request: %s", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Sprintf("cannot connect to TEI at %s — %s", teiURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Sprintf("TEI at %s returned HTTP %d", teiURL, resp.StatusCode)
	}
	return true, fmt.Sprintf("TEI connected at %s", teiURL)
}

// parsePGDSN extracts the host and port from a PostgreSQL connection string.
// Supports both URL format (postgresql://host:port/db) and keyword format
// (host=... port=...). Returns ("localhost", "5432") as default if parsing
// fails.
func parsePGDSN(dsn string) (host, port string) {
	host = "localhost"
	port = "5432"

	// URL format: postgresql://user@host:port/db
	if strings.HasPrefix(dsn, "postgresql://") || strings.HasPrefix(dsn, "postgres://") {
		// Strip scheme
		rest := dsn
		if idx := strings.Index(dsn, "://"); idx >= 0 {
			rest = dsn[idx+3:]
		}
		// Strip credentials (user:pass@)
		if atIdx := strings.LastIndex(rest, "@"); atIdx >= 0 {
			rest = rest[atIdx+1:]
		}
		// Strip path
		if slashIdx := strings.Index(rest, "/"); slashIdx >= 0 {
			rest = rest[:slashIdx]
		}
		// Now rest is host:port or just host
		if colonIdx := strings.LastIndex(rest, ":"); colonIdx >= 0 {
			host = rest[:colonIdx]
			port = rest[colonIdx+1:]
		} else {
			host = rest
		}
		if host == "" {
			host = "localhost"
		}
		return host, port
	}

	// Keyword format: host=... port=... dbname=...
	// Split by spaces and look for host= and port= keys.
	for _, part := range strings.Fields(dsn) {
		if strings.HasPrefix(part, "host=") {
			h := strings.TrimPrefix(part, "host=")
			if h != "" {
				host = h
			}
		}
		if strings.HasPrefix(part, "port=") {
			p := strings.TrimPrefix(part, "port=")
			if p != "" {
				port = p
			}
		}
	}

	return host, port
}

// tcpDial attempts a TCP connection to the given host:port with the context's
// deadline. It returns nil on success, or an error describing the failure.
func tcpDial(ctx context.Context, host, port string) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}
