package indexer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"

	"github.com/enowdev/enowx-rag/pkg/rag"
)

// mockProvider captures documents passed to Index and tracks embed calls.
type mockProvider struct {
	mu             sync.Mutex
	indexedDocs    []rag.Document
	indexCalls     int
	existingPoints []rag.PointInfo
	listPointsErr  error
}

func (m *mockProvider) CreateCollection(ctx context.Context, projectID string) error {
	return nil
}
func (m *mockProvider) DeleteCollection(ctx context.Context, projectID string) error { return nil }
func (m *mockProvider) Index(ctx context.Context, projectID string, docs []rag.Document) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.indexCalls++
	m.indexedDocs = append(m.indexedDocs, docs...)
	return nil
}
func (m *mockProvider) SemanticSearch(ctx context.Context, projectID, query string, limit int) ([]rag.Result, error) {
	return nil, nil
}
func (m *mockProvider) Embed(ctx context.Context, text string) ([]float32, error) { return nil, nil }
func (m *mockProvider) DeletePoints(ctx context.Context, projectID string, pointIDs []string) error {
	return nil
}
func (m *mockProvider) ListPointIDs(ctx context.Context, projectID string, metaFilter map[string]string) ([]string, error) {
	return nil, nil
}
func (m *mockProvider) ListPoints(ctx context.Context, projectID string, metaFilter map[string]string) ([]rag.PointInfo, error) {
	if m.listPointsErr != nil {
		return nil, m.listPointsErr
	}
	return m.existingPoints, nil
}
func (m *mockProvider) Close() error { return nil }

func (m *mockProvider) getIndexCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.indexCalls
}

func (m *mockProvider) getIndexedDocs() []rag.Document {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.indexedDocs
}

// --- Tests ---

// TestComputeContentHash_Format verifies that computeContentHash returns
// exactly 16 lowercase hex characters (first 8 bytes of SHA-256).
func TestComputeContentHash_Format(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"empty string", ""},
		{"hello world", "hello world"},
		{"code snippet", "func main() { fmt.Println(\"hello\") }"},
		{"unicode", "日本語テスト"},
	}
	hexRegex := regexp.MustCompile(`^[0-9a-f]{16}$`)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := computeContentHash(tt.content)
			if !hexRegex.MatchString(hash) {
				t.Errorf("computeContentHash(%q) = %q, want 16 lowercase hex chars matching ^[0-9a-f]{16}$", tt.content, hash)
			}
			if len(hash) != 16 {
				t.Errorf("computeContentHash(%q) length = %d, want 16", tt.content, len(hash))
			}
		})
	}
}

// TestComputeContentHash_Deterministic verifies that the same content always
// produces the same hash, and different content produces different hashes.
func TestComputeContentHash_Deterministic(t *testing.T) {
	hash1 := computeContentHash("hello world")
	hash2 := computeContentHash("hello world")
	if hash1 != hash2 {
		t.Errorf("same content produced different hashes: %q vs %q", hash1, hash2)
	}

	hash3 := computeContentHash("hello world!")
	if hash1 == hash3 {
		t.Errorf("different content produced same hash: %q", hash1)
	}
}

// TestComputeContentHash_KnownValue verifies against a known SHA-256 value.
// SHA-256("hello") = 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
// First 8 bytes hex = 2cf24dba5fb0a30e
func TestComputeContentHash_KnownValue(t *testing.T) {
	hash := computeContentHash("hello")
	expected := "2cf24dba5fb0a30e"
	if hash != expected {
		t.Errorf("computeContentHash(\"hello\") = %q, want %q", hash, expected)
	}
}

