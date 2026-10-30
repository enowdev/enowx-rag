package rag

import (
	"context"
	"hash/fnv"
	"path/filepath"
	"strings"
	"testing"
)

// wordsEmbedder embeds a text as its words hashed into 64 buckets: texts
// sharing words come out near each other, enough to test a store.
type wordsEmbedder struct{}

func (wordsEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v := make([]float32, 64)
		for _, w := range strings.Fields(strings.ToLower(t)) {
			h := fnv.New32a()
			h.Write([]byte(strings.Trim(w, ".,:()")))
			v[h.Sum32()%64]++
		}
		out[i] = v
	}
	return out, nil
}

func (wordsEmbedder) VectorSize() int { return 64 }

// The built-in store keeps points per project, finds them by meaning and,
// hybrid, by words too; lists, exports, deletes and survives a reopen.
func TestSQLiteProvider(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rag.db")
	p, err := NewSQLiteProvider(path, wordsEmbedder{})
	if err != nil {
		t.Fatal(err)
	}
	docs := []Document{
		{ID: "a", Content: "func ParseConfig reads the yaml config file", Meta: map[string]string{"source_file": "config.go", "source_dir": "x"}},
		{ID: "b", Content: "the http server routes requests to handlers", Meta: map[string]string{"source_file": "server.go", "source_dir": "x"}},
		{ID: "c", Content: "migrations move rows between vector stores", Meta: map[string]string{"source_file": "migrate.go", "source_dir": "x"}},
	}
	if err := p.Index(ctx, "proj", docs); err != nil {
		t.Fatal(err)
	}
	p.Index(ctx, "other", []Document{{ID: "z", Content: "yaml config file elsewhere"}})
	got, err := p.SemanticSearch(ctx, "proj", "read the config file", 2)
	if err != nil || len(got) != 2 || got[0].ID != "a" || got[0].Meta["source_file"] != "config.go" {
		t.Fatalf("dense: %+v %v", got, err)
	}
	hy, err := p.SemanticSearchHybrid(ctx, "proj", "handlers", 3)
	if err != nil || len(hy) == 0 || hy[0].ID != "b" || !hy[0].InLexical {
		t.Fatalf("hybrid: %+v %v", hy, err)
	}
	if n, _ := p.CountPoints(ctx, "proj"); n != 3 {
		t.Fatalf("count %d", n)
	}
	if pts, _ := p.ListPoints(ctx, "proj", map[string]string{"source_dir": "x"}); len(pts) != 3 || pts[0].SourceFile != "config.go" {
		t.Fatalf("list %+v", pts)
	}
	if err := p.DeletePoints(ctx, "proj", []string{"a"}); err != nil {
		t.Fatal(err)
	}
	p.Close()
	p, err = NewSQLiteProvider(path, wordsEmbedder{})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if docs, _ := p.ExportPoints(ctx, "proj"); len(docs) != 2 {
		t.Fatalf("after a reopen: %d", len(docs))
	}
	if ids, _ := p.ListProjectIDs(ctx); len(ids) != 2 {
		t.Fatalf("projects %v", ids)
	}
	if got, _ := p.SemanticSearch(ctx, "proj", "config file", 3); len(got) == 0 || got[0].ID == "a" {
		t.Fatalf("deleted still found: %+v", got)
	}
}
