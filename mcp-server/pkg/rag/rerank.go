package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// voyageRerankURL is the default endpoint for the Voyage AI rerank API.
	voyageRerankURL = "https://api.voyageai.com/v1/rerank"
	// DefaultRerankModel is the default rerank model used by VoyageReranker.
	DefaultRerankModel = "rerank-2.5"
)

// VoyageReranker implements the Reranker interface by calling the
// Voyage AI rerank API (https://api.voyageai.com/v1/rerank).
type VoyageReranker struct {
	APIKey  string
	Model   string // defaults to "rerank-2.5"
	client  *http.Client
	baseURL string // override for testing; defaults to voyageRerankURL
}

// NewVoyageReranker creates a VoyageReranker with the given API key.
// If model is empty, it defaults to "rerank-2.5".
func NewVoyageReranker(apiKey, model string) *VoyageReranker {
	if model == "" {
		model = DefaultRerankModel
	}
	return &VoyageReranker{
		APIKey: apiKey,
		Model:  model,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

// voyageRerankRequest is the JSON body sent to the Voyage rerank API.
type voyageRerankRequest struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	Model     string   `json:"model"`
	TopK      int      `json:"top_k"`
}

// voyageRerankResponse is the JSON response from the Voyage rerank API.
type voyageRerankResponse struct {
	Data []RerankHit `json:"data"`
}

// Rerank sends the query and documents to the Voyage AI rerank API and
// returns a slice of RerankHit containing the index and relevance score
// for each reranked document, truncated to topK.
func (r *VoyageReranker) Rerank(ctx context.Context, query string, docs []string, topK int) ([]RerankHit, error) {
	if len(docs) == 0 {
		return nil, nil
	}

	body, err := json.Marshal(voyageRerankRequest{
		Query:     query,
		Documents: docs,
		Model:     r.Model,
		TopK:      topK,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal rerank request: %w", err)
	}

	url := r.baseURL
	if url == "" {
		url = voyageRerankURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create rerank request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.APIKey)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("voyage rerank request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("voyage rerank returned %d: %s", resp.StatusCode, string(b))
	}

	var result voyageRerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode voyage rerank response: %w", err)
	}

	return result.Data, nil
}

// Compile-time assertion that VoyageReranker implements Reranker.
var _ Reranker = (*VoyageReranker)(nil)
