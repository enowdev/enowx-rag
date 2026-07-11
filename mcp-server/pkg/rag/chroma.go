package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ChromaProvider implements a lightweight REST Chroma provider with no generated client.
// It assumes the embedding is done via an external TEI/OpenAI client.
type ChromaProvider struct {
	baseURL  string
	embedder EmbeddingClient
	client   *http.Client
}

// NewChromaProvider creates a Chroma provider at the given base URL.
func NewChromaProvider(baseURL string, embedder EmbeddingClient) *ChromaProvider {
	return &ChromaProvider{
		baseURL:  strings.TrimSuffix(baseURL, "/"),
		embedder: embedder,
		client:   &http.Client{},
	}
}

func (p *ChromaProvider) collectionName(projectID string) string {
	return "project_" + sanitize(projectID)
}

func (p *ChromaProvider) CreateCollection(ctx context.Context, projectID string) error {
	body := map[string]any{
		"name":         p.collectionName(projectID),
		"metadata":     map[string]string{"project_id": projectID},
		"get_or_create": true,
	}
	return p.do(ctx, http.MethodPost, "/api/v1/collections", body, nil)
}

func (p *ChromaProvider) DeleteCollection(ctx context.Context, projectID string) error {
	return p.do(ctx, http.MethodDelete, "/api/v1/collections/"+p.collectionName(projectID), nil, nil)
}

func (p *ChromaProvider) Index(ctx context.Context, projectID string, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}
	name := p.collectionName(projectID)

	texts := make([]string, len(docs))
	ids := make([]string, len(docs))
	metas := make([]map[string]any, len(docs))
	for i, d := range docs {
		texts[i] = d.Content
		ids[i] = d.ID
		if ids[i] == "" {
			ids[i] = strings.ReplaceAll(fmt.Sprintf("%x", d.Content), "/", "_") // fallback deterministic
		}
		metas[i] = map[string]any{"content": d.Content}
		for k, v := range d.Meta {
			metas[i][k] = v
		}
	}

	vectors, err := p.embedder.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	if len(vectors) != len(docs) {
		return fmt.Errorf("embedding count mismatch")
	}

	// Convert vectors to float64 slices because the Chroma REST API expects JSON numbers.
	embeddings := make([][]float64, len(vectors))
	for i, v := range vectors {
		embeddings[i] = make([]float64, len(v))
		for j, x := range v {
			embeddings[i][j] = float64(x)
		}
	}

	body := map[string]any{
		"ids":        ids,
		"embeddings": embeddings,
		"metadatas":  metas,
		"documents":  texts,
	}
	return p.do(ctx, http.MethodPost, "/api/v1/collections/"+name+"/add", body, nil)
}

func (p *ChromaProvider) SemanticSearch(ctx context.Context, projectID, query string, limit int) ([]Result, error) {
	if limit <= 0 {
		limit = 5
	}
	var queryVec []float32
	if qe, ok := p.embedder.(QueryEmbedder); ok {
		v, err := qe.EmbedQuery(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("embed query: %w", err)
		}
		queryVec = v
	} else {
		vectors, err := p.embedder.Embed(ctx, []string{query})
		if err != nil {
			return nil, fmt.Errorf("embed query: %w", err)
		}
		queryVec = vectors[0]
	}
	embedding := make([]float64, len(queryVec))
	for i, v := range queryVec {
		embedding[i] = float64(v)
	}

	body := map[string]any{
		"query_embeddings": []any{embedding},
		"n_results":        limit,
		"include":          []string{"metadatas", "documents", "distances"},
	}
	var resp chromaQueryResponse
	if err := p.do(ctx, http.MethodPost, "/api/v1/collections/"+p.collectionName(projectID)+"/query", body, &resp); err != nil {
		return nil, err
	}

	out := make([]Result, 0, len(resp.IDs))
	for i, ids := range resp.IDs {
		for j, id := range ids {
			content := ""
			if len(resp.Documents) > i && len(resp.Documents[i]) > j {
				content = resp.Documents[i][j]
			}
			score := 0.0
			if len(resp.Distances) > i && len(resp.Distances[i]) > j {
				score = 1 - resp.Distances[i][j]
			}
			meta := map[string]string{}
			if len(resp.Metadatas) > i && len(resp.Metadatas[i]) > j {
				for k, v := range resp.Metadatas[i][j] {
					if s, ok := v.(string); ok {
						meta[k] = s
					}
				}
			}
			out = append(out, Result{ID: id, Content: content, Score: score, Meta: meta})
		}
	}
	return out, nil
}

func (p *ChromaProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	vectors, err := p.embedder.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return vectors[0], nil
}

func (p *ChromaProvider) DeletePoints(ctx context.Context, projectID string, pointIDs []string) error {
	if len(pointIDs) == 0 {
		return nil
	}
	body := map[string]any{"ids": pointIDs}
	return p.do(ctx, http.MethodPost, "/api/v1/collections/"+p.collectionName(projectID)+"/delete", body, nil)
}

func (p *ChromaProvider) ListPointIDs(ctx context.Context, projectID string, metaFilter map[string]string) ([]string, error) {
	body := map[string]any{
		"include": []string{"metadatas"},
	}
	if len(metaFilter) > 0 {
		where := map[string]any{}
		for k, v := range metaFilter {
			where[k] = v
		}
		body["where"] = where
	}
	var resp chromaQueryResponse
	if err := p.do(ctx, http.MethodPost, "/api/v1/collections/"+p.collectionName(projectID)+"/get", body, &resp); err != nil {
		return nil, err
	}
	var ids []string
	for _, batch := range resp.IDs {
		ids = append(ids, batch...)
	}
	return ids, nil
}

func (p *ChromaProvider) ListPoints(ctx context.Context, projectID string, metaFilter map[string]string) ([]PointInfo, error) {
	body := map[string]any{
		"include": []string{"metadatas"},
	}
	if len(metaFilter) > 0 {
		where := map[string]any{}
		for k, v := range metaFilter {
			where[k] = v
		}
		body["where"] = where
	}
	var resp chromaQueryResponse
	if err := p.do(ctx, http.MethodPost, "/api/v1/collections/"+p.collectionName(projectID)+"/get", body, &resp); err != nil {
		return nil, err
	}
	var points []PointInfo
	for bi, batch := range resp.IDs {
		for pi, id := range batch {
			info := PointInfo{ID: id}
			if bi < len(resp.Metadatas) && pi < len(resp.Metadatas[bi]) {
				if sf, ok := resp.Metadatas[bi][pi]["source_file"].(string); ok {
					info.SourceFile = sf
				}
			}
			points = append(points, info)
		}
	}
	return points, nil
}

func (p *ChromaProvider) Close() error { return nil }

type chromaQueryResponse struct {
	IDs        [][]string
	Distances  [][]float64
	Documents  [][]string
	Metadatas  [][]map[string]any
}

func (p *ChromaProvider) do(ctx context.Context, method, path string, body any, out any) error {
	var bodyReader io.Reader
	if body != nil {
		buf := &bytes.Buffer{}
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return err
		}
		bodyReader = buf
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("chroma %s %s returned %d: %s", method, path, resp.StatusCode, string(b))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
