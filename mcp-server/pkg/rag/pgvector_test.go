package rag

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
)

// pgvectorDSN is the connection string for integration tests.
const pgvectorDSN = "postgresql://enowdev@localhost:5432/enowxrag"

// skipIfNoPostgres skips the test if PostgreSQL is not available.
func skipIfNoPostgres(t *testing.T) {
	t.Helper()
	// Quick connection check
	p, err := NewPGVectorProvider(context.Background(), pgvectorDSN, &mockPlainEmbedder{vectorSize: 3}, "project_memory_test")
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	p.Close()
}

// cleanupTestTable removes all rows for the given project ID.
func cleanupTestTable(t *testing.T, projectID string) {
	t.Helper()
	p, err := NewPGVectorProvider(context.Background(), pgvectorDSN, &mockPlainEmbedder{vectorSize: 3}, "project_memory_test")
	if err != nil {
		t.Fatalf("cleanup connect: %v", err)
	}
	defer p.Close()
	_, err = p.pool.Exec(context.Background(),
		fmt.Sprintf("DELETE FROM %s WHERE project_id = $1", "project_memory_test"), projectID)
	if err != nil {
		// Table might not exist yet, that's fine
		t.Logf("cleanup: %v (may be OK if table doesn't exist)", err)
	}
}

// TestPGVectorSemanticSearchUsesEmbedQuery verifies that when the embedder
// implements QueryEmbedder, pgvector's SemanticSearch calls EmbedQuery (not Embed).
// This is an integration test requiring real PostgreSQL.
func TestPGVectorSemanticSearchUsesEmbedQuery(t *testing.T) {
	skipIfNoPostgres(t)

	embedder := &mockQueryEmbedder{
		queryResult: []float32{0.1, 0.2, 0.3},
		embedResult: [][]float32{{0.9, 0.9, 0.9}}, // should NOT be used
	}

	p, err := NewPGVectorProvider(context.Background(), pgvectorDSN, embedder, "project_memory_test")
	if err != nil {
		t.Fatalf("NewPGVectorProvider: %v", err)
	}
	defer p.Close()

	projectID := "test_query_embed_qe"
	cleanupTestTable(t, projectID)
	defer cleanupTestTable(t, projectID)

	// Index a document so the search has something to find
	docs := []Document{
		{ID: uuid.NewString(), Content: "hello world", Meta: map[string]string{"source_file": "test.go"}},
	}
	if err := p.Index(context.Background(), projectID, docs); err != nil {
		t.Fatalf("Index: %v", err)
	}

	results, err := p.SemanticSearch(context.Background(), projectID, "hello", 5)
	if err != nil {
		t.Fatalf("SemanticSearch: %v", err)
	}

	if embedder.getQueryCalls() == 0 {
		t.Error("EmbedQuery should have been called")
	}
	if embedder.getEmbedCalls() != 0 {
		// Note: Index() calls Embed, but SemanticSearch should not.
		// Index call above increments embedCalls by 1.
		// So total embedCalls should be 1 (from Index only), not more.
		if embedder.getEmbedCalls() > 1 {
			t.Errorf("Embed should NOT have been called by SemanticSearch; got %d total calls (1 expected from Index)", embedder.getEmbedCalls())
		}
	}

	if len(results) == 0 {
		t.Error("expected at least 1 result")
	}
}

// TestPGVectorSemanticSearchFallsBackToEmbed verifies that when the embedder
// does NOT implement QueryEmbedder, pgvector's SemanticSearch falls back to Embed.
func TestPGVectorSemanticSearchFallsBackToEmbed(t *testing.T) {
	skipIfNoPostgres(t)

	embedder := &mockPlainEmbedder{
		embedResult: [][]float32{{0.1, 0.2, 0.3}},
	}

	p, err := NewPGVectorProvider(context.Background(), pgvectorDSN, embedder, "project_memory_test")
	if err != nil {
		t.Fatalf("NewPGVectorProvider: %v", err)
	}
	defer p.Close()

	projectID := "test_query_embed_plain"
	cleanupTestTable(t, projectID)
	defer cleanupTestTable(t, projectID)

	// Index a document
	docs := []Document{
		{ID: uuid.NewString(), Content: "hello world", Meta: map[string]string{"source_file": "test.go"}},
	}
	if err := p.Index(context.Background(), projectID, docs); err != nil {
		t.Fatalf("Index: %v", err)
	}

	// Track embed calls before search
	callsBeforeSearch := embedder.getEmbedCalls()

	_, err = p.SemanticSearch(context.Background(), projectID, "hello", 5)
	if err != nil {
		t.Fatalf("SemanticSearch: %v", err)
	}

	callsAfterSearch := embedder.getEmbedCalls()
	if callsAfterSearch <= callsBeforeSearch {
		t.Error("Embed should have been called by SemanticSearch (fallback)")
	}
}

