// Package core provides the service layer shared by MCP stdio and HTTP API.
// It wraps a rag.Provider, an optional rag.Reranker, and an *indexer.Indexer
// behind a single Service struct with methods that both transport layers call.
package core

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/enowdev/enowx-rag/pkg/indexer"
	"github.com/enowdev/enowx-rag/pkg/rag"
)

// DefaultK is the default number of final results returned by Search.
const DefaultK = 5

// DefaultRecall is the default number of candidates retrieved before rerank.
const DefaultRecall = 40

// SearchOpts controls the behaviour of Service.Search.
type SearchOpts struct {
	K        int  // final top-K results (default 5)
	Recall   int  // retrieval recall before rerank (default 40)
	Hybrid   bool // use dense+lexical RRF (provider must support it)
	Rerank   bool // use reranker if configured
	Compress bool // drop near-duplicate results (same content_hash / identical content)
}

// ProjectStat holds per-project statistics returned by ListProjects.
type ProjectStat struct {
	ProjectID  string `json:"project_id"`
	ChunkCount int    `json:"chunk_count"`
}

// Event is a single SSE event published by the EventBus.
type Event struct {
	Type      string    `json:"type"`              // e.g. "index_started", "query_executed"
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data,omitempty"`
}

// EventBus is a simple pub/sub mechanism for SSE events. Subscribers receive
// events on a buffered channel; the bus is safe for concurrent use.
type EventBus struct {
	mu          sync.Mutex
	subscribers map[chan Event]struct{}
}

// NewEventBus creates a new EventBus.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[chan Event]struct{}),
	}
}

// Subscribe returns a buffered channel on which events are delivered.
// The caller must call Unsubscribe with the returned channel to stop
// receiving and allow the bus to clean up.
func (b *EventBus) Subscribe() chan Event {
	ch := make(chan Event, 64)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel and closes it.
func (b *EventBus) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	delete(b.subscribers, ch)
	b.mu.Unlock()
	// Close the channel so any range-loop consumer exits.
	close(ch)
}

// Publish sends an event to all current subscribers. If a subscriber's
// channel buffer is full the event is dropped for that subscriber (non-blocking).
func (b *EventBus) Publish(ev Event) {
	b.mu.Lock()
	subs := make([]chan Event, 0, len(b.subscribers))
	for ch := range b.subscribers {
		subs = append(subs, ch)
	}
	b.mu.Unlock()

	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	for _, ch := range subs {
		select {
		case ch <- ev:
		default: // drop if buffer full
		}
	}
}

// Service wraps a provider, an optional reranker, and an indexer behind a
// single API used by both the MCP stdio handlers and the HTTP API layer.
type Service struct {
	provider   rag.Provider
	reranker   rag.Reranker // may be nil
	indexer    *indexer.Indexer
	events     *EventBus
	embedModel string // optional: set by main.go for stats endpoint
	metrics    *Metrics
	backend    string // vector store name (e.g. "qdrant"), set by main.go
}

// MetricsStore is an optional interface implemented only by backends that can
// persist query metrics durably (currently pgvector). Service type-asserts the
// provider for this interface; when absent, metrics are in-memory only and the
// snapshot reports Persistent=false — an honest capability signal.
type MetricsStore interface {
	PersistQueryMetric(ctx context.Context, latencyMs float64, comp QueryComposition) error
}

// MetricsSnapshot is the JSON-serializable metrics response returned by
// Service.MetricsSnapshot and the GET /api/metrics endpoint.
type MetricsSnapshot struct {
	QueryCount   int64             `json:"query_count"`
	AvgLatencyMs float64           `json:"avg_latency_ms"`
	P50LatencyMs float64           `json:"p50_latency_ms"`
	P95LatencyMs float64           `json:"p95_latency_ms"`
	TokensTotal  int64             `json:"tokens_total"`
	TokensEmbed  int64             `json:"tokens_embed"`
	TokensRerank int64             `json:"tokens_rerank"`
	Persistent   bool              `json:"persistent"` // true iff backend implements MetricsStore
	Backend      string            `json:"backend"`
	LastQuery    *QueryComposition `json:"last_query,omitempty"` // nil until first query
}

