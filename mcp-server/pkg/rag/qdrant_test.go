package rag

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestQdrantSemanticSearchUsesEmbedQuery verifies that when the embedder
// implements QueryEmbedder, Qdrant's SemanticSearch calls EmbedQuery (not Embed).
func TestQdrantSemanticSearchUsesEmbedQuery(t *testing.T) {
	embedder := &mockQueryEmbedder{}

	// Mock Qdrant server
	var searchBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path != "" {
			// Capture search request body
			if r.URL.Path != "/healthz" {
				body := make(map[string]any)
				_ = json.NewDecoder(r.Body).Decode(&body)
				searchBody = body
			}
		}
		// Return empty search results
		resp := map[string]any{
			"result": []any{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p, err := NewQdrantProvider(context.Background(), srv.URL, "", embedder)
	if err != nil {
		t.Fatalf("NewQdrantProvider: %v", err)
	}
	defer p.Close()

	_, err = p.SemanticSearch(context.Background(), "testproj", "hello world", 5)
	if err != nil {
		t.Fatalf("SemanticSearch: %v", err)
	}

	if embedder.getQueryCalls() == 0 {
		t.Error("EmbedQuery should have been called")
	}
	if embedder.getEmbedCalls() != 0 {
		t.Error("Embed should NOT have been called when QueryEmbedder is implemented")
	}

	// Verify the search request used the query vector (0.4, 0.5, 0.6)
	if searchBody == nil {
		t.Fatal("search request body was not captured")
	}
	vec, ok := searchBody["vector"].([]any)
	if !ok {
		t.Fatal("search request should contain a vector")
	}
	if len(vec) != 3 {
		t.Fatalf("expected 3-dimensional vector, got %d", len(vec))
	}
	// First element should be 0.4 (from EmbedQuery), not 0.1 (from Embed)
	// float32(0.4) becomes 0.4000000059604645 when converted to float64 by JSON
	firstElem, ok := vec[0].(float64)
	if !ok {
		t.Fatalf("expected vector[0] to be float64, got %T", vec[0])
	}
	if math.Abs(firstElem-float64(float32(0.4))) > 1e-6 {
		t.Errorf("expected vector[0]≈0.4 (from EmbedQuery), got %v", vec[0])
	}
}

// TestQdrantSemanticSearchFallsBackToEmbed verifies that when the embedder
// does NOT implement QueryEmbedder, Qdrant's SemanticSearch falls back to Embed.
func TestQdrantSemanticSearchFallsBackToEmbed(t *testing.T) {
	embedder := &mockPlainEmbedder{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		resp := map[string]any{
			"result": []any{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p, err := NewQdrantProvider(context.Background(), srv.URL, "", embedder)
	if err != nil {
		t.Fatalf("NewQdrantProvider: %v", err)
	}
	defer p.Close()

	_, err = p.SemanticSearch(context.Background(), "testproj", "hello world", 5)
	if err != nil {
		t.Fatalf("SemanticSearch: %v", err)
	}

	if embedder.getEmbedCalls() == 0 {
		t.Error("Embed should have been called as fallback")
	}
}

// TestQdrantSemanticSearchEmbedQueryError verifies that if EmbedQuery returns
// an error, SemanticSearch propagates it.
func TestQdrantSemanticSearchEmbedQueryError(t *testing.T) {
	embedder := &mockQueryEmbedder{
		queryErr: context.DeadlineExceeded,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"result": []any{}})
	}))
	defer srv.Close()

	p, err := NewQdrantProvider(context.Background(), srv.URL, "", embedder)
	if err != nil {
		t.Fatalf("NewQdrantProvider: %v", err)
	}
	defer p.Close()

	_, err = p.SemanticSearch(context.Background(), "testproj", "hello world", 5)
	if err == nil {
		t.Fatal("expected error from EmbedQuery failure")
	}
}

// TestQdrantListProjectIDs verifies that ListProjectIDs lists collections and
// strips the "project_" prefix, implementing core.ProjectLister for Qdrant.
func TestQdrantListProjectIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/collections" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"result":{"collections":[{"name":"project_enowx-rag"},{"name":"project_demo"},{"name":"other_thing"}]}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p, err := NewQdrantProvider(context.Background(), srv.URL, "", &mockQueryEmbedder{})
	if err != nil {
		t.Fatalf("NewQdrantProvider: %v", err)
	}
	defer p.Close()

	ids, err := p.ListProjectIDs(context.Background())
	if err != nil {
		t.Fatalf("ListProjectIDs: %v", err)
	}
	// Only project_-prefixed collections, prefix stripped; "other_thing" excluded.
	if len(ids) != 2 {
		t.Fatalf("expected 2 project IDs, got %d: %v", len(ids), ids)
	}
	want := map[string]bool{"enowx-rag": true, "demo": true}
	for _, id := range ids {
		if !want[id] {
			t.Errorf("unexpected project ID %q", id)
		}
	}
}

