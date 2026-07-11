package rag

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestNewVoyageEmbeddingClient_Dim verifies that the dim parameter is stored
// correctly and defaults to 1024 when 0 is passed.
func TestNewVoyageEmbeddingClient_Dim(t *testing.T) {
	tests := []struct {
		name    string
		dim     int
		wantDim int
	}{
		{"dim=512", 512, 512},
		{"dim=0 defaults to 1024", 0, 1024},
		{"dim=256", 256, 256},
		{"dim=1024", 1024, 1024},
		{"dim=2048", 2048, 2048},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewVoyageEmbeddingClient("test-key", "voyage-4", tt.dim)
			if c.Dim != tt.wantDim {
				t.Errorf("Dim = %d, want %d", c.Dim, tt.wantDim)
			}
		})
	}
}

// TestVoyageVectorSize verifies that VectorSize returns the configured dim,
// not a hardcoded 1024. Also checks the fallback when Dim is 0.
func TestVoyageVectorSize(t *testing.T) {
	tests := []struct {
		name string
		dim  int
		want int
	}{
		{"dim=256", 256, 256},
		{"dim=512", 512, 512},
		{"dim=1024", 1024, 1024},
		{"dim=0 defaults to 1024", 0, 1024},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewVoyageEmbeddingClient("test-key", "voyage-4", tt.dim)
			if got := c.VectorSize(); got != tt.want {
				t.Errorf("VectorSize() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestVoyageVectorSize_DirectlyZeroDim verifies the fallback path in VectorSize
// when Dim is manually set to 0 (edge case safety).
func TestVoyageVectorSize_DirectlyZeroDim(t *testing.T) {
	c := &VoyageEmbeddingClient{Dim: 0}
	if got := c.VectorSize(); got != 1024 {
		t.Errorf("VectorSize() with Dim=0 = %d, want 1024", got)
	}
}

// TestVoyageOutputDimension verifies that output_dimension is sent in the
// API request body when Dim > 0, using an httptest.Server.
func TestVoyageOutputDimension(t *testing.T) {
	tests := []struct {
		name          string
		dim           int
		wantOutputDim int
	}{
		{"dim=512 sends output_dimension=512", 512, 512},
		{"dim=1024 sends output_dimension=1024", 1024, 1024},
		{"dim=0 defaults to 1024 sends output_dimension=1024", 0, 1024},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedBody map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if err := json.Unmarshal(body, &capturedBody); err != nil {
					t.Errorf("failed to unmarshal request body: %v", err)
				}
				resp := voyageResponse{
					Data: []struct {
						Embedding []float32 `json:"embedding"`
						Index     int       `json:"index"`
					}{
						{Embedding: make([]float32, tt.wantOutputDim), Index: 0},
					},
				}
				json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			c := NewVoyageEmbeddingClient("test-key", "voyage-4", tt.dim)
			c.baseURL = server.URL

			_, err := c.Embed(context.Background(), []string{"hello"})
			if err != nil {
				t.Fatalf("Embed failed: %v", err)
			}

			od, ok := capturedBody["output_dimension"]
			if !ok {
				t.Fatal("output_dimension not found in request body")
			}
			if int(od.(float64)) != tt.wantOutputDim {
				t.Errorf("output_dimension = %v, want %d", od, tt.wantOutputDim)
			}
		})
	}
}

// TestVoyageEmbedQueryOutputDimension verifies that EmbedQuery also sends
// output_dimension in the request body.
func TestVoyageEmbedQueryOutputDimension(t *testing.T) {
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Errorf("failed to unmarshal request body: %v", err)
		}
		resp := voyageResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: make([]float32, 512), Index: 0},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewVoyageEmbeddingClient("test-key", "voyage-4", 512)
	c.baseURL = server.URL

	_, err := c.EmbedQuery(context.Background(), "test query")
	if err != nil {
		t.Fatalf("EmbedQuery failed: %v", err)
	}

	od, ok := capturedBody["output_dimension"]
	if !ok {
		t.Fatal("output_dimension not found in request body")
	}
	if int(od.(float64)) != 512 {
		t.Errorf("output_dimension = %v, want 512", od)
	}
}

// TestVoyageAuthHeader verifies that the Authorization header is set correctly.
func TestVoyageAuthHeader(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		resp := voyageResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: make([]float32, 1024), Index: 0},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewVoyageEmbeddingClient("my-secret-key", "voyage-4", 1024)
	c.baseURL = server.URL

	_, err := c.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	if authHeader != "Bearer my-secret-key" {
		t.Errorf("Authorization header = %q, want %q", authHeader, "Bearer my-secret-key")
	}
}

// TestVoyageModelDefault verifies that model defaults to voyage-4 when empty.
func TestVoyageModelDefault(t *testing.T) {
	c := NewVoyageEmbeddingClient("key", "", 1024)
	if c.Model != "voyage-4" {
		t.Errorf("Model = %q, want %q", c.Model, "voyage-4")
	}
}

// TestVoyageInputType verifies that input_type is set correctly for Embed vs EmbedQuery.
func TestVoyageInputType(t *testing.T) {
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		resp := voyageResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: make([]float32, 1024), Index: 0},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewVoyageEmbeddingClient("test-key", "voyage-4", 1024)
	c.baseURL = server.URL

	// Embed should use input_type=document
	capturedBody = nil
	_, err := c.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	if it, _ := capturedBody["input_type"].(string); it != "document" {
		t.Errorf("Embed input_type = %q, want %q", it, "document")
	}

	// EmbedQuery should use input_type=query
	capturedBody = nil
	_, err = c.EmbedQuery(context.Background(), "test query")
	if err != nil {
		t.Fatalf("EmbedQuery failed: %v", err)
	}
	if it, _ := capturedBody["input_type"].(string); it != "query" {
		t.Errorf("EmbedQuery input_type = %q, want %q", it, "query")
	}
}
