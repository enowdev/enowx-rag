package rag

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	_ "modernc.org/sqlite" // pure Go, no cgo
)

// SQLiteProvider keeps every project's points in one SQLite file, the
// vectors as blobs searched by brute force (cosine) and the text in an FTS5
// table for hybrid search: a store that runs inside the program, nothing
// to install or keep running. It suits a project of up to a few hundred
// thousand chunks; a project's vectors are held in memory once searched.
type SQLiteProvider struct {
	db       *sql.DB
	embedder EmbeddingClient

	mu    sync.Mutex
	cache map[string][]cached // project → its points' vectors, normalized
}

type cached struct {
	id  string
	vec []float32
}

// NewSQLiteProvider opens (or makes) the store at path.
func NewSQLiteProvider(path string, embedder EmbeddingClient) (*SQLiteProvider, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("sqlite: %w", err)
	}
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	db.SetMaxOpenConns(1) // one writer; reads are fast enough in line
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS points (project TEXT NOT NULL, id TEXT NOT NULL, content TEXT NOT NULL, meta TEXT NOT NULL, vector BLOB NOT NULL, PRIMARY KEY (project, id))`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS points_fts USING fts5(content, project UNINDEXED, id UNINDEXED, tokenize = 'unicode61')`,
		`CREATE TABLE IF NOT EXISTS projects (project TEXT PRIMARY KEY)`,
	} {
		if _, err := db.Exec(q); err != nil {
			db.Close()
			return nil, fmt.Errorf("sqlite: schema: %w", err)
		}
	}
	return &SQLiteProvider{db: db, embedder: embedder, cache: map[string][]cached{}}, nil
}

func (p *SQLiteProvider) CreateCollection(ctx context.Context, projectID string) error {
	_, err := p.db.ExecContext(ctx, `INSERT OR IGNORE INTO projects(project) VALUES (?)`, projectID)
	return err
}

func (p *SQLiteProvider) DeleteCollection(ctx context.Context, projectID string) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{`DELETE FROM points WHERE project = ?`, `DELETE FROM points_fts WHERE project = ?`, `DELETE FROM projects WHERE project = ?`} {
		if _, err := tx.ExecContext(ctx, q, projectID); err != nil {
			return err
		}
	}
	p.forget(projectID)
	return tx.Commit()
}

func (p *SQLiteProvider) forget(projectID string) {
	p.mu.Lock()
	delete(p.cache, projectID)
	p.mu.Unlock()
}

