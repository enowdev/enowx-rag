package httpapi

import (
	"fmt"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
)

// docSection is one documentation page. Body is markdown, rendered by the Docs
// UI and readable by agents via GET /api/docs/{id}.
type docSection struct {
	ID    string
	Title string
	Body  func(base, exe string) string
}

// docSections is the ordered documentation table of contents. Bodies are
// functions so they can embed the live server base URL and binary path.
var docSections = []docSection{
	{ID: "overview", Title: "Overview", Body: docOverview},
	{ID: "quickstart", Title: "Quick start", Body: docQuickstart},
	{ID: "mcp-tools", Title: "MCP tools", Body: docMCPTools},
	{ID: "api-reference", Title: "API reference", Body: docAPIReference},
	{ID: "embedders", Title: "Embedders", Body: docEmbedders},
	{ID: "vector-stores", Title: "Vector stores", Body: docVectorStores},
	{ID: "search", Title: "Search (hybrid / rerank / compress)", Body: docSearch},
	{ID: "migration", Title: "Migration", Body: docMigration},
	{ID: "metrics", Title: "Metrics", Body: docMetrics},
	{ID: "remote", Title: "Remote / daemon", Body: docRemote},
	{ID: "agent-setup", Title: "Agent setup", Body: docAgentSetup},
}

func requestBase(r *http.Request) string {
	if r.TLS != nil {
		return "https://" + r.Host
	}
	return "http://" + r.Host
}

// DocsList handles GET /api/docs — the documentation table of contents.
func (h *Handlers) DocsList(w http.ResponseWriter, r *http.Request) {
	type item struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	out := make([]item, 0, len(docSections))
	for _, s := range docSections {
		out = append(out, item{ID: s.ID, Title: s.Title})
	}
	writeJSON(w, http.StatusOK, out)
}

// DocsSection handles GET /api/docs/{section} — one section's markdown.
func (h *Handlers) DocsSection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "section")
	base := requestBase(r)
	exe, _ := os.Executable()
	for _, s := range docSections {
		if s.ID == id {
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(s.Body(base, exe)))
			return
		}
	}
	writeErr(w, http.StatusNotFound, "unknown docs section")
}

// SetupDocs handles GET /api/docs/setup — kept as an alias for the agent-setup
// section so existing links and the copy-paste prompt keep working.
func (h *Handlers) SetupDocs(w http.ResponseWriter, r *http.Request) {
	base := requestBase(r)
	exe, _ := os.Executable()
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(docAgentSetup(base, exe)))
}

// --- Section bodies ---

func docOverview(base, _ string) string {
	return `# enowx-rag

Per-project RAG (retrieval-augmented generation) memory for AI coding agents.
Each project gets its own vector collection, so an agent can index context about
a codebase and retrieve it quickly and in isolation.

## What it does

- **Index** a project's code/text into a vector store, chunked and embedded.
- **Retrieve** the most relevant chunks for a query (semantic, hybrid, reranked).
- **Persist** design decisions, gotchas, and facts as per-project memory.

## Two run modes (one binary)

- **MCP stdio** (default): the agent talks to enowx-rag over the Model Context
  Protocol. This is what you configure in Claude Code, Cursor, etc.
- **HTTP + dashboard** (` + "`--serve`" + `): a REST API, an SSE event stream, and an
  embedded web dashboard (Overview, Playground, Chunks, Migration, Docs, Setup).

## Building blocks

- **Vector stores**: Qdrant, pgvector, Chroma (see *Vector stores*).
- **Embedders**: Voyage AI, any OpenAI-compatible API, or self-hosted TEI (see
  *Embedders*).
- **Search**: dense, plus optional hybrid (dense + lexical RRF), reranking, and
  near-duplicate compression (see *Search*).

Start at *Quick start*, wire it into your agent via *Agent setup*, and use the
*API reference* for automation.`
}

func docQuickstart(base, exe string) string {
	return fmt.Sprintf(`# Quick start

## 1. Configure

Set a vector store + embedder. The fastest path is Qdrant + Voyage AI (free
tier). Either use the **Setup** wizard in the dashboard, or set env vars:

    RAG_VECTOR_STORE=qdrant
    RAG_QDRANT_URL=http://localhost:6333
    RAG_EMBEDDER=voyage
    RAG_VOYAGE_API_KEY=pa-...

## 2. Run

- MCP mode (for your agent): run the binary with no flags.
- Dashboard: run with ` + "`--serve`" + ` and open the UI.

Binary: %s

## 3. Connect your agent

Use **Setup → Install** in the dashboard to write the MCP config into your
client (Claude Code, Cursor, …), or let an agent do it — see *Agent setup*.

## 4. Index a project

From your agent call the ` + "`rag_index_project`" + ` MCP tool with the project
directory, or from the API:

    POST %s/api/projects/<PROJECT_ID>/reindex   { "directory": "/abs/path" }

## 5. Retrieve

Ask your agent to use ` + "`rag_retrieve_context`" + `, or try the **Playground** in
the dashboard.`, exe, base)
}

