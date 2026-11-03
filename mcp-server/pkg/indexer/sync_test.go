package indexer

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/enowdev/enowx-rag/mcp-server/pkg/rag"
)

// countingEmbedder embeds texts as hashed word buckets and counts how many
// texts it was asked to embed, so tests can tell what was re-embedded.
type countingEmbedder struct {
	mu    sync.Mutex
	texts []string
}

func (e *countingEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	e.mu.Lock()
	e.texts = append(e.texts, texts...)
	e.mu.Unlock()
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v := make([]float32, 64)
		for _, w := range strings.Fields(strings.ToLower(t)) {
			h := fnv.New32a()
			h.Write([]byte(w))
			v[h.Sum32()%64]++
		}
		out[i] = v
	}
	return out, nil
}

func (e *countingEmbedder) VectorSize() int { return 64 }

func (e *countingEmbedder) reset() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	got := e.texts
	e.texts = nil
	return got
}

// goFunc returns a Go function of roughly 450 characters: with a 1000-char
// chunk size it is above the "small" threshold, so it gets a chunk of its own.
func goFunc(name string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "func %s() {\n", name)
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&b, "\t_ = %q // %s body line %d\n", name, name, i)
	}
	b.WriteString("}\n\n")
	return b.String()
}

func writeGoFile(t *testing.T, path string, names ...string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("package auth\n\n")
	for _, n := range names {
		b.WriteString(goFunc(n))
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newSQLiteIndexer(t *testing.T) (*Indexer, *rag.SQLiteProvider, *countingEmbedder) {
	t.Helper()
	emb := &countingEmbedder{}
	p, err := rag.NewSQLiteProvider(filepath.Join(t.TempDir(), "rag.db"), emb)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close() })
	idx := NewIndexer(p, 1000)
	// No git in the temp dir: list every file under it.
	idx.Files = nil
	return idx, p, emb
}

func storedContents(t *testing.T, p *rag.SQLiteProvider, project string) []string {
	t.Helper()
	docs, err := p.ExportPoints(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(docs))
	for i, d := range docs {
		out[i] = d.Content
	}
	return out
}