// NewService creates a Service from the given components. reranker may be nil.
// indexer may be nil; if so, IndexProject will create one with default chunk size.
func NewService(provider rag.Provider, reranker rag.Reranker, idx *indexer.Indexer) *Service {
	svc := &Service{
		provider: provider,
		reranker: reranker,
		indexer:  idx,
		events:   NewEventBus(),
		metrics:  NewMetrics(),
	}
	return svc
}

// Events returns the EventBus for SSE publishing/subscribing.
func (s *Service) Events() *EventBus {
	return s.events
}

// Provider returns the underlying rag.Provider. This is intended for
// advanced use cases where the caller needs direct provider access.
func (s *Service) Provider() rag.Provider {
	return s.provider
}

// SetEmbedModel sets the embedding model name used by the service. This is
// used by the HTTP API stats endpoint to report the active embedding model.
func (s *Service) SetEmbedModel(model string) {
	s.embedModel = model
}

// EmbedModel returns the embedding model name, or "unknown" if not set.
func (s *Service) EmbedModel() string {
	if s.embedModel != "" {
		return s.embedModel
	}
	return "unknown"
}

// SetBackend records the active vector store name (e.g. "qdrant", "pgvector")
// so the metrics snapshot can report it honestly.
func (s *Service) SetBackend(name string) {
	s.backend = name
}

// MetricsSnapshot returns a point-in-time view of query metrics: in-memory
// latency aggregates and query count, plus token usage read from the provider
// and reranker when they implement rag.TokenCounter. Persistent reflects
// whether the backend can store metrics durably (implements MetricsStore).
func (s *Service) MetricsSnapshot(ctx context.Context) MetricsSnapshot {
	avg, p50, p95, count, comp, haveComp := s.metrics.snapshot()

	var embedTok, rerankTok int64
	if tc, ok := s.provider.(rag.TokenCounter); ok {
		embedTok = tc.TokensUsed()
	}
	if tc, ok := s.reranker.(rag.TokenCounter); ok {
		rerankTok = tc.TokensUsed()
	}
	_, persistent := s.provider.(MetricsStore)

	snap := MetricsSnapshot{
		QueryCount:   count,
		AvgLatencyMs: avg,
		P50LatencyMs: p50,
		P95LatencyMs: p95,
		TokensEmbed:  embedTok,
		TokensRerank: rerankTok,
		TokensTotal:  embedTok + rerankTok,
		Persistent:   persistent,
		Backend:      s.backend,
	}
	if haveComp {
		c := comp
		snap.LastQuery = &c
	}
	return snap
}

// retrieveCandidates fetches recall candidates from the provider. When hybrid
// is true and the provider implements HybridSearcher, it uses the hybrid
// search path (dense + lexical RRF). Otherwise it falls back to dense-only
// SemanticSearch.
func (s *Service) retrieveCandidates(ctx context.Context, projectID, query string, recall int, hybrid bool) ([]rag.Result, error) {
	if hybrid {
		if hs, ok := s.provider.(rag.HybridSearcher); ok {
			return hs.SemanticSearchHybrid(ctx, projectID, query, recall)
		}
	}
	return s.provider.SemanticSearch(ctx, projectID, query, recall)
}

