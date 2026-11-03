package core

import (
	"context"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/enowdev/enowx-rag/mcp-server/pkg/indexer"
	"github.com/enowdev/enowx-rag/mcp-server/pkg/rag"
)

// wordEmbedder embeds texts as hashed word buckets: enough for a search on
// a distinctive word to find the chunk holding it.
type wordEmbedder struct{}

func (wordEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v := make([]float32, 128)
		for _, w := range strings.FieldsFunc(strings.ToLower(t), func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
		}) {
			h := fnv.New32a()
			h.Write([]byte(w))
			v[h.Sum32()%128]++
		}
		out[i] = v
	}
	return out, nil
}

func (wordEmbedder) VectorSize() int { return 128 }

func newSyncService(t *testing.T, every time.Duration) (*Service, *DirRegistry, string) {
	t.Helper()
	p, err := rag.NewSQLiteProvider(filepath.Join(t.TempDir(), "rag.db"), wordEmbedder{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close() })
	svc := NewService(p, nil, indexer.NewIndexer(p, 1000))
	regPath := filepath.Join(t.TempDir(), "project_dirs.json")
	reg, err := NewDirRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	svc.EnableAutoSync(reg, every)
	return svc, reg, regPath
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func searchHas(t *testing.T, svc *Service, query, want string) bool {
	t.Helper()
	res, err := svc.Search(context.Background(), "p", query, SearchOpts{K: 3})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if strings.Contains(r.Content, want) {
			return true
		}
	}
	return false
}

// After a directory is indexed once, a search sees later edits without the
// agent calling rag_index_project again, including code that was removed.
func TestAutoSync_SearchSeesEditsWithoutReindex(t *testing.T) {
	svc, reg, regPath := newSyncService(t, time.Nanosecond) // always due
	dir := t.TempDir()
	file := filepath.Join(dir, "billing.go")
	writeFile(t, file, "package billing\n\nfunc ChargeCard() {\n\t// legacy stripe charge flow\n}\n")
	if _, err := svc.IndexProject(context.Background(), "p", dir); err != nil {
		t.Fatal(err)
	}
	if got := reg.Dirs("p"); len(got) != 1 {
		t.Fatalf("directory not recorded: %v", got)
	}

	writeFile(t, file, "package billing\n\nfunc RefundInvoice() {\n\t// zebra refund flow\n}\n")

	if !searchHas(t, svc, "zebra refund", "RefundInvoice") {
		t.Error("search did not see the new function")
	}
	if searchHas(t, svc, "legacy stripe charge", "ChargeCard") {
		t.Error("search still returns the removed function")
	}

	// The registry survives a restart (a new stdio session).
	reopened, err := NewDirRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Dirs("p"); len(got) != 1 || got[0] != dir {
		t.Errorf("registry not persisted: %v", got)
	}
}

// Within the interval no re-sync happens; with auto-sync off, never.
func TestAutoSync_RespectsIntervalAndOff(t *testing.T) {
	for _, tc := range []struct {
		name  string
		every time.Duration
	}{{"interval not elapsed", time.Hour}, {"disabled", 0}} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _ := newSyncService(t, tc.every)
			dir := t.TempDir()
			file := filepath.Join(dir, "a.go")
			writeFile(t, file, "package a\n\nfunc Alpha() {}\n")
			if _, err := svc.IndexProject(context.Background(), "p", dir); err != nil {
				t.Fatal(err)
			}
			writeFile(t, file, "package a\n\nfunc Quokka() {}\n")
			if searchHas(t, svc, "quokka", "Quokka") {
				t.Error("re-synced although not due / disabled")
			}
		})
	}
}

// A recorded directory that disappeared is skipped: the search still works
// and the index is not wiped.
func TestAutoSync_MissingDirKeepsIndex(t *testing.T) {
	svc, _, _ := newSyncService(t, time.Nanosecond)
	parent := t.TempDir()
	dir := filepath.Join(parent, "repo")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "a.go"), "package a\n\nfunc Pelican() {}\n")
	if _, err := svc.IndexProject(context.Background(), "p", dir); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if !searchHas(t, svc, "pelican", "Pelican") {
		t.Error("index lost after its directory disappeared")
	}
}

// Deleting a project forgets its directories.
func TestAutoSync_DeleteProjectForgetsDirs(t *testing.T) {
	svc, reg, _ := newSyncService(t, time.Hour)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.go"), "package a\n\nfunc A() {}\n")
	if _, err := svc.IndexProject(context.Background(), "p", dir); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteProject(context.Background(), "p"); err != nil {
		t.Fatal(err)
	}
	if got := reg.Dirs("p"); len(got) != 0 {
		t.Errorf("dirs kept after delete: %v", got)
	}
}