func docMCPTools(_, _ string) string {
	return "# MCP tools\n\n" +
		"enowx-rag exposes six MCP tools over stdio. Project IDs isolate collections.\n\n" +
		"## `rag_create_project`\nCreate a project collection. Input: `project_id`.\n\n" +
		"## `rag_delete_project`\nDelete a project collection and all its data. Input: `project_id`.\n\n" +
		"## `rag_index`\nIndex documents you pass directly. Input: `project_id`, `documents` " +
		"(each `{id?, content, meta?}`). Use for saving facts/decisions.\n\n" +
		"## `rag_index_project`\nScan a directory and index all code/text files (insertions, edits, " +
		"deletions handled incrementally; skips node_modules/.git/vendor/dist/build). " +
		"Input: `project_id`, `directory`. Run this whenever the codebase changes.\n\n" +
		"## `rag_semantic_search`\nSearch a project. Input: `project_id`, `query`, `limit`, and " +
		"optionally `recall`, `hybrid`, `rerank`, `compress` (hybrid/rerank default on). Returns " +
		"chunks with scores.\n\n" +
		"## `rag_retrieve_context`\nLike search but returns a compact concatenated context string " +
		"plus the chunks — convenient for feeding an LLM. Same options as `rag_semantic_search`.\n\n" +
		"**Typical loop:** retrieve before coding → do the work → `rag_index` new facts → " +
		"`rag_index_project` to sync file changes."
}

func docAPIReference(base, _ string) string {
	return fmt.Sprintf(`# API reference

All endpoints are under %s/api. Endpoints that write files or config are
restricted to localhost or a valid `+"`RAG_ADMIN_TOKEN`"+` bearer token.

## Projects & search
- `+"`GET /api/projects`"+` — list projects with chunk counts
- `+"`GET /api/projects/{id}`"+` — project detail
- `+"`GET /api/projects/{id}/points`"+` — list chunks (`+"`?source_file=&offset=&limit=`"+`)
- `+"`DELETE /api/projects/{id}/points/{pointId}`"+` — delete one chunk
- `+"`POST /api/projects/{id}/reindex`"+` — index a directory (`+"`{directory}`"+`)
- `+"`DELETE /api/projects/{id}`"+` — delete a project
- `+"`POST /api/search`"+` — search (`+"`{project_id, query, k, recall, hybrid, rerank, compress}`"+`)

## Stats, metrics, events
- `+"`GET /api/stats`"+` — totals + embed model
- `+"`GET /api/metrics`"+` — latency, tokens, backend, persistence (see *Metrics*)
- `+"`GET /api/events`"+` — SSE stream (index/search/migration events)

## Setup & install
- `+"`POST /api/setup/test`"+` — test vector store + embedder connectivity
- `+"`POST /api/setup/apply`"+` — save config to ~/.enowx-rag/config.yaml
- `+"`GET /api/setup/status`"+` — is config present
- `+"`GET /api/setup/clients`"+` — supported MCP clients
- `+"`POST /api/setup/install-mcp`"+` — write the server into a client's config (merge + backup)
- `+"`GET /api/setup/mcp-snippet?client_id=`"+` — manual config snippet
- `+"`GET /api/setup/skill-guide`"+` — skill install instructions
- `+"`GET /api/setup/probe?client=&dir=`"+` — what's already installed (for idempotent setup)
- `+"`POST /api/setup/write-agents-md`"+` — merge the enowx-rag block into AGENTS.md

## Migration
- `+"`POST /api/migrate`"+` — re-embed/move a project to a new destination (async, SSE)

## Docs
- `+"`GET /api/docs`"+` — this table of contents
- `+"`GET /api/docs/{section}`"+` — one section as markdown`, base)
}

func docEmbedders(_, _ string) string {
	return "# Embedders\n\n" +
		"Set with `RAG_EMBEDDER`. The embedding model and dimension must stay consistent " +
		"within a collection — changing them requires re-indexing or a *Migration*.\n\n" +
		"## Voyage AI (`voyage`)\nHosted, high quality, free tier. `RAG_VOYAGE_API_KEY`, " +
		"`RAG_VOYAGE_MODEL` (default `voyage-4`). Also powers reranking (`RAG_RERANKER_MODEL`).\n\n" +
		"## OpenAI-compatible (`openai`)\nAny `/v1/embeddings` API — OpenAI, Together, Jina, " +
		"Mistral, a local Ollama, LiteLLM, etc. Set `RAG_OPENAI_BASE_URL`, `RAG_OPENAI_MODEL`, " +
		"`RAG_OPENAI_API_KEY` (empty for local), `RAG_OPENAI_DIM` (0 = auto-detect).\n\n" +
		"## TEI (`tei`)\nSelf-hosted Text Embeddings Inference. Serve **any** local model " +
		"(BGE, GTE, E5, nomic-embed, …) and point `RAG_TEI_URL` at it. TEI is a server, not a " +
		"single model."
}

