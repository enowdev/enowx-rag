# enowx-rag skill

Use this skill when the user wants to set up, configure, or deploy the enowx-rag MCP server and RAG backend.

## When to use

- User says "setup RAG memory", "install enowx-rag", "buat project RAG", "buat MCP RAG", "connect RAG", "rag per project", or similar.
- User asks about configuring a Model Context Protocol (MCP) server for project memory.
- User wants to deploy the RAG backend locally or connect to an existing Coolify RAG stack.

## Goal

Walk the user through installing the `enowx-rag` MCP server and optionally deploying or connecting the vector store + embedding backend.

## Constraints

- Do not expose secrets or API keys in plain text in the final output; store them in `.env` files.
- If deploying to Coolify, prefer managed Docker services or the existing `robloxkit-rag` stack if healthy.
- Do not modify the user's existing code repositories unless explicitly asked.
- Use the exact config format documented by each tool. Do not guess formats.

## Setup flow

1. **Detect context**
   - Check if the user already has a clone of `enowx-rag`.
   - **Ask which embedding option they prefer:**
     - **Voyage AI (recommended):** Hosted API, no GPU needed. Free at [voyageai.com](https://voyageai.com) — `voyage-4` has 200M free tokens. Ask for their API key.
     - **Self-hosted TEI:** Runs locally via Docker. Ask if they already have Qdrant + TEI running, or offer to start them.
   - If self-hosted TEI, ask for vector store type (`qdrant`, `chroma`, `pgvector`), Qdrant URL, and TEI URL (or offer Docker setup).
   - If local TEI backend: generate or update `docker-compose.yml` for Qdrant + TEI.

2. **Install / start the backend (when needed — TEI only)**
   - Verify Docker/Colima is available.
   - Run `docker compose up -d qdrant tei-embedding`.
   - Wait for health checks (`curl -f http://localhost:6333/healthz` and `curl -f http://localhost:8081/health`).
   - Skip this step entirely when using Voyage AI.

3. **Build / configure the MCP server**
   - Build the Go binary with `go build ./cmd/mcp-server`.
   - Create `.env` from the chosen configuration.
   - Provide the MCP client config snippet for all supported tools using the correct format per tool.

4. **Optional Coolify deployment**
   - If the user wants to deploy to Coolify, use the `coolify` MCP tools to create/update the service/app.
   - Use the existing `robloxkit-rag` service (Qdrant + TEI) if it is healthy and matches the requirements.

## Required user info

Ask these questions one at a time or in a compact batch:

1. `project_path` - Where should the `enowx-rag` repository be created/cloned? (default: `/Users/enowdev/Project/enowx-rag`)
2. `embedder` - Which embedding option?
   - **`voyage`** (default, recommended): hosted API, free at voyageai.com. Ask for `voyage_api_key`.
   - **`tei`**: self-hosted. Ask for Qdrant URL + TEI URL, or offer Docker setup.
3. If `voyage`:
   - `voyage_api_key` — Voyage AI API key
   - `voyage_model` (default `voyage-4`)
   - `qdrant_url` (REST URL, e.g. `http://localhost:6333`)
4. If `tei`:
   - `vector_store` (`qdrant`/`chroma`/`pgvector`)
   - `qdrant_url` (REST URL, e.g. `http://localhost:6333`)
   - `tei_url` (e.g. `http://localhost:8081`)
   - `pgvector_dsn` (if vector_store is `pgvector`)
   - If no existing backend: `tei_port` (default `8081`), `qdrant_rest_port` (default `6333`), `qdrant_grpc_port` (default `6334`)
5. `tools_to_install` — Which coding tools should use this MCP server?
6. `target_project` — Path to the project that should receive `AGENTS.md` + `CLAUDE.md` (optional).

## Output artifacts

- `enowx-rag/.env`
- `enowx-rag/mcp-server/docker-compose.yml` (only for local mode, with chosen model)
- MCP client config snippet (correct format per tool)
- Verification commands
- `AGENTS.md` and `CLAUDE.md` in the target project (if requested)

## Example `.env` (Voyage AI — recommended)

```bash
RAG_VECTOR_STORE=qdrant
RAG_QDRANT_URL=http://localhost:6333
RAG_VOYAGE_API_KEY=your-voyage-api-key
RAG_VOYAGE_MODEL=voyage-4
```

## Example `.env` (self-hosted TEI)

```bash
RAG_VECTOR_STORE=qdrant
RAG_EMBEDDER=tei
RAG_QDRANT_URL=http://localhost:6333
RAG_TEI_URL=http://localhost:8081
RAG_MODEL_ID=BAAI/bge-small-en-v1.5
```

## Example docker-compose.yml (local, with chosen model)

```yaml
services:
  qdrant:
    image: qdrant/qdrant:latest
    container_name: enowx-rag-qdrant
    ports:
      - "6333:6333"
      - "6334:6334"
    volumes:
      - qdrant-data:/qdrant/storage
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:6333/healthz"]
      interval: 30s
      timeout: 10s
      retries: 3

  tei-embedding:
    image: ghcr.io/huggingface/text-embeddings-inference:cpu-1.7
    container_name: enowx-rag-tei
    ports:
      - "8081:80"
    environment:
      - MODEL_ID=${RAG_MODEL_ID:-BAAI/bge-small-en-v1.5}
      - RUST_LOG=info
      - MAX_BATCH_TOKENS=16384
    volumes:
      - tei-data:/data
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost/health"]
      interval: 30s
      timeout: 10s
      retries: 3

volumes:
  qdrant-data:
  tei-data:
```

---

## MCP client config per tool (from official docs)

Each tool has a different config format and file location. Use these exact formats.

Each config block below shows the Voyage AI setup (recommended). Replace with `RAG_EMBEDDER=tei` + `RAG_TEI_URL` if using self-hosted TEI instead.

### Claude Code (Anthropic CLI)

**Config file:** `~/.claude.json` (user scope) or `.mcp.json` in project root (project scope)

**CLI command:**
```bash
claude mcp add --transport stdio enowx-rag \
  --env RAG_VECTOR_STORE=qdrant \
  --env RAG_QDRANT_URL=http://localhost:6333 \
  --env RAG_VOYAGE_API_KEY=your-voyage-api-key \
  -- /Users/enowdev/Project/enowx-rag/mcp-server/mcp-server
```

**JSON format (`.mcp.json` or `~/.claude.json`):**
```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_URL": "http://localhost:6333",
        "RAG_VOYAGE_API_KEY": "your-voyage-api-key",
        "RAG_VOYAGE_MODEL": "voyage-4"
      }
    }
  }
}
```

Source: https://code.claude.com/docs/en/mcp

### Claude Desktop

**Config file:** `~/Library/Application Support/Claude/claude_desktop_config.json`

```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_URL": "http://localhost:6333",
        "RAG_VOYAGE_API_KEY": "your-voyage-api-key",
        "RAG_VOYAGE_MODEL": "voyage-4"
      }
    }
  }
}
```

Source: https://code.claude.com/docs/en/mcp-quickstart

### Cline

**Config file (IDE):** Open Cline panel > MCP Servers > Configure > Configure MCP Servers. This opens the settings JSON.
**Config file (CLI):** `~/.cline/mcp.json`

```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_URL": "http://localhost:6333",
        "RAG_VOYAGE_API_KEY": "your-voyage-api-key",
        "RAG_VOYAGE_MODEL": "voyage-4"
      },
      "disabled": false,
      "autoApprove": []
    }
  }
}
```

Source: https://docs.cline.bot/mcp/configuring-mcp-servers

### Cursor

**Config file (global):** `~/.cursor/mcp.json`
**Config file (project):** `.cursor/mcp.json` in project root

```json
{
  "mcpServers": {
    "enowx-rag": {
      "type": "stdio",
      "command": "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_URL": "http://localhost:6333",
        "RAG_VOYAGE_API_KEY": "your-voyage-api-key",
        "RAG_VOYAGE_MODEL": "voyage-4"
      }
    }
  }
}
```

Note: Cursor uses `"type": "stdio"` explicitly. Supports `${env:VAR_NAME}` interpolation.

Source: https://cursor.com/docs/mcp

### OpenCode

**Config file:** `~/.opencode/settings.json` (or `opencode.json` in project root)

OpenCode uses a different schema: `mcp` key (not `mcpServers`), `command` as array (not string + args), `environment` (not `env`).

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "enowx-rag": {
      "type": "local",
      "command": ["/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server"],
      "enabled": true,
      "environment": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_URL": "http://localhost:6333",
        "RAG_VOYAGE_API_KEY": "your-voyage-api-key",
        "RAG_VOYAGE_MODEL": "voyage-4"
      }
    }
  }
}
```

Source: https://opencode.ai/docs/mcp-servers/

### Codex (OpenAI)

**Config file:** `~/.codex/config.toml` (TOML, not JSON)

**CLI command:**
```bash
codex mcp add enowx-rag \
  --env RAG_VECTOR_STORE=qdrant \
  --env RAG_QDRANT_URL=http://localhost:6333 \
  --env RAG_VOYAGE_API_KEY=your-voyage-api-key \
  -- /Users/enowdev/Project/enowx-rag/mcp-server/mcp-server
