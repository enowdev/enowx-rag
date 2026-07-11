package rag

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// TestPGVectorCompositePrimaryKey verifies that the project_memory table
// has a composite PRIMARY KEY on (project_id, id), not a single-column PK
// on id alone.
func TestPGVectorCompositePrimaryKey(t *testing.T) {
	skipIfNoPostgres(t)

	p, err := NewPGVectorProvider(context.Background(), pgvectorDSN, &mockPlainEmbedder{vectorSize: 3}, "project_memory_test")
	if err != nil {
		t.Fatalf("NewPGVectorProvider: %v", err)
	}
	defer p.Close()

	// Query pg_constraint to find the primary key constraint and verify
	// it covers both project_id and id columns.
	var numCols int
	err = p.pool.QueryRow(context.Background(), `
SELECT array_length(con.conkey, 1)
FROM pg_constraint con
JOIN pg_class rel ON rel.oid = con.conrelid
WHERE rel.relname = 'project_memory_test'
  AND con.contype = 'p'
`).Scan(&numCols)
	if err != nil {
		t.Fatalf("could not query PK constraint: %v", err)
	}

	if numCols != 2 {
		t.Errorf("expected composite PK with 2 columns, got %d", numCols)
	}

	// Also verify the PK covers exactly project_id and id (in any order).
	var pkColumns []string
	rows, err := p.pool.Query(context.Background(), `
SELECT a.attname
FROM pg_constraint con
JOIN pg_class rel ON rel.oid = con.conrelid
JOIN pg_attribute a ON a.attrelid = rel.oid AND a.attnum = ANY(con.conkey)
WHERE rel.relname = 'project_memory_test'
  AND con.contype = 'p'
ORDER BY a.attname
`)
	if err != nil {
		t.Fatalf("could not query PK columns: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			t.Fatalf("scan: %v", err)
		}
		pkColumns = append(pkColumns, col)
	}
	if len(pkColumns) != 2 {
		t.Fatalf("expected 2 PK columns, got %d", len(pkColumns))
	}
	if pkColumns[0] != "id" || pkColumns[1] != "project_id" {
		t.Errorf("expected PK columns [id, project_id], got %v", pkColumns)
	}
}

// TestPGVectorSameDocIDDifferentProjects verifies that indexing the same
// doc ID into different projects does NOT overwrite data. This is the core
// issue the composite key fixes: doc IDs like "config/config.go#chunk0"
// are the same across projects and must be allowed to coexist.
func TestPGVectorSameDocIDDifferentProjects(t *testing.T) {
	skipIfNoPostgres(t)

	embedder := &mockQueryEmbedder{
		queryResult: []float32{0.1, 0.2, 0.3},
		embedResult: [][]float32{{0.9, 0.8, 0.7}},
	}

	p, err := NewPGVectorProvider(context.Background(), pgvectorDSN, embedder, "project_memory_test")
	if err != nil {
		t.Fatalf("NewPGVectorProvider: %v", err)
	}
	defer p.Close()

	projectA := "test_composite_proj_a"
	projectB := "test_composite_proj_b"
	cleanupTestTable(t, projectA)
	cleanupTestTable(t, projectB)
	defer cleanupTestTable(t, projectA)
	defer cleanupTestTable(t, projectB)

	// Use the same doc ID for both projects (like the indexer generates
	// "dir/file.go#chunk0")
	sharedDocID := "config/config.go#chunk0"

	// Index into project A
	docsA := []Document{
		{ID: sharedDocID, Content: "content for project A", Meta: map[string]string{"source_file": "config.go"}},
	}
	if err := p.Index(context.Background(), projectA, docsA); err != nil {
		t.Fatalf("Index into project A: %v", err)
	}

	// Index into project B with the same doc ID
	docsB := []Document{
		{ID: sharedDocID, Content: "content for project B", Meta: map[string]string{"source_file": "config.go"}},
	}
	if err := p.Index(context.Background(), projectB, docsB); err != nil {
		t.Fatalf("Index into project B: %v", err)
	}

	// Verify both projects have 1 point each with the same ID
	idsA, err := p.ListPointIDs(context.Background(), projectA, nil)
	if err != nil {
		t.Fatalf("ListPointIDs project A: %v", err)
	}
	if len(idsA) != 1 || idsA[0] != sharedDocID {
		t.Errorf("project A: expected [%s], got %v", sharedDocID, idsA)
	}

	idsB, err := p.ListPointIDs(context.Background(), projectB, nil)
	if err != nil {
		t.Fatalf("ListPointIDs project B: %v", err)
	}
	if len(idsB) != 1 || idsB[0] != sharedDocID {
		t.Errorf("project B: expected [%s], got %v", sharedDocID, idsB)
	}

	// Verify the content is different (not overwritten)
	pointsA, err := p.ListPoints(context.Background(), projectA, nil)
	if err != nil {
		t.Fatalf("ListPoints project A: %v", err)
	}
	if len(pointsA) != 1 {
		t.Fatalf("expected 1 point in project A, got %d", len(pointsA))
	}

	pointsB, err := p.ListPoints(context.Background(), projectB, nil)
	if err != nil {
		t.Fatalf("ListPoints project B: %v", err)
	}
	if len(pointsB) != 1 {
		t.Fatalf("expected 1 point in project B, got %d", len(pointsB))
	}

	// Verify content is correct per project by querying directly
	var contentA, contentB string
	err = p.pool.QueryRow(context.Background(),
		fmt.Sprintf("SELECT content FROM %s WHERE project_id = $1 AND id = $2", "project_memory_test"),
		projectA, sharedDocID,
	).Scan(&contentA)
	if err != nil {
		t.Fatalf("query content A: %v", err)
	}
	if contentA != "content for project A" {
		t.Errorf("project A content = %q, want %q", contentA, "content for project A")
	}

	err = p.pool.QueryRow(context.Background(),
		fmt.Sprintf("SELECT content FROM %s WHERE project_id = $1 AND id = $2", "project_memory_test"),
		projectB, sharedDocID,
	).Scan(&contentB)
	if err != nil {
		t.Fatalf("query content B: %v", err)
	}
	if contentB != "content for project B" {
		t.Errorf("project B content = %q, want %q", contentB, "content for project B")
	}
}

