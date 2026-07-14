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
	if err := p.migrateCompositeKey(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return p, nil
}

func (p *PGVectorProvider) ensureTable(ctx context.Context) error {
	// The id column is TEXT (not UUID) because the indexer generates string
	// IDs like "dir/file.go#chunk0". A UUID column would reject these IDs.
	//
	// PRIMARY KEY is (project_id, id) — a composite key — because the same
	// source directory indexed into different projects generates identical
	// doc IDs (e.g. "config/config.go#chunk0"). A single-column PK on id
	// would cause cross-project data collisions.
	query := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	id TEXT NOT NULL,
	project_id TEXT NOT NULL,
	content TEXT NOT NULL,
	metadata JSONB,
	embedding vector(%d),
	content_tsv tsvector GENERATED ALWAYS AS (to_tsvector('simple', content)) STORED,
	PRIMARY KEY (project_id, id)
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

// migrateCompositeKey drops and recreates the table if it was created with
// the old single-column PRIMARY KEY on id alone. This is necessary because
// ALTER TABLE cannot change a PRIMARY KEY constraint in-place when existing
// data may already have collisions under the new composite key. The caller
// should only invoke this when upgrading from an older schema.
func (p *PGVectorProvider) migrateCompositeKey(ctx context.Context) error {
	// Check whether the table has the old single-column PK on "id".
	var constraintName string
	err := p.pool.QueryRow(ctx, `
SELECT con.conname
FROM pg_constraint con
JOIN pg_class rel ON rel.oid = con.conrelid
JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
WHERE rel.relname = $1
  AND con.contype = 'p'
  AND array_length(con.conkey, 1) = 1
  AND con.conkey[1] = (
      SELECT attnum FROM pg_attribute
      WHERE attrelid = rel.oid AND attname = 'id'
  )
  AND nsp.nspname = current_schema()
`, p.table).Scan(&constraintName)
	if err != nil {
		// No rows: either the table doesn't exist or the PK is already
		// composite. Either way, nothing to migrate.
		return nil
	}
	// Old single-column PK detected. Drop and recreate the table so the
	// new composite PK takes effect. Data loss is expected during migration.
	_, err = p.pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", p.table))
	if err != nil {
		return fmt.Errorf("migrate: drop old table: %w", err)
	}
	return p.ensureTable(ctx)
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
ON CONFLICT (project_id, id) DO UPDATE SET
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
       (COALESCE(1.0/(60+d.rank), 0) + COALESCE(1.0/(60+l.rank), 0)) AS score,
       (d.id IS NOT NULL) AS in_dense,
       (l.id IS NOT NULL) AS in_lexical
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
		if err := rows.Scan(&r.ID, &r.Content, &meta, &r.Score, &r.InDense, &r.InLexical); err != nil {
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
	q := fmt.Sprintf("SELECT id, metadata->>'source_file', metadata->>'content_hash', metadata->>'chunk_version', metadata->>'doc_id', LEFT(content, 200), metadata->>'chunk_index' FROM %s WHERE project_id = $1", p.table)
	args := []any{projectID}
	if v, ok := metaFilter["source_file"]; ok {
		q += " AND metadata->>'source_file' = $2"
		args = append(args, v)
	}
	q += " ORDER BY metadata->>'source_file', metadata->>'chunk_index'"
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
		var chunkVersion *string
		var docID *string
		var contentPreview *string
		var chunkIndex *string
		if err := rows.Scan(&id, &sourceFile, &contentHash, &chunkVersion, &docID, &contentPreview, &chunkIndex); err != nil {
			return nil, err
		}
		pi := PointInfo{ID: id}
		if sourceFile != nil {
			pi.SourceFile = *sourceFile
		}
		if contentHash != nil {
			pi.ContentHash = *contentHash
		}
		if chunkVersion != nil {
			pi.ChunkVersion = *chunkVersion
		}
		if docID != nil {
			pi.DocID = *docID
		}
		if contentPreview != nil {
			pi.Content = *contentPreview
		}
		if chunkIndex != nil {
			pi.ChunkIndex = *chunkIndex
		}
		points = append(points, pi)
	}
	return points, rows.Err()
}

// ExportPoints returns every point in the project with full content and full
// metadata. Implements Exporter (used by migration to re-embed elsewhere).
func (p *PGVectorProvider) ExportPoints(ctx context.Context, projectID string) ([]Document, error) {
	q := fmt.Sprintf("SELECT id, content, metadata FROM %s WHERE project_id = $1 ORDER BY metadata->>'source_file', metadata->>'chunk_index'", p.table)
	rows, err := p.pool.Query(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []Document
	for rows.Next() {
		var id, content string
		var meta map[string]string
		if err := rows.Scan(&id, &content, &meta); err != nil {
			return nil, err
		}
		if meta == nil {
			meta = map[string]string{}
		}
		// Prefer the original doc_id as the Document ID so identity is preserved.
		docID := id
		if v := meta["doc_id"]; v != "" {
			docID = v
		}
		docs = append(docs, Document{ID: docID, Content: content, Meta: meta})
	}
	return docs, rows.Err()
}

func (p *PGVectorProvider) Close() error {
	p.pool.Close()
	return nil
}

// TokensUsed forwards the embedder's cumulative token count when the embedder
// tracks it (e.g. Voyage), so core.Service can report embed tokens without
// direct embedder access. Returns 0 for embedders that don't (e.g. TEI).
func (p *PGVectorProvider) TokensUsed() int64 {
	if tc, ok := p.embedder.(TokenCounter); ok {
		return tc.TokensUsed()
	}
	return 0
}

// ListProjectIDs returns all distinct project_id values that currently have
// at least one row in the project memory table. This implements the
// core.ProjectLister interface, enabling GET /api/projects to enumerate
// projects without prior knowledge.
// CountPoints returns the number of rows for a project via COUNT(*), avoiding a
// full row fetch. Implements core.ProjectCounter.
func (p *PGVectorProvider) CountPoints(ctx context.Context, projectID string) (int, error) {
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE project_id = $1", p.table)
	var n int
	if err := p.pool.QueryRow(ctx, q, projectID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (p *PGVectorProvider) ListProjectIDs(ctx context.Context) ([]string, error) {
	q := fmt.Sprintf("SELECT DISTINCT project_id FROM %s ORDER BY project_id", p.table)
	rows, err := p.pool.Query(ctx, q)
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

func pgVectorLiteral(v []float32) string {
	parts := make([]string, len(v))
	for i, x := range v {
		parts[i] = fmt.Sprintf("%f", x)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// Compile-time assertion that PGVectorProvider implements HybridSearcher.
var _ HybridSearcher = (*PGVectorProvider)(nil)

// projectLister is a local interface matching core.ProjectLister. We define
// it here to avoid a circular import (pkg/core already imports pkg/rag).
// The type assertion in core.Service.ListProjects checks for this method
// dynamically, so any provider with a ListProjectIDs method will work.
type projectLister interface {
	ListProjectIDs(ctx context.Context) ([]string, error)
}

// Compile-time assertion that PGVectorProvider implements projectLister.
var _ projectLister = (*PGVectorProvider)(nil)