// Search performs a semantic search with optional reranking.
//
// Flow:
//  1. Retrieve `recall` candidates via provider.SemanticSearch.
//  2. If rerank is requested and a reranker is configured, rerank candidates
//     and return the top-K results with updated scores.
//  3. If the reranker fails, fall back to semantic order truncated to K.
//  4. If no rerank, truncate to K.
//
// Defaults: K=5, Recall=40.
func (s *Service) Search(ctx context.Context, projectID, query string, opts SearchOpts) ([]rag.Result, error) {
	k := opts.K
	if k <= 0 {
		k = DefaultK
	}
	recall := opts.Recall
	if recall <= 0 {
		recall = DefaultRecall
	}

	// Measure retrieval latency around the provider call only (excludes rerank,
	// which is measured separately by the reranker's own metrics if needed).
	t0 := time.Now()
	cands, err := s.retrieveCandidates(ctx, projectID, query, recall, opts.Hybrid)
	latencyMs := float64(time.Since(t0).Microseconds()) / 1000.0
	if err != nil {
		return nil, err
	}

	hybridUsed := opts.Hybrid && s.providerSupportsHybrid()
	comp := QueryComposition{
		Hybrid:     hybridUsed,
		Candidates: len(cands),
	}
	// Compute dense/lexical composition from per-result origin when the hybrid
	// path populated it (pgvector). Non-hybrid or backends that don't project
	// origin leave these at zero — an honest fallback.
	for _, c := range cands {
		if c.InDense {
			comp.DenseCount++
		}
		if c.InLexical {
			comp.LexicalCount++
		}
	}

	// Record metrics once, at return, regardless of which branch produced out.
	out := cands
	defer func() {
		comp.Results = len(out)
		s.metrics.RecordQuery(latencyMs, comp)
		if ms, ok := s.provider.(MetricsStore); ok {
			// Persist asynchronously so query latency is unaffected. A copy of
			// comp is captured; failures are non-fatal.
			c := comp
			go func() { _ = ms.PersistQueryMetric(context.WithoutCancel(ctx), latencyMs, c) }()
		}
	}()

	// Publish a query event for SSE listeners.
	s.events.Publish(Event{
		Type:      "query_executed",
		Timestamp: time.Now(),
		Data: map[string]any{
			"project_id": projectID,
			"query":      query,
			"candidates": len(cands),
		},
	})

	if opts.Rerank && s.reranker != nil && len(cands) > 0 {
		docs := make([]string, len(cands))
		for i, c := range cands {
			docs[i] = c.Content
		}
		hits, rerr := s.reranker.Rerank(ctx, query, docs, k)
		if rerr == nil && len(hits) > 0 {
			reranked := make([]rag.Result, 0, len(hits))
			for _, h := range hits {
				if h.Index < 0 || h.Index >= len(cands) {
					continue
				}
				r := cands[h.Index]
				r.Score = h.Score
				reranked = append(reranked, r)
			}
			// Defensive clamp: don't rely on the reranker API honoring top_k.
			if len(reranked) > k {
				reranked = reranked[:k]
			}
			comp.Reranked = true
			comp.RerankMoved = rerankMoved(cands, reranked)
			out = maybeCompress(reranked, opts.Compress)
			return out, nil
		}
		// Reranker failed or returned empty: fall back to semantic order.
	}

	if len(cands) > k {
		cands = cands[:k]
	}
	out = maybeCompress(cands, opts.Compress)
	return out, nil
}

// providerSupportsHybrid reports whether the provider implements the hybrid
// search path, mirroring the type assertion in retrieveCandidates.
func (s *Service) providerSupportsHybrid() bool {
	_, ok := s.provider.(rag.HybridSearcher)
	return ok
}

// rerankMoved counts how many of the reranked results changed position relative
// to their original order in cands (by result ID).
func rerankMoved(cands, reranked []rag.Result) int {
	origPos := make(map[string]int, len(cands))
	for i, c := range cands {
		origPos[c.ID] = i
	}
	moved := 0
	for newPos, r := range reranked {
		if old, ok := origPos[r.ID]; ok && old != newPos {
			moved++
		}
	}
	return moved
}

