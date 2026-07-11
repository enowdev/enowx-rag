package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// --- ModelNamer tests ---

// TestVoyageModelName verifies that VoyageEmbeddingClient implements ModelNamer.
func TestVoyageModelName(t *testing.T) {
	c := NewVoyageEmbeddingClient("key", "voyage-4", 1024)
	mn, ok := any(c).(ModelNamer)
	if !ok {
		t.Fatal("VoyageEmbeddingClient should implement ModelNamer")
	}
	if mn.ModelName() != "voyage-4" {
		t.Errorf("ModelName() = %q, want %q", mn.ModelName(), "voyage-4")
	}
}

// TestTEIDoesNotImplementModelNamer verifies that TEIEmbeddingClient does NOT
// implement ModelNamer (it has no ModelName method).
func TestTEIDoesNotImplementModelNamer(t *testing.T) {
	embedder := NewTEIEmbeddingClient("http://localhost:8081")
	_, ok := any(embedder).(ModelNamer)
	if ok {
		t.Fatal("TEIEmbeddingClient should NOT implement ModelNamer")
	}
}

// mockModelNamerEmbedder implements EmbeddingClient + ModelNamer.
type mockModelNamerEmbedder struct {
	mockPlainEmbedder
	modelName string
}

func (m *mockModelNamerEmbedder) ModelName() string {
	return m.modelName
}

// --- Qdrant embed_model/embed_dim injection tests ---

// TestQdrantIndexInjectsEmbedMetadata verifies that Qdrant's Index method
// injects embed_model and embed_dim into the point payload.
func TestQdrantIndexInjectsEmbedMetadata(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		body := make(map[string]any)
		_ = json.NewDecoder(r.Body).Decode(&body)
		capturedBody = body
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"operation_id": 0, "status": "completed"}})
	}))
	defer srv.Close()

	embedder := &mockModelNamerEmbedder{
		mockPlainEmbedder: mockPlainEmbedder{vectorSize: 3},
		modelName:         "voyage-4",
	}

	p, err := NewQdrantProvider(context.Background(), srv.URL, "", embedder)
	if err != nil {
		t.Fatalf("NewQdrantProvider: %v", err)
	}
	defer p.Close()

	docs := []Document{
		{ID: "doc1", Content: "hello", Meta: map[string]string{"source_file": "test.go"}},
	}
	if err := p.Index(context.Background(), "testproj", docs); err != nil {
		t.Fatalf("Index: %v", err)
	}

	points, ok := capturedBody["points"].([]any)
	if !ok || len(points) == 0 {
		t.Fatal("expected at least 1 point in request body")
	}
	firstPoint, ok := points[0].(map[string]any)
	if !ok {
		t.Fatal("expected point to be a map")
	}
	payload, ok := firstPoint["payload"].(map[string]any)
	if !ok {
		t.Fatal("expected payload in point")
	}

	if payload["embed_model"] != "voyage-4" {
		t.Errorf("embed_model = %v, want %q", payload["embed_model"], "voyage-4")
	}
	// embed_dim is stored as int but JSON round-trips as float64
	if embedDimVal, ok := payload["embed_dim"].(float64); !ok || embedDimVal != 3 {
		t.Errorf("embed_dim = %v, want 3", payload["embed_dim"])
	}
}

// TestQdrantIndexInjectsEmbedMetadataNoModelNamer verifies that when the embedder
// does NOT implement ModelNamer, embed_model defaults to "unknown".
func TestQdrantIndexInjectsEmbedMetadataNoModelNamer(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		body := make(map[string]any)
		_ = json.NewDecoder(r.Body).Decode(&body)
		capturedBody = body
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"operation_id": 0, "status": "completed"}})
	}))
	defer srv.Close()

	// mockPlainEmbedder does NOT implement ModelNamer
	embedder := &mockPlainEmbedder{vectorSize: 3}

	p, err := NewQdrantProvider(context.Background(), srv.URL, "", embedder)
	if err != nil {
		t.Fatalf("NewQdrantProvider: %v", err)
	}
	defer p.Close()

	docs := []Document{
		{ID: "doc1", Content: "hello", Meta: map[string]string{}},
	}
	if err := p.Index(context.Background(), "testproj", docs); err != nil {
		t.Fatalf("Index: %v", err)
	}

	points, _ := capturedBody["points"].([]any)
	firstPoint, _ := points[0].(map[string]any)
	payload, _ := firstPoint["payload"].(map[string]any)

	if payload["embed_model"] != "unknown" {
		t.Errorf("embed_model = %v, want %q", payload["embed_model"], "unknown")
	}
}