```

**TOML format (`config.toml`):**
```toml
[mcp_servers.enowx-rag]
command = "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server"

[mcp_servers.enowx-rag.env]
RAG_VECTOR_STORE = "qdrant"
RAG_QDRANT_URL = "http://localhost:6333"
RAG_VOYAGE_API_KEY = "your-voyage-api-key"
RAG_VOYAGE_MODEL = "voyage-4"
```

Source: https://developers.openai.com/codex/mcp

### Factory Droid

**CLI command:**
```bash
droid mcp add enowx-rag /Users/enowdev/Project/enowx-rag/mcp-server/mcp-server
```

Source: Factory Droid CLI documentation

### Roo Code

**Config file (global):** Open Roo Code > MCP settings > Edit Global MCP (opens `mcp_settings.json`)
**Config file (project):** `.roo/mcp.json` in project root

```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_URL": "http://localhost:6333",
        "RAG_VOYAGE_API_KEY": "your-voyage-api-key",
        "RAG_VOYAGE_MODEL": "voyage-4"
      },
      "alwaysAllow": [],
      "disabled": false
    }
  }
}
```

Source: https://docs.roocode.com/features/mcp/using-mcp-in-roo

### Zed

**Config file:** `~/.config/zed/settings.json` (JSON, but key is `context_servers`, not `mcpServers`)

```json
{
  "context_servers": {
    "enowx-rag": {
      "command": "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server",
      "args": [],
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_URL": "http://localhost:6333",
        "RAG_VOYAGE_API_KEY": "your-voyage-api-key",
        "RAG_VOYAGE_MODEL": "voyage-4"
      }
    }
  }
}
```

Note: Zed also supports adding MCP servers via the UI: Command Palette > `agent: add context server`.

Source: https://zed.dev/docs/ai/mcp

### Windsurf

**Config file:** `~/.codeium/windsurf/mcp_config.json`

```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_URL": "http://localhost:6333",
        "RAG_VOYAGE_API_KEY": "your-voyage-api-key",
        "RAG_VOYAGE_MODEL": "voyage-4"
      }
    }
  }
}
```

Note: Windsurf also supports adding MCP servers via the MCP Marketplace UI.

Source: https://docs.windsurf.com/windsurf/cascade/mcp

### Continue

**Config file:** `~/.continue/config.yaml` (YAML, not JSON)

Continue uses a `mcpServers` list (array of objects, not a map). Each entry has `name`, `command`, `args`, `env`.

```yaml
mcpServers:
  - name: enowx-rag
    command: /Users/enowdev/Project/enowx-rag/mcp-server/mcp-server
    env:
      RAG_VECTOR_STORE: qdrant
      RAG_QDRANT_URL: http://localhost:6333
      RAG_VOYAGE_API_KEY: your-voyage-api-key
      RAG_VOYAGE_MODEL: voyage-4