// TestQdrantExportPoints verifies export returns FULL content (not truncated to
// 200 chars) and preserves doc_id + metadata.
func TestQdrantExportPoints(t *testing.T) {
	longContent := ""
	for i := 0; i < 500; i++ {
		longContent += "x"
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{"result": map[string]any{
			"points": []map[string]any{
				{"id": "uuid-1", "payload": map[string]any{"content": longContent, "doc_id": "file.go#chunk0", "source_file": "file.go", "content_hash": "h1"}},
			},
			"next_page_offset": nil,
		}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p, err := NewQdrantProvider(context.Background(), srv.URL, "", &mockQueryEmbedder{})
	if err != nil {
		t.Fatalf("NewQdrantProvider: %v", err)
	}
	docs, err := p.ExportPoints(context.Background(), "proj")
	if err != nil {
		t.Fatalf("ExportPoints: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
	if len(docs[0].Content) != 500 {
		t.Errorf("content truncated: got %d chars, want 500 (export must not truncate)", len(docs[0].Content))
	}
	if docs[0].ID != "file.go#chunk0" {
		t.Errorf("ID = %q, want doc_id preserved", docs[0].ID)
	}
	if docs[0].Meta["content_hash"] != "h1" {
		t.Error("metadata not preserved")
	}
}

// TestQdrantSearchMetaExcludesContent is the token-cost regression guard: the
// document text is promoted to Result.Content, so it must not also appear in
// Result.Meta. Returning both ships the same text twice in every response.
func TestQdrantSearchMetaExcludesContent(t *testing.T) {
	const docText = "Title: fleet-notes\n\nThe SCTB counter tracks secret fish."

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		resp := map[string]any{
			"result": []any{
				map[string]any{
					"id":    "abc-123",
					"score": 0.87,
					"payload": map[string]any{
						"content": docText,
						"doc_id":  "memory/trackstat/fleet-notes",
						"bucket":  "trackstat",
						"kind":    "ref",
						"ts":      "2026-08-04T10:00+07:00",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p, err := NewQdrantProvider(context.Background(), srv.URL, "", &mockQueryEmbedder{})
	if err != nil {
		t.Fatalf("NewQdrantProvider: %v", err)
	}
	defer p.Close()

	results, err := p.SemanticSearch(context.Background(), "memory", "sctb", 5)
	if err != nil {
		t.Fatalf("SemanticSearch: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]

	if r.Content != docText {
		t.Errorf("Content = %q, want the document text", r.Content)
	}
	if _, present := r.Meta["content"]; present {
		t.Error(`Meta["content"] is set: document text returned twice in one response`)
	}
	for k, v := range r.Meta {
		if v == docText {
			t.Errorf("Meta[%q] duplicates the document text", k)
		}
	}

	// The genuinely useful metadata must survive the exclusion.
	for k, want := range map[string]string{
		"bucket": "trackstat",
		"kind":   "ref",
		"doc_id": "memory/trackstat/fleet-notes",
	} {
		if got := r.Meta[k]; got != want {
			t.Errorf("Meta[%q] = %q, want %q", k, got, want)
		}
	}
}

// TestQdrantListPointsSurfacesMeta verifies that ListPoints exposes the
// bucket/kind/ts tags the agent-memory workflow filters on, while still
// keeping the full document text out of Meta.
func TestQdrantListPointsSurfacesMeta(t *testing.T) {
	const docText = "Title: deploy-log\n\nDeployed at 03:00."

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		resp := map[string]any{
			"result": map[string]any{
				"points": []any{
					map[string]any{
						"id": "pt-1",
						"payload": map[string]any{
							"content":     docText,
							"doc_id":      "memory/travelya/deploy-log",
							"bucket":      "travelya",
							"kind":        "log",
							"ts":          "2026-08-04T03:00+07:00",
							"source_file": "notes.md",
						},
					},
				},
				"next_page_offset": nil,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p, err := NewQdrantProvider(context.Background(), srv.URL, "", &mockQueryEmbedder{})
	if err != nil {
		t.Fatalf("NewQdrantProvider: %v", err)
	}
	defer p.Close()

	points, err := p.ListPoints(context.Background(), "memory", map[string]string{"bucket": "travelya"})
	if err != nil {
		t.Fatalf("ListPoints: %v", err)
	}
	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(points))
	}
	pt := points[0]

	if pt.Meta["bucket"] != "travelya" || pt.Meta["kind"] != "log" {
		t.Errorf("Meta missing bucket/kind tags: %v", pt.Meta)
	}
	if pt.Meta["ts"] == "" {
		t.Error("Meta[\"ts\"] missing: memory listings sort on it")
	}
	if _, present := pt.Meta["content"]; present {
		t.Error(`Meta["content"] is set: full text leaked into the listing`)
	}
	// Fields promoted to their own struct field must not be echoed in Meta.
	for _, k := range []string{"source_file", "doc_id"} {
		if _, present := pt.Meta[k]; present {
			t.Errorf("Meta[%q] duplicates a promoted PointInfo field", k)
		}
	}
	if pt.SourceFile != "notes.md" || pt.DocID != "memory/travelya/deploy-log" {
		t.Errorf("promoted fields not populated: %+v", pt)
	}
}
