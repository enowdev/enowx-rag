# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **MCP over HTTP (remote daemon)**: `enowx-rag --serve` now also exposes the MCP
  server at `/mcp` (Streamable HTTP transport, stateless), so agents can use
  enowx-rag as a centralized remote daemon — e.g. on a VPS — instead of a local
  stdio process. It's gated by the same `RAG_ADMIN_TOKEN` bearer as `/api`.
  Connect a client with `{ "url": ".../mcp", "headers": { "Authorization":
  "Bearer <token>" } }`. Local stdio mode is unchanged.
- **Agent setup**: point an AI agent at `GET /api/docs/setup` and it configures
  enowx-rag for a project, idempotently. `GET /api/setup/probe` reports what's
  already installed (MCP per client, skill, AGENTS.md block) so finished steps
  are skipped; `POST /api/setup/write-agents-md` merges an enowx-rag section into
  the project's AGENTS.md via `<!-- enowx-rag:start/end -->` markers (create /
  append / update, preserving the user's own content). The Install step shows a
  short copy-paste prompt.
- **Migration** page + engine: re-embed a project's stored text into a new
  destination to change embedding model/dimension or move between vector stores
  (Qdrant/pgvector/Chroma). Raw vectors aren't copied (they're model-specific);
  the text — stored alongside every chunk — is re-embedded by the destination.
  `POST /api/migrate` runs asynchronously with live SSE progress; the UI shows a
  progress bar and offers to delete the source after success.
- **Cloud import**: pull from an external vector DB and re-embed. Qdrant Cloud is
  verified (reuses the tested Qdrant provider); Pinecone, Weaviate, and Chroma
  Cloud connectors are **experimental** (built from vendor docs, mock-tested
  only — not verified against a live account) and labelled as such in the UI.
- OpenAI-compatible embedder (`RAG_EMBEDDER=openai`): works with any
  `/v1/embeddings` API — OpenAI, Together, Jina, Mistral, a local Ollama,
  LiteLLM, etc. — via `RAG_OPENAI_BASE_URL` / `RAG_OPENAI_MODEL` /
  `RAG_OPENAI_API_KEY` / `RAG_OPENAI_DIM`. Surfaced in the wizard's Embedding
  step, which also clarifies that TEI serves any local model.
- Query metrics: latency (avg/p50/p95), Voyage token usage, and dense/lexical
  retrieval breakdown, exposed at `GET /api/metrics` and shown on the dashboard.
  Persisted durably to `~/.enowx-rag/metrics.db` (pure-Go SQLite, no cgo), so
  metrics survive restarts on any backend.
- `compress` search option: deterministic near-duplicate result dedup.
- `enowx-rag setup [--run]` CLI subcommand to generate/run the backend
  docker-compose (never over HTTP).
- Efficient project chunk counts (`points/count` / `COUNT(*)` / `/count`),
  making `/api/projects` and `/api/stats` fast on large Qdrant/pgvector.
- Qdrant & Chroma now implement project listing, so the dashboard works on all
  backends, not only pgvector.

### Changed
- MCP `rag_semantic_search` / `rag_retrieve_context` now accept and apply
  `hybrid`/`rerank`/`recall`/`compress` (hybrid & rerank default on).
- Dashboard shows only real, backend-sourced data — removed all mock/hardcoded
  metrics, dead controls, and the fake auto-setup simulation.

### Security
- `/api/setup/apply` and `/api/setup/test` require localhost or a valid admin
  token; `/api/setup/apply` no longer echoes the saved config (API key) back.
- Removed the deprecated `middleware.RealIP` (X-Forwarded-For spoofing risk).

### Notes
- The Chroma provider is **experimental** (legacy `/api/v1`, mock-tested only);
  use Qdrant or pgvector for a supported setup.

## [0.1.0] - 2026-07-12

First tagged release. A per-project RAG memory skill and MCP server for AI
coding agents, distributed as a single self-contained binary.

### Added

- **MCP server** (stdio) exposing per-project RAG tools: create/delete project,
  index, retrieve context, semantic search.
- **Vector store providers**: Qdrant, Chroma, and pgvector, behind a common
  `rag.Provider` interface (one project = one collection).
- **Embedding backends**: Voyage AI (with Matryoshka `output_dimension` and
  `input_type=query`/`document` split) and self-hosted TEI.
- **Reranking**: Voyage `rerank-2.5` integrated into search (retrieve → rerank →
  top-k), disable-able and with graceful fallback.
- **Hybrid search** on pgvector: dense + lexical full-text fused with
  Reciprocal Rank Fusion (RRF, k=60) over a GIN `tsvector` index.
- **HTTP + web UI** (`--serve`): chi-based REST API, Server-Sent Events activity
  stream, and an embedded React dashboard (Overview, Playground, Chunks) served
  from the single binary via `embed.FS`.
- **Onboarding wizard**: guided setup for vector store, embedder, and connection
  testing.
- **Incremental indexing**: `content_hash` / `chunk_version` metadata so
  unchanged chunks are skipped on re-index; `embed_model`/`embed_dim` recorded.
- **Auth**: optional `RAG_ADMIN_TOKEN` protecting `/api/*` with constant-time
  comparison.
- **Packaging**: Makefile build pipeline and an all-in-one Docker Compose.

### Security

- SSE event stream (`/api/events`) is same-origin only by default; cross-origin
  access must be explicitly enabled via `RAG_CORS_ORIGIN`.
- Reranked result sets are defensively truncated to `k` regardless of the
  rerank API response.

[Unreleased]: https://github.com/enowdev/enowx-rag/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/enowdev/enowx-rag/releases/tag/v0.1.0
