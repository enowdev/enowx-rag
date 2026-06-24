# enowx-rag

Per-project RAG memory MCP server. Each project gets its own vector collection, so an LLM can index context about a codebase and retrieve it quickly.

This README is designed to be copy-pasted by any AI agent to install both the **Factory Droid skill** and the **MCP server** in the user's environment.

---

## What agents should do

1. Install the Factory Droid skill.
2. Build the Go MCP server.
3. Start the RAG backend (if local mode).
4. Install the MCP server in every coding tool the user uses.
5. Generate `AGENTS.md` and `CLAUDE.md` in the target project so all agents automatically use the RAG memory.

---

## 1. Install the Factory Droid skill

Copy the skill file into the Droid skill directory:

```bash
mkdir -p ~/.factory/skills/enowx-rag
cp /Users/enowdev/Project/enowx-rag/skill/enowx-rag.md ~/.factory/skills/enowx-rag/skill.md
```

If the skill must be reloaded, restart the Droid session or refresh skills.

---

## 2. Build the MCP server

```bash
cd /Users/enowdev/Project/enowx-rag/mcp-server
go build ./cmd/mcp-server
```

This produces the binary `/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server`.

---

## 3. Start the RAG backend (local mode)

If the user does not already have a Qdrant + TEI backend, start the local one:

```bash
cd /Users/enowdev/Project/enowx-rag/mcp-server
docker compose up -d qdrant tei-embedding
```

Verify:

```bash
curl -f http://localhost:6333/healthz
curl -f http://localhost:8081/health
```

If using an existing backend, skip this step and set the correct URLs in the tool config below.

---

## 4. Install the MCP server in every coding tool

Use this universal MCP server block. Only the config file location changes.

```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_EMBEDDER": "tei",
        "RAG_QDRANT_ADDR": "localhost:6334",
        "RAG_TEI_URL": "http://localhost:8081"
      }
    }
  }
}
```

### Tool-specific config locations

| Tool | Config file |
| --- | --- |
| Codebuddy Code / Codebuddy Desktop | `~/Library/Application Support/Codebuddy/claude_desktop_config.json` or `~/.claude/config.json` |
| Cline | `~/Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev/settings/claude_mcp_settings.json` |
| Cursor | `~/.cursor/mcp.json` |
| OpenCode | `~/.opencode/settings.json` or `.opencode/mcp.json` |
| Codex (OpenAI) | `~/.codex/config.json` |
| Factory Droid | `droid mcp add enowx-rag /Users/enowdev/Project/enowx-rag/mcp-server/mcp-server` |
| Roo Code | `~/Library/Application Support/Code/User/globalStorage/rooveterinaryinc.roo/settings.json` |
| Zed | `~/.config/zed/settings.json` under `mcp_servers` |
| Windsurf | `~/.windsurf/mcp_config.json` or `~/.codeium/windsurf/mcp_config.json` |
| Continue | `~/.continue/config.json` under `mcpServers` |

Replace environment values if using Chroma or pgvector instead of Qdrant.

---

## 5. Enable RAG memory for the target project

Create these two files in the root of the project that should use RAG memory. Replace `PROJECT_ID` with the actual project name or slug.

### `AGENTS.md`

```markdown
# Project agent instructions

This project uses the `enowx-rag` MCP server for per-project memory.

## Before coding

1. Call `rag_retrieve_context` with the project ID `PROJECT_ID` and the user's query.
2. Read the returned context. If relevant, use it to shape your answer or plan.

## After coding

1. Summarize what you changed.
2. Call `rag_index` with useful new facts, design decisions, gotchas, or patterns under project ID `PROJECT_ID`.

Keep chunks concise (one idea per chunk). Use metadata tags like `type:architecture`, `type:decision`, `type:api`, `type:bugfix`, `type:howto`, or `type:snippet`.
```

### `CLAUDE.md`

```markdown
# Codebuddy instructions for this project

Always consult project memory before making significant changes. Use `rag_retrieve_context` with project ID `PROJECT_ID`.

Always update project memory after completing work. Use `rag_index` to store concise new knowledge.
```

---

## Layout

```
enowx-rag/
├── AGENTS.md           # Universal agent install guide (this repo)
├── CLAUDE.md           # Codebuddy-family quick reference (this repo)
├── README.md           # This file
├── mcp-server/         # Go MCP server (stdio transport)
│   ├── cmd/mcp-server
│   ├── pkg/rag         # Provider interface + Qdrant, Chroma, pgvector
│   ├── Dockerfile
│   └── docker-compose.yml
└── skill/              # Factory Droid skill
    ├── enowx-rag.md
    └── templates/
        └── AGENTS.md
```

## Providers

| Vector store | Embedder | Status |
| --- | --- | --- |
| Qdrant | TEI | Ready |
| Chroma | TEI | Ready |
| pgvector | TEI | Ready |

## Environment variables

| Variable | Default | Description |
| --- | --- | --- |
| `RAG_VECTOR_STORE` | `qdrant` | `qdrant`, `chroma`, `pgvector` |
| `RAG_EMBEDDER` | `tei` | `tei` |
| `RAG_QDRANT_ADDR` | `localhost:6334` | Qdrant gRPC address |
| `RAG_CHROMA_URL` | `http://localhost:8000` | Chroma REST URL |
| `RAG_PGVECTOR_DSN` | - | Postgres connection string |
| `RAG_TEI_URL` | `http://localhost:8081` | Text Embeddings Inference URL |

## Tools

- `rag_create_project` — create a project collection
- `rag_delete_project` — delete a project collection
- `rag_index` — index documents into a project
- `rag_semantic_search` — semantic search across project memory
- `rag_retrieve_context` — compact context string for LLM consumption

## Notes

- The MCP server uses stdio transport by default.
- Each project gets its own collection/index: `project_<project_id>`.
- The existing Coolify `robloxkit-rag` stack exposes Qdrant on `localhost:6333` and TEI on `localhost:8081` when Docker/Colima is running.

