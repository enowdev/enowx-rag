package core

import (
	"sort"
	"sync"
	"sync/atomic"
)

// latencyRingSize is the number of recent query latencies retained in memory
// for percentile computation. A ring buffer bounds memory regardless of uptime.
const latencyRingSize = 512

// QueryComposition captures how a query's results were assembled. Dense/lexical
// counts are populated only when the hybrid pgvector path runs and the provider
// projects per-result origin; otherwise they are zero — an honest fallback for
// dense-only or non-pgvector backends.
type QueryComposition struct {
	Hybrid       bool `json:"hybrid"`
	Reranked     bool `json:"reranked"`
	Candidates   int  `json:"candidates"`
	Results      int  `json:"results"`
	DenseCount   int  `json:"dense_count"`   // results that appeared in the dense ranking
	LexicalCount int  `json:"lexical_count"` // results that appeared in the lexical ranking
	RerankMoved  int  `json:"rerank_moved"`  // results whose position changed after rerank
}

// Metrics holds in-memory, always-on query metrics. Safe for concurrent use.
// Latency samples live in a fixed-size ring buffer; token counts are read
// on-demand from the provider/reranker (see Service.MetricsSnapshot).
type Metrics struct {
	queries atomic.Int64

	mu       sync.Mutex
	lat      [latencyRingSize]float64 // ms, ring buffer
	latCount int                      // number of filled slots (<= latencyRingSize)
	latHead  int                      // next write index
	lastComp QueryComposition
	haveComp bool
}

// NewMetrics returns an empty Metrics ready for concurrent use.
func NewMetrics() *Metrics { return &Metrics{} }

// RecordQuery records one query's retrieval latency (ms) and composition.
// comp may be a zero value for backends without composition data.
func (m *Metrics) RecordQuery(latMs float64, comp QueryComposition) {
	m.queries.Add(1)
	m.mu.Lock()
	m.lat[m.latHead] = latMs
	m.latHead = (m.latHead + 1) % latencyRingSize
	if m.latCount < latencyRingSize {
		m.latCount++
	}
	m.lastComp = comp
	m.haveComp = true
	m.mu.Unlock()
}

// snapshot returns latency aggregates over the retained samples plus the last
// recorded composition. When no queries have run, all values are zero and
// haveComp is false.
func (m *Metrics) snapshot() (avg, p50, p95 float64, count int64, comp QueryComposition, haveComp bool) {
	count = m.queries.Load()

	m.mu.Lock()
	n := m.latCount
	samples := make([]float64, n)
	copy(samples, m.lat[:n])
	comp = m.lastComp
	haveComp = m.haveComp
	m.mu.Unlock()

	if n == 0 {
		return 0, 0, 0, count, comp, haveComp
	}

	var sum float64
	for _, v := range samples {
		sum += v
	}
	avg = sum / float64(n)

	sort.Float64s(samples)
	p50 = percentile(samples, 50)
	p95 = percentile(samples, 95)
	return avg, p50, p95, count, comp, haveComp
}

// percentile returns the p-th percentile (0-100) of a pre-sorted slice using
// nearest-rank. sorted must be non-empty and ascending.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}
	rank := int((p/100)*float64(len(sorted)-1) + 0.5)
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}
