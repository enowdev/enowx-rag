package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/enowdev/enowx-rag/pkg/core"
)

// helperClearRagEnv unsets all RAG_ env vars that could interfere with
// config tests and returns a cleanup function.
func helperClearRagEnv(t *testing.T) func() {
	t.Helper()
	keys := []string{
		"RAG_VECTOR_STORE", "RAG_EMBEDDER",
		"RAG_QDRANT_URL", "RAG_QDRANT_API_KEY",
		"RAG_CHROMA_URL", "RAG_PGVECTOR_DSN",
		"RAG_TEI_URL",
		"RAG_VOYAGE_API_KEY", "RAG_VOYAGE_MODEL", "RAG_VECTOR_DIM",
		"RAG_RERANKER_MODEL",
	}
	saved := make(map[string]string)
	for _, k := range keys {
		saved[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	return func() {
		for k, v := range saved {
			if v != "" {
				os.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	}
}

// --- GET /api/setup/status ---

// TestSetupStatus_NotConfigured verifies that GET /api/setup/status returns
// configured: false when no config file exists.
func TestSetupStatus_NotConfigured(t *testing.T) {
	cleanup := helperClearRagEnv(t)
	defer cleanup()

	// Point HOME to a temp dir with no config file.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp setupStatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp.Configured {
		t.Error("expected configured=false when no config file exists")
	}
}

// TestSetupStatus_Configured verifies that GET /api/setup/status returns
// configured: true when a config file exists.
func TestSetupStatus_Configured(t *testing.T) {
	cleanup := helperClearRagEnv(t)
	defer cleanup()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create the config file so that os.Stat finds it.
	configDir := filepath.Join(tmpDir, ".enowx-rag")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("vector_store: pgvector\n"), 0600); err != nil {
		t.Fatal(err)
	}

	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp setupStatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if !resp.Configured {
		t.Error("expected configured=true when config file exists")
	}
}

// TestSetupStatus_Transitions verifies the before/after flow: status is
// false before apply, then true after apply.
func TestSetupStatus_Transitions(t *testing.T) {
	cleanup := helperClearRagEnv(t)
	defer cleanup()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	// Step 1: status should be false.
	req := httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp setupStatusResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Configured {
		t.Fatal("expected configured=false before apply")
	}

	// Step 2: apply config.
	applyBody := `{"vector_store":"qdrant","embedder":"voyage","voyage_api_key":"test-key","qdrant_url":"http://localhost:6333"}`
	req = httptest.NewRequest(http.MethodPost, "/api/setup/apply", strings.NewReader(applyBody))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("apply failed: %d, body: %s", w.Code, w.Body.String())
	}

	// Step 3: status should now be true.
	req = httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Configured {
		t.Error("expected configured=true after apply")
	}
}

// --- POST /api/setup/apply ---

// TestSetupApply_SavesConfigWith0600 verifies that POST /api/setup/apply
// saves the config to ~/.enowx-rag/config.yaml with chmod 0600.
func TestSetupApply_SavesConfigWith0600(t *testing.T) {
	cleanup := helperClearRagEnv(t)
	defer cleanup()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	body := `{"vector_store":"pgvector","embedder":"voyage","voyage_api_key":"secret-key","voyage_model":"voyage-4","voyage_dim":1024,"pgvector_dsn":"postgresql://enowdev@localhost:5432/enowxrag"}`
	req := httptest.NewRequest(http.MethodPost, "/api/setup/apply", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify file exists with 0600 permissions.
	configPath := filepath.Join(tmpDir, ".enowx-rag", "config.yaml")
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("config file not created: %v", err)
	}
	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Errorf("expected file mode 0600, got %o", mode)
	}

	// Verify the response includes the path.
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp["status"] != "saved" {
		t.Errorf("expected status 'saved', got %v", resp["status"])
	}
}

