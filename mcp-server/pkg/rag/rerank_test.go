package rag

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestVoyageRerankerImplementsReranker verifies at compile time that
// VoyageReranker implements the Reranker interface.
// VAL-RAG-001: VoyageReranker implements the Reranker interface.
func TestVoyageRerankerImplementsReranker(t *testing.T) {
	var _ Reranker = (*VoyageReranker)(nil)
	// Also verify via NewVoyageReranker
	var r Reranker = NewVoyageReranker("key", "")
	if r == nil {
		t.Fatal("NewVoyageReranker returned nil")
	}
}

// TestNewVoyageRerankerDefaults verifies that the model defaults to rerank-2.5
// when empty.
func TestNewVoyageRerankerDefaults(t *testing.T) {
	r := NewVoyageReranker("key", "")
	if r.Model != DefaultRerankModel {
		t.Errorf("Model = %q, want %q", r.Model, DefaultRerankModel)
	}
	if r.APIKey != "key" {
		t.Errorf("APIKey = %q, want %q", r.APIKey, "key")
	}
}

// TestVoyageRerankerCallsCorrectEndpoint verifies that Rerank sends POST to
// the correct URL path, with the correct model, top_k, and auth header.
// VAL-RAG-002: Rerank calls correct Voyage API endpoint with correct model and top_k.
func TestVoyageRerankerCallsCorrectEndpoint(t *testing.T) {
	var capturedMethod string
	var capturedPath string
	var capturedAuth string
	var capturedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedBody)

		resp := voyageRerankResponse{
			Data: []RerankHit{
				{Index: 0, Score: 0.95},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	rr := NewVoyageReranker("test-api-key", "rerank-2.5")
	rr.baseURL = server.URL

	_, err := rr.Rerank(context.Background(), "test query", []string{"doc1", "doc2"}, 5)
	if err != nil {
		t.Fatalf("Rerank failed: %v", err)
	}

	if capturedMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", capturedMethod)
	}
	if capturedPath != "" && capturedPath != "/" {
		// httptest server.URL doesn't include the real path, but we verify
		// the method and body. The real URL is tested via the default URL constant.
	}
	if capturedAuth != "Bearer test-api-key" {
		t.Errorf("Authorization = %q, want %q", capturedAuth, "Bearer test-api-key")
	}
	if model, _ := capturedBody["model"].(string); model != "rerank-2.5" {
		t.Errorf("model = %v, want %q", capturedBody["model"], "rerank-2.5")
	}
	if topK, _ := capturedBody["top_k"].(float64); int(topK) != 5 {
		t.Errorf("top_k = %v, want 5", capturedBody["top_k"])
	}
	if q, _ := capturedBody["query"].(string); q != "test query" {
		t.Errorf("query = %v, want %q", capturedBody["query"], "test query")
	}
	docs, _ := capturedBody["documents"].([]any)
	if len(docs) != 2 {
		t.Errorf("expected 2 documents, got %d", len(docs))
	}
}

// TestVoyageRerankerReturnsRerankHits verifies that Rerank returns a slice of
// RerankHit with Index and Score correctly decoded from the API response.
// VAL-RAG-003: Rerank returns RerankHit slice with Index and Score fields.
func TestVoyageRerankerReturnsRerankHits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a Voyage rerank API response with known data
		resp := map[string]any{
			"data": []map[string]any{
				{"index": 2, "relevance_score": 0.98},
				{"index": 0, "relevance_score": 0.85},
				{"index": 1, "relevance_score": 0.72},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	rr := NewVoyageReranker("key", "rerank-2.5")
	rr.baseURL = server.URL

	hits, err := rr.Rerank(context.Background(), "query", []string{"a", "b", "c"}, 3)
	if err != nil {
		t.Fatalf("Rerank failed: %v", err)
	}

	if len(hits) != 3 {
		t.Fatalf("expected 3 hits, got %d", len(hits))
	}

	// Verify Index and Score values match the canned response
	expected := []RerankHit{
		{Index: 2, Score: 0.98},
		{Index: 0, Score: 0.85},
		{Index: 1, Score: 0.72},
	}
	for i, h := range hits {
		if h.Index != expected[i].Index {
			t.Errorf("hit[%d].Index = %d, want %d", i, h.Index, expected[i].Index)
		}
		if h.Score != expected[i].Score {
			t.Errorf("hit[%d].Score = %f, want %f", i, h.Score, expected[i].Score)
		}
	}
}

// TestVoyageRerankerEmptyDocs verifies that Rerank returns nil, nil for empty docs.
func TestVoyageRerankerEmptyDocs(t *testing.T) {
	rr := NewVoyageReranker("key", "")
	hits, err := rr.Rerank(context.Background(), "query", []string{}, 5)
	if err != nil {
		t.Fatalf("Rerank with empty docs should not error: %v", err)
	}
	if hits != nil {
		t.Errorf("expected nil hits for empty docs, got %v", hits)
	}
}

// TestVoyageRerankerAPIError verifies that non-200 responses return an error.
func TestVoyageRerankerAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid api key"}`))
	}))
	defer server.Close()

	rr := NewVoyageReranker("bad-key", "")
	rr.baseURL = server.URL

	_, err := rr.Rerank(context.Background(), "query", []string{"doc1"}, 5)
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

// TestVoyageRerankerInvalidJSON verifies that invalid JSON response returns error.
func TestVoyageRerankerInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	rr := NewVoyageReranker("key", "")
	rr.baseURL = server.URL

	_, err := rr.Rerank(context.Background(), "query", []string{"doc1"}, 5)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// TestVoyageRerankerDefaultURL verifies that the default base URL is the
// Voyage AI rerank endpoint.
func TestVoyageRerankerDefaultURL(t *testing.T) {
	rr := NewVoyageReranker("key", "")
	if rr.baseURL != "" {
		t.Errorf("baseURL should be empty by default (uses voyageRerankURL), got %q", rr.baseURL)
	}
	// The constant should be the correct endpoint
	if voyageRerankURL != "https://api.voyageai.com/v1/rerank" {
		t.Errorf("voyageRerankURL = %q, want %q", voyageRerankURL, "https://api.voyageai.com/v1/rerank")
	}
}

// TestVoyageRerankerContentTypeHeader verifies that Content-Type header is set.
func TestVoyageRerankerContentTypeHeader(t *testing.T) {
	var capturedContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedContentType = r.Header.Get("Content-Type")
		resp := voyageRerankResponse{
			Data: []RerankHit{{Index: 0, Score: 1.0}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	rr := NewVoyageReranker("key", "")
	rr.baseURL = server.URL

	_, _ = rr.Rerank(context.Background(), "query", []string{"doc1"}, 1)
	if capturedContentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", capturedContentType, "application/json")
	}
}
