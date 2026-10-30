package indexer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/enowdev/enowx-rag/mcp-server/pkg/rag"
)

// Code is cut at its declarations, each chunk knowing its lines and the
// symbol it starts with, the comment above a declaration kept with it.
func TestChunkCode(t *testing.T) {
	src := "package x\n\nimport \"fmt\"\n\n// Parse reads it.\nfunc Parse(s string) int {\n\treturn len(s)\n}\n\ntype Server struct {\n\tAddr string\n}\n\nfunc (s *Server) Start() {\n\tfmt.Println(s.Addr)\n}\n"
	got := chunkCode(src, "x.go", 70)
	find := func(sym string) Chunk {
		for _, c := range got {
			if c.Symbol == sym {
				return c
			}
		}
		t.Fatalf("no chunk of %s in %+v", sym, got)
		return Chunk{}
	}
	if parse := find("Parse"); parse.Start != 5 || !strings.HasPrefix(parse.Text, "// Parse reads it.") {
		t.Fatalf("Parse: %+v", parse)
	}
	if start := find("Start"); start.Start != 14 || !strings.HasPrefix(start.Text, "func (s *Server) Start()") {
		t.Fatalf("Start: %+v", start)
	}
	for _, c := range got {
		if len(c.Text) > 70 {
			t.Fatalf("over the size: %+v", c)
		}
	}
	// Small ones pack together within the size.
	if big := chunkCode(src, "x.go", 1000); len(big) != 1 || big[0].Start != 1 {
		t.Fatalf("packed: %+v", big)
	}
	// A language without declarations known: blank lines.
	if txt := chunkCode("a\nb\n\nc\nd\n", "notes.txt", 4); len(txt) < 2 {
		t.Fatalf("paragraphs: %+v", txt)
	}
}

// recording keeps what it's given to index.
type recording struct {
	rag.Provider
	mu   sync.Mutex
	docs []rag.Document
}

func (r *recording) Index(_ context.Context, _ string, docs []rag.Document) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.docs = append(r.docs, docs...)
	return nil
}

func (r *recording) ListPoints(context.Context, string, map[string]string) ([]rag.PointInfo, error) {
	return nil, nil
}

// A repository's ignored files and its secrets are never indexed, nor a
// chunk holding a key.
func TestIndexSkipsIgnoredAndSecrets(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755)
		os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644)
	}
	write(".gitignore", "build/\n*.log\n")
	write("main.go", "package main\n\nfunc main() {}\n")
	write("build/out.go", "package build\n")
	write("debug.log", "x")
	write(".env", "TOKEN=abc\n")
	write("certs/server.pem", "x")
	write("config/keys.go", "package config\n\nconst key = \"AKIAABCDEFGHIJKLMNOP\"\n")
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git: %v %s", err, out)
	}
	rec := &recording{}
	idx := NewIndexer(rec, 1000)
	if _, err := idx.IndexProject(context.Background(), "p", dir); err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, d := range rec.docs {
		files = append(files, d.Meta["source_file"])
		if strings.Contains(d.Content, "AKIA") {
			t.Fatal("a chunk with a key was indexed")
		}
	}
	got := strings.Join(files, ",")
	if !strings.Contains(got, "main.go") || strings.Contains(got, "build/") || strings.Contains(got, ".env") || strings.Contains(got, ".pem") || strings.Contains(got, ".log") {
		t.Fatalf("indexed %s", got)
	}
	if rec.docs[0].Meta["start_line"] == "" || !strings.HasPrefix(rec.docs[0].Content, "File: ") {
		t.Fatalf("lines: %+v", rec.docs[0])
	}
}
