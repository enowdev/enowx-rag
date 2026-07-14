package cloud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSourceExperimental(t *testing.T) {
	if (Source{Provider: "qdrant"}).Experimental() {
		t.Error("qdrant should not be experimental")
	}
	for _, p := range []string{"pinecone", "weaviate", "chroma"} {
		if !(Source{Provider: p}).Experimental() {
			t.Errorf("%s should be experimental", p)
		}
	}
}

// TestPineconeExporter verifies list+fetch parsing and text extraction.
func TestPineconeExporter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/vectors/list":
			w.Write([]byte(`{"vectors":[{"id":"v1"},{"id":"v2"}],"pagination":{}}`))
		case r.URL.Path == "/vectors/fetch":
			w.Write([]byte(`{"vectors":{"v1":{"id":"v1","metadata":{"content":"hello","source_file":"a.go"}},"v2":{"id":"v2","metadata":{"content":"world"}}}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	exp := &pineconeExporter{url: srv.URL, apiKey: "k", index: "idx", textField: "content"}
	docs, err := exp.ExportPoints(context.Background(), "")
	if err != nil {
		t.Fatalf("ExportPoints: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("got %d docs, want 2", len(docs))
	}
	byID := map[string]string{}
	for _, d := range docs {
		byID[d.ID] = d.Content
	}
	if byID["v1"] != "hello" || byID["v2"] != "world" {
		t.Errorf("content mismatch: %v", byID)
	}
}

// TestWeaviateExporter verifies GraphQL response parsing.
func TestWeaviateExporter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"Get":{"Doc":[{"text":"chunk one","source_file":"x.go","_additional":{"id":"id-1"}}]}}}`))
	}))
	defer srv.Close()

	exp := &weaviateExporter{url: srv.URL, class: "Doc", textField: "text"}
	docs, err := exp.ExportPoints(context.Background(), "")
	if err != nil {
		t.Fatalf("ExportPoints: %v", err)
	}
	if len(docs) != 1 || docs[0].Content != "chunk one" || docs[0].ID != "id-1" {
		t.Fatalf("unexpected docs: %+v", docs)
	}
	if docs[0].Meta["source_file"] != "x.go" {
		t.Error("metadata not extracted")
	}
}

// TestChromaCloudExporter verifies /get parsing.
func TestChromaCloudExporter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ids":[["c1"]],"documents":[["doc text"]],"metadatas":[[{"doc_id":"orig-1"}]]}`))
	}))
	defer srv.Close()

	exp := &chromaCloudExporter{url: srv.URL, collection: "col", textField: "content"}
	docs, err := exp.ExportPoints(context.Background(), "")
	if err != nil {
		t.Fatalf("ExportPoints: %v", err)
	}
	if len(docs) != 1 || docs[0].Content != "doc text" {
		t.Fatalf("unexpected docs: %+v", docs)
	}
}

// TestNewExporterUnknown errors on unknown provider.
func TestNewExporterUnknown(t *testing.T) {
	if _, err := NewExporter(context.Background(), Source{Provider: "unknown"}); err == nil {
		t.Fatal("expected error for unknown provider")
	}
}
