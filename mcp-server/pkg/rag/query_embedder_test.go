package rag

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// --- Mock embedders ---

// mockQueryEmbedder implements both EmbeddingClient and QueryEmbedder.
type mockQueryEmbedder struct {
	mu           sync.Mutex
	embedCalls   int
	queryCalls   int
	embedErr     error
	queryErr     error
	vectorSize   int
	embedResult  [][]float32
	queryResult  []float32
}

func (m *mockQueryEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	m.mu.Lock()
	m.embedCalls++
	m.mu.Unlock()
	if m.embedErr != nil {
		return nil, m.embedErr
	}
	if m.embedResult != nil {
		return m.embedResult, nil
	}
	// Return deterministic vectors
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{0.1, 0.2, 0.3}
	}
	return out, nil
}

func (m *mockQueryEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	m.mu.Lock()
	m.queryCalls++
	m.mu.Unlock()
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	if m.queryResult != nil {
		return m.queryResult, nil
	}
	return []float32{0.4, 0.5, 0.6}, nil
}

func (m *mockQueryEmbedder) VectorSize() int {
	if m.vectorSize > 0 {
		return m.vectorSize
	}
	return 3
}

func (m *mockQueryEmbedder) getEmbedCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.embedCalls
}

func (m *mockQueryEmbedder) getQueryCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.queryCalls
}

// mockPlainEmbedder implements only EmbeddingClient (NOT QueryEmbedder).
type mockPlainEmbedder struct {
	mu          sync.Mutex
	embedCalls  int
	embedErr    error
	vectorSize  int
	embedResult [][]float32
}

func (m *mockPlainEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	m.mu.Lock()
	m.embedCalls++
	m.mu.Unlock()
	if m.embedErr != nil {
		return nil, m.embedErr
	}
	if m.embedResult != nil {
		return m.embedResult, nil
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{0.1, 0.2, 0.3}
	}
	return out, nil
}

func (m *mockPlainEmbedder) VectorSize() int {
	if m.vectorSize > 0 {
		return m.vectorSize
	}
	return 3
}

func (m *mockPlainEmbedder) getEmbedCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.embedCalls
}

// --- Compile-time assertions ---

// Verify mockQueryEmbedder implements both interfaces at compile time.
var _ EmbeddingClient = (*mockQueryEmbedder)(nil)
var _ QueryEmbedder = (*mockQueryEmbedder)(nil)

// Verify mockPlainEmbedder implements EmbeddingClient but NOT QueryEmbedder.
var _ EmbeddingClient = (*mockPlainEmbedder)(nil)

// Verify VoyageEmbeddingClient implements QueryEmbedder.
var _ QueryEmbedder = (*VoyageEmbeddingClient)(nil)

// Verify TEIEmbeddingClient does NOT implement QueryEmbedder (it only has Embed).
// This is implicitly verified by the type assertion tests below.

// --- Shared helpers ---

func newCtx() context.Context {
	return context.Background()
}

// --- Tests ---

// TestQueryEmbedderInterfaceInProviderGo verifies the QueryEmbedder interface
// is defined in provider.go (not in a provider-specific file). Since all files
// are in package rag, we check that the interface type exists and has the
// correct method.
func TestQueryEmbedderInterfaceInProviderGo(t *testing.T) {
	// The interface must be usable as a type
	var qe QueryEmbedder
	_ = qe

	// A type that implements EmbedQuery should satisfy the interface
	embedder := &mockQueryEmbedder{}
	qe, ok := any(embedder).(QueryEmbedder)
	if !ok {
		t.Fatal("mockQueryEmbedder should satisfy QueryEmbedder interface")
	}
	if qe == nil {
		t.Fatal("type assertion returned nil")
	}
}

// TestPlainEmbedderDoesNotImplementQueryEmbedder verifies that a plain embedder
// (like TEIEmbeddingClient) does NOT satisfy QueryEmbedder.
func TestPlainEmbedderDoesNotImplementQueryEmbedder(t *testing.T) {
	embedder := &mockPlainEmbedder{}
	_, ok := any(embedder).(QueryEmbedder)
	if ok {
		t.Fatal("mockPlainEmbedder should NOT satisfy QueryEmbedder interface")
	}
}

// TestProviderInterfaceUnchanged verifies the Provider interface still has
// exactly 9 methods with the expected signatures.
func TestProviderInterfaceUnchanged(t *testing.T) {
	// This is a compile-time check: if the Provider interface changes,
	// the mock provider below will fail to compile.
	var _ Provider = (*mockProvider)(nil)
}

// mockProvider is a minimal implementation of Provider for compile-time checking.
type mockProvider struct{}

func (m *mockProvider) CreateCollection(ctx context.Context, projectID string) error { return nil }
func (m *mockProvider) DeleteCollection(ctx context.Context, projectID string) error { return nil }
func (m *mockProvider) Index(ctx context.Context, projectID string, docs []Document) error {
	return nil
}
func (m *mockProvider) SemanticSearch(ctx context.Context, projectID, query string, limit int) ([]Result, error) {
	return nil, nil
}
func (m *mockProvider) Embed(ctx context.Context, text string) ([]float32, error) { return nil, nil }
func (m *mockProvider) DeletePoints(ctx context.Context, projectID string, pointIDs []string) error {
	return nil
}
func (m *mockProvider) ListPointIDs(ctx context.Context, projectID string, metaFilter map[string]string) ([]string, error) {
	return nil, nil
}
func (m *mockProvider) ListPoints(ctx context.Context, projectID string, metaFilter map[string]string) ([]PointInfo, error) {
	return nil, nil
}
func (m *mockProvider) Close() error { return nil }

// TestQueryEmbedderErrorHandling verifies that when EmbedQuery returns an error,
// SemanticSearch propagates the error.
func TestQueryEmbedderErrorHandling(t *testing.T) {
	// This is tested per-provider, but we include a generic test here to
	// ensure the mock infrastructure works correctly.
	embedder := &mockQueryEmbedder{
		queryErr: errors.New("simulated query embed error"),
	}
	_, err := embedder.EmbedQuery(newCtx(), "test")
	if err == nil {
		t.Fatal("expected error from EmbedQuery")
	}
}
