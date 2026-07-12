package core

import (
	"sync"
	"testing"
)

// TestMetricsEmpty verifies a fresh Metrics reports zero and no composition.
func TestMetricsEmpty(t *testing.T) {
	m := NewMetrics()
	avg, p50, p95, count, _, haveComp := m.snapshot()
	if avg != 0 || p50 != 0 || p95 != 0 || count != 0 {
		t.Errorf("empty metrics = avg %v p50 %v p95 %v count %d, want all 0", avg, p50, p95, count)
	}
	if haveComp {
		t.Error("empty metrics should not have composition")
	}
}

// TestMetricsRecordAndSnapshot verifies latency aggregates and last composition.
func TestMetricsRecordAndSnapshot(t *testing.T) {
	m := NewMetrics()
	for _, v := range []float64{10, 20, 30, 40, 100} {
		m.RecordQuery(v, QueryComposition{Candidates: int(v)})
	}
	avg, p50, p95, count, comp, haveComp := m.snapshot()
	if count != 5 {
		t.Errorf("count = %d, want 5", count)
	}
	if avg != 40 { // (10+20+30+40+100)/5
		t.Errorf("avg = %v, want 40", avg)
	}
	if p50 != 30 {
		t.Errorf("p50 = %v, want 30", p50)
	}
	if p95 != 100 {
		t.Errorf("p95 = %v, want 100", p95)
	}
	if !haveComp || comp.Candidates != 100 {
		t.Errorf("last composition = %+v haveComp=%v, want Candidates 100", comp, haveComp)
	}
}

// TestMetricsRingWrap verifies the ring buffer bounds retained samples and that
// snapshot still computes without panic after wrap.
func TestMetricsRingWrap(t *testing.T) {
	m := NewMetrics()
	total := latencyRingSize + 100
	for i := 0; i < total; i++ {
		m.RecordQuery(float64(i), QueryComposition{})
	}
	_, _, _, count, _, _ := m.snapshot()
	if count != int64(total) {
		t.Errorf("count = %d, want %d (query count is not ring-bounded)", count, total)
	}
	// latCount is bounded to ring size internally; snapshot must not panic.
	if m.latCount != latencyRingSize {
		t.Errorf("latCount = %d, want %d", m.latCount, latencyRingSize)
	}
}

// TestMetricsConcurrent verifies RecordQuery is safe under concurrent writers.
func TestMetricsConcurrent(t *testing.T) {
	m := NewMetrics()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				m.RecordQuery(float64(n), QueryComposition{})
			}
		}(i)
	}
	wg.Wait()
	if _, _, _, count, _, _ := m.snapshot(); count != 1000 {
		t.Errorf("count = %d, want 1000", count)
	}
}