// maybeCompress removes near-duplicate results when compress is true, preserving
// input order (which is already score-sorted). Two results are considered
// duplicates when they share a non-empty content_hash or have identical content.
func maybeCompress(results []rag.Result, compress bool) []rag.Result {
	if !compress || len(results) < 2 {
		return results
	}
	seenHash := make(map[string]struct{}, len(results))
	seenContent := make(map[string]struct{}, len(results))
	out := results[:0:0] // new backing array; don't mutate caller's slice
	for _, r := range results {
		hash := r.Meta["content_hash"]
		if hash != "" {
			if _, dup := seenHash[hash]; dup {
				continue
			}
			seenHash[hash] = struct{}{}
		}
		if _, dup := seenContent[r.Content]; dup {
			continue
		}
		seenContent[r.Content] = struct{}{}
		out = append(out, r)
	}
	return out
}

// IndexProject scans the given directory and indexes all code/text files
// into the project collection. Delegates to the indexer.
func (s *Service) IndexProject(ctx context.Context, projectID, dir string) (*indexer.SyncResult, error) {
	s.events.Publish(Event{
		Type:      "index_started",
		Timestamp: time.Now(),
		Data:      map[string]any{"project_id": projectID, "directory": dir},
	})

	// Ensure the collection exists before indexing. The MCP flow calls
	// CreateProject first, but the HTTP reindex endpoint indexes a project
	// directly, so a first-time index would otherwise fail with a missing
	// collection. CreateCollection is idempotent across providers.
	if err := s.provider.CreateCollection(ctx, projectID); err != nil {
		s.events.Publish(Event{
			Type:      "index_failed",
			Timestamp: time.Now(),
			Data:      map[string]any{"project_id": projectID, "error": err.Error()},
		})
		return nil, err
	}

	idx := s.indexer
	if idx == nil {
		idx = indexer.NewIndexer(s.provider, 1500)
	}

	result, err := idx.IndexProject(ctx, projectID, dir)
	if err != nil {
		s.events.Publish(Event{
			Type:      "index_failed",
			Timestamp: time.Now(),
			Data:      map[string]any{"project_id": projectID, "error": err.Error()},
		})
		return nil, err
	}

	s.events.Publish(Event{
		Type:      "index_completed",
		Timestamp: time.Now(),
		Data: map[string]any{
			"project_id":    projectID,
			"indexed":       result.Indexed,
			"deleted":       result.Deleted,
			"files_scanned": result.FilesScanned,
			"skipped":       result.Skipped,
		},
	})

	return result, nil
}

// ListProjects returns statistics for all known projects. It queries the
// provider's ListPoints for each project ID it can discover.
// Since the Provider interface doesn't have a "list collections" method,
// this method uses a best-effort approach by checking known project IDs
// via the provider. The caller may pass project IDs to check.
func (s *Service) ListProjects(ctx context.Context) ([]ProjectStat, error) {
	// The Provider interface does not expose a "list collections" method.
	// We rely on a helper that may be implemented by specific providers.
	// For now, we use a type assertion to check if the provider supports
	// listing project IDs.
	lister, ok := s.provider.(ProjectLister)
	if ok {
		ids, err := lister.ListProjectIDs(ctx)
		if err != nil {
			return nil, err
		}
		stats := make([]ProjectStat, 0, len(ids))
		for _, id := range ids {
			points, err := s.provider.ListPoints(ctx, id, nil)
			if err != nil {
				continue
			}
			stats = append(stats, ProjectStat{
				ProjectID:  id,
				ChunkCount: len(points),
			})
		}
		return stats, nil
	}
	return []ProjectStat{}, nil
}

// ProjectLister is an optional interface that providers may implement to
// support listing all project IDs. This allows ListProjects to enumerate
// collections without prior knowledge.
type ProjectLister interface {
	ListProjectIDs(ctx context.Context) ([]string, error)
}

