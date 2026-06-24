package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/enowdev/enowx-rag/pkg/rag"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Config holds environment-based configuration for the RAG provider.
type Config struct {
	VectorStore string // qdrant, chroma, pgvector
	Embedder    string // tei, openai
	QdrantURL   string // REST URL, e.g. http://localhost:6333 or https://qdrant.example.com
	ChromaURL   string
	PGVectorDSN string
	TEIBaseURL  string // e.g. http://localhost:8081
	OpenAIKey   string
	OpenAIBase  string
	OpenAIModel string
	VectorDim   int
}

func loadConfig() Config {
	c := Config{
		VectorStore: getEnv("RAG_VECTOR_STORE", "qdrant"),
		Embedder:    getEnv("RAG_EMBEDDER", "tei"),
		QdrantURL:   getEnv("RAG_QDRANT_URL", "http://localhost:6333"),
		ChromaURL:   getEnv("RAG_CHROMA_URL", "http://localhost:8000"),
		PGVectorDSN: getEnv("RAG_PGVECTOR_DSN", ""),
		TEIBaseURL:  getEnv("RAG_TEI_URL", "http://localhost:8081"),
		OpenAIKey:   getEnv("RAG_OPENAI_API_KEY", ""),
		OpenAIBase:  getEnv("RAG_OPENAI_BASE_URL", "https://api.openai.com/v1"),
		OpenAIModel: getEnv("RAG_OPENAI_MODEL", "text-embedding-3-small"),
	}
	if d, err := strconv.Atoi(os.Getenv("RAG_VECTOR_DIM")); err == nil && d > 0 {
		c.VectorDim = d
	}
	return c
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func buildProvider(ctx context.Context, cfg Config) (rag.Provider, error) {
	var embedder rag.EmbeddingClient
	switch strings.ToLower(cfg.Embedder) {
	case "tei":
		embedder = rag.NewTEIEmbeddingClient(cfg.TEIBaseURL)
	default:
		return nil, fmt.Errorf("unsupported embedder: %s", cfg.Embedder)
	}

	switch strings.ToLower(cfg.VectorStore) {
	case "qdrant":
		return rag.NewQdrantProvider(ctx, cfg.QdrantURL, embedder)
	case "chroma":
		return rag.NewChromaProvider(cfg.ChromaURL, embedder), nil
	case "pgvector":
		return rag.NewPGVectorProvider(ctx, cfg.PGVectorDSN, embedder, "project_memory")
	default:
		return nil, fmt.Errorf("unsupported vector store: %s", cfg.VectorStore)
	}
}

// Tool inputs.
type CreateProjectInput struct {
	ProjectID string `json:"project_id" jsonschema:"Unique identifier for the project/collection"`
}

type DeleteProjectInput struct {
	ProjectID string `json:"project_id" jsonschema:"Unique identifier for the project/collection"`
}

type IndexProjectInput struct {
	ProjectID string `json:"project_id" jsonschema:"Project identifier"`
	Documents []struct {
		ID      string            `json:"id" jsonschema:"Optional document ID; generated if empty"`
		Content string            `json:"content" jsonschema:"Text to index"`
		Meta    map[string]string `json:"meta" jsonschema:"Optional metadata key-value pairs"`
	} `json:"documents" jsonschema:"Documents to index into the project memory"`
}

type SemanticSearchInput struct {
	ProjectID string `json:"project_id" jsonschema:"Project identifier"`
	Query     string `json:"query" jsonschema:"Query text"`
	Limit     int    `json:"limit" jsonschema:"Number of results to return (default 5)"`
}

type RetrieveContextInput struct {
	ProjectID string `json:"project_id" jsonschema:"Project identifier"`
	Query     string `json:"query" jsonschema:"Question or topic to retrieve context for"`
	Limit     int    `json:"limit" jsonschema:"Number of chunks to retrieve (default 5)"`
}

func main() {
	cfg := loadConfig()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	provider, err := buildProvider(ctx, cfg)
	cancel()
	if err != nil {
		log.Fatalf("failed to build provider: %v", err)
	}
	defer provider.Close()

	server := mcp.NewServer(&mcp.Implementation{Name: "enowx-rag", Version: "0.1.0"}, &mcp.ServerOptions{
		Instructions: "Per-project RAG memory: create collections, index project documents, and retrieve/semantic-search context. Connects to Qdrant/Chroma/pgvector with TEI embeddings.",
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rag_create_project",
		Description: "Create a new RAG collection for a project. Safe to call if collection already exists.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in CreateProjectInput) (*mcp.CallToolResult, any, error) {
		if err := provider.CreateCollection(ctx, in.ProjectID); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"status": "ok", "project_id": in.ProjectID}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rag_delete_project",
		Description: "Delete the RAG collection for a project and all its indexed memory.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in DeleteProjectInput) (*mcp.CallToolResult, any, error) {
		if err := provider.DeleteCollection(ctx, in.ProjectID); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"status": "deleted", "project_id": in.ProjectID}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rag_index",
		Description: "Index documents into a project collection. Each document is embedded and stored as a retrievable chunk.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in IndexProjectInput) (*mcp.CallToolResult, any, error) {
		docs := make([]rag.Document, len(in.Documents))
		for i, d := range in.Documents {
			docs[i] = rag.Document{ID: d.ID, Content: d.Content, Meta: d.Meta}
		}
		if err := provider.Index(ctx, in.ProjectID, docs); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"status": "indexed", "count": len(docs)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rag_semantic_search",
		Description: "Semantic search over a project collection. Returns the most relevant chunks with similarity scores.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in SemanticSearchInput) (*mcp.CallToolResult, any, error) {
		res, err := provider.SemanticSearch(ctx, in.ProjectID, in.Query, in.Limit)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"results": res}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rag_retrieve_context",
		Description: "Retrieve a compact context string for a project. Fetches top chunks and concatenates them for LLM context.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in RetrieveContextInput) (*mcp.CallToolResult, any, error) {
		res, err := provider.SemanticSearch(ctx, in.ProjectID, in.Query, in.Limit)
		if err != nil {
			return nil, nil, err
		}
		parts := make([]string, 0, len(res))
		for _, r := range res {
			parts = append(parts, fmt.Sprintf("[score %.3f] %s", r.Score, r.Content))
		}
		context := strings.Join(parts, "\n\n")
		return nil, map[string]any{"context": context, "chunks": res}, nil
	})

	// Log tool configuration to stderr so it doesn't interfere with stdio transport.
	fmt.Fprintf(os.Stderr, "enowx-rag mcp-server ready: vector_store=%s embedder=%s\n", cfg.VectorStore, cfg.Embedder)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

