package rag

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// QdrantProvider stores per-project vectors in Qdrant collections via REST API (no gRPC).
// Supports optional API key authentication for secured Qdrant instances.
type QdrantProvider struct {
	baseURL   string
	apiKey    string
	embedder  EmbeddingClient
	vectorDim int
	client    *http.Client
}

// NewQdrantProvider connects to Qdrant via REST API.
// baseURL should be the Qdrant REST endpoint, e.g. http://localhost:6333 or https://qdrant.example.com
// apiKey is optional; set empty string for unsecured instances.
func NewQdrantProvider(ctx context.Context, baseURL, apiKey string, embedder EmbeddingClient) (*QdrantProvider, error) {
	baseURL = strings.TrimSuffix(baseURL, "/")
	p := &QdrantProvider{
		baseURL:   baseURL,
		apiKey:    apiKey,
		embedder:  embedder,
		client:    &http.Client{},
		vectorDim: embedder.VectorSize(),
	}
	if p.vectorDim <= 0 {
		p.vectorDim = 384
	}

	// Health check
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/healthz", nil)
	if err != nil {
		return nil, fmt.Errorf("qdrant health check request: %w", err)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qdrant health check failed (is Qdrant running at %s?): %w", baseURL, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qdrant health check returned %d", resp.StatusCode)
	}

	return p, nil
}

func (p *QdrantProvider) collectionName(projectID string) string {
	return "project_" + sanitize(projectID)
}

func (p *QdrantProvider) CreateCollection(ctx context.Context, projectID string) error {
	name := p.collectionName(projectID)
	body := map[string]any{
		"vectors": map[string]any{
			"size":     p.vectorDim,
			"distance": "Cosine",
		},
	}
	return p.do(ctx, http.MethodPut, "/collections/"+name+"?wait=true", body, nil)
}

func (p *QdrantProvider) DeleteCollection(ctx context.Context, projectID string) error {
	return p.do(ctx, http.MethodDelete, "/collections/"+p.collectionName(projectID)+"?wait=true", nil, nil)
}

func (p *QdrantProvider) Index(ctx context.Context, projectID string, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}
	name := p.collectionName(projectID)

	texts := make([]string, len(docs))
	for i, d := range docs {
		texts[i] = d.Content
	}
	vectors, err := p.embedder.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	if len(vectors) != len(docs) {
		return fmt.Errorf("embedding count mismatch: got %d, want %d", len(vectors), len(docs))
	}

	points := make([]map[string]any, len(docs))
	for i, d := range docs {
		// Qdrant only accepts an unsigned integer or a UUID as a point ID, but
		// callers pass human-readable IDs like "path/to/file.go#chunk0". Map
		// such IDs to a stable UUID (same input -> same UUID) so upserts keep
		// overwriting the same point, and preserve the original under "doc_id".
		rawID := d.ID
		if rawID == "" {
			rawID = uuid.NewString()
		}
		id := pointID(rawID)
		payload := map[string]any{"content": d.Content, "doc_id": rawID}
		for k, v := range d.Meta {
			payload[k] = v
		}
		// Convert []float32 to []float64 for JSON
		vec := make([]float64, len(vectors[i]))
		for j, x := range vectors[i] {
			vec[j] = float64(x)
		}
		points[i] = map[string]any{
			"id":       id,
			"vector":   vec,
			"payload":  payload,
		}
	}

	body := map[string]any{"points": points}
	return p.do(ctx, http.MethodPut, "/collections/"+name+"/points?wait=true", body, nil)
}