func docVectorStores(_, _ string) string {
	return "# Vector stores\n\n" +
		"Set with `RAG_VECTOR_STORE`. One project = one collection.\n\n" +
		"## Qdrant (`qdrant`)\nSupported. Per-collection dimension (flexible for migration). " +
		"`RAG_QDRANT_URL`, optional `RAG_QDRANT_API_KEY` (cloud).\n\n" +
		"## pgvector (`pgvector`)\nSupported; recommended for **hybrid search** and the " +
		"dense/lexical **retrieval breakdown**. `RAG_PGVECTOR_DSN`. Note: all projects share one " +
		"table with a **fixed** vector dimension — changing dimension means migrating to a new " +
		"table.\n\n" +
		"## Chroma (`chroma`)\n**Experimental** — targets the legacy `/api/v1` REST API and is " +
		"mock-tested only, not verified against a live server (Chroma ≥ 0.6 moved to `/api/v2`). " +
		"Prefer Qdrant or pgvector.\n\n" +
		"| Feature | Qdrant | pgvector | Chroma |\n| --- | :---: | :---: | :---: |\n" +
		"| Index / search / delete | ✅ | ✅ | ⚠️ |\n| Project list + stats | ✅ | ✅ | ⚠️ |\n" +
		"| Hybrid search | — | ✅ | — |\n| Retrieval breakdown | — | ✅ | — |"
}

func docSearch(_, _ string) string {
	return "# Search: hybrid, rerank, compress\n\n" +
		"Options on `POST /api/search` and the search MCP tools.\n\n" +
		"## Recall vs. K\n`recall` (default 40) candidates are retrieved, then narrowed to `k` " +
		"(default 5) final results. Reranking works best with recall > k.\n\n" +
		"## Hybrid (`hybrid`)\nCombines dense vector similarity with lexical full-text search " +
		"using Reciprocal Rank Fusion (RRF, k=60). **pgvector only** — other backends fall back to " +
		"dense. Great for keyword-heavy queries.\n\n" +
		"## Rerank (`rerank`)\nRe-orders candidates with Voyage `rerank-2.5` when configured. " +
		"Retrieve `recall` → rerank → keep top `k`. Falls back to semantic order if the reranker " +
		"is unavailable.\n\n" +
		"## Compress (`compress`)\nDrops near-duplicate results (same content hash / identical " +
		"content) after ranking — deterministic, no LLM. Tightens context sent to the model.\n\n" +
		"## Retrieval breakdown\nOn pgvector hybrid searches, the dashboard shows how many results " +
		"came from the dense vs. lexical ranking (see *Metrics*)."
}

func docMigration(base, _ string) string {
	return "# Migration\n\n" +
		"Re-embed a project's stored text into a new destination — to change embedding " +
		"model/dimension or move between vector stores. Raw vectors are model-specific and not " +
		"portable, so migration re-embeds from the text stored alongside every chunk.\n\n" +
		"## Use cases\n" +
		"- **Change model / dimension** — pick a new embedder/model/dim; the destination is " +
		"re-embedded. (For pgvector, a new dimension needs a **new table name**.)\n" +
		"- **Move between stores** — e.g. Qdrant → pgvector.\n" +
		"- **Import from cloud** — Qdrant Cloud is verified; Pinecone, Weaviate, and Chroma Cloud " +
		"connectors are *experimental* (mock-tested only) and labelled as such in the UI.\n\n" +
		"## How\nUse the **Migration** page, or `POST /api/migrate`. It runs asynchronously with " +
		"live progress over SSE (`migration_started/progress/completed/failed`). The source is " +
		"never auto-deleted — after success the UI offers an explicit delete.\n\n" +
		"Body (in-store): `{source_project, dest_project, vector_store, embedder, ...connection/model/dim}`.\n" +
		"Body (cloud import): add `cloud_source: {provider, url, api_key, index, text_field}`."
}

