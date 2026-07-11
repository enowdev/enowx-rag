package rag

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestChromaSemanticSearchUsesEmbedQuery verifies that when the embedder
// implements QueryEmbedder, Chroma's SemanticSearch calls EmbedQuery (not Embed).
func TestChromaSemanticSearchUsesEmbedQuery(t *testing.T) {
	embedder := &mockQueryEmbedder{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Chroma query endpoint returns results
		resp := map[string]any{
			"ids":        [][]string{{}},
			"distances":  [][]float64{{}},
			"documents":  [][]string{{}},
			"metadatas":  [][]map[string]any{{}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewChromaProvider(srv.URL, embedder)

	_, err := p.SemanticSearch(context.Background(), "testproj", "hello world", 5)
	if err != nil {
		t.Fatalf("SemanticSearch: %v", err)
	}

	if embedder.getQueryCalls() == 0 {
		t.Error("EmbedQuery should have been called")
	}
	if embedder.getEmbedCalls() != 0 {
		t.Error("Embed should NOT have been called when QueryEmbedder is implemented")
	}
}

// TestChromaSemanticSearchFallsBackToEmbed verifies that when the embedder
// does NOT implement QueryEmbedder, Chroma's SemanticSearch falls back to Embed.
func TestChromaSemanticSearchFallsBackToEmbed(t *testing.T) {
	embedder := &mockPlainEmbedder{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"ids":        [][]string{{}},
			"distances":  [][]float64{{}},
			"documents":  [][]string{{}},
			"metadatas":  [][]map[string]any{{}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewChromaProvider(srv.URL, embedder)

	_, err := p.SemanticSearch(context.Background(), "testproj", "hello world", 5)
	if err != nil {
		t.Fatalf("SemanticSearch: %v", err)
	}

	if embedder.getEmbedCalls() == 0 {
		t.Error("Embed should have been called as fallback")
	}
}

// TestChromaSemanticSearchEmbedQueryError verifies that if EmbedQuery returns
// an error, SemanticSearch propagates it.
func TestChromaSemanticSearchEmbedQueryError(t *testing.T) {
	embedder := &mockQueryEmbedder{
		queryErr: context.DeadlineExceeded,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"ids":       [][]string{{}},
			"distances": [][]float64{{}},
			"documents": [][]string{{}},
			"metadatas": [][]map[string]any{{}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewChromaProvider(srv.URL, embedder)

	_, err := p.SemanticSearch(context.Background(), "testproj", "hello world", 5)
	if err == nil {
		t.Fatal("expected error from EmbedQuery failure")
	}
}

// TestChromaSemanticSearchReturnsResults verifies that results are parsed correctly.
func TestChromaSemanticSearchReturnsResults(t *testing.T) {
	embedder := &mockQueryEmbedder{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"ids":       [][]string{{"chunk1", "chunk2"}},
			"distances": [][]float64{{0.1, 0.5}},
			"documents": [][]string{{"content1", "content2"}},
			"metadatas": [][]map[string]any{{{"source_file": "file1.go"}, {"source_file": "file2.go"}}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewChromaProvider(srv.URL, embedder)

	results, err := p.SemanticSearch(context.Background(), "testproj", "hello", 5)
	if err != nil {
		t.Fatalf("SemanticSearch: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "chunk1" {
		t.Errorf("expected first result ID 'chunk1', got '%s'", results[0].ID)
	}
	if results[0].Content != "content1" {
		t.Errorf("expected first result content 'content1', got '%s'", results[0].Content)
	}
	// score = 1 - distance
	if results[0].Score != 0.9 {
		t.Errorf("expected first result score 0.9, got %f", results[0].Score)
	}
	if results[0].Meta["source_file"] != "file1.go" {
		t.Errorf("expected first result meta source_file 'file1.go', got '%s'", results[0].Meta["source_file"])
	}
}