func TestParseAutoSyncInterval(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"": DefaultAutoSyncInterval, "  ": DefaultAutoSyncInterval,
		"0": 0, "off": 0, "OFF": 0, "false": 0,
		"90": 90 * time.Second, "90s": 90 * time.Second, "5m": 5 * time.Minute,
	} {
		got, err := ParseAutoSyncInterval(in)
		if err != nil || got != want {
			t.Errorf("ParseAutoSyncInterval(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"soon", "-5", "-1m"} {
		if _, err := ParseAutoSyncInterval(bad); err == nil {
			t.Errorf("ParseAutoSyncInterval(%q): want an error", bad)
		}
	}
}

// Two processes sharing the registry file see each other's changes.
func TestDirRegistry_SharedBetweenProcesses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project_dirs.json")
	r1, err := NewDirRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := NewDirRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := r1.Add("one", "/tmp/one"); err != nil {
		t.Fatal(err)
	}
	if err := r2.Add("two", "/tmp/two"); err != nil {
		t.Fatal(err)
	}
	if got := r1.Dirs("two"); len(got) != 1 {
		t.Errorf("r1 does not see r2's project: %v", got)
	}
	if err := r2.Remove("one"); err != nil {
		t.Fatal(err)
	}
	if got := r1.Dirs("one"); len(got) != 0 {
		t.Errorf("r1 still sees a project r2 removed: %v", got)
	}
	reread, _ := NewDirRegistry(path)
	if len(reread.Dirs("two")) != 1 || len(reread.Dirs("one")) != 0 {
		t.Errorf("file lost a change: one=%v two=%v", reread.Dirs("one"), reread.Dirs("two"))
	}
}

// A project deleted elsewhere (another process, the dashboard) is not
// recreated by a search that still has its directory registered.
func TestAutoSync_DeletedProjectNotRecreated(t *testing.T) {
	svc, _, _ := newSyncService(t, time.Nanosecond)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.go"), "package a\n\nfunc Heron() {}\n")
	if _, err := svc.IndexProject(context.Background(), "p", dir); err != nil {
		t.Fatal(err)
	}
	// Deleted behind this Service's back: the registry still lists the dir.
	if err := svc.Provider().DeleteCollection(context.Background(), "p"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Search(context.Background(), "p", "heron", SearchOpts{K: 3}); err != nil {
		t.Fatal(err)
	}
	if svc.ProjectExists(context.Background(), "p") {
		t.Error("auto-sync recreated a deleted project")
	}
}

// A second directory with the same base name as one already indexed into
// the project is refused: their chunks share an ID prefix.
func TestIndexProject_RefusesSecondDirWithSameName(t *testing.T) {
	svc, _, _ := newSyncService(t, time.Hour)
	a := filepath.Join(t.TempDir(), "app")
	b := filepath.Join(t.TempDir(), "app")
	for _, d := range []string{a, b} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(a, "a.go"), "package a\n\nfunc Ibis() {}\n")
	writeFile(t, filepath.Join(b, "b.go"), "package b\n\nfunc Kiwi() {}\n")
	if _, err := svc.IndexProject(context.Background(), "p", a); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.IndexProject(context.Background(), "p", b); err == nil {
		t.Error("want an error for a second directory named app")
	}
	if !searchHas(t, svc, "ibis", "Ibis") {
		t.Error("first directory's chunks lost")
	}
	// Re-indexing the same directory is fine.
	if _, err := svc.IndexProject(context.Background(), "p", a); err != nil {
		t.Errorf("re-index of the same directory refused: %v", err)
	}
}

// If the registry already holds two same-named directories for a project
// (recorded by an older version), auto-sync skips them rather than letting
// one replace the other's chunks on every search.
func TestAutoSync_SkipsDirsSharingABaseName(t *testing.T) {
	svc, reg, _ := newSyncService(t, time.Nanosecond)
	a := filepath.Join(t.TempDir(), "app")
	b := filepath.Join(t.TempDir(), "app")
	for _, d := range []string{a, b} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(a, "a.go"), "package a\n\nfunc Ibis() {}\n")
	writeFile(t, filepath.Join(b, "b.go"), "package b\n\nfunc Kiwi() {}\n")
	if _, err := svc.IndexProject(context.Background(), "p", a); err != nil {
		t.Fatal(err)
	}
	if err := reg.Add("p", b); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(a, "a.go"), "package a\n\nfunc Ibis() {}\n\nfunc Narwhal() {}\n")
	if searchHas(t, svc, "narwhal", "Narwhal") {
		t.Error("auto-synced a directory whose base name is shared")
	}
	if !searchHas(t, svc, "ibis", "Ibis") {
		t.Error("a directory's chunks were replaced by the other one's")
	}
}