// CreateProject creates a new RAG collection for the given project.
func (s *Service) CreateProject(ctx context.Context, projectID string) error {
	if err := s.provider.CreateCollection(ctx, projectID); err != nil {
		return err
	}
	s.events.Publish(Event{
		Type:      "project_created",
		Timestamp: time.Now(),
		Data:      map[string]any{"project_id": projectID},
	})
	return nil
}

// DeleteProject deletes the project collection and all its indexed memory.
func (s *Service) DeleteProject(ctx context.Context, projectID string) error {
	if err := s.provider.DeleteCollection(ctx, projectID); err != nil {
		return err
	}
	s.events.Publish(Event{
		Type:      "project_deleted",
		Timestamp: time.Now(),
		Data:      map[string]any{"project_id": projectID},
	})
	return nil
}

// ListPoints returns all points (ID + metadata) in the project collection,
// optionally filtered by metadata.
func (s *Service) ListPoints(ctx context.Context, projectID string, metaFilter map[string]string) ([]rag.PointInfo, error) {
	return s.provider.ListPoints(ctx, projectID, metaFilter)
}

// ProjectExists checks whether a project with the given ID has any indexed
// data. It first tries the ProjectLister interface (if the provider supports
// it) for an O(1) set lookup; otherwise it falls back to ListPoints which
// works for all providers. Returns false if the project does not exist or
// an error occurs during the check.
func (s *Service) ProjectExists(ctx context.Context, projectID string) bool {
	// Fast path: if the provider can list project IDs, check membership.
	if lister, ok := s.provider.(ProjectLister); ok {
		ids, err := lister.ListProjectIDs(ctx)
		if err != nil {
			return false
		}
		for _, id := range ids {
			if id == projectID {
				return true
			}
		}
		return false
	}
	// Fallback: query ListPoints for the project. If it returns nil or an
	// error, the project doesn't exist.
	points, err := s.provider.ListPoints(ctx, projectID, nil)
	if err != nil {
		return false
	}
	return points != nil
}

// DeletePoints removes specific points by ID from the project collection.
func (s *Service) DeletePoints(ctx context.Context, projectID string, pointIDs []string) error {
	if err := s.provider.DeletePoints(ctx, projectID, pointIDs); err != nil {
		return err
	}
	s.events.Publish(Event{
		Type:      "points_deleted",
		Timestamp: time.Now(),
		Data:      map[string]any{"project_id": projectID, "count": len(pointIDs)},
	})
	return nil
}

// IndexDocuments indexes a batch of documents into the project collection
// directly (without scanning a directory). This is used by the rag_index MCP tool.
func (s *Service) IndexDocuments(ctx context.Context, projectID string, docs []rag.Document) error {
	if err := s.provider.Index(ctx, projectID, docs); err != nil {
		return fmt.Errorf("index documents: %w", err)
	}
	s.events.Publish(Event{
		Type:      "documents_indexed",
		Timestamp: time.Now(),
		Data:      map[string]any{"project_id": projectID, "count": len(docs)},
	})
	return nil
}

// RetrieveContext performs a semantic search and returns a concatenated
// context string suitable for feeding into an LLM, along with the raw results.
func (s *Service) RetrieveContext(ctx context.Context, projectID, query string, limit int) (string, []rag.Result, error) {
	if limit <= 0 {
		limit = DefaultK
	}
	results, err := s.Search(ctx, projectID, query, SearchOpts{K: limit, Recall: limit})
	if err != nil {
		return "", nil, err
	}
	parts := make([]string, 0, len(results))
	for _, r := range results {
		parts = append(parts, fmt.Sprintf("[score %.3f] %s", r.Score, r.Content))
	}
	return strings.Join(parts, "\n\n"), results, nil
}

// joinStrings is retained for backward compatibility but now delegates to
// strings.Join. This avoids importing strings just for one call was the
// original rationale, but using the stdlib is cleaner and more maintainable.
func joinStrings(parts []string, sep string) string {
	return strings.Join(parts, sep)
}
