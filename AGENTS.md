# Agent instructions: install enowx-rag skill and MCP server

This project includes a per-project RAG memory MCP server (`enowx-rag`) and a Factory Droid skill.

When asked to install, set up, or configure RAG memory for this project, follow the steps below.

## 1. Install the Factory Droid skill

The skill lives at `skill/enowx-rag.md` in this repository.

### For Factory Droid (this tool)

1. Copy the skill file to the Droid skill directory:

   ```bash
   mkdir -p ~/.factory/skills/enowx-rag
   cp /Users/enowdev/Project/enowx-rag/skill/enowx-rag.md ~/.factory/skills/enowx-rag/skill.md
   ```

2. If Factory Droid requires a reload, restart the session or run any skill-refresh command available.

3. After installing, use the skill by following the setup wizard in `skill/enowx-rag.md` whenever the user asks for RAG setup.

## 2. Build the MCP server

The MCP server is written in Go and lives in `mcp-server/`.

```bash
cd /Users/enowdev/Project/enowx-rag/mcp-server
go build ./cmd/mcp-server
```

## 3. Choose embedding option

**Option A — Voyage AI (recommended):** No local setup needed. Get a free API key at [voyageai.com](https://voyageai.com) (`voyage-4` has 200M free tokens). Set `RAG_VOYAGE_API_KEY` in the MCP config.

**Option B — Self-hosted TEI:** Start Qdrant + TEI with Docker:

```bash
cd /Users/enowdev/Project/enowx-rag/mcp-server
docker compose up -d qdrant tei-embedding
```

If connecting to an existing backend, skip this step and set the correct URLs in the tool config.

## 4. Install the MCP server in coding tools

Each tool has a different config format and file location. See [README.md](https://github.com/enowdev/enowx-rag/blob/main/README.md) for the exact config block per tool.

### Quick reference

| Tool | Format | Key | Config file |
| --- | --- | --- | --- |
| Claude Code | JSON | `mcpServers` | `~/.claude.json` or `.mcp.json` |
| Claude Desktop | JSON | `mcpServers` | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Cline | JSON | `mcpServers` | `~/.cline/mcp.json` or IDE settings |
| Cursor | JSON | `mcpServers` | `~/.cursor/mcp.json` or `.cursor/mcp.json` |
| OpenCode | JSON | `mcp` | `~/.opencode/settings.json` or `opencode.json` |
| Codex (OpenAI) | TOML | `[mcp_servers.*]` | `~/.codex/config.toml` |
| Factory Droid | CLI | — | `droid mcp add enowx-rag /path/to/mcp-server` |
| Roo Code | JSON | `mcpServers` | global `mcp_settings.json` or `.roo/mcp.json` |
| Zed | JSON | `context_servers` | `~/.config/zed/settings.json` |
| Windsurf | JSON | `mcpServers` | `~/.codeium/windsurf/mcp_config.json` |
| Continue | YAML | `mcpServers` (list) | `~/.continue/config.yaml` |

**Important differences from the standard JSON format:**
- **OpenCode**: uses `mcp` key (not `mcpServers`), `command` as array, `environment` (not `env`)
- **Codex**: uses TOML format with `[mcp_servers.<name>]` tables
- **Zed**: uses `context_servers` key (not `mcpServers`)
- **Continue**: uses YAML with `mcpServers` as a list of objects (not a map)

Replace environment values if using Chroma or pgvector instead of Qdrant.

## 5. Generate AGENTS.md and CLAUDE.md for the target project

If the user wants to enable RAG memory for a specific project, create or update these files in the target project root:

- `AGENTS.md` — universal instructions for all AI agents.
- `CLAUDE.md` — Claude-family specific instructions.

**Important: merge, do not replace.** If the file already exists, append the RAG section below existing content separated by `---`. Use templates from `skill/templates/AGENTS.md` and `skill/templates/CLAUDE.md`.

**Per-project collection:** Each project gets its own collection `project_<PROJECT_ID>`. Call `rag_create_project` to create it. Multiple projects share the same MCP server, but each has isolated memory.

## 6. Always use RAG memory

Once installed, follow this workflow:

- **Before coding:** call `rag_retrieve_context` with the user's query and the project ID.
- **After coding:** call `rag_index` to save new knowledge, design decisions, gotchas, and reusable patterns.
- Each project has its own collection. Never mix project memories.

## Reference

Full setup guide: https://github.com/enowdev/enowx-rag/blob/main/README.md

Skill guide: https://github.com/enowdev/enowx-rag/blob/main/skill/enowx-rag.md

