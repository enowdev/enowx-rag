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
- Keep the MCP server config snippet copy-pasteable for Claude Desktop, Cline, or Cursor.

## Setup flow

1. **Detect context**
   - Check if the user already has a clone of `enowx-rag`.
   - **Ask first:** "Do you already have Qdrant and TEI installed and running?"
     - If **yes**: ask for the Qdrant gRPC address and TEI URL, then continue to build MCP server.
     - If **no**: ask whether the user wants to install locally with Docker, and offer an embedding model suited to their hardware.
   - **Choose embedding model** (if installing locally):
     - Default: `BAAI/bge-small-en-v1.5` (384-dim, ~133 MB, good balance)
     - Low-resource / older CPU: `sentence-transformers/all-MiniLM-L6-v2` (384-dim, smaller, faster)
     - Better quality / more RAM: `BAAI/bge-base-en-v1.5` (768-dim, larger and slower)
   - If existing backend: ask for vector store type (`qdrant`, `chroma`, `pgvector`), URLs, and any API keys.
   - If local backend: generate or update `docker-compose.yml` for Qdrant + TEI with the chosen model.

2. **Install / start the backend (when needed)**
   - Verify Docker/Colima is available.
   - Run `docker compose up -d qdrant tei-embedding`.
   - Wait for health checks (`curl -f http://localhost:6333/healthz` and `curl -f http://localhost:8081/health`).

3. **Build / configure the MCP server**
   - Build the Go binary with `go build ./cmd/mcp-server`.
   - Create `.env` from the chosen configuration.
   - Provide the MCP client config snippet for all supported tools.

4. **Optional Coolify deployment**
   - If the user wants to deploy to Coolify, use the `coolify` MCP tools to create/update the service/app.
   - Use the existing `robloxkit-rag` service (Qdrant + TEI) if it is healthy and matches the requirements.

## Required user info

Ask these questions one at a time or in a compact batch:

1. `project_path` - Where should the `enowx-rag` repository be created/cloned? (default: `/Users/enowdev/Project/enowx-rag`)
2. `backend_ready` - Do you already have Qdrant + TEI installed and running? (`yes`/`no`)
3. If `yes`:
   - `vector_store` (`qdrant`/`chroma`/`pgvector`)
   - `qdrant_addr` (host:port, e.g. `localhost:6334`)
   - `tei_url` (e.g. `http://localhost:8081`)
   - `pgvector_dsn` (if vector_store is `pgvector`)
4. If `no`:
   - `embedding_model` — choose from:
     - `BAAI/bge-small-en-v1.5` (default, 384-dim, ~133 MB)
     - `sentence-transformers/all-MiniLM-L6-v2` (lightweight, 384-dim, faster on low-end CPUs)
     - `BAAI/bge-base-en-v1.5` (better quality, 768-dim, needs more RAM)
   - `tei_port` (default `8081`)
   - `qdrant_rest_port` (default `6333`)
   - `qdrant_grpc_port` (default `6334`)
   - `hf_token` (optional, only if the chosen model is gated)
5. `tools_to_install` — Which coding tools should use this MCP server? (Claude, Cline, Cursor, OpenCode, Codex, Factory Droid, Roo, Zed, Windsurf, Continue). Provide configs for all of them anyway, but prioritize the ones the user selects.
6. `target_project` — Path to the project that should receive `AGENTS.md` + `CLAUDE.md` (optional).

## Output artifacts

- `enowx-rag/.env`
- `enowx-rag/mcp-server/docker-compose.yml` (only for local mode, with chosen model)
- MCP client config snippet (JSON)
- Verification commands (curl / docker ps / go test)
- `AGENTS.md` and `CLAUDE.md` in the target project (if requested)

## Example `.env` (existing backend)

```bash
RAG_VECTOR_STORE=qdrant
RAG_EMBEDDER=tei
RAG_QDRANT_ADDR=localhost:6334
RAG_TEI_URL=http://localhost:8081
```

## Example `.env` (local backend)

```bash
RAG_VECTOR_STORE=qdrant
RAG_EMBEDDER=tei
RAG_QDRANT_ADDR=localhost:6334
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

## Example MCP client config

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

For Cline, place the same JSON under `mcpServers` in `~/Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev/settings/claude_mcp_settings.json`.

## Verification checklist

After setup, verify with these commands:

```bash
# Qdrant health
curl -f http://localhost:6333/healthz