// TestPGVectorSemanticSearchEmbedQueryError verifies that if EmbedQuery returns
// an error, SemanticSearch propagates it.
func TestPGVectorSemanticSearchEmbedQueryError(t *testing.T) {
	skipIfNoPostgres(t)

	embedder := &mockQueryEmbedder{
		queryErr: fmt.Errorf("simulated query embed error"),
	}

	p, err := NewPGVectorProvider(context.Background(), pgvectorDSN, embedder, "project_memory_test")
	if err != nil {
		t.Fatalf("NewPGVectorProvider: %v", err)
	}
	defer p.Close()

	_, err = p.SemanticSearch(context.Background(), "testproj", "hello", 5)
	if err == nil {
		t.Fatal("expected error from EmbedQuery failure")
	}
}

// TestPGVectorEmbedFallbackNoPanic verifies that a plain embedder (no QueryEmbedder)
// doesn't cause a panic in SemanticSearch.
func TestPGVectorEmbedFallbackNoPanic(t *testing.T) {
	skipIfNoPostgres(t)

	// Use a plain embedder that does NOT implement QueryEmbedder
	embedder := &mockPlainEmbedder{}

	p, err := NewPGVectorProvider(context.Background(), pgvectorDSN, embedder, "project_memory_test")
	if err != nil {
		t.Fatalf("NewPGVectorProvider: %v", err)
	}
	defer p.Close()

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SemanticSearch panicked with plain embedder: %v", r)
		}
	}()

	_, _ = p.SemanticSearch(context.Background(), "nonexistent_project", "hello", 5)
}

// TestPGVectorWithVoyageEmbedder verifies that VoyageEmbeddingClient (which
// implements QueryEmbedder) is used correctly by pgvector.
// This is a compile-time check that the type assertion works.
func TestPGVectorWithVoyageEmbedder(t *testing.T) {
	// Verify VoyageEmbeddingClient implements QueryEmbedder at compile time
	var embedder EmbeddingClient = &VoyageEmbeddingClient{
		APIKey: "test",
		Model:  "voyage-4",
		Dim:    1024,
	}

	// The type assertion should succeed
	_, ok := embedder.(QueryEmbedder)
	if !ok {
		t.Fatal("VoyageEmbeddingClient should implement QueryEmbedder")
	}
}

// TestTEIDoesNotImplementQueryEmbedder verifies that TEIEmbeddingClient does NOT
// implement QueryEmbedder (it only has Embed, not EmbedQuery).
func TestTEIDoesNotImplementQueryEmbedder(t *testing.T) {
	embedder := NewTEIEmbeddingClient("http://localhost:8081")
	_, ok := any(embedder).(QueryEmbedder)
	if ok {
		t.Fatal("TEIEmbeddingClient should NOT implement QueryEmbedder")
	}
}

// TestPGVectorSemanticSearchDefaultLimit verifies that limit=0 defaults to 5.
func TestPGVectorSemanticSearchDefaultLimit(t *testing.T) {
	skipIfNoPostgres(t)

	embedder := &mockQueryEmbedder{}

	p, err := NewPGVectorProvider(context.Background(), pgvectorDSN, embedder, "project_memory_test")
	if err != nil {
		t.Fatalf("NewPGVectorProvider: %v", err)
	}
	defer p.Close()

	projectID := "test_default_limit"
	cleanupTestTable(t, projectID)
	defer cleanupTestTable(t, projectID)

	// Index multiple documents
	docs := []Document{
		{ID: uuid.NewString(), Content: "apple banana", Meta: map[string]string{}},
		{ID: uuid.NewString(), Content: "cherry date", Meta: map[string]string{}},
	}
	if err := p.Index(context.Background(), projectID, docs); err != nil {
		t.Fatalf("Index: %v", err)
	}

	// limit=0 should default to 5
	results, err := p.SemanticSearch(context.Background(), projectID, "apple", 0)
	if err != nil {
		t.Fatalf("SemanticSearch: %v", err)
	}
	// Should get at most 2 results (we only indexed 2 docs)
	if len(results) > 2 {
		t.Errorf("expected at most 2 results, got %d", len(results))
	}
}

