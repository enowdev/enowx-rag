package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/enowdev/enowx-rag/pkg/rag"
)

// --- Mock Provider ---

type mockProvider struct {
	mu               sync.Mutex
	createCalls      int
	deleteCalls      int
	indexCalls       int
	searchCalls      int
	deletePtCalls    int
	listPointsCalls  int
	closeCalls       int

	// Hybrid search tracking
	hybridCalls int

	// Configurable behaviours
	searchErr   error
	searchLimit int // captured limit
	results     []rag.Result
	points      []rag.PointInfo
	indexErr    error
	createErr   error
	deleteErr   error
	deletePtErr  error
	listPtsErr   error
	listedProjectIDs []string
}

func (m *mockProvider) CreateCollection(ctx context.Context, projectID string) error {
	m.mu.Lock()
	m.createCalls++
	m.mu.Unlock()
	if m.createErr != nil {
		return m.createErr
	}
	return nil
}

func (m *mockProvider) DeleteCollection(ctx context.Context, projectID string) error {
	m.mu.Lock()
	m.deleteCalls++
	m.mu.Unlock()
	if m.deleteErr != nil {
		return m.deleteErr
	}
	return nil
}

func (m *mockProvider) Index(ctx context.Context, projectID string, docs []rag.Document) error {
	m.mu.Lock()
	m.indexCalls++
	m.mu.Unlock()
	if m.indexErr != nil {
		return m.indexErr
	}
	return nil
}

func (m *mockProvider) SemanticSearch(ctx context.Context, projectID, query string, limit int) ([]rag.Result, error) {
	m.mu.Lock()
	m.searchCalls++
	m.searchLimit = limit
	m.mu.Unlock()
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	return m.generateResults()
}

// SemanticSearchHybrid implements rag.HybridSearcher for testing hybrid search.
func (m *mockProvider) SemanticSearchHybrid(ctx context.Context, projectID, query string, limit int) ([]rag.Result, error) {
	m.mu.Lock()
	m.hybridCalls++
	m.searchLimit = limit
	m.mu.Unlock()
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	return m.generateResults()
}

func (m *mockProvider) generateResults() ([]rag.Result, error) {
	if m.results != nil {
		return m.results, nil
	}
	// Generate default results
	results := make([]rag.Result, 10)
	for i := range results {
		results[i] = rag.Result{
			ID:      fmt.Sprintf("result-%d", i),
			Content: fmt.Sprintf("content-%d", i),
			Score:   float64(10-i) / 10.0,
			Meta: map[string]string{
				"source_file":  fmt.Sprintf("file%d.go", i),
				"content_hash": fmt.Sprintf("%016x", i),
			},
		}
	}
	return results, nil
}

func (m *mockProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3}, nil
}

func (m *mockProvider) DeletePoints(ctx context.Context, projectID string, pointIDs []string) error {
	m.mu.Lock()
	m.deletePtCalls++
	m.mu.Unlock()
	if m.deletePtErr != nil {
		return m.deletePtErr
	}
	return nil
}

func (m *mockProvider) ListPointIDs(ctx context.Context, projectID string, metaFilter map[string]string) ([]string, error) {
	return []string{"p1", "p2", "p3"}, nil
}

func (m *mockProvider) ListPoints(ctx context.Context, projectID string, metaFilter map[string]string) ([]rag.PointInfo, error) {
	m.mu.Lock()
	m.listPointsCalls++
	m.mu.Unlock()
	if m.listPtsErr != nil {
		return nil, m.listPtsErr
	}
	if m.points != nil {
		return m.points, nil
	}
	return []rag.PointInfo{
		{ID: "p1", SourceFile: "file1.go", ContentHash: "abc123"},
		{ID: "p2", SourceFile: "file2.go", ContentHash: "def456"},
	}, nil
}

func (m *mockProvider) Close() error {
	m.mu.Lock()
	m.closeCalls++
	m.mu.Unlock()
	return nil
}

