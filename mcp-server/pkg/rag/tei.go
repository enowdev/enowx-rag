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

// teiMaxBatch is the maximum number of inputs TEI accepts per /embed request.
// TEI's default MAX_CLIENT_BATCH_SIZE is 32; larger requests are rejected with
// HTTP 413. Embed transparently splits oversized input into sub-batches so
// callers never have to care about this limit.
const teiMaxBatch = 32

// TEIEmbeddingClient talks to a Hugging Face Text Embeddings Inference server.
type TEIEmbeddingClient struct {
	BaseURL string
	Client  *http.Client
}

type teiEmbedRequest struct {
	Inputs  []string `json:"inputs"`
	Options struct {
		WaitForModel bool `json:"wait_for_model"`
	} `json:"options"`
}

type teiEmbedResponse struct {
	Embedding []float32 `json:"embedding"`
}

// NewTEIEmbeddingClient creates a client for the given TEI base URL.
func NewTEIEmbeddingClient(baseURL string) *TEIEmbeddingClient {
	return &TEIEmbeddingClient{
		BaseURL: baseURL,
		Client:  &http.Client{Timeout: 120 * time.Second},
	}
}

// Embed returns one embedding per input text. Inputs larger than teiMaxBatch
// are automatically split into multiple TEI requests and reassembled in order.
func (c *TEIEmbeddingClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float32, 0, len(texts))
	for i := 0; i < len(texts); i += teiMaxBatch {
		end := i + teiMaxBatch
		if end > len(texts) {
			end = len(texts)
		}
		vecs, err := c.embedBatch(ctx, texts[i:end])
		if err != nil {
			return nil, err
		}
		out = append(out, vecs...)
	}
	return out, nil
}

// embedBatch sends a single /embed request. The caller must ensure len(texts)
// does not exceed TEI's MAX_CLIENT_BATCH_SIZE.
func (c *TEIEmbeddingClient) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	body := teiEmbedRequest{Inputs: texts}
	body.Options.WaitForModel = true

	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/embed", buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tei request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tei returned %d: %s", resp.StatusCode, string(b))
	}

	// Try to decode as a batch of embeddings first: [[...], [...]].
	var rawBatch []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&rawBatch); err != nil {
		return nil, fmt.Errorf("decode embed response: %w", err)
	}
	if len(rawBatch) != len(texts) {
		return nil, fmt.Errorf("unexpected embedding count: got %d, want %d", len(rawBatch), len(texts))
	}

	out := make([][]float32, len(rawBatch))
	for i, raw := range rawBatch {
		// Each entry may be either [...] or {"embedding": [...]}.
		var vec []float32
		if err := json.Unmarshal(raw, &vec); err == nil {
			out[i] = vec
			continue
		}
		var r teiEmbedResponse
		if err := json.Unmarshal(raw, &r); err != nil {
			return nil, fmt.Errorf("decode embedding %d: %w", i, err)
		}
		out[i] = r.Embedding
	}
	return out, nil
}

func (c *TEIEmbeddingClient) embedSingle(ctx context.Context, text string) ([]float32, error) {
	body := map[string]any{
		"inputs": text,
		"options": map[string]any{
			"wait_for_model": true,
		},
	}
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/embed", buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var r teiEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return r.Embedding, nil
}

// VectorSize returns the expected embedding dimension. TEI exposes it via /info.
func (c *TEIEmbeddingClient) VectorSize() int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/info", nil)
	if err != nil {
		return 384 // default for BAAI/bge-small-en-v1.5
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return 384
	}
	defer resp.Body.Close()

	var info struct {
		MaxInputLength  int `json:"max_input_length"`
		MaxBatchTokens  int `json:"max_batch_tokens"`
		TokenLength     int `json:"token_length"`
		Dim             int `json:"dim"`
		EstimatedMemory int `json:"estimated_memory"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err == nil && info.Dim > 0 {
		return info.Dim
	}
	return 384
}