# TEI health
curl -f http://localhost:8081/health

# Build MCP server
cd /Users/enowdev/Project/enowx-rag/mcp-server && go build ./cmd/mcp-server
```

When the user wants to install this skill for ALL their coding tools, generate configs for each supported tool and provide copy-paste paths. Ask which tools they actually use, but still provide examples for all of them so the instructions are future-proof.

### Supported MCP client configs

The same server block can be reused; only the file location differs. Use the absolute path to the built binary.

#### macOS / Linux paths

- **Claude Code (anthropic CLI):** `~/.claude/CLAUDE.md` and `~/.claude/config.json` for MCP. MCP config is usually in `~/.claude/config.json` under `mcpServers`.
- **Cline:** `~/Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev/settings/claude_mcp_settings.json`
- **Claude Desktop:** `~/Library/Application Support/Claude/settings.json`
- **Claude Code (VS Code):** same as Claude Desktop settings, or workspace `.vscode/mcp.json` if supported.
- **OpenCode:** `~/.opencode/settings.json` (or workspace `.opencode/mcp.json` if supported).
- **Codex (OpenAI):** `~/.codex/config.json` or `~/.codex/settings.json` depending on version; look for `mcpServers`.
- **Factory Droid:** place the same JSON under the Droid settings for MCP, or guide the user to run `droid mcp add <command>`. Factory Droid usually supports the same `mcpServers` format in its CLI config.
- **Cursor:** `~/.cursor/mcp.json`
- **Roo Code:** `~/Library/Application Support/Code/User/globalStorage/rooveterinaryinc.roo/settings.json` or Roo's own MCP panel export.
- **Zed:** `~/.config/zed/settings.json` under `mcp_servers` or `assistant.mcp.servers` (check latest Zed docs; format is similar).
- **Windsurf:** `~/.windsurf/mcp_config.json` or `~/.codeium/windsurf/mcp_config.json`.
- **Continue:** `~/.continue/config.json` under `models` or `mcpServers` (format may vary; prefer `mcpServers` if present).
- **Generic / Claude Desktop:** `claude_desktop_config.json` located at `~/Library/Application Support/Claude/claude_desktop_config.json`.

### Universal MCP server block

```json
"enowx-rag": {
  "command": "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server",
  "env": {
    "RAG_VECTOR_STORE": "qdrant",
    "RAG_EMBEDDER": "tei",
    "RAG_QDRANT_ADDR": "localhost:6334",
    "RAG_TEI_URL": "http://localhost:8081"
  }
}
```

### Full per-tool examples

#### Claude Code / Claude Desktop

File: `~/Library/Application Support/Claude/claude_desktop_config.json`

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

Claude Code also reads `CLAUDE.md` from the project root. Make sure to generate/update that file in the user's project.

#### Cline

File: `~/Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev/settings/claude_mcp_settings.json`

```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_ADDR": "localhost:6334",
        "RAG_TEI_URL": "http://localhost:8081"
      }
    }
  }
}
```

#### Claude Desktop / Claude Code

File: `~/Library/Application Support/Claude/settings.json`

```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_ADDR": "localhost:6334",
        "RAG_TEI_URL": "http://localhost:8081"
      }
    }
  }
}
```

#### OpenCode

File: `~/.opencode/settings.json` (or workspace `.opencode/mcp.json`)

```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_ADDR": "localhost:6334",
        "RAG_TEI_URL": "http://localhost:8081"
      }
    }
  }
}
```

#### Codex (OpenAI)

File: `~/.codex/config.json`

```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_ADDR": "localhost:6334",
        "RAG_TEI_URL": "http://localhost:8081"
      }
    }
  }
}
```

#### Factory Droid

Factory Droid supports the standard `mcpServers` format. If the user uses `droid` CLI, they can run:

```bash
droid mcp add enowx-rag /Users/enowdev/Project/enowx-rag/mcp-server/mcp-server
```

Or place the JSON in the Droid MCP settings if available:

```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_ADDR": "localhost:6334",
        "RAG_TEI_URL": "http://localhost:8081"
      }
    }
  }
}
```

#### Cursor

File: `~/.cursor/mcp.json`

```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_ADDR": "localhost:6334",
        "RAG_TEI_URL": "http://localhost:8081"
      }
    }
  }
}
```

#### Roo Code

File: `~/Library/Application Support/Code/User/globalStorage/rooveterinaryinc.roo/settings.json` (or Roo MCP panel export)

```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_ADDR": "localhost:6334",
        "RAG_TEI_URL": "http://localhost:8081"
      }
    }
  }
}
```

#### Zed

File: `~/.config/zed/settings.json` (Zed format may vary; check `mcp_servers` key)

```json
{
  "mcp_servers": {
    "enowx-rag": {
      "command": "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_ADDR": "localhost:6334",
        "RAG_TEI_URL": "http://localhost:8081"
      }
    }
  }
}
```

#### Windsurf

File: `~/.windsurf/mcp_config.json` or `~/.codeium/windsurf/mcp_config.json`

```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_ADDR": "localhost:6334",
        "RAG_TEI_URL": "http://localhost:8081"
      }
    }
  }
}
```

#### Continue

File: `~/.continue/config.json` (use `mcpServers` if available)

```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_ADDR": "localhost:6334",
        "RAG_TEI_URL": "http://localhost:8081"
      }
    }
  }
}
```

## Project agent instructions (AGENTS.md + CLAUDE.md)

When setting up a project for RAG memory, generate or update two files in the project root:

1. `AGENTS.md` — universal agent instructions.
2. `CLAUDE.md` — Claude-family specific instructions (Claude Code, Cline, Cursor, etc.).

These files remind every AI coding assistant to consult project memory before and after work.

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
3. Keep chunks focused and concise (one idea per chunk). Include metadata tags when helpful.

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

Always consult project memory before making significant changes. Use tool:

- `rag_retrieve_context` — to get context for a query.

Always update project memory after completing work. Use tool:

- `rag_index` — to store concise chunks of new knowledge.

Use project ID: `PROJECT_ID`

Keep index chunks small, factual, and tagged with metadata. Prefer updating context over repeating what is already known.
```