// TestIndexerAddsContentHash verifies that IndexProject adds content_hash
// (16 hex chars) to each document's metadata.
func TestIndexerAddsContentHash(t *testing.T) {
	dir := t.TempDir()
	// Create a test file
	if err := os.WriteFile(filepath.Join(dir, "test.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	provider := &mockProvider{}
	idx := NewIndexer(provider, 1000)

	result, err := idx.IndexProject(context.Background(), "testproj", dir)
	if err != nil {
		t.Fatalf("IndexProject: %v", err)
	}
	if result.Indexed == 0 {
		t.Fatal("expected at least 1 indexed chunk")
	}

	docs := provider.getIndexedDocs()
	hexRegex := regexp.MustCompile(`^[0-9a-f]{16}$`)
	for i, d := range docs {
		hash, ok := d.Meta["content_hash"]
		if !ok {
			t.Errorf("doc %d: content_hash not in metadata", i)
			continue
		}
		if !hexRegex.MatchString(hash) {
			t.Errorf("doc %d: content_hash = %q, want 16 lowercase hex chars", i, hash)
		}
	}
}

// TestIndexerAddsChunkVersion verifies that IndexProject adds chunk_version="v2"
// to each document's metadata.
func TestIndexerAddsChunkVersion(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	provider := &mockProvider{}
	idx := NewIndexer(provider, 1000)

	_, err := idx.IndexProject(context.Background(), "testproj", dir)
	if err != nil {
		t.Fatalf("IndexProject: %v", err)
	}

	docs := provider.getIndexedDocs()
	for i, d := range docs {
		version, ok := d.Meta["chunk_version"]
		if !ok {
			t.Errorf("doc %d: chunk_version not in metadata", i)
			continue
		}
		if version != "v2" {
			t.Errorf("doc %d: chunk_version = %q, want %q", i, version, "v2")
		}
	}
}

// TestIndexerSkipUnchangedChunks verifies that on a second IndexProject call
// with the same files, chunks whose content_hash matches existing points are
// skipped (provider.Index is NOT called for them).
func TestIndexerSkipUnchangedChunks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	provider := &mockProvider{}
	idx := NewIndexer(provider, 1000)

	// First scan: all chunks are new, nothing skipped
	result1, err := idx.IndexProject(context.Background(), "testproj", dir)
	if err != nil {
		t.Fatalf("first IndexProject: %v", err)
	}
	if result1.Indexed == 0 {
		t.Fatal("first scan: expected at least 1 indexed chunk")
	}
	if result1.Skipped != 0 {
		t.Errorf("first scan: expected 0 skipped, got %d", result1.Skipped)
	}
	callsAfterFirst := provider.getIndexCalls()
	if callsAfterFirst == 0 {
		t.Fatal("first scan: expected Index to be called at least once")
	}
	docsAfterFirst := provider.getIndexedDocs()

	// Simulate existing points being stored: populate existingPoints with the
	// same docIDs and content_hashes that were indexed in the first scan.
	provider.existingPoints = make([]rag.PointInfo, len(docsAfterFirst))
	for i, d := range docsAfterFirst {
		provider.existingPoints[i] = rag.PointInfo{
			DocID:       d.ID,
			ContentHash: d.Meta["content_hash"],
		}
	}

	// Reset index call counter for the second scan
	provider.mu.Lock()
	provider.indexCalls = 0
	provider.indexedDocs = nil
	provider.mu.Unlock()

	// Second scan: same files, hashes match -> all chunks skipped
	result2, err := idx.IndexProject(context.Background(), "testproj", dir)
	if err != nil {
		t.Fatalf("second IndexProject: %v", err)
	}
	callsAfterSecond := provider.getIndexCalls()
	if callsAfterSecond != 0 {
		t.Errorf("second scan: expected 0 Index calls (all skipped), got %d", callsAfterSecond)
	}
	if result2.Indexed != 0 {
		t.Errorf("second scan: expected 0 indexed, got %d", result2.Indexed)
	}
	if result2.Skipped == 0 {
		t.Errorf("second scan: expected >0 skipped chunks, got 0")
	}
}

// TestIndexerReindexesChangedChunks verifies that when file content changes,
// the content_hash differs and the chunk IS re-indexed.
func TestIndexerReindexesChangedChunks(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.go")

	if err := os.WriteFile(filePath, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	provider := &mockProvider{}
	idx := NewIndexer(provider, 1000)

	// First scan
	result1, err := idx.IndexProject(context.Background(), "testproj", dir)
	if err != nil {
		t.Fatalf("first IndexProject: %v", err)
	}
	if result1.Indexed == 0 {
		t.Fatal("first scan: expected at least 1 indexed chunk")
	}
	docsAfterFirst := provider.getIndexedDocs()

	// Populate existing points with old hashes (simulating first scan persisted)
	provider.existingPoints = make([]rag.PointInfo, len(docsAfterFirst))
	for i, d := range docsAfterFirst {
		provider.existingPoints[i] = rag.PointInfo{
			DocID:       d.ID,
			ContentHash: d.Meta["content_hash"],
		}
	}

	// Change file content
	if err := os.WriteFile(filePath, []byte("package main\n\nfunc newFunc() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Reset counters
	provider.mu.Lock()
	provider.indexCalls = 0
	provider.indexedDocs = nil
	provider.mu.Unlock()

	// Second scan: content changed -> should re-index
	result2, err := idx.IndexProject(context.Background(), "testproj", dir)
	if err != nil {
		t.Fatalf("second IndexProject: %v", err)
	}
	if result2.Indexed == 0 {
		t.Error("second scan: expected re-indexed chunks after content change, got 0")
	}
}

// TestIndexerContentHashCorrectness verifies that the content_hash stored in
// metadata matches the expected hash computed from the chunk content.
func TestIndexerContentHashCorrectness(t *testing.T) {
	dir := t.TempDir()
	content := "package main\n"
	if err := os.WriteFile(filepath.Join(dir, "test.go"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	provider := &mockProvider{}
	idx := NewIndexer(provider, 1000)

	_, err := idx.IndexProject(context.Background(), "testproj", dir)
	if err != nil {
		t.Fatalf("IndexProject: %v", err)
	}

	docs := provider.getIndexedDocs()
	if len(docs) == 0 {
		t.Fatal("no documents indexed")
	}

	// The chunk content in the doc is "File: test.go\n\n" + chunk
	// The hash should be computed from the raw chunk (without the "File:" prefix)
	// Check that the hash matches computeContentHash of the raw chunk text
	expectedHash := computeContentHash(content)
	if docs[0].Meta["content_hash"] != expectedHash {
		t.Errorf("content_hash = %q, want %q (hash of raw chunk content)", docs[0].Meta["content_hash"], expectedHash)
	}
}

// TestIndexerSkippedFieldInSyncResult verifies that the Skipped field is
// populated in the SyncResult.
func TestIndexerSkippedFieldInSyncResult(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	provider := &mockProvider{}
	idx := NewIndexer(provider, 1000)

	// First scan
	result1, err := idx.IndexProject(context.Background(), "testproj", dir)
	if err != nil {
		t.Fatalf("first IndexProject: %v", err)
	}
	if result1.Skipped != 0 {
		t.Errorf("first scan: expected Skipped=0, got %d", result1.Skipped)
	}
	docsAfterFirst := provider.getIndexedDocs()

	// Populate existing points
	provider.existingPoints = make([]rag.PointInfo, len(docsAfterFirst))
	for i, d := range docsAfterFirst {
		provider.existingPoints[i] = rag.PointInfo{
			DocID:       d.ID,
			ContentHash: d.Meta["content_hash"],
		}
	}

	// Second scan: all chunks match -> all skipped
	result2, err := idx.IndexProject(context.Background(), "testproj", dir)
	if err != nil {
		t.Fatalf("second IndexProject: %v", err)
	}
	if result2.Skipped == 0 {
		t.Errorf("second scan: expected Skipped > 0, got 0")
	}
	if result2.Skipped != result1.Indexed {
		t.Errorf("second scan: expected Skipped=%d (same as first Indexed), got %d", result1.Indexed, result2.Skipped)
	}
}

// TestIndexerListPointsErrorProceedsWithoutSkip verifies that if ListPoints
// fails, the indexer still indexes all chunks (no skip optimization, but no crash).
func TestIndexerListPointsErrorProceedsWithoutSkip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	provider := &mockProvider{
		listPointsErr: fmt.Errorf("connection refused"),
	}
	idx := NewIndexer(provider, 1000)

	result, err := idx.IndexProject(context.Background(), "testproj", dir)
	if err != nil {
		t.Fatalf("IndexProject with ListPoints error: %v", err)
	}
	if result.Indexed == 0 {
		t.Error("expected chunks to be indexed even when ListPoints fails")
	}
}