// Implement ProjectLister for testing ListProjects
func (m *mockProvider) ListProjectIDs(ctx context.Context) ([]string, error) {
	if m.listedProjectIDs != nil {
		return m.listedProjectIDs, nil
	}
	return []string{"proj1", "proj2"}, nil
}

// Compile-time assertions
var _ rag.Provider = (*mockProvider)(nil)
var _ ProjectLister = (*mockProvider)(nil)
var _ rag.HybridSearcher = (*mockProvider)(nil)

// --- Mock Reranker ---

type mockReranker struct {
	mu           sync.Mutex
	calls        int
	lastQuery    string
	lastDocs     []string
	lastTopK     int
	rerankErr    error
	hits         []rag.RerankHit
}

func (m *mockReranker) Rerank(ctx context.Context, query string, docs []string, topK int) ([]rag.RerankHit, error) {
	m.mu.Lock()
	m.calls++
	m.lastQuery = query
	m.lastDocs = docs
	m.lastTopK = topK
	m.mu.Unlock()
	if m.rerankErr != nil {
		return nil, m.rerankErr
	}
	if m.hits != nil {
		return m.hits, nil
	}
	// Default: return top-K reversed (to distinguish from semantic order)
	hits := make([]rag.RerankHit, 0, topK)
	for i := 0; i < topK && i < len(docs); i++ {
		hits = append(hits, rag.RerankHit{
			Index: len(docs) - 1 - i, // reversed order
			Score: float64(topK-i) / float64(topK),
		})
	}
	return hits, nil
}

var _ rag.Reranker = (*mockReranker)(nil)

// --- Tests ---

// TestNewService verifies that a Service can be constructed with a mock
// provider and nil reranker, and is non-nil.
func TestNewService_NonNil(t *testing.T) {
	p := &mockProvider{}
	svc := NewService(p, nil, nil)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if svc.provider == nil {
		t.Error("Service.provider should be non-nil")
	}
	if svc.reranker != nil {
		t.Error("Service.reranker should be nil when nil is passed")
	}
	if svc.events == nil {
		t.Error("Service.events should be non-nil (EventBus auto-created)")
	}
}

// TestSearchDefaults verifies that zero-valued SearchOpts default to K=5, Recall=40.
func TestSearchDefaults(t *testing.T) {
	p := &mockProvider{}
	svc := NewService(p, nil, nil)

	_, err := svc.Search(context.Background(), "proj", "query", SearchOpts{})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.searchLimit != DefaultRecall {
		t.Errorf("expected SemanticSearch called with limit=%d, got %d", DefaultRecall, p.searchLimit)
	}
}

// TestSearchDefaultK verifies that results are truncated to K=5 by default.
func TestSearchDefaultK(t *testing.T) {
	p := &mockProvider{}
	svc := NewService(p, nil, nil)

	results, err := svc.Search(context.Background(), "proj", "query", SearchOpts{})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	if len(results) != DefaultK {
		t.Errorf("expected %d results (default K), got %d", DefaultK, len(results))
	}
}

