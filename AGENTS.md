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

## 3. Start the RAG backend (if local mode)

Use Docker Compose to start Qdrant and TEI:

```bash
cd /Users/enowdev/Project/enowx-rag/mcp-server
docker compose up -d qdrant tei-embedding
```

If connecting to an existing backend, skip this step and set the correct URLs in the tool config.

## 4. Install the MCP server in coding tools

Use the same MCP server block in every tool. Only the file location changes.

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
| OpenCode | `~/.opencode/settings.json` |
| Codex (OpenAI) | `~/.codex/config.json` |
| Factory Droid | `droid mcp add enowx-rag /Users/enowdev/Project/enowx-rag/mcp-server/mcp-server` |
| Roo Code | `~/Library/Application Support/Code/User/globalStorage/rooveterinaryinc.roo/settings.json` |
| Zed | `~/.config/zed/settings.json` under `mcp_servers` |
| Windsurf | `~/.windsurf/mcp_config.json` or `~/.codeium/windsurf/mcp_config.json` |
| Continue | `~/.continue/config.json` under `mcpServers` |

Replace environment values if using Chroma or pgvector instead of Qdrant.

## 5. Generate AGENTS.md and CLAUDE.md for the target project

If the user wants to enable RAG memory for a specific project, create these files in the target project root:

- `AGENTS.md` — universal instructions for all AI agents.
- `CLAUDE.md` — Codebuddy-family specific instructions.

Use the templates in `skill/templates/AGENTS.md` and adapt the `PROJECT_ID` placeholder to the target project name.

## 6. Always use RAG memory

Once installed, follow this workflow:

- **Before coding:** call `rag_retrieve_context` with the user's query and the project ID.
- **After coding:** call `rag_index` to save new knowledge, design decisions, gotchas, and reusable patterns.

## Reference

Full setup guide: https://github.com/enowX-Labs/enowx-rag/blob/main/README.md

Skill guide: https://github.com/enowX-Labs/enowx-rag/blob/main/skill/enowx-rag.md