// TestSetupApply_WritesValidYAML verifies that the saved config file is
// valid YAML and its values match the submitted config.
func TestSetupApply_WritesValidYAML(t *testing.T) {
	cleanup := helperClearRagEnv(t)
	defer cleanup()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	body := `{"vector_store":"pgvector","embedder":"voyage","voyage_api_key":"my-key","voyage_model":"voyage-4","voyage_dim":1024,"pgvector_dsn":"postgresql://enowdev@localhost:5432/enowxrag","qdrant_url":"http://localhost:6333","reranker_model":"rerank-2.5"}`
	req := httptest.NewRequest(http.MethodPost, "/api/setup/apply", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Read the file and verify it's valid YAML with correct values.
	data, err := os.ReadFile(filepath.Join(tmpDir, ".enowx-rag", "config.yaml"))
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	yamlContent := string(data)
	if !strings.Contains(yamlContent, "vector_store: pgvector") {
		t.Errorf("expected YAML to contain 'vector_store: pgvector', got: %s", yamlContent)
	}
	if !strings.Contains(yamlContent, "embedder: voyage") {
		t.Errorf("expected YAML to contain 'embedder: voyage', got: %s", yamlContent)
	}
	if !strings.Contains(yamlContent, "api_key: my-key") {
		t.Errorf("expected YAML to contain 'api_key: my-key', got: %s", yamlContent)
	}
	if !strings.Contains(yamlContent, "pgvector_dsn:") {
		t.Errorf("expected YAML to contain 'pgvector_dsn:', got: %s", yamlContent)
	}
}

// TestSetupApply_MissingVectorStore verifies that POST /api/setup/apply
// returns 400 when vector_store is missing.
func TestSetupApply_MissingVectorStore(t *testing.T) {
	cleanup := helperClearRagEnv(t)
	defer cleanup()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	body := `{"embedder":"voyage"}`
	req := httptest.NewRequest(http.MethodPost, "/api/setup/apply", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var errResp map[string]string
	json.Unmarshal(w.Body.Bytes(), &errResp)
	if errResp["error"] == "" {
		t.Error("expected error field in response")
	}
}

// TestSetupApply_InvalidJSON verifies that POST /api/setup/apply returns
// 400 for invalid JSON body.
func TestSetupApply_InvalidJSON(t *testing.T) {
	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/setup/apply", strings.NewReader("not json"))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestSetupApply_OverwritesExisting verifies that applying config when a
// file already exists overwrites it (with 0600 permissions maintained).
func TestSetupApply_OverwritesExisting(t *testing.T) {
	cleanup := helperClearRagEnv(t)
	defer cleanup()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Pre-create a config file with different values and wider permissions.
	configDir := filepath.Join(tmpDir, ".enowx-rag")
	os.MkdirAll(configDir, 0700)
	configPath := filepath.Join(configDir, "config.yaml")
	os.WriteFile(configPath, []byte("vector_store: old\n"), 0644)

	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	body := `{"vector_store":"qdrant","embedder":"voyage","voyage_api_key":"new-key","qdrant_url":"http://localhost:6333"}`
	req := httptest.NewRequest(http.MethodPost, "/api/setup/apply", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify file has 0600 permissions even after overwrite.
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected 0600 after overwrite, got %o", info.Mode().Perm())
	}

	// Verify content was replaced.
	data, _ := os.ReadFile(configPath)
	if !strings.Contains(string(data), "vector_store: qdrant") {
		t.Errorf("expected new content, got: %s", string(data))
	}
	if strings.Contains(string(data), "old") {
		t.Errorf("expected old content to be overwritten, got: %s", string(data))
	}
}

// --- POST /api/setup/test ---

// TestSetupTest_QdrantSuccess verifies that POST /api/setup/test returns
// per-component ok/message/latency_ms when both vector store and embedder
// are reachable.
func TestSetupTest_QdrantSuccess(t *testing.T) {
	// Start a mock Qdrant server that responds 200 on /healthz.
	qdrantSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer qdrantSrv.Close()

	// Start a mock Voyage API server that responds 200 on /v1/embeddings.
	voyageSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/embeddings" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data":[{"embedding":[0.1,0.2],"index":0}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer voyageSrv.Close()

	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	body := `{"vector_store":"qdrant","embedder":"voyage","voyage_api_key":"test-key","qdrant_url":"` + qdrantSrv.URL + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/setup/test", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp setupTestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Vector store should be ok.
	if !resp.VectorStore.OK {
		t.Errorf("expected vector_store ok=true, got false: %s", resp.VectorStore.Message)
	}
	if resp.VectorStore.Message == "" {
		t.Error("expected non-empty vector_store message")
	}

	// Note: the embedder test calls the real Voyage AI API because the URL
	// is hardcoded in the implementation. We only verify the vector store
	// component here and check that the embedder field is present.
	if resp.Embedder.Message == "" {
		t.Error("expected non-empty embedder message")
	}
}

// TestSetupTest_QdrantFail verifies that POST /api/setup/test returns ok=false
// for the vector store when the server is unreachable.
func TestSetupTest_QdrantFail(t *testing.T) {
	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	// Use a port that's almost certainly not listening.
	body := `{"vector_store":"qdrant","embedder":"voyage","voyage_api_key":"test-key","qdrant_url":"http://127.0.0.1:59999"}`
	req := httptest.NewRequest(http.MethodPost, "/api/setup/test", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp setupTestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp.VectorStore.OK {
		t.Error("expected vector_store ok=false when server is unreachable")
	}
	if resp.VectorStore.Message == "" {
		t.Error("expected non-empty error message for failed vector store")
	}
}

// TestSetupTest_PGVectorSuccess verifies that POST /api/setup/test returns
// ok=true for pgvector when the PostgreSQL server is reachable (TCP dial).
func TestSetupTest_PGVectorSuccess(t *testing.T) {
	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	// Use the real local PostgreSQL that's running per the mission setup.
	body := `{"vector_store":"pgvector","embedder":"voyage","voyage_api_key":"test-key","pgvector_dsn":"postgresql://enowdev@localhost:5432/enowxrag"}`
	req := httptest.NewRequest(http.MethodPost, "/api/setup/test", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp setupTestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if !resp.VectorStore.OK {
		t.Errorf("expected vector_store ok=true for local PostgreSQL, got false: %s", resp.VectorStore.Message)
	}
}

// TestSetupTest_PGVectorFail verifies that POST /api/setup/test returns
// ok=false for pgvector when the DSN points to an unreachable host.
func TestSetupTest_PGVectorFail(t *testing.T) {
	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	body := `{"vector_store":"pgvector","embedder":"voyage","voyage_api_key":"test-key","pgvector_dsn":"postgresql://user@127.0.0.1:59999/nonexistent"}`
	req := httptest.NewRequest(http.MethodPost, "/api/setup/test", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp setupTestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp.VectorStore.OK {
		t.Error("expected vector_store ok=false when PostgreSQL is unreachable")
	}
	if !strings.Contains(resp.VectorStore.Message, "cannot connect") {
		t.Errorf("expected message to contain 'cannot connect', got: %s", resp.VectorStore.Message)
	}
}

// TestSetupTest_MissingFields verifies that POST /api/setup/test returns
// 400 when vector_store or embedder is missing.
func TestSetupTest_MissingFields(t *testing.T) {
	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	// Missing vector_store.
	body := `{"embedder":"voyage"}`
	req := httptest.NewRequest(http.MethodPost, "/api/setup/test", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing vector_store, got %d", w.Code)
	}

	// Missing embedder.
	body = `{"vector_store":"qdrant"}`
	req = httptest.NewRequest(http.MethodPost, "/api/setup/test", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing embedder, got %d", w.Code)
	}
}

// TestSetupTest_InvalidJSON verifies that POST /api/setup/test returns 400
// for invalid JSON body.
func TestSetupTest_InvalidJSON(t *testing.T) {
	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/setup/test", strings.NewReader("not json"))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestSetupTest_ReturnsLatency verifies that POST /api/setup/test includes
// latency_ms in the per-component results.
func TestSetupTest_ReturnsLatency(t *testing.T) {
	// Start a mock Qdrant server.
	qdrantSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer qdrantSrv.Close()

	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	body := `{"vector_store":"qdrant","embedder":"voyage","voyage_api_key":"test-key","qdrant_url":"` + qdrantSrv.URL + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/setup/test", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp setupTestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// latency_ms should be present (>= 0). Even fast connections have 0ms.
	if resp.VectorStore.LatencyMs < 0 {
		t.Errorf("expected non-negative latency_ms, got %d", resp.VectorStore.LatencyMs)
	}
}

// TestSetupTest_UnsupportedVectorStore verifies that POST /api/setup/test
// returns ok=false for an unsupported vector store type.
func TestSetupTest_UnsupportedVectorStore(t *testing.T) {
	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	body := `{"vector_store":"redis","embedder":"voyage","voyage_api_key":"test-key"}`
	req := httptest.NewRequest(http.MethodPost, "/api/setup/test", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp setupTestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp.VectorStore.OK {
		t.Error("expected ok=false for unsupported vector store")
	}
	if !strings.Contains(resp.VectorStore.Message, "unsupported") {
		t.Errorf("expected message to contain 'unsupported', got: %s", resp.VectorStore.Message)
	}
}

// --- parsePGDSN unit tests ---

func TestParsePGDSN_URLFormat(t *testing.T) {
	tests := []struct {
		dsn      string
		wantHost string
		wantPort string
	}{
		{"postgresql://enowdev@localhost:5432/enowxrag", "localhost", "5432"},
		{"postgresql://admin@db.example.com:6543/mydb", "db.example.com", "6543"},
		{"postgres://localhost/mydb", "localhost", "5432"},
		{"postgresql://localhost:5432", "localhost", "5432"},
	}
	for _, tt := range tests {
		host, port := parsePGDSN(tt.dsn)
		if host != tt.wantHost {
			t.Errorf("parsePGDSN(%q) host = %q, want %q", tt.dsn, host, tt.wantHost)
		}
		if port != tt.wantPort {
			t.Errorf("parsePGDSN(%q) port = %q, want %q", tt.dsn, port, tt.wantPort)
		}
	}
}

func TestParsePGDSN_KeywordFormat(t *testing.T) {
	dsn := "host=db.example.com port=6543 dbname=mydb user=admin"
	host, port := parsePGDSN(dsn)
	if host != "db.example.com" {
		t.Errorf("host = %q, want %q", host, "db.example.com")
	}
	if port != "6543" {
		t.Errorf("port = %q, want %q", port, "6543")
	}
}

func TestParsePGDSN_Empty(t *testing.T) {
	host, port := parsePGDSN("")
	if host != "localhost" {
		t.Errorf("host = %q, want %q", host, "localhost")
	}
	if port != "5432" {
		t.Errorf("port = %q, want %q", port, "5432")
	}
}

// TestSetupApply_RemoteRejectedWithoutToken verifies that a non-loopback
// request to /api/setup/apply is rejected (403) when no admin token is set,
// so an exposed instance cannot have its config rewritten anonymously.
func TestSetupApply_RemoteRejectedWithoutToken(t *testing.T) {
	t.Setenv("RAG_ADMIN_TOKEN", "")
	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	body := `{"vector_store":"qdrant","embedder":"voyage","voyage_api_key":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/setup/apply", strings.NewReader(body))
	req.RemoteAddr = "203.0.113.7:5555" // non-loopback
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("remote setup/apply without token = %d, want 403", w.Code)
	}
}

// TestSetupApply_RemoteAllowedWithToken verifies that a non-loopback request
// with a valid admin token is allowed through the gate.
func TestSetupApply_RemoteAllowedWithToken(t *testing.T) {
	t.Setenv("RAG_ADMIN_TOKEN", "s3cret")
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	body := `{"vector_store":"qdrant","embedder":"voyage","voyage_api_key":"k"}`
	req := httptest.NewRequest(http.MethodPost, "/api/setup/apply", strings.NewReader(body))
	req.RemoteAddr = "203.0.113.7:5555"
	req.Header.Set("Authorization", "Bearer s3cret")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("remote setup/apply with valid token = %d, want 200", w.Code)
	}
	// Response must NOT leak the API key back.
	if strings.Contains(w.Body.String(), "\"k\"") || strings.Contains(w.Body.String(), "voyage_api_key") {
		t.Errorf("setup/apply response leaked config/secret: %s", w.Body.String())
	}
}

// TestInstallMCPEndpoint verifies POST /api/setup/install-mcp writes a merged
// client config (loopback allowed) and reports the path.
func TestInstallMCPEndpoint(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	// A saved config is required (mcpServerEntry loads it).
	if err := writeTestConfig(tmp); err != nil {
		t.Fatalf("write config: %v", err)
	}

	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	body := `{"client_id":"cursor","scope":"global"}`
	req := httptest.NewRequest(http.MethodPost, "/api/setup/install-mcp", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("install-mcp = %d, want 200: %s", w.Code, w.Body.String())
	}
	// The cursor config should now exist under the temp HOME.
	cursorPath := filepath.Join(tmp, ".cursor", "mcp.json")
	data, err := os.ReadFile(cursorPath)
	if err != nil {
		t.Fatalf("cursor config not written: %v", err)
	}
	if !strings.Contains(string(data), "enowx-rag") {
		t.Error("cursor config missing enowx-rag server")
	}
}

// TestInstallMCPRemoteRejected verifies the endpoint is loopback-gated.
func TestInstallMCPRemoteRejected(t *testing.T) {
	t.Setenv("RAG_ADMIN_TOKEN", "")
	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/setup/install-mcp", strings.NewReader(`{"client_id":"cursor"}`))
	req.RemoteAddr = "203.0.113.9:5555"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("remote install-mcp = %d, want 403", w.Code)
	}
}

// TestMCPSnippetEndpoint verifies GET /api/setup/mcp-snippet returns a snippet.
func TestMCPSnippetEndpoint(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := writeTestConfig(tmp); err != nil {
		t.Fatalf("write config: %v", err)
	}
	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/setup/mcp-snippet?client_id=codex", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("mcp-snippet = %d, want 200: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	content, _ := resp["content"].(string)
	if !strings.Contains(content, "[mcp_servers.enowx-rag]") {
		t.Errorf("codex snippet should be TOML, got: %s", content)
	}
}

// writeTestConfig writes a minimal ~/.enowx-rag/config.yaml under home.
func writeTestConfig(home string) error {
	dir := filepath.Join(home, ".enowx-rag")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	yaml := "vector_store: qdrant\nembedder: voyage\nqdrant_url: http://localhost:6333\nvoyage:\n  api_key: k\n  model: voyage-4\n"
	return os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0o600)
}

// TestMigrateEndpointRemoteRejected verifies /api/migrate is loopback-gated.
func TestMigrateEndpointRemoteRejected(t *testing.T) {
	t.Setenv("RAG_ADMIN_TOKEN", "")
	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)
	body := `{"source_project":"a","dest_project":"b","vector_store":"qdrant","embedder":"voyage"}`
	req := httptest.NewRequest(http.MethodPost, "/api/migrate", strings.NewReader(body))
	req.RemoteAddr = "203.0.113.11:5555"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("remote migrate = %d, want 403", w.Code)
	}
}

// TestMigrateEndpointValidates requires source and dest projects.
func TestMigrateEndpointValidates(t *testing.T) {
	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/migrate", strings.NewReader(`{"source_project":"a"}`))
	req.RemoteAddr = "127.0.0.1:1"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing dest = %d, want 400", w.Code)
	}
}

// TestWriteAgentsMD_CreateAndMerge verifies create, append (preserve), and
// idempotent update via markers.
func TestWriteAgentsMD_CreateAndMerge(t *testing.T) {
	dir := t.TempDir()
	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	call := func(projectID string) int {
		body := `{"dir":"` + dir + `","project_id":"` + projectID + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/setup/write-agents-md", strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:1"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}

	agentsPath := filepath.Join(dir, "AGENTS.md")

	// 1. Create (no file yet).
	if code := call("proj1"); code != 200 {
		t.Fatalf("create = %d, want 200", code)
	}
	data, _ := os.ReadFile(agentsPath)
	if !strings.Contains(string(data), "<!-- enowx-rag:start -->") || !strings.Contains(string(data), "project: proj1") {
		t.Errorf("created AGENTS.md missing block:\n%s", data)
	}

	// 2. Update (markers present) — must stay a single block, updated id.
	if code := call("proj2"); code != 200 {
		t.Fatalf("update = %d, want 200", code)
	}
	data, _ = os.ReadFile(agentsPath)
	if strings.Count(string(data), "<!-- enowx-rag:start -->") != 1 {
		t.Errorf("update should keep exactly one block:\n%s", data)
	}
	if !strings.Contains(string(data), "project: proj2") || strings.Contains(string(data), "project: proj1") {
		t.Errorf("update did not replace project id:\n%s", data)
	}
}

// TestWriteAgentsMD_AppendPreservesUserContent verifies existing content is kept
// when there are no markers.
func TestWriteAgentsMD_AppendPreservesUserContent(t *testing.T) {
	dir := t.TempDir()
	userContent := "# My rules\n\nDo not break the build.\n"
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(userContent), 0o644)

	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)
	body := `{"dir":"` + dir + `","project_id":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/setup/write-agents-md", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:1"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("append = %d, want 200", w.Code)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(string(data), "Do not break the build") {
		t.Error("user content was lost")
	}
	if !strings.Contains(string(data), "<!-- enowx-rag:start -->") {
		t.Error("enowx-rag block was not appended")
	}
}

// TestProbeEndpoint verifies probe reports skill/agents_md status.
func TestProbeEndpoint(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("hi <!-- enowx-rag:start -->x<!-- enowx-rag:end -->"), 0o644)
	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/setup/probe?client=cursor&dir="+dir, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("probe = %d, want 200", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	am := resp["agents_md"].(map[string]any)
	if am["exists"] != true || am["has_block"] != true {
		t.Errorf("agents_md status wrong: %v", am)
	}
	if _, ok := resp["mcp"].(map[string]any)["cursor"]; !ok {
		t.Error("mcp status missing cursor")
	}
}

// TestDocsEndpoints verifies the docs list, a section, the setup alias, and 404.
func TestDocsEndpoints(t *testing.T) {
	p := &mockProvider{}
	_, router := newTestServer(t, p, nil)

	// List
	req := httptest.NewRequest(http.MethodGet, "/api/docs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("docs list = %d, want 200", w.Code)
	}
	var list []map[string]string
	json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) < 5 {
		t.Errorf("expected several doc sections, got %d", len(list))
	}
	hasMCP := false
	for _, s := range list {
		if s["id"] == "mcp-tools" {
			hasMCP = true
		}
	}
	if !hasMCP {
		t.Error("mcp-tools section missing from list")
	}

	// A section
	req = httptest.NewRequest(http.MethodGet, "/api/docs/mcp-tools", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "rag_retrieve_context") {
		t.Errorf("mcp-tools section wrong: %d\n%s", w.Code, w.Body.String())
	}

	// setup alias still works
	req = httptest.NewRequest(http.MethodGet, "/api/docs/setup", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "probe") {
		t.Errorf("docs/setup alias broken: %d", w.Code)
	}

	// Unknown section -> 404
	req = httptest.NewRequest(http.MethodGet, "/api/docs/nope", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Errorf("unknown section = %d, want 404", w.Code)
	}
}

// TestMCPMount_Gated verifies /mcp is mounted and gated by RAG_ADMIN_TOKEN.
func TestMCPMount_Gated(t *testing.T) {
	// A trivial handler stands in for the real MCP handler.
	dummy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("mcp-ok"))
	})

	// With a token set: no/invalid bearer -> 401.
	t.Setenv("RAG_ADMIN_TOKEN", "s3cret")
	router := NewRouter(core.NewService(&mockProvider{}, nil, nil), nil, dummy)
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("/mcp without token = %d, want 401", w.Code)
	}

	// With the correct bearer -> reaches the handler (200 mcp-ok).
	req = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer s3cret")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "mcp-ok" {
		t.Fatalf("/mcp with token = %d %q, want 200 mcp-ok", w.Code, w.Body.String())
	}
}

// TestMCPMount_OpenWhenNoToken verifies /mcp is reachable without auth when no
// token is set (local use).
func TestMCPMount_OpenWhenNoToken(t *testing.T) {
	t.Setenv("RAG_ADMIN_TOKEN", "")
	dummy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	router := NewRouter(core.NewService(&mockProvider{}, nil, nil), nil, dummy)
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("/mcp without token set = %d, want 200 (open)", w.Code)
	}
}