Replace `PROJECT_ID` with the actual project identifier (e.g., repo name, slug, or UUID). Derive it from the project folder name or ask the user.

## Agent instruction enforcement

When generating these files, also remind the user to:

- Commit `AGENTS.md` and `CLAUDE.md` to version control.
- Keep them at the project root.
- Update `PROJECT_ID` if the project is renamed.
- Re-index the agent instructions themselves if the workflow changes.

## Example setup summary output

After running the skill, produce a summary like this:

```markdown
## enowx-rag setup complete

### Backend
- Vector store: qdrant
- Qdrant gRPC: localhost:6334
- TEI: http://localhost:8081

### MCP client config
- Claude Desktop: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Cline: `~/Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev/settings/claude_mcp_settings.json`
- Claude Desktop: `~/Library/Application Support/Claude/settings.json`
- Cursor: `~/.cursor/mcp.json`
- Factory Droid: `droid mcp add enowx-rag /Users/enowdev/Project/enowx-rag/mcp-server/mcp-server`

### Project agent instructions
Created:
- `/path/to/project/AGENTS.md`
- `/path/to/project/CLAUDE.md`

### Next steps
1. Start the backend: `cd /Users/enowdev/Project/enowx-rag/mcp-server && docker compose up -d qdrant tei-embedding`
2. Build MCP server: `cd /Users/enowdev/Project/enowx-rag/mcp-server && go build ./cmd/mcp-server`
3. Restart your coding tool or reload MCP server list.
4. Test by asking the agent to retrieve project context.
```

## Verification checklist

After setup, verify with these commands:

```bash
# Qdrant health
curl -f http://localhost:6333/healthz

# TEI health
curl -f http://localhost:8081/health

# Build MCP server
cd /Users/enowdev/Project/enowx-rag/mcp-server && go build ./cmd/mcp-server

# Test MCP server is callable (optional, requires an MCP client)
# Use Claude Code, Cline, or Factory Droid tool panel to call `rag_create_project`.
```

## Notes

- The MCP server uses stdio transport by default.
- Each project gets its own collection/index: `project_<project_id>`.
- The `robloxkit-rag` Coolify service already exposes Qdrant on `localhost:6333` and TEI on `localhost:8081` if Colima/Docker is running.
- `AGENTS.md` and `CLAUDE.md` are opt-in: generate them only when the user says yes or asks to enable "always use RAG memory for this project".
