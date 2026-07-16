package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

const openaiMaxBatch = 128

// OpenAIEmbeddingClient calls any OpenAI-compatible embeddings endpoint
// (/v1/embeddings). This covers OpenAI itself plus the many providers that
// speak the same protocol — Together, Jina, Mistral, DeepInfra, LiteLLM, a
// local Ollama (/v1), etc. — by pointing BaseURL at the provider and setting
// Model. Dim is sent as "dimensions" when > 0 (honored by models like
// text-embedding-3-*; ignored by providers that don't support it).
type OpenAIEmbeddingClient struct {
	APIKey  string
	Model   string
	Dim     int
	BaseURL string // full embeddings URL, e.g. https://api.openai.com/v1/embeddings
	client  *http.Client
	tokens  atomic.Int64
}

// NewOpenAIEmbeddingClient creates an OpenAI-compatible embedding client.
// baseURL may be the provider root (".../v1"), the full embeddings path
// (".../v1/embeddings"), or empty for OpenAI's default. model is required.
func NewOpenAIEmbeddingClient(apiKey, model, baseURL string, dim int) *OpenAIEmbeddingClient {
	return &OpenAIEmbeddingClient{
		APIKey:  apiKey,
		Model:   model,
		Dim:     dim,
		BaseURL: normalizeOpenAIURL(baseURL),
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

// normalizeOpenAIURL resolves a user-supplied base URL to the embeddings
// endpoint. Empty → OpenAI default. A root or /v1 URL gets /embeddings
// appended; a URL already ending in /embeddings is used as-is.
func normalizeOpenAIURL(u string) string {
	u = strings.TrimRight(strings.TrimSpace(u), "/")
	if u == "" {
		return "https://api.openai.com/v1/embeddings"
	}
	if strings.HasSuffix(u, "/embeddings") {
		return u
	}
	return u + "/embeddings"
}

type openaiRequest struct {
	Input      []string `json:"input"`
	Model      string   `json:"model"`
	Dimensions int      `json:"dimensions,omitempty"`
}

type openaiResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		TotalTokens int64 `json:"total_tokens"`
	} `json:"usage"`
}

// Embed returns one embedding per input text, batching automatically.
func (c *OpenAIEmbeddingClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float32, len(texts))
	for i := 0; i < len(texts); i += openaiMaxBatch {
		end := i + openaiMaxBatch
		if end > len(texts) {
			end = len(texts)
		}
		vecs, err := c.embedBatch(ctx, texts[i:end])
		if err != nil {
			return nil, err
		}
		copy(out[i:end], vecs)
	}
	return out, nil
}

func (c *OpenAIEmbeddingClient) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	body, _ := json.Marshal(openaiRequest{
		Input:      texts,
		Model:      c.Model,
		Dimensions: c.Dim,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embeddings request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai embeddings returned %d: %s", resp.StatusCode, string(b))
	}

	var result openaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode openai response: %w", err)
	}
	c.tokens.Add(result.Usage.TotalTokens)

	vecs := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index >= 0 && d.Index < len(vecs) {
			vecs[d.Index] = d.Embedding
		}
	}
	return vecs, nil
}

// VectorSize returns the configured dimension. When Dim is 0 (provider default,
// unknown ahead of time), it embeds a probe string once to discover the size.
func (c *OpenAIEmbeddingClient) VectorSize() int {
	if c.Dim > 0 {
		return c.Dim
	}
	vecs, err := c.Embed(context.Background(), []string{"dimension probe"})
	if err == nil && len(vecs) == 1 {
		c.Dim = len(vecs[0])
	}
	return c.Dim
}

// ModelName returns the configured model name. Implements ModelNamer.
func (c *OpenAIEmbeddingClient) ModelName() string { return c.Model }

// TokensUsed returns cumulative total_tokens reported by the API. Implements TokenCounter.
func (c *OpenAIEmbeddingClient) TokensUsed() int64 { return c.tokens.Load() }

var (
	_ EmbeddingClient = (*OpenAIEmbeddingClient)(nil)
	_ ModelNamer      = (*OpenAIEmbeddingClient)(nil)
	_ TokenCounter    = (*OpenAIEmbeddingClient)(nil)
)
