package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/enowdev/enowx-rag/pkg/config"
	"github.com/enowdev/enowx-rag/pkg/core"
	"github.com/enowdev/enowx-rag/pkg/httpapi"
	"github.com/enowdev/enowx-rag/pkg/indexer"
	"github.com/enowdev/enowx-rag/pkg/rag"
	"github.com/enowdev/enowx-rag/pkg/ragbuild"
	"github.com/enowdev/enowx-rag/web"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RuntimeConfig holds the resolved configuration used to build the service
// layer. It is populated from three sources with strict priority:
// environment variables > config file (~/.enowx-rag/config.yaml) > defaults.
type RuntimeConfig struct {
	VectorStore   string
	Embedder      string
	QdrantURL     string
	QdrantAPIKey  string
	ChromaURL     string
	PGVectorDSN   string
	TEIBaseURL    string
	VoyageAPIKey  string
	VoyageModel   string
	OpenAIAPIKey  string
	OpenAIModel   string
	OpenAIBaseURL string
	OpenAIDim     int
	RerankerModel string
	VectorDim     int
}

// resolveConfig builds a RuntimeConfig from the three-tier priority:
// env var > config file > default. It uses pkg/config.Resolve() which
// handles the file loading and env override logic.
func resolveConfig() (*RuntimeConfig, error) {
	cfg, err := config.Resolve()
	if err != nil {
		return nil, fmt.Errorf("resolve config: %w", err)
	}

	rc := &RuntimeConfig{
		VectorStore:   cfg.VectorStore,
		Embedder:      cfg.Embedder,
		QdrantURL:     cfg.QdrantURL,
		QdrantAPIKey:  cfg.QdrantAPIKey,
		ChromaURL:     cfg.ChromaURL,
		PGVectorDSN:   cfg.PGVectorDSN,
		TEIBaseURL:    cfg.TEIURL,
		VoyageAPIKey:  cfg.Voyage.APIKey,
		VoyageModel:   cfg.Voyage.Model,
		OpenAIAPIKey:  cfg.OpenAI.APIKey,
		OpenAIModel:   cfg.OpenAI.Model,
		OpenAIBaseURL: cfg.OpenAI.BaseURL,
		OpenAIDim:     cfg.OpenAI.Dim,
		RerankerModel: cfg.RerankerModel,
		VectorDim:     cfg.Voyage.Dim,
	}

	// Fall back to tei if voyage key is missing and embedder wasn't set
	// explicitly via env var. This preserves the original main.go behavior.
	if rc.Embedder == "voyage" && rc.VoyageAPIKey == "" && os.Getenv("RAG_EMBEDDER") == "" {
		rc.Embedder = "tei"
	}

	return rc, nil
}

func buildProvider(ctx context.Context, cfg *RuntimeConfig) (rag.Provider, error) {
	return ragbuild.BuildProvider(ctx, ragbuild.Spec{
		VectorStore:   cfg.VectorStore,
		Embedder:      cfg.Embedder,
		QdrantURL:     cfg.QdrantURL,
		QdrantAPIKey:  cfg.QdrantAPIKey,
		ChromaURL:     cfg.ChromaURL,
		PGVectorDSN:   cfg.PGVectorDSN,
		VoyageAPIKey:  cfg.VoyageAPIKey,
		VoyageModel:   cfg.VoyageModel,
		VoyageDim:     cfg.VectorDim,
		OpenAIAPIKey:  cfg.OpenAIAPIKey,
		OpenAIModel:   cfg.OpenAIModel,
		OpenAIBaseURL: cfg.OpenAIBaseURL,
		OpenAIDim:     cfg.OpenAIDim,
		TEIURL:        cfg.TEIBaseURL,
	})
}

// buildService wraps a provider, optional reranker, and indexer into a
// core.Service that both MCP and HTTP handlers call.
func buildService(provider rag.Provider, reranker rag.Reranker, idx *indexer.Indexer) *core.Service {
	return core.NewService(provider, reranker, idx)
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
	Recall    int    `json:"recall" jsonschema:"Candidates retrieved before reranking (default 40)"`
	Hybrid    *bool  `json:"hybrid" jsonschema:"Combine dense + lexical search when the backend supports it (default true)"`
	Rerank    *bool  `json:"rerank" jsonschema:"Rerank candidates with the reranker when configured (default true)"`
	Compress  bool   `json:"compress" jsonschema:"Drop near-duplicate results (default false)"`
}

