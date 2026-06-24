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

Each tool has a different config format and file location. Use the correct one per tool.

### Quick reference table

| Tool | Format | Key | Config file |
| --- | --- | --- | --- |
| Claude Code | JSON | `mcpServers` | `~/.claude.json` or `.mcp.json` in project root |
| Claude Desktop | JSON | `mcpServers` | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Cline | JSON | `mcpServers` | `~/.cline/mcp.json` or IDE MCP settings |
| Cursor | JSON | `mcpServers` | `~/.cursor/mcp.json` or `.cursor/mcp.json` in project root |
| OpenCode | JSON | `mcp` | `~/.opencode/settings.json` or `opencode.json` in project root |
| Codex (OpenAI) | TOML | `[mcp_servers.*]` | `~/.codex/config.toml` |
| Factory Droid | CLI | — | `droid mcp add enowx-rag /path/to/mcp-server` |
| Roo Code | JSON | `mcpServers` | global `mcp_settings.json` or `.roo/mcp.json` in project root |
| Zed | JSON | `context_servers` | `~/.config/zed/settings.json` |
| Windsurf | JSON | `mcpServers` | `~/.codeium/windsurf/mcp_config.json` |
| Continue | YAML | `mcpServers` (list) | `~/.continue/config.yaml` |

### Claude Code

**CLI:**
```bash
claude mcp add --transport stdio enowx-rag \
  --env RAG_VECTOR_STORE=qdrant \
  --env RAG_QDRANT_ADDR=localhost:6334 \
  --env RAG_TEI_URL=http://localhost:8081 \
  -- /path/to/enowx-rag/mcp-server/mcp-server
```

**JSON (`~/.claude.json` or `.mcp.json`):**
```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/path/to/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_ADDR": "localhost:6334",
        "RAG_TEI_URL": "http://localhost:8081"
      }
    }
  }
}
```

### Claude Desktop

**File:** `~/Library/Application Support/Claude/claude_desktop_config.json`

```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/path/to/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_ADDR": "localhost:6334",
        "RAG_TEI_URL": "http://localhost:8081"
      }
    }
  }
}
```

### Cline

**File:** `~/.cline/mcp.json` (CLI) or open Cline panel > MCP Servers > Configure (IDE)

```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/path/to/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_ADDR": "localhost:6334",
        "RAG_TEI_URL": "http://localhost:8081"
      },
      "disabled": false,
      "autoApprove": []
    }
  }
}
```

### Cursor

**File:** `~/.cursor/mcp.json` (global) or `.cursor/mcp.json` (project)

```json
{
  "mcpServers": {
    "enowx-rag": {
      "type": "stdio",
      "command": "/path/to/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_ADDR": "localhost:6334",
        "RAG_TEI_URL": "http://localhost:8081"
      }
    }
  }
}
```

### OpenCode

**File:** `~/.opencode/settings.json` or `opencode.json` in project root

OpenCode uses `mcp` key (not `mcpServers`), `command` as array, and `environment` (not `env`).

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "enowx-rag": {
      "type": "local",
      "command": ["/path/to/enowx-rag/mcp-server/mcp-server"],
      "enabled": true,
      "environment": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_ADDR": "localhost:6334",
        "RAG_TEI_URL": "http://localhost:8081"
      }
    }
  }
}
```

### Codex (OpenAI)

**File:** `~/.codex/config.toml` (TOML, not JSON)

**CLI:**
```bash
codex mcp add enowx-rag \
  --env RAG_VECTOR_STORE=qdrant \
  --env RAG_QDRANT_ADDR=localhost:6334 \
  --env RAG_TEI_URL=http://localhost:8081 \
  -- /path/to/enowx-rag/mcp-server/mcp-server
```

**TOML:**
```toml
[mcp_servers.enowx-rag]
command = "/path/to/enowx-rag/mcp-server/mcp-server"

[mcp_servers.enowx-rag.env]
RAG_VECTOR_STORE = "qdrant"
RAG_QDRANT_ADDR = "localhost:6334"
RAG_TEI_URL = "http://localhost:8081"
```

### Factory Droid

```bash
droid mcp add enowx-rag /path/to/enowx-rag/mcp-server/mcp-server
```

### Roo Code

**File:** global `mcp_settings.json` (via Roo Code > MCP > Edit Global MCP) or `.roo/mcp.json` (project)

```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/path/to/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_ADDR": "localhost:6334",
        "RAG_TEI_URL": "http://localhost:8081"
      },
      "alwaysAllow": [],
      "disabled": false
    }
  }
}
```

### Zed

**File:** `~/.config/zed/settings.json`

Zed uses `context_servers` key (not `mcpServers`). Can also be added via UI: Command Palette > `agent: add context server`.

```json
{
  "context_servers": {
    "enowx-rag": {
      "command": "/path/to/enowx-rag/mcp-server/mcp-server",
      "args": [],
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_ADDR": "localhost:6334",
        "RAG_TEI_URL": "http://localhost:8081"
      }
    }
  }
}
```

### Windsurf

**File:** `~/.codeium/windsurf/mcp_config.json`

Can also be added via MCP Marketplace UI in Windsurf.

```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/path/to/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_ADDR": "localhost:6334",
        "RAG_TEI_URL": "http://localhost:8081"
      }
    }
  }
}
```

### Continue

**File:** `~/.continue/config.yaml` (YAML, not JSON)

Continue uses `mcpServers` as a list of objects with `name`, `command`, `env`.

```yaml
mcpServers:
  - name: enowx-rag
    command: /path/to/enowx-rag/mcp-server/mcp-server
    env:
      RAG_VECTOR_STORE: qdrant
      RAG_QDRANT_ADDR: localhost:6334
      RAG_TEI_URL: http://localhost:8081
```

Replace `/path/to/enowx-rag/mcp-server/mcp-server` with the actual absolute path to the built binary. Replace environment values if using Chroma or pgvector instead of Qdrant.

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
# Claude instructions for this project

Always consult project memory before making significant changes. Use `rag_retrieve_context` with project ID `PROJECT_ID`.

Always update project memory after completing work. Use `rag_index` to store concise new knowledge.
```

---

## Layout

```
enowx-rag/
├── AGENTS.md           # Universal agent install guide (this repo)
├── CLAUDE.md           # Claude-family quick reference (this repo)
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