```

Source: https://docs.continue.dev/reference (see `mcpServers` section)

---

## Quick reference table

| Tool | Format | Key | File |
| --- | --- | --- | --- |
| Claude Code | JSON | `mcpServers` | `~/.claude.json` or `.mcp.json` |
| Claude Desktop | JSON | `mcpServers` | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Cline | JSON | `mcpServers` | `~/.cline/mcp.json` or IDE settings |
| Cursor | JSON | `mcpServers` | `~/.cursor/mcp.json` or `.cursor/mcp.json` |
| OpenCode | JSON | `mcp` | `~/.opencode/settings.json` or `opencode.json` |
| Codex | TOML | `[mcp_servers.*]` | `~/.codex/config.toml` |
| Factory Droid | CLI | — | `droid mcp add` |
| Roo Code | JSON | `mcpServers` | global `mcp_settings.json` or `.roo/mcp.json` |
| Zed | JSON | `context_servers` | `~/.config/zed/settings.json` |
| Windsurf | JSON | `mcpServers` | `~/.codeium/windsurf/mcp_config.json` |
| Continue | YAML | `mcpServers` (list) | `~/.continue/config.yaml` |

---

## Project agent instructions (AGENTS.md + CLAUDE.md)

When setting up a project for RAG memory, generate or update two files in the project root:

1. `AGENTS.md` — universal agent instructions.
2. `CLAUDE.md` — Claude-family specific instructions (Claude Code, Cline, etc.).

These files remind every AI coding assistant to consult project memory before and after work.

### Important: merge, do not replace

If `AGENTS.md` or `CLAUDE.md` already exists in the target project, **append the RAG section to the existing content**. Do not overwrite the file. Follow these steps:

1. Read the existing file.
2. Check if an `enowx-rag` or `RAG memory` section already exists. If so, update it in place.
3. If no RAG section exists, append the template content below the existing content, separated by a `---` divider.
4. Preserve all existing instructions, rules, and project-specific content.

### Per-project collection

Each project gets its own isolated collection in the vector store. The collection name is `project_<project_id>`. When setting up a new project:

1. Derive the `PROJECT_ID` from the project folder name, repo name, or ask the user.
2. Call `rag_create_project` with the project ID to create the collection. This is safe to call even if the collection already exists.
3. Multiple projects can share the same MCP server instance and backend. Each project's memory is isolated by collection name.
4. Never mix project memories. Always use the correct `PROJECT_ID` when indexing or retrieving.

### AGENTS.md template

```markdown
# Project agent instructions