type RetrieveContextInput struct {
	ProjectID string `json:"project_id" jsonschema:"Project identifier"`
	Query     string `json:"query" jsonschema:"Question or topic to retrieve context for"`
	Limit     int    `json:"limit" jsonschema:"Number of chunks to retrieve (default 5)"`
	Recall    int    `json:"recall" jsonschema:"Candidates retrieved before reranking (default 40)"`
	Hybrid    *bool  `json:"hybrid" jsonschema:"Combine dense + lexical search when the backend supports it (default true)"`
	Rerank    *bool  `json:"rerank" jsonschema:"Rerank candidates with the reranker when configured (default true)"`
	Compress  bool   `json:"compress" jsonschema:"Drop near-duplicate results (default false)"`
}

// searchOptsFromMCP builds SearchOpts from MCP tool inputs. Hybrid and Rerank
// default to true (nil pointer) so the built-in hybrid/rerank features are used
// by default from MCP clients; they only take effect when the backend/reranker
// supports them (otherwise Search falls back gracefully).
func searchOptsFromMCP(limit, recall int, hybrid, rerank *bool, compress bool) core.SearchOpts {
	b := func(p *bool) bool { return p == nil || *p }
	return core.SearchOpts{
		K:        limit,
		Recall:   recall,
		Hybrid:   b(hybrid),
		Rerank:   b(rerank),
		Compress: compress,
	}
}

type ScanProjectInput struct {
	ProjectID string `json:"project_id" jsonschema:"Project identifier"`
	Directory string `json:"directory" jsonschema:"Absolute path to the project directory to scan and index"`
}