func (p *SQLiteProvider) Index(ctx context.Context, projectID string, docs []Document) error {
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
	model := "unknown"
	if mn, ok := p.embedder.(ModelNamer); ok {
		model = mn.ModelName()
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO projects(project) VALUES (?)`, projectID); err != nil {
		return err
	}
	for i, d := range docs {
		id := d.ID
		if id == "" {
			id = uuid.NewString()
		}
		meta := map[string]string{"embed_model": model, "embed_dim": fmt.Sprint(len(vectors[i])), "doc_id": id}
		for k, v := range d.Meta {
			meta[k] = v
		}
		raw, _ := json.Marshal(meta)
		if _, err := tx.ExecContext(ctx, `INSERT INTO points(project, id, content, meta, vector) VALUES (?, ?, ?, ?, ?)
ON CONFLICT(project, id) DO UPDATE SET content = excluded.content, meta = excluded.meta, vector = excluded.vector`,
			projectID, id, d.Content, string(raw), encodeVector(vectors[i])); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM points_fts WHERE project = ? AND id = ?`, projectID, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO points_fts(content, project, id) VALUES (?, ?, ?)`, d.Content, projectID, id); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	p.forget(projectID)
	return nil
}

// vectors are project's points' vectors, normalized, read once.
func (p *SQLiteProvider) vectors(ctx context.Context, projectID string) ([]cached, error) {
	p.mu.Lock()
	if c, ok := p.cache[projectID]; ok {
		p.mu.Unlock()
		return c, nil
	}
	p.mu.Unlock()
	rows, err := p.db.QueryContext(ctx, `SELECT id, vector FROM points WHERE project = ?`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []cached
	for rows.Next() {
		var id string
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			return nil, err
		}
		out = append(out, cached{id: id, vec: normalize(decodeVector(blob))})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.cache[projectID] = out
	p.mu.Unlock()
	return out, nil
}

func (p *SQLiteProvider) queryVector(ctx context.Context, query string) ([]float32, error) {
	if qe, ok := p.embedder.(QueryEmbedder); ok {
		return qe.EmbedQuery(ctx, query)
	}
	v, err := p.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	if len(v) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return v[0], nil
}

// dense is the ids of project's points nearest q, with their cosine.
func (p *SQLiteProvider) dense(ctx context.Context, projectID string, q []float32, limit int) ([]Result, error) {
	all, err := p.vectors(ctx, projectID)
	if err != nil {
		return nil, err
	}
	q = normalize(q)
	scored := make([]Result, 0, len(all))
	for _, c := range all {
		if len(c.vec) != len(q) {
			continue
		}
		var dot float64
		for i := range q {
			dot += float64(q[i]) * float64(c.vec[i])
		}
		scored = append(scored, Result{ID: c.id, Score: dot})
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })
	if len(scored) > limit {
		scored = scored[:limit]
	}
	return scored, nil
}

// fill reads rs' content and metadata.
func (p *SQLiteProvider) fill(ctx context.Context, projectID string, rs []Result) ([]Result, error) {
	for i := range rs {
		var content, raw string
		err := p.db.QueryRowContext(ctx, `SELECT content, meta FROM points WHERE project = ? AND id = ?`, projectID, rs[i].ID).Scan(&content, &raw)
		if err != nil {
			return nil, err
		}
		rs[i].Content = content
		_ = json.Unmarshal([]byte(raw), &rs[i].Meta)
	}
	return rs, nil
}

func (p *SQLiteProvider) SemanticSearch(ctx context.Context, projectID, query string, limit int) ([]Result, error) {
	if limit <= 0 {
		limit = 5
	}
	q, err := p.queryVector(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	rs, err := p.dense(ctx, projectID, q, limit)
	if err != nil {
		return nil, err
	}
	return p.fill(ctx, projectID, rs)
}

// ftsQuery is query as words FTS5 matches any of, its own syntax quoted.
func ftsQuery(query string) string {
	var terms []string
	for _, w := range strings.FieldsFunc(query, func(r rune) bool {
		return !(r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r > 127)
	}) {
		if len(w) > 1 {
			terms = append(terms, `"`+w+`"`)
		}
	}
	return strings.Join(terms, " OR ")
}

// SemanticSearchHybrid fuses the dense ranking with FTS5's (BM25) by
// Reciprocal Rank Fusion, as pgvector's does.
func (p *SQLiteProvider) SemanticSearchHybrid(ctx context.Context, projectID, query string, limit int) ([]Result, error) {
	if limit <= 0 {
		limit = 5
	}
	q, err := p.queryVector(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	dense, err := p.dense(ctx, projectID, q, limit*4)
	if err != nil {
		return nil, err
	}
	fused := map[string]*Result{}
	for i, r := range dense {
		fused[r.ID] = &Result{ID: r.ID, Score: 1.0 / float64(60+i+1), InDense: true}
	}
	if fq := ftsQuery(query); fq != "" {
		rows, err := p.db.QueryContext(ctx, `SELECT id FROM points_fts WHERE points_fts MATCH ? AND project = ? ORDER BY bm25(points_fts) LIMIT ?`, fq, projectID, limit*4)
		if err == nil {
			i := 0
			for rows.Next() {
				var id string
				if rows.Scan(&id) == nil {
					r := fused[id]
					if r == nil {
						r = &Result{ID: id}
						fused[id] = r
					}
					r.Score += 1.0 / float64(60+i+1)
					r.InLexical = true
					i++
				}
			}
			rows.Close()
		}
	}
	out := make([]Result, 0, len(fused))
	for _, r := range fused {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > limit {
		out = out[:limit]
	}
	return p.fill(ctx, projectID, out)
}

func (p *SQLiteProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	v, err := p.embedder.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(v) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return v[0], nil
}

func (p *SQLiteProvider) DeletePoints(ctx context.Context, projectID string, pointIDs []string) error {
	if len(pointIDs) == 0 {
		return nil
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range pointIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM points WHERE project = ? AND id = ?`, projectID, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM points_fts WHERE project = ? AND id = ?`, projectID, id); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	p.forget(projectID)
	return nil
}

// points reads project's points' ids, metadata and content (cut to max
// when max > 0), those matching metaFilter.
func (p *SQLiteProvider) points(ctx context.Context, projectID string, metaFilter map[string]string, max int) ([]Document, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT id, content, meta FROM points WHERE project = ?`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Document
	for rows.Next() {
		var d Document
		var raw string
		if err := rows.Scan(&d.ID, &d.Content, &raw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(raw), &d.Meta)
		match := true
		for k, v := range metaFilter {
			if d.Meta[k] != v {
				match = false
			}
		}
		if !match {
			continue
		}
		if max > 0 && len(d.Content) > max {
			d.Content = d.Content[:max]
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (p *SQLiteProvider) ListPointIDs(ctx context.Context, projectID string, metaFilter map[string]string) ([]string, error) {
	docs, err := p.points(ctx, projectID, metaFilter, 1)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(docs))
	for i, d := range docs {
		ids[i] = d.ID
	}
	return ids, nil
}

func (p *SQLiteProvider) ListPoints(ctx context.Context, projectID string, metaFilter map[string]string) ([]PointInfo, error) {
	docs, err := p.points(ctx, projectID, metaFilter, 200)
	if err != nil {
		return nil, err
	}
	out := make([]PointInfo, len(docs))
	for i, d := range docs {
		out[i] = PointInfo{ID: d.ID, SourceFile: d.Meta["source_file"], ContentHash: d.Meta["content_hash"], ChunkVersion: d.Meta["chunk_version"],
			DocID: d.Meta["doc_id"], Content: d.Content, ChunkIndex: d.Meta["chunk_index"]}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SourceFile != out[j].SourceFile {
			return out[i].SourceFile < out[j].SourceFile
		}
		return out[i].ChunkIndex < out[j].ChunkIndex
	})
	return out, nil
}

// ExportPoints returns every point of the project, its content whole.
func (p *SQLiteProvider) ExportPoints(ctx context.Context, projectID string) ([]Document, error) {
	return p.points(ctx, projectID, nil, 0)
}

func (p *SQLiteProvider) CountPoints(ctx context.Context, projectID string) (int, error) {
	var n int
	err := p.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM points WHERE project = ?`, projectID).Scan(&n)
	return n, err
}

func (p *SQLiteProvider) ListProjectIDs(ctx context.Context) ([]string, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT project FROM projects UNION SELECT DISTINCT project FROM points ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (p *SQLiteProvider) TokensUsed() int64 {
	if tc, ok := p.embedder.(TokenCounter); ok {
		return tc.TokensUsed()
	}
	return 0
}

func (p *SQLiteProvider) Close() error { return p.db.Close() }

func encodeVector(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(f))
	}
	return b
}

func decodeVector(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return v
}

func normalize(v []float32) []float32 {
	var n float64
	for _, f := range v {
		n += float64(f) * float64(f)
	}
	if n == 0 {
		return v
	}
	n = math.Sqrt(n)
	out := make([]float32, len(v))
	for i, f := range v {
		out[i] = float32(float64(f) / n)
	}
	return out
}
