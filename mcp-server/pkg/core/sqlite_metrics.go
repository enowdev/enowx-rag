package core

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (no cgo)
)

// SQLiteMetricsStore is a durable MetricsStore backed by a local SQLite file.
// It works with any vector-store backend (the metrics live independently of the
// vector store), keeping the "local-first, single static binary" goal intact —
// modernc.org/sqlite is pure Go, so no cgo is required.
type SQLiteMetricsStore struct {
	db *sql.DB
}

// NewSQLiteMetricsStore opens (creating if needed) a SQLite metrics database at
// path and ensures the schema exists. Callers should Close it on shutdown.
func NewSQLiteMetricsStore(path string) (*SQLiteMetricsStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open metrics db: %w", err)
	}
	// A single connection avoids "database is locked" under concurrent writes
	// from the async persist goroutines; SQLite serializes writers anyway.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS query_metrics (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	ts            INTEGER NOT NULL DEFAULT (unixepoch()),
	latency_ms    REAL    NOT NULL,
	hybrid        INTEGER NOT NULL DEFAULT 0,
	reranked      INTEGER NOT NULL DEFAULT 0,
	candidates    INTEGER NOT NULL DEFAULT 0,
	results       INTEGER NOT NULL DEFAULT 0,
	dense_count   INTEGER NOT NULL DEFAULT 0,
	lexical_count INTEGER NOT NULL DEFAULT 0,
	rerank_moved  INTEGER NOT NULL DEFAULT 0
);`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create metrics schema: %w", err)
	}

	return &SQLiteMetricsStore{db: db}, nil
}

// Close releases the database handle.
func (s *SQLiteMetricsStore) Close() error { return s.db.Close() }

// PersistQueryMetric inserts one query's latency and composition.
func (s *SQLiteMetricsStore) PersistQueryMetric(ctx context.Context, latencyMs float64, comp QueryComposition) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO query_metrics
	(latency_ms, hybrid, reranked, candidates, results, dense_count, lexical_count, rerank_moved)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		latencyMs, b2i(comp.Hybrid), b2i(comp.Reranked), comp.Candidates,
		comp.Results, comp.DenseCount, comp.LexicalCount, comp.RerankMoved)
	return err
}

// Summary computes durable aggregates over all persisted queries. Percentiles
// use SQLite's window functions over the latency column.
func (s *SQLiteMetricsStore) Summary(ctx context.Context) (MetricsSummary, error) {
	var out MetricsSummary
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(AVG(latency_ms), 0) FROM query_metrics`).
		Scan(&out.QueryCount, &out.AvgLatencyMs)
	if err != nil {
		return out, err
	}
	if out.QueryCount == 0 {
		return out, nil
	}
	out.P50LatencyMs = s.percentile(ctx, 0.50)
	out.P95LatencyMs = s.percentile(ctx, 0.95)
	return out, nil
}

// percentile returns the nearest-rank p-quantile (0..1) of latency_ms, or 0.
func (s *SQLiteMetricsStore) percentile(ctx context.Context, p float64) float64 {
	// nearest-rank: order ascending, pick row at ceil(p*N). OFFSET is 0-based.
	var v float64
	row := s.db.QueryRowContext(ctx, `
SELECT latency_ms FROM query_metrics
ORDER BY latency_ms
LIMIT 1 OFFSET CAST(ROUND(? * (SELECT COUNT(*) - 1 FROM query_metrics)) AS INTEGER)`, p)
	if err := row.Scan(&v); err != nil {
		return 0
	}
	return v
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