// TestPGVectorReindexSameProjectOverwrites verifies that re-indexing the same
// project with the same doc ID does an UPDATE (not a duplicate INSERT),
// because the composite key (project_id, id) should conflict within the same project.
func TestPGVectorReindexSameProjectOverwrites(t *testing.T) {
	skipIfNoPostgres(t)

	embedder := &mockQueryEmbedder{
		queryResult: []float32{0.1, 0.2, 0.3},
		embedResult: [][]float32{{0.9, 0.8, 0.7}},
	}

	p, err := NewPGVectorProvider(context.Background(), pgvectorDSN, embedder, "project_memory_test")
	if err != nil {
		t.Fatalf("NewPGVectorProvider: %v", err)
	}
	defer p.Close()

	projectID := "test_composite_reindex"
	cleanupTestTable(t, projectID)
	defer cleanupTestTable(t, projectID)

	docID := "main.go#chunk0"
	// First index
	docs := []Document{
		{ID: docID, Content: "original content", Meta: map[string]string{"source_file": "main.go"}},
	}
	if err := p.Index(context.Background(), projectID, docs); err != nil {
		t.Fatalf("Index (first): %v", err)
	}

	// Second index with same ID, different content (simulates re-index after file change)
	docs2 := []Document{
		{ID: docID, Content: "updated content", Meta: map[string]string{"source_file": "main.go"}},
	}
	if err := p.Index(context.Background(), projectID, docs2); err != nil {
		t.Fatalf("Index (second): %v", err)
	}

	// Verify only 1 row exists (ON CONFLICT did UPDATE, not INSERT)
	ids, err := p.ListPointIDs(context.Background(), projectID, nil)
	if err != nil {
		t.Fatalf("ListPointIDs: %v", err)
	}
	if len(ids) != 1 {
		t.Errorf("expected 1 point after reindex, got %d", len(ids))
	}

	// Verify content was updated
	var content string
	err = p.pool.QueryRow(context.Background(),
		fmt.Sprintf("SELECT content FROM %s WHERE project_id = $1 AND id = $2", "project_memory_test"),
		projectID, docID,
	).Scan(&content)
	if err != nil {
		t.Fatalf("query content: %v", err)
	}
	if content != "updated content" {
		t.Errorf("content = %q, want %q", content, "updated content")
	}
}