// A file that shrinks must not leave chunks of the code it lost behind:
// they would be served to agents as if the code still existed.
func TestIndexProject_ShrinkingFileDropsStaleChunks(t *testing.T) {
	ctx := context.Background()
	idx, p, _ := newSQLiteIndexer(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "auth.go")

	writeGoFile(t, file, "Login", "Logout", "Refresh", "VerifyToken", "HashPassword")
	if _, err := idx.IndexProject(ctx, "p", dir); err != nil {
		t.Fatal(err)
	}

	writeGoFile(t, file, "Login", "Logout")
	res, err := idx.IndexProject(ctx, "p", dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range storedContents(t, p, "p") {
		for _, gone := range []string{"Refresh", "VerifyToken", "HashPassword"} {
			if strings.Contains(c, gone) {
				t.Errorf("stale chunk for removed func %s still stored:\n%s", gone, c)
			}
		}
	}
	if res.Deleted == 0 {
		t.Errorf("expected stale chunks to be reported deleted, got %+v", res)
	}
}

// Inserting a function at the top of a file embeds only the chunks around
// it (the new function, and the old first one that loses the file header);
// the chunks further down kept their text and only get new line numbers.
func TestIndexProject_InsertAtTopOnlyEmbedsNearbyChunks(t *testing.T) {
	ctx := context.Background()
	idx, p, emb := newSQLiteIndexer(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "auth.go")

	writeGoFile(t, file, "Login", "Logout", "Refresh")
	if _, err := idx.IndexProject(ctx, "p", dir); err != nil {
		t.Fatal(err)
	}
	emb.reset()

	writeGoFile(t, file, "Signup", "Login", "Logout", "Refresh")
	if _, err := idx.IndexProject(ctx, "p", dir); err != nil {
		t.Fatal(err)
	}

	embedded := emb.reset()
	joined := strings.Join(embedded, "\n---\n")
	if !strings.Contains(joined, "func Signup") || len(embedded) > 2 {
		t.Fatalf("want the Signup chunk (and at most the old first chunk) embedded, got %d texts:\n%s", len(embedded), joined)
	}
	for _, untouched := range []string{"func Logout", "func Refresh"} {
		if strings.Contains(joined, untouched) {
			t.Errorf("%s did not change but was re-embedded", untouched)
		}
	}

	// The moved chunks must carry their new line numbers.
	content, _ := os.ReadFile(file)
	lines := strings.Split(string(content), "\n")
	wantStart := 0
	for i, l := range lines {
		if strings.HasPrefix(l, "func Refresh") {
			wantStart = i + 1
		}
	}
	found := false
	for _, c := range storedContents(t, p, "p") {
		if strings.Contains(c, "func Refresh") {
			found = true
			head := strings.SplitN(c, "\n", 2)[0]
			if !strings.HasPrefix(head, fmt.Sprintf("File: auth.go:%d-", wantStart)) {
				t.Errorf("Refresh chunk header has stale lines: %q (want start %d)", head, wantStart)
			}
		}
	}
	if !found {
		t.Fatal("Refresh chunk missing")
	}
	if n := len(storedContents(t, p, "p")); n != 4 {
		t.Errorf("want 4 chunks stored, got %d", n)
	}
}

// Points written by an older version (IDs "dir/file#chunkN") are replaced,
// and memory notes indexed with rag_index are never touched by a sync.
func TestIndexProject_ReplacesLegacyPointsKeepsNotes(t *testing.T) {
	ctx := context.Background()
	idx, p, _ := newSQLiteIndexer(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "auth.go")
	writeGoFile(t, file, "Login")
	base := filepath.Base(dir)

	legacy := rag.Document{
		ID:      base + "/auth.go#chunk0",
		Content: "File: auth.go\n\nfunc OldLogin() {}",
		Meta: map[string]string{"source_file": "auth.go", "source_dir": base,
			"chunk_index": "0", "content_hash": "0000000000000000", "chunk_version": "v2"},
	}
	note := rag.Document{ID: "decision-1", Content: "We hash passwords with argon2id.",
		Meta: map[string]string{"type": "decision"}}
	if err := p.Index(ctx, "p", []rag.Document{legacy, note}); err != nil {
		t.Fatal(err)
	}

	if _, err := idx.IndexProject(ctx, "p", dir); err != nil {
		t.Fatal(err)
	}
	contents := strings.Join(storedContents(t, p, "p"), "\n---\n")
	if strings.Contains(contents, "OldLogin") {
		t.Errorf("legacy chunk not replaced:\n%s", contents)
	}
	if !strings.Contains(contents, "argon2id") {
		t.Errorf("memory note was deleted by the sync:\n%s", contents)
	}
	if !strings.Contains(contents, "func Login") {
		t.Errorf("current code not indexed:\n%s", contents)
	}
}

// A directory that does not exist (a typo, a moved repo) must be an error,
// not an empty scan that deletes everything indexed from it.
func TestIndexProject_MissingDirIsErrorAndKeepsPoints(t *testing.T) {
	ctx := context.Background()
	idx, p, _ := newSQLiteIndexer(t)
	parent := t.TempDir()
	dir := filepath.Join(parent, "repo")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeGoFile(t, filepath.Join(dir, "auth.go"), "Login", "Logout")
	if _, err := idx.IndexProject(ctx, "p", dir); err != nil {
		t.Fatal(err)
	}
	before := len(storedContents(t, p, "p"))

	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.IndexProject(ctx, "p", dir); err == nil {
		t.Error("want an error for a missing directory")
	}
	if after := len(storedContents(t, p, "p")); after != before {
		t.Errorf("missing directory wiped the index: %d -> %d points", before, after)
	}
}

// Two identical chunks in one file are both kept (distinct IDs).
func TestIndexProject_DuplicateChunksInOneFile(t *testing.T) {
	ctx := context.Background()
	idx, p, _ := newSQLiteIndexer(t)
	dir := t.TempDir()
	body := "# Notes\n\nsame paragraph\n\n# Notes\n\nsame paragraph\n"
	// Force one chunk per heading with a tiny chunk size.
	idx.chunkSize = 40
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.IndexProject(ctx, "p", dir); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.IndexProject(ctx, "p", dir); err != nil {
		t.Fatal(err)
	}
	if n := len(storedContents(t, p, "p")); n != 2 {
		t.Errorf("want 2 chunks for two identical sections, got %d", n)
	}
}

// storeWithoutUpdater is a provider that keeps points in memory and does not
// implement rag.PayloadUpdater, like most remote stores.
type storeWithoutUpdater struct {
	recording
	pts map[string]rag.Document
}

func (s *storeWithoutUpdater) Index(_ context.Context, _ string, docs []rag.Document) error {
	s.recording.Index(context.Background(), "", docs)
	for _, d := range docs {
		s.pts[d.ID] = d
	}
	return nil
}

func (s *storeWithoutUpdater) ListPoints(_ context.Context, _ string, f map[string]string) ([]rag.PointInfo, error) {
	var out []rag.PointInfo
	for id, d := range s.pts {
		if d.Meta["source_dir"] != f["source_dir"] {
			continue
		}
		preview := d.Content
		if len(preview) > 200 {
			preview = preview[:200]
		}
		out = append(out, rag.PointInfo{ID: id, DocID: id, SourceFile: d.Meta["source_file"], ContentHash: d.Meta["content_hash"], Content: preview})
	}
	return out, nil
}

func (s *storeWithoutUpdater) DeletePoints(_ context.Context, _ string, ids []string) error {
	for _, id := range ids {
		delete(s.pts, id)
	}
	return nil
}

// Without a PayloadUpdater, a chunk whose lines moved is re-indexed so its
// stored header stays right.
func TestIndexProject_MovedChunksReindexedWithoutUpdater(t *testing.T) {
	ctx := context.Background()
	store := &storeWithoutUpdater{pts: map[string]rag.Document{}}
	idx := NewIndexer(store, 1000)
	dir := t.TempDir()
	file := filepath.Join(dir, "auth.go")
	writeGoFile(t, file, "Login", "Logout", "Refresh")
	if _, err := idx.IndexProject(ctx, "p", dir); err != nil {
		t.Fatal(err)
	}
	store.docs = nil
	writeGoFile(t, file, "Signup", "Login", "Logout", "Refresh")
	res, err := idx.IndexProject(ctx, "p", dir)
	if err != nil {
		t.Fatal(err)
	}
	reindexed := false
	for _, d := range store.docs {
		if strings.Contains(d.Content, "func Refresh") {
			reindexed = true
		}
	}
	if !reindexed || res.Updated != 0 {
		t.Fatalf("moved Refresh chunk not re-indexed: %+v", res)
	}
	if len(store.pts) != 4 {
		t.Fatalf("want 4 points, got %d", len(store.pts))
	}
}

// A second sync of unchanged files embeds nothing and deletes nothing.
func TestIndexProject_UnchangedSecondSyncIsNoop(t *testing.T) {
	ctx := context.Background()
	idx, _, emb := newSQLiteIndexer(t)
	dir := t.TempDir()
	writeGoFile(t, filepath.Join(dir, "auth.go"), "Login", "Logout", "Refresh")
	if _, err := idx.IndexProject(ctx, "p", dir); err != nil {
		t.Fatal(err)
	}
	emb.reset()
	res, err := idx.IndexProject(ctx, "p", dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(emb.reset()); n != 0 || res.Indexed != 0 || res.Deleted != 0 || res.Updated != 0 || res.Skipped == 0 {
		t.Fatalf("second sync not a no-op: embedded=%d result=%+v", n, res)
	}
}

// ignoresDirFilter mimics pgvector, whose ListPoints applies only a
// source_file filter: it returns the points of every directory.
type ignoresDirFilter struct{ storeWithoutUpdater }

func (s *ignoresDirFilter) ListPoints(ctx context.Context, project string, _ map[string]string) ([]rag.PointInfo, error) {
	var all []rag.PointInfo
	for _, dir := range []string{"repo-a", "repo-b"} {
		pts, _ := s.storeWithoutUpdater.ListPoints(ctx, project, map[string]string{"source_dir": dir})
		all = append(all, pts...)
	}
	return all, nil
}

// Syncing one directory never deletes another directory's chunks, even
// when the provider returns them from ListPoints.
func TestIndexProject_ProviderIgnoringDirFilterKeepsOtherDirs(t *testing.T) {
	ctx := context.Background()
	store := &ignoresDirFilter{storeWithoutUpdater{pts: map[string]rag.Document{}}}
	idx := NewIndexer(store, 1000)
	parent := t.TempDir()
	a, b := filepath.Join(parent, "repo-a"), filepath.Join(parent, "repo-b")
	for _, d := range []string{a, b} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeGoFile(t, filepath.Join(a, "a.go"), "Alpha")
	writeGoFile(t, filepath.Join(b, "b.go"), "Bravo")
	for _, d := range []string{a, b, a} {
		if _, err := idx.IndexProject(ctx, "p", d); err != nil {
			t.Fatal(err)
		}
	}
	var hasB bool
	for _, d := range store.pts {
		if strings.Contains(d.Content, "func Bravo") {
			hasB = true
		}
	}
	if !hasB {
		t.Fatal("syncing repo-a deleted repo-b's chunks")
	}
}

// A file that exists but can't be read keeps its chunks.
func TestIndexProject_UnreadableFileKeepsChunks(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read any file")
	}
	ctx := context.Background()
	idx, p, _ := newSQLiteIndexer(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "auth.go")
	writeGoFile(t, file, "Login")
	if _, err := idx.IndexProject(ctx, "p", dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(file, 0o644)
	if _, err := idx.IndexProject(ctx, "p", dir); err != nil {
		t.Fatal(err)
	}
	if n := len(storedContents(t, p, "p")); n == 0 {
		t.Fatal("chunks of an unreadable file were deleted")
	}
}
