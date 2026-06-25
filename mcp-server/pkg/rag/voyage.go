package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	voyageAPIURL   = "https://api.voyageai.com/v1/embeddings"
	voyageMaxBatch = 128
)

// VoyageEmbeddingClient calls the Voyage AI embeddings API.
type VoyageEmbeddingClient struct {
	APIKey string
	Model  string
	client *http.Client
}

// NewVoyageEmbeddingClient creates a Voyage AI embedding client.
// model defaults to "voyage-3.5" if empty.
func NewVoyageEmbeddingClient(apiKey, model string) *VoyageEmbeddingClient {
	if model == "" {
		model = "voyage-4"
	}
	return &VoyageEmbeddingClient{
		APIKey: apiKey,
		Model:  model,
		client: &http.Client{},
	}
}

type voyageRequest struct {
	Input     []string `json:"input"`
	Model     string   `json:"model"`
	InputType string   `json:"input_type,omitempty"`
}

type voyageResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

// Embed returns one embedding per input text.
// Splits into batches of voyageMaxBatch automatically.
func (c *VoyageEmbeddingClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return c.embedWithType(ctx, texts, "document")
}

// EmbedQuery embeds a single query text with input_type=query for better retrieval.
func (c *VoyageEmbeddingClient) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vecs, err := c.embedWithType(ctx, []string{text}, "query")
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}
	return vecs[0], nil
}

func (c *VoyageEmbeddingClient) embedWithType(ctx context.Context, texts []string, inputType string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float32, len(texts))
	for i := 0; i < len(texts); i += voyageMaxBatch {
		end := i + voyageMaxBatch
		if end > len(texts) {
			end = len(texts)
		}
		vecs, err := c.embedBatch(ctx, texts[i:end], inputType)
		if err != nil {
			return nil, err
		}
		copy(out[i:end], vecs)
	}
	return out, nil
}

func (c *VoyageEmbeddingClient) embedBatch(ctx context.Context, texts []string, inputType string) ([][]float32, error) {
	body, _ := json.Marshal(voyageRequest{
		Input:     texts,
		Model:     c.Model,
		InputType: inputType,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, voyageAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("voyage request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("voyage returned %d: %s", resp.StatusCode, string(b))
	}

	var result voyageResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode voyage response: %w", err)
	}

	vecs := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index < len(vecs) {
			vecs[d.Index] = d.Embedding
		}
	}
	return vecs, nil
}

// VectorSize returns 1024 (voyage-3.5 default dimension).
func (c *VoyageEmbeddingClient) VectorSize() int {
	return 1024
}
