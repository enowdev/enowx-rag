package rag

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGVectorProvider stores per-project vectors in a single PostgreSQL table.
// Rows are partitioned by project_id for simplicity.
type PGVectorProvider struct {
	pool     *pgxpool.Pool
	embedder EmbeddingClient
	dim      int
	table    string
}

// NewPGVectorProvider connects to Postgres and ensures the project table exists.
func NewPGVectorProvider(ctx context.Context, dsn string, embedder EmbeddingClient, table string) (*PGVectorProvider, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if table == "" {
		table = "project_memory"
	}
	p := &PGVectorProvider{
		pool:     pool,
		embedder: embedder,
		dim:      embedder.VectorSize(),
		table:    table,
	}
	if p.dim <= 0 {
		p.dim = 384
	}
	if err := p.ensureTable(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return p, nil
}

func (p *PGVectorProvider) ensureTable(ctx context.Context) error {
	query := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	id UUID PRIMARY KEY,
	project_id TEXT NOT NULL,
	content TEXT NOT NULL,
	metadata JSONB,
	embedding vector(%d)
);
CREATE INDEX IF NOT EXISTS idx_%s_project_id ON %s (project_id);
CREATE INDEX IF NOT EXISTS idx_%s_embedding ON %s USING hnsw (embedding vector_cosine_ops);
`, p.table, p.dim, p.table, p.table, p.table, p.table)
	_, err := p.pool.Exec(ctx, query)
	return err
}

func (p *PGVectorProvider) CreateCollection(ctx context.Context, projectID string) error {
	// No-op: pgvector uses a single table with project_id filter.
	return nil
}

func (p *PGVectorProvider) DeleteCollection(ctx context.Context, projectID string) error {
	_, err := p.pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE project_id = $1", p.table), projectID)
	return err
}

func (p *PGVectorProvider) Index(ctx context.Context, projectID string, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}
	texts := make([]string, len(docs))
	for i, d := range docs {
		texts[i] = d.Content
	}
	vectors, err := p.embedder.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	if len(vectors) != len(docs) {
		return fmt.Errorf("embedding count mismatch")
	}

	batch := &pgx.Batch{}
	for i, d := range docs {
		id := d.ID
		if id == "" {
			id = uuid.NewString()
		}
		meta := map[string]string{}
		for k, v := range d.Meta {
			meta[k] = v
		}
		batch.Queue(fmt.Sprintf(`
INSERT INTO %s (id, project_id, content, metadata, embedding)
VALUES ($1, $2, $3, $4, $5::vector)
ON CONFLICT (id) DO UPDATE SET
	content = EXCLUDED.content,
	metadata = EXCLUDED.metadata,
	embedding = EXCLUDED.embedding
`, p.table), id, projectID, d.Content, meta, pgVectorLiteral(vectors[i]))
	}
	br := p.pool.SendBatch(ctx, batch)
	return br.Close()
}

func (p *PGVectorProvider) SemanticSearch(ctx context.Context, projectID, query string, limit int) ([]Result, error) {
	if limit <= 0 {
		limit = 5
	}
	vectors, err := p.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	q := fmt.Sprintf(`
SELECT id, content, metadata, 1 - (embedding <=> $1::vector) AS score
FROM %s
WHERE project_id = $2
ORDER BY embedding <=> $1::vector
LIMIT $3
`, p.table)
	rows, err := p.pool.Query(ctx, q, pgVectorLiteral(vectors[0]), projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Result{}
	for rows.Next() {
		var r Result
		var meta map[string]string
		if err := rows.Scan(&r.ID, &r.Content, &meta, &r.Score); err != nil {
			return nil, err
		}
		r.Meta = meta
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *PGVectorProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	vectors, err := p.embedder.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return vectors[0], nil
}

func (p *PGVectorProvider) DeletePoints(ctx context.Context, projectID string, pointIDs []string) error {
	if len(pointIDs) == 0 {
		return nil
	}
	_, err := p.pool.Exec(ctx, fmt.Sprintf(
		"DELETE FROM %s WHERE project_id = $1 AND id = ANY($2)", p.table,
	), projectID, pointIDs)
	return err
}

func (p *PGVectorProvider) ListPointIDs(ctx context.Context, projectID string, metaFilter map[string]string) ([]string, error) {
	q := fmt.Sprintf("SELECT id FROM %s WHERE project_id = $1", p.table)
	args := []any{projectID}
	if v, ok := metaFilter["source_file"]; ok {
		q += " AND metadata->>'source_file' = $2"
		args = append(args, v)
	}
	rows, err := p.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (p *PGVectorProvider) ListPoints(ctx context.Context, projectID string, metaFilter map[string]string) ([]PointInfo, error) {
	q := fmt.Sprintf("SELECT id, metadata->>'source_file' FROM %s WHERE project_id = $1", p.table)
	args := []any{projectID}
	if v, ok := metaFilter["source_file"]; ok {
		q += " AND metadata->>'source_file' = $2"
		args = append(args, v)
	}
	rows, err := p.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var points []PointInfo
	for rows.Next() {
		var id string
		var sourceFile *string
		if err := rows.Scan(&id, &sourceFile); err != nil {
			return nil, err
		}
		pi := PointInfo{ID: id}
		if sourceFile != nil {
			pi.SourceFile = *sourceFile
		}
		points = append(points, pi)
	}
	return points, rows.Err()
}

func (p *PGVectorProvider) Close() error {
	p.pool.Close()
	return nil
}

func pgVectorLiteral(v []float32) string {
	parts := make([]string, len(v))
	for i, x := range v {
		parts[i] = fmt.Sprintf("%f", x)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
