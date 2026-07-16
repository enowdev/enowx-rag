package core

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *SQLiteMetricsStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "metrics.db")
	store, err := NewSQLiteMetricsStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteMetricsStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// TestSQLiteMetricsEmpty verifies an empty store reports zeroes.
func TestSQLiteMetricsEmpty(t *testing.T) {
	store := newTestStore(t)
	sum, err := store.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if sum.QueryCount != 0 || sum.AvgLatencyMs != 0 || sum.P50LatencyMs != 0 {
		t.Errorf("empty summary = %+v, want all zero", sum)
	}
}

// TestSQLiteMetricsPersistAndSummary verifies inserts are aggregated correctly.
func TestSQLiteMetricsPersistAndSummary(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for _, lat := range []float64{10, 20, 30, 40, 100} {
		if err := store.PersistQueryMetric(ctx, lat, QueryComposition{Results: 3}); err != nil {
			t.Fatalf("PersistQueryMetric: %v", err)
		}
	}
	sum, err := store.Summary(ctx)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if sum.QueryCount != 5 {
		t.Errorf("QueryCount = %d, want 5", sum.QueryCount)
	}
	if sum.AvgLatencyMs != 40 { // (10+20+30+40+100)/5
		t.Errorf("AvgLatencyMs = %v, want 40", sum.AvgLatencyMs)
	}
	if sum.P50LatencyMs != 30 {
		t.Errorf("P50LatencyMs = %v, want 30", sum.P50LatencyMs)
	}
	if sum.P95LatencyMs != 100 {
		t.Errorf("P95LatencyMs = %v, want 100", sum.P95LatencyMs)
	}
}

// TestSQLiteMetricsDurable verifies persisted metrics survive reopening the DB
// (the whole point of durable persistence).
func TestSQLiteMetricsDurable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.db")
	ctx := context.Background()

	store1, err := NewSQLiteMetricsStore(path)
	if err != nil {
		t.Fatalf("open 1: %v", err)
	}
	if err := store1.PersistQueryMetric(ctx, 50, QueryComposition{}); err != nil {
		t.Fatalf("persist: %v", err)
	}
	store1.Close()

	// Reopen the same file — the metric must still be there.
	store2, err := NewSQLiteMetricsStore(path)
	if err != nil {
		t.Fatalf("open 2: %v", err)
	}
	defer store2.Close()
	sum, err := store2.Summary(ctx)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if sum.QueryCount != 1 {
		t.Errorf("after reopen QueryCount = %d, want 1 (metrics must be durable)", sum.QueryCount)
	}
}

// TestServiceUsesMetricsStore verifies Search persists to the injected store and
// MetricsSnapshot reports Persistent=true with durable aggregates.
func TestServiceUsesMetricsStore(t *testing.T) {
	store := newTestStore(t)
	p := &mockProvider{}
	svc := NewService(p, nil, nil)
	svc.SetMetricsStore(store)

	if _, err := svc.Search(context.Background(), "proj", "q", SearchOpts{K: 3, Recall: 10}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	// The async persist goroutine may not have completed; poll the store.
	persisted := false
	for i := 0; i < 100; i++ {
		if sum, _ := store.Summary(context.Background()); sum.QueryCount >= 1 {
			persisted = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !persisted {
		t.Fatal("metric was not persisted within timeout")
	}

	snap := svc.MetricsSnapshot(context.Background())
	if !snap.Persistent {
		t.Error("snapshot.Persistent should be true when a store is set")
	}
	if snap.QueryCount < 1 {
		t.Errorf("snapshot.QueryCount = %d, want >= 1", snap.QueryCount)
	}
}