// TestPGVectorStringID verifies that the pgvector table accepts string IDs
// like the ones the indexer generates (e.g. "dir/file.go#chunk0") and that
// all CRUD operations (Index, ListPointIDs, ListPoints, DeletePoints, SemanticSearch)
// work correctly with TEXT primary key.
func TestPGVectorStringID(t *testing.T) {
	skipIfNoPostgres(t)

	embedder := &mockQueryEmbedder{
		queryResult: []float32{0.1, 0.2, 0.3},
	}

	p, err := NewPGVectorProvider(context.Background(), pgvectorDSN, embedder, "project_memory_test")
	if err != nil {
		t.Fatalf("NewPGVectorProvider: %v", err)
	}
	defer p.Close()

	projectID := "test_string_id"
	cleanupTestTable(t, projectID)
	defer cleanupTestTable(t, projectID)

	// Use string IDs like the indexer generates
	stringID := "enowx-rag/pkg/rag/pgvector.go#chunk0"
	docs := []Document{
		{ID: stringID, Content: "hello world from string id test", Meta: map[string]string{"source_file": "pgvector.go"}},
	}
	if err := p.Index(context.Background(), projectID, docs); err != nil {
		t.Fatalf("Index with string ID: %v", err)
	}

	// Verify ListPointIDs returns the string ID
	ids, err := p.ListPointIDs(context.Background(), projectID, nil)
	if err != nil {
		t.Fatalf("ListPointIDs: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 point ID, got %d", len(ids))
	}
	if ids[0] != stringID {
		t.Errorf("expected id %q, got %q", stringID, ids[0])
	}

	// Verify ListPoints returns the string ID
	points, err := p.ListPoints(context.Background(), projectID, nil)
	if err != nil {
		t.Fatalf("ListPoints: %v", err)
	}
	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(points))
	}
	if points[0].ID != stringID {
		t.Errorf("expected point id %q, got %q", stringID, points[0].ID)
	}

	// Verify SemanticSearch works
	results, err := p.SemanticSearch(context.Background(), projectID, "hello", 5)
	if err != nil {
		t.Fatalf("SemanticSearch: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != stringID {
		t.Errorf("expected result id %q, got %q", stringID, results[0].ID)
	}

	// Verify DeletePoints works with string ID
	if err := p.DeletePoints(context.Background(), projectID, []string{stringID}); err != nil {
		t.Fatalf("DeletePoints: %v", err)
	}

	// Verify the point is gone
	ids, err = p.ListPointIDs(context.Background(), projectID, nil)
	if err != nil {
		t.Fatalf("ListPointIDs after delete: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 point IDs after delete, got %d", len(ids))
	}
}

// TestPGVectorTextPrimaryKeySchema verifies that the id column is TEXT, not UUID.
func TestPGVectorTextPrimaryKeySchema(t *testing.T) {
	skipIfNoPostgres(t)

	p, err := NewPGVectorProvider(context.Background(), pgvectorDSN, &mockPlainEmbedder{vectorSize: 3}, "project_memory_test")
	if err != nil {
		t.Fatalf("NewPGVectorProvider: %v", err)
	}
	defer p.Close()

	var dataType string
	err = p.pool.QueryRow(context.Background(), `
SELECT data_type FROM information_schema.columns
WHERE table_name = 'project_memory_test' AND column_name = 'id'
`).Scan(&dataType)
	if err != nil {
		t.Fatalf("query id column type: %v", err)
	}
	if dataType != "text" {
		t.Errorf("expected id column data_type 'text', got %q", dataType)
	}
}

// Ensure the test binary doesn't fail if env vars are missing.
func init() {
	// Set a dummy Voyage API key for any tests that create Voyage clients
	if os.Getenv("RAG_VOYAGE_API_KEY") == "" {
		os.Setenv("RAG_VOYAGE_API_KEY", "dummy")
	}
}
