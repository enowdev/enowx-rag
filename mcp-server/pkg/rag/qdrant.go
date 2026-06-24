package rag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
)

// QdrantProvider stores per-project vectors in Qdrant collections named "project_<id>".
type QdrantProvider struct {
	client    *qdrant.Client
	embedder  EmbeddingClient
	vectorDim uint64
}

// NewQdrantProvider connects to Qdrant with the given gRPC endpoint and embedding client.
// grpcAddr accepts either "host:port" or a full qdrant.Config URL; defaults to localhost:6334.
func NewQdrantProvider(ctx context.Context, grpcAddr string, embedder EmbeddingClient) (*QdrantProvider, error) {
	host, port, err := net.SplitHostPort(grpcAddr)
	if err != nil {
		// If it looks like a host without port, default to 6334.
		host = grpcAddr
		port = "6334"
	}
	portNum, _ := strconv.Atoi(port)

	client, err := qdrant.NewClient(&qdrant.Config{Host: host, Port: portNum})
	if err != nil {
		return nil, fmt.Errorf("qdrant connect: %w", err)
	}

	if _, err := client.HealthCheck(ctx); err != nil {
		return nil, fmt.Errorf("qdrant health check: %w", err)
	}

	dim := embedder.VectorSize()
	if dim <= 0 {
		dim = 384
	}
	return &QdrantProvider{
		client:    client,
		embedder:  embedder,
		vectorDim: uint64(dim),
	}, nil
}

func (p *QdrantProvider) collectionName(projectID string) string {
	return "project_" + sanitize(projectID)
}

func (p *QdrantProvider) CreateCollection(ctx context.Context, projectID string) error {
	name := p.collectionName(projectID)
	return p.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: name,
		VectorsConfig: &qdrant.VectorsConfig{
			Config: &qdrant.VectorsConfig_Params{
				Params: &qdrant.VectorParams{
					Size:     p.vectorDim,
					Distance: qdrant.Distance_Cosine,
				},
			},
		},
	})
}

func (p *QdrantProvider) DeleteCollection(ctx context.Context, projectID string) error {
	return p.client.DeleteCollection(ctx, p.collectionName(projectID))
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

	points := make([]*qdrant.PointStruct, len(docs))
	for i, d := range docs {
		id := d.ID
		if id == "" {
			id = uuid.NewString()
		}
		meta := make(map[string]*qdrant.Value, len(d.Meta)+1)
		meta["content"] = &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: d.Content}}
		for k, v := range d.Meta {
			meta[k] = &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: v}}
		}
		points[i] = &qdrant.PointStruct{
			Id:      &qdrant.PointId{PointIdOptions: &qdrant.PointId_Uuid{Uuid: id}},
			Payload: meta,
			Vectors: &qdrant.Vectors{VectorsOptions: &qdrant.Vectors_Vector{Vector: &qdrant.Vector{Data: vectors[i]}}},
		}
	}

	_, err = p.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: name,
		Points:         points,
	})
	return err
}

func (p *QdrantProvider) SemanticSearch(ctx context.Context, projectID, query string, limit int) ([]Result, error) {
	if limit <= 0 {
		limit = 5
	}
	vectors, err := p.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	limitU64 := uint64(limit)
	resp, err := p.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: p.collectionName(projectID),
		Query: &qdrant.Query{Variant: &qdrant.Query_Nearest{
			Nearest: &qdrant.VectorInput{Variant: &qdrant.VectorInput_Dense{
				Dense: &qdrant.DenseVector{Data: vectors[0]},
			}},
		}},
		Limit:       &limitU64,
		WithPayload: &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
	})
	if err != nil {
		return nil, fmt.Errorf("qdrant query: %w", err)
	}

	out := make([]Result, 0, len(resp))
	for _, r := range resp {
		content := ""
		if v, ok := r.Payload["content"]; ok {
			content = v.GetStringValue()
		}
		meta := make(map[string]string, len(r.Payload))
		for k, v := range r.Payload {
			meta[k] = v.GetStringValue()
		}
		out = append(out, Result{
			ID:      r.Id.GetUuid(),
			Content: content,
			Score:   float64(r.Score),
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

func (p *QdrantProvider) Close() error {
	return p.client.Close()
}

func sanitize(s string) string {
	// Keep simple ASCII alphanumerics and underscores; hash the rest.
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