This project uses `enowx-rag` MCP server for per-project memory (RAG).

## Before you start coding

1. Call `rag_retrieve_context` with the user's task/query and the project ID `PROJECT_ID`.
2. Read the returned context. If it is empty or irrelevant, continue as normal.
3. If the context changes how you would approach the task, explain the relevant insights briefly.

## After you finish coding

1. Summarize what you changed and why.
2. Call `rag_index` with useful new facts, design decisions, gotchas, or patterns under project ID `PROJECT_ID`.
3. Call `rag_index_project` with the project directory to sync all file changes into RAG. Always do this — it handles new files, edits, and deletions automatically.
4. Keep chunks focused and concise (one idea per chunk). Include metadata tags when helpful.

## Project ID

Use project ID: `PROJECT_ID`

## Useful metadata tags

- `type:architecture`
- `type:decision`
- `type:api`
- `type:bugfix`
- `type:howto`
- `type:snippet`

## When to index

- New endpoints, functions, or components.
- Non-obvious configuration or environment setup.
- Workarounds, known issues, or pitfalls.
- Reusable code patterns.
- Explanations of business/domain logic.

## When to retrieve

- Before answering questions about existing code or architecture.
- Before planning new features that may overlap with existing systems.
- Before debugging issues that may have been seen before.
```

### CLAUDE.md template

```markdown
# Claude instructions for this project

You are working with a project that has an `enowx-rag` MCP server installed.

## RAG memory workflow

