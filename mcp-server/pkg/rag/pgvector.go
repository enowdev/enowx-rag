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
	embedding vector(%d),
	content_tsv tsvector GENERATED ALWAYS AS (to_tsvector('simple', content)) STORED
);
-- For tables created before content_tsv was added, add the column via ALTER.
ALTER TABLE %s ADD COLUMN IF NOT EXISTS content_tsv tsvector
	GENERATED ALWAYS AS (to_tsvector('simple', content)) STORED;
CREATE INDEX IF NOT EXISTS idx_%s_project_id ON %s (project_id);
CREATE INDEX IF NOT EXISTS idx_%s_embedding ON %s USING hnsw (embedding vector_cosine_ops);
CREATE INDEX IF NOT EXISTS idx_%s_tsv ON %s USING GIN (content_tsv);
`, p.table, p.dim, p.table, p.table, p.table, p.table, p.table, p.table, p.table)
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

	// Determine embed_model and embed_dim for metadata injection.
	embedModel := "unknown"
	if mn, ok := p.embedder.(ModelNamer); ok {
		embedModel = mn.ModelName()
	}
	embedDim := p.dim

	batch := &pgx.Batch{}
	for i, d := range docs {
		id := d.ID
		if id == "" {
			id = uuid.NewString()
		}
		meta := map[string]string{
			"embed_model": embedModel,
			"embed_dim":   fmt.Sprintf("%d", embedDim),
		}
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
	var queryVec []float32
	if qe, ok := p.embedder.(QueryEmbedder); ok {
		v, err := qe.EmbedQuery(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("embed query: %w", err)
		}
		queryVec = v
	} else {
		vectors, err := p.embedder.Embed(ctx, []string{query})
		if err != nil {
			return nil, fmt.Errorf("embed query: %w", err)
		}
		queryVec = vectors[0]
	}
	q := fmt.Sprintf(`
SELECT id, content, metadata, 1 - (embedding <=> $1::vector) AS score
FROM %s
WHERE project_id = $2
ORDER BY embedding <=> $1::vector
LIMIT $3
`, p.table)
	rows, err := p.pool.Query(ctx, q, pgVectorLiteral(queryVec), projectID, limit)
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

// SemanticSearchHybrid implements the HybridSearcher interface. It combines
// dense vector similarity (cosine) with lexical full-text search (ts_rank)
// using Reciprocal Rank Fusion (RRF) with k=60. The dense and lexical
// rankings are merged via a FULL OUTER JOIN on the document id.
//
// RRF score = COALESCE(1.0/(60+dense_rank), 0) + COALESCE(1.0/(60+lexical_rank), 0)
//
// This returns different results than dense-only search for keyword-heavy
// queries because documents that match the query text exactly get a boost
// from the lexical ranking that dense-only search would miss.
func (p *PGVectorProvider) SemanticSearchHybrid(ctx context.Context, projectID, query string, limit int) ([]Result, error) {
	if limit <= 0 {
		limit = 5
	}
	var queryVec []float32
	if qe, ok := p.embedder.(QueryEmbedder); ok {
		v, err := qe.EmbedQuery(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("embed query: %w", err)
		}
		queryVec = v
	} else {
		vectors, err := p.embedder.Embed(ctx, []string{query})
		if err != nil {
			return nil, fmt.Errorf("embed query: %w", err)
		}
		queryVec = vectors[0]
	}
	q := fmt.Sprintf(`
WITH dense AS (
  SELECT id, content, metadata,
         ROW_NUMBER() OVER (ORDER BY embedding <=> $1::vector) AS rank
  FROM %s
  WHERE project_id = $2
  ORDER BY embedding <=> $1::vector
  LIMIT $3
),
lexical AS (
  SELECT id, content, metadata,
         ROW_NUMBER() OVER (ORDER BY ts_rank(content_tsv, plainto_tsquery('simple', $4)) DESC) AS rank
  FROM %s
  WHERE project_id = $2 AND content_tsv @@ plainto_tsquery('simple', $4)
  LIMIT $3
)
SELECT COALESCE(d.id, l.id) AS id,
       COALESCE(d.content, l.content) AS content,
       COALESCE(d.metadata, l.metadata) AS metadata,
       (COALESCE(1.0/(60+d.rank), 0) + COALESCE(1.0/(60+l.rank), 0)) AS score
FROM dense d
FULL OUTER JOIN lexical l ON d.id = l.id
ORDER BY score DESC
LIMIT $3
`, p.table, p.table)
	rows, err := p.pool.Query(ctx, q, pgVectorLiteral(queryVec), projectID, limit, query)
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
	q := fmt.Sprintf("SELECT id::text, metadata->>'source_file', metadata->>'content_hash', metadata->>'doc_id' FROM %s WHERE project_id = $1", p.table)
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
		var contentHash *string
		var docID *string
		if err := rows.Scan(&id, &sourceFile, &contentHash, &docID); err != nil {
			return nil, err
		}
		pi := PointInfo{ID: id}
		if sourceFile != nil {
			pi.SourceFile = *sourceFile
		}
		if contentHash != nil {
			pi.ContentHash = *contentHash
		}
		if docID != nil {
			pi.DocID = *docID
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

// Compile-time assertion that PGVectorProvider implements HybridSearcher.
var _ HybridSearcher = (*PGVectorProvider)(nil)