// TestPGVectorDeletePointsScopedToProject verifies that DeletePoints only
// deletes points for the specified project, not across projects.
func TestPGVectorDeletePointsScopedToProject(t *testing.T) {
	skipIfNoPostgres(t)

	embedder := &mockQueryEmbedder{
		embedResult: [][]float32{{0.9, 0.8, 0.7}},
	}

	p, err := NewPGVectorProvider(context.Background(), pgvectorDSN, embedder, "project_memory_test")
	if err != nil {
		t.Fatalf("NewPGVectorProvider: %v", err)
	}
	defer p.Close()

	projectA := "test_composite_delete_a"
	projectB := "test_composite_delete_b"
	cleanupTestTable(t, projectA)
	cleanupTestTable(t, projectB)
	defer cleanupTestTable(t, projectA)
	defer cleanupTestTable(t, projectB)

	sharedDocID := "shared.go#chunk0"

	// Index into both projects
	docs := []Document{
		{ID: sharedDocID, Content: "content A", Meta: map[string]string{"source_file": "shared.go"}},
	}
	if err := p.Index(context.Background(), projectA, docs); err != nil {
		t.Fatalf("Index A: %v", err)
	}
	docs2 := []Document{
		{ID: sharedDocID, Content: "content B", Meta: map[string]string{"source_file": "shared.go"}},
	}
	if err := p.Index(context.Background(), projectB, docs2); err != nil {
		t.Fatalf("Index B: %v", err)
	}

	// Delete from project A only
	if err := p.DeletePoints(context.Background(), projectA, []string{sharedDocID}); err != nil {
		t.Fatalf("DeletePoints: %v", err)
	}

	// Verify project A has 0 points
	idsA, err := p.ListPointIDs(context.Background(), projectA, nil)
	if err != nil {
		t.Fatalf("ListPointIDs A: %v", err)
	}
	if len(idsA) != 0 {
		t.Errorf("expected 0 points in project A after delete, got %d", len(idsA))
	}

	// Verify project B still has 1 point
	idsB, err := p.ListPointIDs(context.Background(), projectB, nil)
	if err != nil {
		t.Fatalf("ListPointIDs B: %v", err)
	}
	if len(idsB) != 1 {
		t.Errorf("expected 1 point in project B (untouched), got %d", len(idsB))
	}
	if idsB[0] != sharedDocID {
		t.Errorf("project B id = %q, want %q", idsB[0], sharedDocID)
	}
}

// TestPGVectorSearchScopedToProject verifies that SemanticSearch only returns
// results from the specified project, not from other projects with the same doc ID.
func TestPGVectorSearchScopedToProject(t *testing.T) {
	skipIfNoPostgres(t)

	embedder := &mockQueryEmbedder{
		queryResult: []float32{0.1, 0.2, 0.3},
		embedResult: [][]float32{{0.1, 0.2, 0.3}},
	}

	p, err := NewPGVectorProvider(context.Background(), pgvectorDSN, embedder, "project_memory_test")
	if err != nil {
		t.Fatalf("NewPGVectorProvider: %v", err)
	}
	defer p.Close()

	projectA := "test_composite_search_a"
	projectB := "test_composite_search_b"
	cleanupTestTable(t, projectA)
	cleanupTestTable(t, projectB)
	defer cleanupTestTable(t, projectA)
	defer cleanupTestTable(t, projectB)

	sharedDocID := "handler.go#chunk0"

	// Index different content into each project with the same doc ID
	docsA := []Document{
		{ID: sharedDocID, Content: "alpha content in project A", Meta: map[string]string{"source_file": "handler.go"}},
	}
	if err := p.Index(context.Background(), projectA, docsA); err != nil {
		t.Fatalf("Index A: %v", err)
	}

	docsB := []Document{
		{ID: sharedDocID, Content: "beta content in project B", Meta: map[string]string{"source_file": "handler.go"}},
	}
	if err := p.Index(context.Background(), projectB, docsB); err != nil {
		t.Fatalf("Index B: %v", err)
	}

	// Search project A — should only return project A's content
	resultsA, err := p.SemanticSearch(context.Background(), projectA, "alpha", 5)
	if err != nil {
		t.Fatalf("SemanticSearch A: %v", err)
	}
	if len(resultsA) != 1 {
		t.Fatalf("expected 1 result from project A, got %d", len(resultsA))
	}
	if !strings.Contains(resultsA[0].Content, "project A") {
		t.Errorf("project A search returned wrong content: %q", resultsA[0].Content)
	}

	// Search project B — should only return project B's content
	resultsB, err := p.SemanticSearch(context.Background(), projectB, "beta", 5)
	if err != nil {
		t.Fatalf("SemanticSearch B: %v", err)
	}
	if len(resultsB) != 1 {
		t.Fatalf("expected 1 result from project B, got %d", len(resultsB))
	}
	if !strings.Contains(resultsB[0].Content, "project B") {
		t.Errorf("project B search returned wrong content: %q", resultsB[0].Content)
	}
}