### Before making significant changes

1. Call `rag_retrieve_context` with the project ID `PROJECT_ID` and the user's query.
2. Read the returned context. If it is empty or irrelevant, continue as normal.
3. If the context changes how you would approach the task, explain the relevant insights briefly.

### After completing work

1. Summarize what you changed and why.
2. Call `rag_index` with useful new facts, design decisions, gotchas, or patterns under project ID `PROJECT_ID`.
3. Call `rag_index_project` with the project directory to sync all file changes into RAG. Always do this — it handles new files, edits, and deletions automatically.
4. Keep chunks focused and concise (one idea per chunk). Include metadata tags when helpful.

## Project ID

Use project ID: `PROJECT_ID`

## Metadata tags

Use these tags when indexing:

- `type:architecture`
- `type:decision`
- `type:api`
- `type:bugfix`
- `type:howto`
- `type:snippet`

## When to index

- New endpoints, functions, or components.
- Non-obvious configuration or environment setup.
- Workarounds, known issues, or pitfalls.
- Reusable code patterns.
- Explanations of business/domain logic.

## When to retrieve

- Before answering questions about existing code or architecture.
- Before planning new features that may overlap with existing systems.
- Before debugging issues that may have been seen before.

## Rules

- Keep index chunks small, factual, and tagged with metadata.
- Prefer updating context over repeating what is already known.
- Each project has its own collection: `project_PROJECT_ID`. Do not mix project memories.
```

Replace `PROJECT_ID` with the actual project identifier (e.g., repo name, slug, or UUID). Derive it from the project folder name or ask the user.

## Agent instruction enforcement

When generating these files, also remind the user to:

- Commit `AGENTS.md` and `CLAUDE.md` to version control.
- Keep them at the project root.
- Update `PROJECT_ID` if the project is renamed.
- Re-index the agent instructions themselves if the workflow changes.
- If the file already exists, merge the RAG section instead of replacing the entire file.
- Use templates from `skill/templates/AGENTS.md` and `skill/templates/CLAUDE.md` as the source for the RAG section.

## Example setup summary output

After running the skill, produce a summary like this:

```markdown
## enowx-rag setup complete

### Backend
- Vector store: qdrant
- Qdrant REST: http://localhost:6333
- Embedder: voyage-4 (Voyage AI)

### MCP client config installed
- Claude Code: `~/.claude.json`
- Claude Desktop: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Cline: `~/.cline/mcp.json`
- Cursor: `~/.cursor/mcp.json`
- OpenCode: `~/.opencode/settings.json`
- Codex: `~/.codex/config.toml`
- Factory Droid: `droid mcp add`
- Roo Code: global `mcp_settings.json` or `.roo/mcp.json`
- Zed: `~/.config/zed/settings.json`
- Windsurf: `~/.codeium/windsurf/mcp_config.json`
- Continue: `~/.continue/config.yaml`

### Project agent instructions
Created:
- `/path/to/project/AGENTS.md`
- `/path/to/project/CLAUDE.md`

### Next steps
1. Build MCP server: `cd /Users/enowdev/Project/enowx-rag/mcp-server && go build ./cmd/mcp-server`
2. Restart your coding tool or reload MCP server list.
3. Test by asking the agent to retrieve project context.
```

## Verification checklist

After setup, verify with these commands:

```bash
# Qdrant health (always needed)
curl -f http://localhost:6333/healthz

# TEI health (only if using self-hosted TEI)
curl -f http://localhost:8081/health

# Build MCP server
cd /Users/enowdev/Project/enowx-rag/mcp-server && go build ./cmd/mcp-server
```

## Notes

- The MCP server uses stdio transport by default.
- Each project gets its own collection/index: `project_<project_id>`.
- Default embedder is `voyage-4` (Voyage AI). Falls back to TEI if `RAG_VOYAGE_API_KEY` is not set and `RAG_EMBEDDER` is not specified.
- `AGENTS.md` and `CLAUDE.md` are opt-in: generate them only when the user says yes or asks to enable "always use RAG memory for this project".