// --- Chroma embed_model/embed_dim injection tests ---

// TestChromaIndexInjectsEmbedMetadata verifies that Chroma's Index method
// injects embed_model and embed_dim into the document metadata.
func TestChromaIndexInjectsEmbedMetadata(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make(map[string]any)
		_ = json.NewDecoder(r.Body).Decode(&body)
		capturedBody = body
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	embedder := &mockModelNamerEmbedder{
		mockPlainEmbedder: mockPlainEmbedder{vectorSize: 3},
		modelName:         "voyage-4",
	}

	p := NewChromaProvider(srv.URL, embedder)

	docs := []Document{
		{ID: "doc1", Content: "hello", Meta: map[string]string{"source_file": "test.go"}},
	}
	if err := p.Index(context.Background(), "testproj", docs); err != nil {
		t.Fatalf("Index: %v", err)
	}

	metas, ok := capturedBody["metadatas"].([]any)
	if !ok || len(metas) == 0 {
		t.Fatal("expected at least 1 metadata entry")
	}
	firstMeta, ok := metas[0].(map[string]any)
	if !ok {
		t.Fatal("expected metadata to be a map")
	}

	if firstMeta["embed_model"] != "voyage-4" {
		t.Errorf("embed_model = %v, want %q", firstMeta["embed_model"], "voyage-4")
	}
	// embed_dim is stored as int but JSON round-trips as float64
	if embedDimVal, ok := firstMeta["embed_dim"].(float64); !ok || embedDimVal != 3 {
		t.Errorf("embed_dim = %v, want 3", firstMeta["embed_dim"])
	}
}

// --- pgvector embed_model/embed_dim injection test (integration) ---

// TestPGVectorIndexInjectsEmbedMetadata verifies that pgvector's Index method
// injects embed_model and embed_dim into the stored metadata.
func TestPGVectorIndexInjectsEmbedMetadata(t *testing.T) {
	skipIfNoPostgres(t)

	embedder := &mockModelNamerEmbedder{
		mockPlainEmbedder: mockPlainEmbedder{vectorSize: 3},
		modelName:         "voyage-4",
	}

	p, err := NewPGVectorProvider(context.Background(), pgvectorDSN, embedder, "project_memory_test")
	if err != nil {
		t.Fatalf("NewPGVectorProvider: %v", err)
	}
	defer p.Close()

	projectID := "test_embed_meta_inject"
	cleanupTestTable(t, projectID)
	defer cleanupTestTable(t, projectID)

	docs := []Document{
		{ID: uuid.NewString(), Content: "hello world", Meta: map[string]string{"source_file": "test.go"}},
	}
	if err := p.Index(context.Background(), projectID, docs); err != nil {
		t.Fatalf("Index: %v", err)
	}

	// Query the stored metadata directly
	var meta map[string]string
	err = p.pool.QueryRow(context.Background(),
		fmt.Sprintf("SELECT metadata FROM %s WHERE project_id = $1 AND id = $2::uuid", "project_memory_test"),
		projectID, docs[0].ID,
	).Scan(&meta)
	if err != nil {
		t.Fatalf("query metadata: %v", err)
	}

	if meta["embed_model"] != "voyage-4" {
		t.Errorf("embed_model = %q, want %q", meta["embed_model"], "voyage-4")
	}
	if meta["embed_dim"] != "3" {
		t.Errorf("embed_dim = %q, want %q", meta["embed_dim"], "3")
	}
}