func docMetrics(base, _ string) string {
	return fmt.Sprintf("# Metrics\n\n"+
		"Every search records metrics, exposed at `GET %s/api/metrics` and shown on the "+
		"dashboard Overview.\n\n"+
		"## What's tracked\n"+
		"- **Latency** — average, p50, p95 over recent queries.\n"+
		"- **Token usage** — embed + rerank tokens reported by the Voyage API (0 for backends "+
		"that don't report, e.g. TEI).\n"+
		"- **Retrieval breakdown** — dense vs. lexical counts on pgvector hybrid searches.\n"+
		"- **Backend** and **persistent** flag.\n\n"+
		"## Persistence\nMetrics are stored in a local SQLite file (`~/.enowx-rag/metrics.db`, "+
		"pure-Go, no external service), so they survive restarts on **any** backend. If the file "+
		"can't be opened, metrics fall back to in-memory (`\"persistent\": false`).", base)
}

func docRemote(base, _ string) string {
	return "# Remote / daemon\n\n" +
		"Run enowx-rag as a **daemon** (e.g. on a VPS) and let agents connect **remotely** over " +
		"MCP-over-HTTP — a centralized RAG memory shared across machines/agents.\n\n" +
		"## Run the daemon\n" +
		"`enowx-rag --serve` exposes three things on one port:\n" +
		"- `/api/*` — REST API\n" +
		"- `/mcp` — MCP server over HTTP (Streamable HTTP transport, stateless)\n" +
		"- `/` — the web dashboard\n\n" +
		"## Secure it (required when public)\n" +
		"Set `RAG_ADMIN_TOKEN` to a strong secret. It gates **both** `/api/*` and `/mcp` with " +
		"`Authorization: Bearer <token>`. When unset, there is **no auth** — only safe for a trusted " +
		"local network. Terminate TLS with a reverse proxy (Caddy/nginx) in front.\n\n" +
		"    RAG_ADMIN_TOKEN=$(openssl rand -hex 32) enowx-rag --serve --addr :7777\n\n" +
		"## Connect an agent (MCP remote)\n" +
		"Point your MCP client at the daemon URL with the bearer header:\n\n" +
		"    {\n      \"mcpServers\": {\n        \"enowx-rag\": {\n          \"url\": \"https://rag.example.com/mcp\",\n" +
		"          \"headers\": { \"Authorization\": \"Bearer <RAG_ADMIN_TOKEN>\" }\n        }\n      }\n    }\n\n" +
		"All six MCP tools (see *MCP tools*) work identically to the local stdio mode.\n\n" +
		"## Local vs. remote\n" +
		"- **Local**: run `enowx-rag` with no flags — stdio, spawned by the client. No daemon needed.\n" +
		"- **Remote**: run `enowx-rag --serve` on a host; clients connect to `/mcp` by URL.\n\n" +
		"The vector store (Qdrant/pgvector) can be local to the daemon or a managed cloud instance."
}

func docAgentSetup(base, exe string) string {
	return fmt.Sprintf(`# Agent setup

Set up enowx-rag (per-project RAG memory) for the current project. Do the steps
below in order. Skip any step that the probe reports as already done. All calls
go to the enowx-rag server at %s.

## 1. Probe what already exists

GET %s/api/setup/probe?client=<CLIENT_ID>&dir=<ABS_PROJECT_DIR>

Response:
- mcp: { "<client>": true|false }  — is the enowx-rag MCP server in that client's config
- skill: { installed: bool, dir }  — is the skill installed
- agents_md: { exists, has_block } — does the project's AGENTS.md have the enowx-rag block

Pick CLIENT_ID from: claude-code, claude-desktop, cursor, cline, windsurf, codex, zed, continue.

## 2. Install the MCP server (skip if mcp[client] is true)

POST %s/api/setup/install-mcp  { "client_id": "<CLIENT_ID>", "scope": "global" }

This merges the enowx-rag server into the client's config (backing up the
original). For a manual snippet instead: GET %s/api/setup/mcp-snippet?client_id=<CLIENT_ID>.

The MCP server binary is: %s

## 3. Install the skill (skip if skill.installed is true)

Skills are supported by some clients only (e.g. Claude Code, Factory). Get the
exact commands: GET %s/api/setup/skill-guide — then run them.

## 4. Write the project's AGENTS.md (skip if agents_md.has_block is true)

POST %s/api/setup/write-agents-md  { "dir": "<ABS_PROJECT_DIR>", "project_id": "<PROJECT_ID>" }

This merges an enowx-rag section into AGENTS.md idempotently (markers
<!-- enowx-rag:start --> ... <!-- enowx-rag:end -->), preserving existing content.

## 5. Index the project (optional but recommended)

POST %s/api/projects/<PROJECT_ID>/reindex  { "directory": "<ABS_PROJECT_DIR>" }

## Notes
- Use an absolute project directory for dir.
- PROJECT_ID is a short slug for this project (e.g. the repo name).
- Endpoints that write files require the request to originate from localhost (or a valid RAG_ADMIN_TOKEN).
`, base, base, base, base, exe, base, base, base)
}