// TestSearchCustomK verifies that a custom K is respected.
func TestSearchCustomK(t *testing.T) {
	p := &mockProvider{}
	svc := NewService(p, nil, nil)

	results, err := svc.Search(context.Background(), "proj", "query", SearchOpts{K: 3, Recall: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.searchLimit != 10 {
		t.Errorf("expected SemanticSearch called with limit=10, got %d", p.searchLimit)
	}
}

// TestSearchWithRerank verifies that when Rerank=true and a reranker is
// configured, the reranker is called with topK=K and the results have
// reranker scores.
func TestSearchWithRerank(t *testing.T) {
	p := &mockProvider{}
	reranker := &mockReranker{
		hits: []rag.RerankHit{
			{Index: 2, Score: 0.99},
			{Index: 0, Score: 0.85},
			{Index: 1, Score: 0.70},
		},
	}
	svc := NewService(p, reranker, nil)

	results, err := svc.Search(context.Background(), "proj", "query", SearchOpts{
		K: 3, Recall: 10, Rerank: true,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Verify results have reranker scores, not semantic scores
	if results[0].Score != 0.99 {
		t.Errorf("expected score 0.99 from reranker, got %f", results[0].Score)
	}

	// Verify reranker was called with correct topK
	reranker.mu.Lock()
	defer reranker.mu.Unlock()
	if reranker.calls != 1 {
		t.Errorf("expected 1 reranker call, got %d", reranker.calls)
	}
	if reranker.lastTopK != 3 {
		t.Errorf("expected topK=3, got %d", reranker.lastTopK)
	}
	if len(reranker.lastDocs) != 10 {
		t.Errorf("expected 10 docs sent to reranker, got %d", len(reranker.lastDocs))
	}
}

// TestSearchRerankClampsToK verifies that if the reranker returns more hits
// than K (e.g. the rerank API ignores top_k), Search still truncates the
// final result set to K defensively.
func TestSearchRerankClampsToK(t *testing.T) {
	p := &mockProvider{}
	reranker := &mockReranker{
		hits: []rag.RerankHit{
			{Index: 0, Score: 0.99},
			{Index: 1, Score: 0.95},
			{Index: 2, Score: 0.90},
			{Index: 3, Score: 0.85},
			{Index: 4, Score: 0.80},
		},
	}
	svc := NewService(p, reranker, nil)

	results, err := svc.Search(context.Background(), "proj", "query", SearchOpts{
		K: 3, Recall: 10, Rerank: true,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected results clamped to K=3, got %d", len(results))
	}
}

// TestSearchRecordsMetrics verifies that Search records a query into the
// in-memory metrics and that MetricsSnapshot reflects it.
func TestSearchRecordsMetrics(t *testing.T) {
	p := &mockProvider{}
	svc := NewService(p, nil, nil)
	svc.SetBackend("qdrant")

	snap0 := svc.MetricsSnapshot(context.Background())
	if snap0.QueryCount != 0 || snap0.LastQuery != nil {
		t.Fatalf("before search: count=%d lastQuery=%v, want 0/nil", snap0.QueryCount, snap0.LastQuery)
	}
	if snap0.Backend != "qdrant" {
		t.Errorf("backend = %q, want qdrant", snap0.Backend)
	}
	if snap0.Persistent {
		t.Error("mock provider must not be Persistent")
	}

	if _, err := svc.Search(context.Background(), "proj", "query", SearchOpts{K: 3, Recall: 10}); err != nil {
		t.Fatalf("Search error: %v", err)
	}

	snap := svc.MetricsSnapshot(context.Background())
	if snap.QueryCount != 1 {
		t.Errorf("query_count = %d, want 1", snap.QueryCount)
	}
	if snap.LastQuery == nil {
		t.Fatal("last_query should be populated after a search")
	}
	if snap.LastQuery.Results != 3 {
		t.Errorf("last_query.Results = %d, want 3", snap.LastQuery.Results)
	}
}

// TestSearchCompressDedup verifies that Compress drops near-duplicate results
// (identical content_hash or identical content) while preserving order.
func TestSearchCompressDedup(t *testing.T) {
	p := &mockProvider{results: []rag.Result{
		{ID: "1", Content: "alpha", Score: 0.9, Meta: map[string]string{"content_hash": "h1"}},
		{ID: "2", Content: "beta", Score: 0.8, Meta: map[string]string{"content_hash": "h1"}}, // dup hash
		{ID: "3", Content: "gamma", Score: 0.7, Meta: map[string]string{"content_hash": "h2"}},
		{ID: "4", Content: "gamma", Score: 0.6, Meta: map[string]string{"content_hash": "h3"}}, // dup content
		{ID: "5", Content: "delta", Score: 0.5, Meta: map[string]string{"content_hash": "h4"}},
	}}
	svc := NewService(p, nil, nil)

	// Without compress: all 5 (K high enough).
	plain, err := svc.Search(context.Background(), "proj", "q", SearchOpts{K: 10, Recall: 10})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(plain) != 5 {
		t.Fatalf("without compress: got %d, want 5", len(plain))
	}

	// With compress: drop id 2 (dup hash h1) and id 4 (dup content "gamma") → 3 left.
	compressed, err := svc.Search(context.Background(), "proj", "q", SearchOpts{K: 10, Recall: 10, Compress: true})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(compressed) != 3 {
		t.Fatalf("with compress: got %d, want 3", len(compressed))
	}
	wantIDs := []string{"1", "3", "5"}
	for i, r := range compressed {
		if r.ID != wantIDs[i] {
			t.Errorf("compressed[%d].ID = %q, want %q (order must be preserved)", i, r.ID, wantIDs[i])
		}
	}
}

// TestSearchRerankerErrorFallback verifies that if the reranker returns an
// error, Search falls back to semantic order (no error propagated).
func TestSearchRerankerErrorFallback(t *testing.T) {
	p := &mockProvider{}
	reranker := &mockReranker{
		rerankErr: errors.New("reranker unavailable"),
	}
	svc := NewService(p, reranker, nil)

	results, err := svc.Search(context.Background(), "proj", "query", SearchOpts{
		K: 5, Recall: 10, Rerank: true,
	})
	if err != nil {
		t.Fatalf("Search should not propagate reranker error: %v", err)
	}

	if len(results) != 5 {
		t.Errorf("expected 5 results, got %d", len(results))
	}

	// Verify results are in semantic order (original order from mockProvider)
	if results[0].ID != "result-0" {
		t.Errorf("expected first result in semantic order (result-0), got %s", results[0].ID)
	}
}

// TestSearchNilRerankerIgnored verifies that when reranker is nil and
// Rerank=true, search returns semantic results without panicking.
func TestSearchNilRerankerIgnored(t *testing.T) {
	p := &mockProvider{}
	svc := NewService(p, nil, nil)

	results, err := svc.Search(context.Background(), "proj", "query", SearchOpts{
		K: 5, Recall: 10, Rerank: true,
	})
	if err != nil {
		t.Fatalf("Search should not error with nil reranker: %v", err)
	}

	if len(results) != 5 {
		t.Errorf("expected 5 results, got %d", len(results))
	}
}

// TestSearchProviderError verifies that provider errors are propagated.
func TestSearchProviderError(t *testing.T) {
	p := &mockProvider{
		searchErr: errors.New("provider unavailable"),
	}
	svc := NewService(p, nil, nil)

	_, err := svc.Search(context.Background(), "proj", "query", SearchOpts{})
	if err == nil {
		t.Fatal("expected error from provider failure")
	}
}

// TestSearchRecallZero verifies Recall=0 defaults to 40.
func TestSearchRecallZero(t *testing.T) {
	p := &mockProvider{}
	svc := NewService(p, nil, nil)

	_, err := svc.Search(context.Background(), "proj", "query", SearchOpts{K: 3})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.searchLimit != DefaultRecall {
		t.Errorf("expected recall limit=%d, got %d", DefaultRecall, p.searchLimit)
	}
}

// TestSearchRerankerCalledWithRecallCandidates verifies that the reranker
// receives exactly `recall` candidate documents.
func TestSearchRerankerCalledWithRecallCandidates(t *testing.T) {
	// Generate 40 candidate results so we can verify the reranker receives all of them
	results := make([]rag.Result, 40)
	for i := range results {
		results[i] = rag.Result{
			ID:      fmt.Sprintf("result-%d", i),
			Content: fmt.Sprintf("content-%d", i),
			Score:   float64(40-i) / 40.0,
		}
	}
	p := &mockProvider{results: results}
	reranker := &mockReranker{}
	svc := NewService(p, reranker, nil)

	_, err := svc.Search(context.Background(), "proj", "query", SearchOpts{
		K: 5, Recall: 40, Rerank: true,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	reranker.mu.Lock()
	defer reranker.mu.Unlock()
	if len(reranker.lastDocs) != 40 {
		t.Errorf("expected 40 candidate docs sent to reranker, got %d", len(reranker.lastDocs))
	}
}

// TestCreateProject delegates to provider.
func TestCreateProject(t *testing.T) {
	p := &mockProvider{}
	svc := NewService(p, nil, nil)

	err := svc.CreateProject(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("CreateProject error: %v", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.createCalls != 1 {
		t.Errorf("expected 1 CreateCollection call, got %d", p.createCalls)
	}
}

// TestDeleteProject delegates to provider.
func TestDeleteProject(t *testing.T) {
	p := &mockProvider{}
	svc := NewService(p, nil, nil)

	err := svc.DeleteProject(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("DeleteProject error: %v", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.deleteCalls != 1 {
		t.Errorf("expected 1 DeleteCollection call, got %d", p.deleteCalls)
	}
}

// TestListPoints delegates to provider.
func TestListPoints(t *testing.T) {
	p := &mockProvider{}
	svc := NewService(p, nil, nil)

	points, err := svc.ListPoints(context.Background(), "proj1", nil)
	if err != nil {
		t.Fatalf("ListPoints error: %v", err)
	}

	if len(points) != 2 {
		t.Errorf("expected 2 points, got %d", len(points))
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.listPointsCalls != 1 {
		t.Errorf("expected 1 ListPoints call, got %d", p.listPointsCalls)
	}
}

// TestDeletePoints delegates to provider.
func TestDeletePoints(t *testing.T) {
	p := &mockProvider{}
	svc := NewService(p, nil, nil)

	err := svc.DeletePoints(context.Background(), "proj1", []string{"p1", "p2"})
	if err != nil {
		t.Fatalf("DeletePoints error: %v", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.deletePtCalls != 1 {
		t.Errorf("expected 1 DeletePoints call, got %d", p.deletePtCalls)
	}
}

// TestListProjects verifies that ListProjects returns project stats.
func TestListProjects(t *testing.T) {
	p := &mockProvider{
		listedProjectIDs: []string{"proj1", "proj2"},
	}
	svc := NewService(p, nil, nil)

	stats, err := svc.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects error: %v", err)
	}

	if len(stats) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(stats))
	}

	if stats[0].ProjectID != "proj1" {
		t.Errorf("expected first project 'proj1', got %s", stats[0].ProjectID)
	}
	if stats[0].ChunkCount != 2 {
		t.Errorf("expected 2 chunks for proj1, got %d", stats[0].ChunkCount)
	}
}

// TestIndexDocuments delegates to provider.Index.
func TestIndexDocuments(t *testing.T) {
	p := &mockProvider{}
	svc := NewService(p, nil, nil)

	docs := []rag.Document{
		{ID: "d1", Content: "hello", Meta: map[string]string{"k": "v"}},
	}
	err := svc.IndexDocuments(context.Background(), "proj1", docs)
	if err != nil {
		t.Fatalf("IndexDocuments error: %v", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.indexCalls != 1 {
		t.Errorf("expected 1 Index call, got %d", p.indexCalls)
	}
}

// TestRetrieveContext verifies that RetrieveContext returns a concatenated
// string and raw results.
func TestRetrieveContext(t *testing.T) {
	p := &mockProvider{}
	svc := NewService(p, nil, nil)

	ctxStr, results, err := svc.RetrieveContext(context.Background(), "proj1", "query", 3)
	if err != nil {
		t.Fatalf("RetrieveContext error: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
	if ctxStr == "" {
		t.Error("expected non-empty context string")
	}
}

// TestRetrieveContextDefaultLimit verifies that limit=0 defaults to 5.
func TestRetrieveContextDefaultLimit(t *testing.T) {
	p := &mockProvider{}
	svc := NewService(p, nil, nil)

	_, results, err := svc.RetrieveContext(context.Background(), "proj1", "query", 0)
	if err != nil {
		t.Fatalf("RetrieveContext error: %v", err)
	}

	if len(results) != DefaultK {
		t.Errorf("expected %d results, got %d", DefaultK, len(results))
	}
}

// --- EventBus tests ---

func TestEventBusSubscribeUnsubscribe(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe()

	// Verify channel is registered
	bus.mu.Lock()
	count := len(bus.subscribers)
	bus.mu.Unlock()
	if count != 1 {
		t.Errorf("expected 1 subscriber, got %d", count)
	}

	bus.Unsubscribe(ch)

	bus.mu.Lock()
	count = len(bus.subscribers)
	bus.mu.Unlock()
	if count != 0 {
		t.Errorf("expected 0 subscribers after unsubscribe, got %d", count)
	}
}

func TestEventBusPublish(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	ev := Event{Type: "test_event", Data: map[string]any{"key": "value"}}
	bus.Publish(ev)

	select {
	case received := <-ch:
		if received.Type != "test_event" {
			t.Errorf("expected type 'test_event', got %s", received.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive event within timeout")
	}
}

func TestEventBusMultipleSubscribers(t *testing.T) {
	bus := NewEventBus()
	ch1 := bus.Subscribe()
	ch2 := bus.Subscribe()
	defer bus.Unsubscribe(ch1)
	defer bus.Unsubscribe(ch2)

	bus.Publish(Event{Type: "broadcast"})

	for i, ch := range []chan Event{ch1, ch2} {
		select {
		case <-ch:
			// OK
		case <-time.After(time.Second):
			t.Errorf("subscriber %d did not receive event", i)
		}
	}
}

func TestEventBusNonBlockingOnFullBuffer(t *testing.T) {
	bus := NewEventBus()
	// Create a subscriber with a small buffer (64 is default)
	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	// Publish more events than the buffer can hold
	for i := 0; i < 200; i++ {
		bus.Publish(Event{Type: "flood"})
	}

	// Verify Publish did not block (we got here)
	// Drain some events to verify at least some were delivered
	received := 0
drain:
	for {
		select {
		case <-ch:
			received++
		default:
			break drain
		}
	}
	if received == 0 {
		t.Error("expected at least some events to be delivered")
	}
}

// TestServiceEventsAccessor verifies that Service.Events() returns the EventBus.
func TestServiceEventsAccessor(t *testing.T) {
	svc := NewService(&mockProvider{}, nil, nil)
	bus := svc.Events()
	if bus == nil {
		t.Fatal("Events() returned nil")
	}
	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	// Trigger an event via CreateProject
	_ = svc.CreateProject(context.Background(), "proj1")

	select {
	case ev := <-ch:
		if ev.Type != "project_created" {
			t.Errorf("expected event type 'project_created', got %s", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive project_created event")
	}
}

// TestSearchPublishesQueryEvent verifies that Search publishes a query_executed event.
func TestSearchPublishesQueryEvent(t *testing.T) {
	p := &mockProvider{}
	svc := NewService(p, nil, nil)
	ch := svc.Events().Subscribe()
	defer svc.Events().Unsubscribe(ch)

	_, _ = svc.Search(context.Background(), "proj", "query", SearchOpts{K: 3, Recall: 5})

	select {
	case ev := <-ch:
		if ev.Type != "query_executed" {
			t.Errorf("expected event type 'query_executed', got %s", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive query_executed event")
	}
}

// TestCreateProjectErrorPropagated verifies that provider errors are returned.
func TestCreateProjectErrorPropagated(t *testing.T) {
	p := &mockProvider{createErr: errors.New("collection error")}
	svc := NewService(p, nil, nil)

	err := svc.CreateProject(context.Background(), "proj1")
	if err == nil {
		t.Fatal("expected error from CreateCollection")
	}
}

// TestDeleteProjectErrorPropagated verifies that provider errors are returned.
func TestDeleteProjectErrorPropagated(t *testing.T) {
	p := &mockProvider{deleteErr: errors.New("delete error")}
	svc := NewService(p, nil, nil)

	err := svc.DeleteProject(context.Background(), "proj1")
	if err == nil {
		t.Fatal("expected error from DeleteCollection")
	}
}

// TestIndexProject verifies that Service.IndexProject delegates to the indexer
// and returns a SyncResult with Indexed > 0 and FilesScanned > 0 when given a
// temporary directory containing at least one indexable file.
//
// This satisfies VAL-FND-015: core.Service.IndexProject delegates to indexer.
func TestIndexProject(t *testing.T) {
	// Create a temporary directory with a test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "hello.go")
	testContent := "package main\n\nfunc main() {\n    println(\"hello world\")\n}\n"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	p := &mockProvider{}
	svc := NewService(p, nil, nil)

	result, err := svc.IndexProject(context.Background(), "test-proj", tmpDir)
	if err != nil {
		t.Fatalf("IndexProject returned error: %v", err)
	}

	if result == nil {
		t.Fatal("IndexProject returned nil result")
	}

	if result.Indexed <= 0 {
		t.Errorf("expected Indexed > 0, got %d", result.Indexed)
	}

	if result.FilesScanned <= 0 {
		t.Errorf("expected FilesScanned > 0, got %d", result.FilesScanned)
	}

	// Verify provider.Index was called (indexer delegates to provider)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.indexCalls == 0 {
		t.Error("expected provider.Index to be called at least once")
	}
}

// TestSearchHybridUsesHybridSearcher verifies that when opts.Hybrid=true and
// the provider implements HybridSearcher, Search calls SemanticSearchHybrid
// instead of SemanticSearch.
// This satisfies VAL-RAG-010 (hybrid path used when hybrid=true).
func TestSearchHybridUsesHybridSearcher(t *testing.T) {
	p := &mockProvider{}
	svc := NewService(p, nil, nil)

	_, err := svc.Search(context.Background(), "proj", "query", SearchOpts{
		K: 5, Recall: 10, Hybrid: true,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.hybridCalls == 0 {
		t.Error("expected SemanticSearchHybrid to be called when Hybrid=true")
	}
	if p.searchCalls != 0 {
		t.Error("SemanticSearch should NOT be called when Hybrid=true and provider implements HybridSearcher")
	}
}

// TestSearchHybridFalseUsesDenseOnly verifies that when opts.Hybrid=false,
// Search calls SemanticSearch (dense-only), not SemanticSearchHybrid.
func TestSearchHybridFalseUsesDenseOnly(t *testing.T) {
	p := &mockProvider{}
	svc := NewService(p, nil, nil)

	_, err := svc.Search(context.Background(), "proj", "query", SearchOpts{
		K: 5, Recall: 10, Hybrid: false,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.searchCalls == 0 {
		t.Error("expected SemanticSearch to be called when Hybrid=false")
	}
	if p.hybridCalls != 0 {
		t.Error("SemanticSearchHybrid should NOT be called when Hybrid=false")
	}
}

// TestSearchHybridWithRerank verifies that hybrid search + rerank works
// end-to-end: hybrid recall -> rerank -> top-K.
// This satisfies VAL-RAG-015.
func TestSearchHybridWithRerank(t *testing.T) {
	// Generate 10 candidate results
	results := make([]rag.Result, 10)
	for i := range results {
		results[i] = rag.Result{
			ID:      fmt.Sprintf("hybrid-%d", i),
			Content: fmt.Sprintf("hybrid content %d", i),
			Score:   float64(10-i) / 10.0,
		}
	}
	p := &mockProvider{results: results}
	reranker := &mockReranker{
		hits: []rag.RerankHit{
			{Index: 3, Score: 0.95},
			{Index: 1, Score: 0.80},
			{Index: 0, Score: 0.70},
		},
	}
	svc := NewService(p, reranker, nil)

	out, err := svc.Search(context.Background(), "proj", "query", SearchOpts{
		K: 3, Recall: 10, Hybrid: true, Rerank: true,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	if len(out) != 3 {
		t.Fatalf("expected 3 results, got %d", len(out))
	}

	// Verify hybrid was used for retrieval
	p.mu.Lock()
	hybridUsed := p.hybridCalls > 0
	p.mu.Unlock()
	if !hybridUsed {
		t.Error("expected SemanticSearchHybrid to be called for hybrid retrieval")
	}

	// Verify reranker was called
	reranker.mu.Lock()
	rerankCalled := reranker.calls > 0
	reranker.mu.Unlock()
	if !rerankCalled {
		t.Error("expected reranker to be called")
	}

	// Verify results have reranker scores
	if out[0].Score != 0.95 {
		t.Errorf("expected first result score 0.95 from reranker, got %f", out[0].Score)
	}
}

// TestSearchHybridProviderNotHybridSearcher verifies that when the provider
// does NOT implement HybridSearcher and Hybrid=true, Search falls back to
// dense-only SemanticSearch without error.
func TestSearchHybridProviderNotHybridSearcher(t *testing.T) {
	// mockProviderNonHybrid implements Provider but NOT HybridSearcher
	p := &mockProviderNonHybrid{}
	svc := NewService(p, nil, nil)

	results, err := svc.Search(context.Background(), "proj", "query", SearchOpts{
		K: 5, Recall: 10, Hybrid: true,
	})
	if err != nil {
		t.Fatalf("Search should not error when provider doesn't implement HybridSearcher: %v", err)
	}

	if len(results) != 5 {
		t.Errorf("expected 5 results, got %d", len(results))
	}

	if p.searchCalls == 0 {
		t.Error("expected SemanticSearch to be called as fallback")
	}
}

// mockProviderNonHybrid implements Provider but NOT HybridSearcher.
type mockProviderNonHybrid struct {
	searchCalls int
}

func (m *mockProviderNonHybrid) CreateCollection(ctx context.Context, projectID string) error { return nil }
func (m *mockProviderNonHybrid) DeleteCollection(ctx context.Context, projectID string) error { return nil }
func (m *mockProviderNonHybrid) Index(ctx context.Context, projectID string, docs []rag.Document) error {
	return nil
}
func (m *mockProviderNonHybrid) SemanticSearch(ctx context.Context, projectID, query string, limit int) ([]rag.Result, error) {
	m.searchCalls++
	results := make([]rag.Result, 10)
	for i := range results {
		results[i] = rag.Result{
			ID:      fmt.Sprintf("result-%d", i),
			Content: fmt.Sprintf("content-%d", i),
			Score:   float64(10-i) / 10.0,
		}
	}
	return results, nil
}
func (m *mockProviderNonHybrid) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, nil
}
func (m *mockProviderNonHybrid) DeletePoints(ctx context.Context, projectID string, pointIDs []string) error {
	return nil
}
func (m *mockProviderNonHybrid) ListPointIDs(ctx context.Context, projectID string, metaFilter map[string]string) ([]string, error) {
	return nil, nil
}
func (m *mockProviderNonHybrid) ListPoints(ctx context.Context, projectID string, metaFilter map[string]string) ([]rag.PointInfo, error) {
	return nil, nil
}
func (m *mockProviderNonHybrid) Close() error { return nil }

var _ rag.Provider = (*mockProviderNonHybrid)(nil)