func main() {
	// Subcommand dispatch (must run before flag.Parse, which only handles the
	// default mode's flags). `enowx-rag setup [--run]` generates/runs the
	// docker-compose backend from the command line — never over HTTP.
	if len(os.Args) > 1 && os.Args[1] == "setup" {
		runSetup(os.Args[2:])
		return
	}

	serve := flag.Bool("serve", false, "run HTTP+UI server instead of stdio MCP")
	addr := flag.String("addr", ":7777", "HTTP listen address (only used with --serve)")
	flag.Parse()

	cfg, err := resolveConfig()
	if err != nil {
		log.Fatalf("failed to resolve config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	provider, err := buildProvider(ctx, cfg)
	cancel()
	if err != nil {
		log.Fatalf("failed to build provider: %v", err)
	}
	defer provider.Close()

	// Build the service layer that wraps provider + indexer.
	idx := indexer.NewIndexer(provider, 1500)

	// Build the reranker when a model is configured and the Voyage API key
	// is available. The reranker shares the same Voyage API key used for
	// embeddings. When RerankerModel is empty, reranker is nil and
	// Search with Rerank=true silently falls back to semantic order.
	var reranker rag.Reranker
	if cfg.RerankerModel != "" && cfg.VoyageAPIKey != "" {
		reranker = rag.NewVoyageReranker(cfg.VoyageAPIKey, cfg.RerankerModel)
	}

	svc := buildService(provider, reranker, idx)
	svc.SetEmbedModel(cfg.VoyageModel)
	svc.SetBackend(cfg.VectorStore)

	// Durable metrics: a local SQLite file (pure-Go, no cgo) that works with any
	// vector-store backend. If it can't be opened, fall back to in-memory
	// metrics rather than failing startup.
	metricsPath := config.MetricsDBPath()
	if err := os.MkdirAll(filepath.Dir(metricsPath), 0o700); err == nil {
		if store, err := core.NewSQLiteMetricsStore(metricsPath); err != nil {
			fmt.Fprintf(os.Stderr, "metrics: durable store unavailable, using in-memory: %v\n", err)
		} else {
			svc.SetMetricsStore(store)
			defer store.Close()
		}
	}

	if *serve {
		runHTTP(svc, *addr, cfg)
		return
	}
	runStdio(svc, cfg)
}

// runStdio starts the MCP server in stdio mode. This is the default mode
// when --serve is not passed. Logs go to stderr (stdout is MCP protocol).
func runStdio(svc *core.Service, cfg *RuntimeConfig) {
	server := mcp.NewServer(&mcp.Implementation{Name: "enowx-rag", Version: "0.1.0"}, &mcp.ServerOptions{
		Instructions: "Per-project RAG memory: create collections, index project documents, and retrieve/semantic-search context. Connects to Qdrant/Chroma/pgvector with TEI embeddings.",
	})

	registerMCPTools(server, svc)

	// Log tool configuration to stderr so it doesn't interfere with stdio transport.
	fmt.Fprintf(os.Stderr, "enowx-rag mcp-server ready: vector_store=%s embedder=%s\n", cfg.VectorStore, cfg.Embedder)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

// runHTTP starts the HTTP API + SPA server on the given address.
func runHTTP(svc *core.Service, addr string, cfg *RuntimeConfig) {
	// Extract the dist subdirectory from the embedded filesystem.
	distFS, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		log.Fatalf("failed to get embedded dist: %v", err)
	}

	handler := httpapi.NewRouter(svc, distFS)

	fmt.Fprintf(os.Stderr, "enowx-rag HTTP server starting on %s: vector_store=%s embedder=%s\n", addr, cfg.VectorStore, cfg.Embedder)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}

// registerMCPTools registers all six MCP tools on the server, each as a thin
// wrapper over the core.Service methods. No direct provider/indexer calls
// are made inside the closures.
func registerMCPTools(server *mcp.Server, svc *core.Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "rag_create_project",
		Description: "Create a new RAG collection for a project. Safe to call if collection already exists.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in CreateProjectInput) (*mcp.CallToolResult, any, error) {
		if err := svc.CreateProject(ctx, in.ProjectID); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"status": "ok", "project_id": in.ProjectID}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rag_delete_project",
		Description: "Delete the RAG collection for a project and all its indexed memory.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in DeleteProjectInput) (*mcp.CallToolResult, any, error) {
		if err := svc.DeleteProject(ctx, in.ProjectID); err != nil {
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
		if err := svc.IndexDocuments(ctx, in.ProjectID, docs); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"status": "indexed", "count": len(docs)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rag_semantic_search",
		Description: "Semantic search over a project collection. Returns the most relevant chunks with similarity scores.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in SemanticSearchInput) (*mcp.CallToolResult, any, error) {
		opts := searchOptsFromMCP(in.Limit, in.Recall, in.Hybrid, in.Rerank, in.Compress)
		res, err := svc.Search(ctx, in.ProjectID, in.Query, opts)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"results": res}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rag_retrieve_context",
		Description: "Retrieve a compact context string for a project. Fetches top chunks and concatenates them for LLM context.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in RetrieveContextInput) (*mcp.CallToolResult, any, error) {
		opts := searchOptsFromMCP(in.Limit, in.Recall, in.Hybrid, in.Rerank, in.Compress)
		context, chunks, err := svc.RetrieveContext(ctx, in.ProjectID, in.Query, opts)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"context": context, "chunks": chunks}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rag_index_project",
		Description: "Scan a project directory and auto-index all code/text files into RAG. Handles insertions (new/changed files) and deletions (removed files). Skips node_modules, .git, vendor, dist, build. Run this when the codebase changes.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ScanProjectInput) (*mcp.CallToolResult, any, error) {
		// Use a long-lived context so large projects don't time out under the
		// MCP framework's short request deadline. 30 minutes is enough for
		// even very large monorepos.
		indexCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		result, err := svc.IndexProject(indexCtx, in.ProjectID, in.Directory)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{
			"status":         "synced",
			"project_id":     in.ProjectID,
			"chunks_indexed": result.Indexed,
			"points_deleted": result.Deleted,
			"files_scanned":  result.FilesScanned,
			"stale_error":    result.StaleError,
		}, nil
	})
}
