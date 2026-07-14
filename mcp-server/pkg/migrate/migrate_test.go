package migrate

import (
	"context"
	"testing"

	"github.com/enowdev/enowx-rag/pkg/rag"
)

// fakeProvider is a minimal in-memory rag.Provider for migration tests. As a
// source it returns exportDocs; as a destination it records Index calls.
type fakeProvider struct {
	exportDocs   []rag.Document
	created      []string
	indexed      []rag.Document
	exportErr    error
}

func (f *fakeProvider) CreateCollection(ctx context.Context, projectID string) error {
	f.created = append(f.created, projectID)
	return nil
}
func (f *fakeProvider) DeleteCollection(ctx context.Context, projectID string) error { return nil }
func (f *fakeProvider) Index(ctx context.Context, projectID string, docs []rag.Document) error {
	f.indexed = append(f.indexed, docs...)
	return nil
}
func (f *fakeProvider) SemanticSearch(ctx context.Context, projectID, query string, limit int) ([]rag.Result, error) {
	return nil, nil
}
func (f *fakeProvider) Embed(ctx context.Context, text string) ([]float32, error) { return nil, nil }
func (f *fakeProvider) DeletePoints(ctx context.Context, projectID string, ids []string) error {
	return nil
}
func (f *fakeProvider) ListPointIDs(ctx context.Context, projectID string, mf map[string]string) ([]string, error) {
	return nil, nil
}
func (f *fakeProvider) ListPoints(ctx context.Context, projectID string, mf map[string]string) ([]rag.PointInfo, error) {
	return nil, nil
}
func (f *fakeProvider) Close() error { return nil }

// ExportPoints implements rag.Exporter.
func (f *fakeProvider) ExportPoints(ctx context.Context, projectID string) ([]rag.Document, error) {
	if f.exportErr != nil {
		return nil, f.exportErr
	}
	return f.exportDocs, nil
}

func TestMigratorRun(t *testing.T) {
	docs := make([]rag.Document, 150)
	for i := range docs {
		docs[i] = rag.Document{ID: "doc" + string(rune('a'+i%26)), Content: "text", Meta: map[string]string{"content_hash": "h"}}
	}
	src := &fakeProvider{exportDocs: docs}
	dst := &fakeProvider{}
	m := &Migrator{Src: src, Dst: dst, BatchSize: 64}

	var progressCalls int
	var lastProgress Progress
	done, err := m.Run(context.Background(), "src-proj", "dst-proj", func(p Progress) {
		progressCalls++
		lastProgress = p
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if done != 150 {
		t.Errorf("migrated %d, want 150", done)
	}
	if len(dst.indexed) != 150 {
		t.Errorf("destination indexed %d docs, want 150", len(dst.indexed))
	}
	if len(dst.created) != 1 || dst.created[0] != "dst-proj" {
		t.Errorf("destination collection not created for dst-proj: %v", dst.created)
	}
	if lastProgress.Done != 150 || lastProgress.Total != 150 {
		t.Errorf("final progress = %+v, want Done/Total 150", lastProgress)
	}
	// doc identity preserved (Meta carried through).
	if dst.indexed[0].Meta["content_hash"] != "h" {
		t.Error("metadata not preserved through migration")
	}
}

// TestMigratorNoSource errors clearly when no source is configured.
func TestMigratorNoSource(t *testing.T) {
	m := &Migrator{Src: nil, Dst: &fakeProvider{}}
	_, err := m.Run(context.Background(), "a", "b", nil)
	if err == nil {
		t.Fatal("expected error when source is nil")
	}
}