func (p *QdrantProvider) SemanticSearch(ctx context.Context, projectID, query string, limit int) ([]Result, error) {
	if limit <= 0 {
		limit = 5
	}

	// Use EmbedQuery if available (Voyage AI distinguishes query vs document)
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

	vec := make([]float64, len(queryVec))
	for i, x := range queryVec {
		vec[i] = float64(x)
	}

	body := map[string]any{
		"vector":      vec,
		"limit":       limit,
		"with_payload": true,
	}

	var resp struct {
		Result []struct {
			ID      any            `json:"id"`
			Score   float64        `json:"score"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	if err := p.do(ctx, http.MethodPost, "/collections/"+p.collectionName(projectID)+"/points/search", body, &resp); err != nil {
		return nil, fmt.Errorf("qdrant search: %w", err)
	}

	out := make([]Result, 0, len(resp.Result))
	for _, r := range resp.Result {
		content := ""
		if v, ok := r.Payload["content"].(string); ok {
			content = v
		}
		meta := make(map[string]string, len(r.Payload))
		for k, v := range r.Payload {
			if s, ok := v.(string); ok {
				meta[k] = s
			}
		}
		out = append(out, Result{
			ID:      fmt.Sprintf("%v", r.ID),
			Content: content,
			Score:   r.Score,
			Meta:    meta,
		})
	}
	return out, nil
}

func (p *QdrantProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	vectors, err := p.embedder.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return vectors[0], nil
}

func (p *QdrantProvider) DeletePoints(ctx context.Context, projectID string, pointIDs []string) error {
	if len(pointIDs) == 0 {
		return nil
	}
	body := map[string]any{"points": pointIDs}
	return p.do(ctx, http.MethodPost, "/collections/"+p.collectionName(projectID)+"/points/delete?wait=true", body, nil)
}

func (p *QdrantProvider) ListPointIDs(ctx context.Context, projectID string, metaFilter map[string]string) ([]string, error) {
	pts, err := p.ListPoints(ctx, projectID, metaFilter)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(pts))
	for i, pt := range pts {
		ids[i] = pt.ID
	}
	return ids, nil
}

// PointInfo is a Qdrant point's ID together with the payload fields needed to
// reconcile it against the current file set.
type PointInfo struct {
	ID         string
	SourceFile string
}

// ListPoints scrolls all points (optionally filtered by metadata), returning
// each point's Qdrant ID and its source_file payload.
func (p *QdrantProvider) ListPoints(ctx context.Context, projectID string, metaFilter map[string]string) ([]PointInfo, error) {
	name := p.collectionName(projectID)
	must := []map[string]any{}
	for k, v := range metaFilter {
		must = append(must, map[string]any{
			"key":   k,
			"match": map[string]any{"value": v},
		})
	}

	var all []PointInfo
	var scrollOffset any = nil
	limit := 256
	for {
		body := map[string]any{
			"limit":        limit,
			"with_payload": []string{"source_file"},
			"with_vector":  false,
		}
		if scrollOffset != nil {
			body["offset"] = scrollOffset
		}
		if len(must) > 0 {
			body["filter"] = map[string]any{"must": must}
		}
		var resp struct {
			Result struct {
				Points []struct {
					ID      any `json:"id"`
					Payload struct {
						SourceFile string `json:"source_file"`
					} `json:"payload"`
				} `json:"points"`
				NextOffset any `json:"next_page_offset"`
			} `json:"result"`
		}
		if err := p.do(ctx, http.MethodPost, "/collections/"+name+"/points/scroll", body, &resp); err != nil {
			return nil, fmt.Errorf("qdrant scroll: %w", err)
		}
		for _, pt := range resp.Result.Points {
			all = append(all, PointInfo{
				ID:         fmt.Sprintf("%v", pt.ID),
				SourceFile: pt.Payload.SourceFile,
			})
		}
		if resp.Result.NextOffset == nil {
			break
		}
		scrollOffset = resp.Result.NextOffset
	}
	return all, nil
}

// pointID maps an arbitrary document ID to a value Qdrant accepts as a point
// ID (a UUID). IDs that are already valid UUIDs are returned unchanged;
// everything else is hashed into a deterministic UUIDv5 so the same document
// ID always yields the same point ID.
func pointID(raw string) string {
	if _, err := uuid.Parse(raw); err == nil {
		return raw
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(raw)).String()
}

func (p *QdrantProvider) Close() error { return nil }

func (p *QdrantProvider) do(ctx context.Context, method, path string, body any, out any) error {
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
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if p.apiKey != "" {
		req.Header.Set("api-key", p.apiKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant %s %s returned %d: %s", method, path, resp.StatusCode, string(b))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func sanitize(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		h := sha256.Sum256([]byte(s))
		return hex.EncodeToString(h[:8])
	}
	return string(out)
}
