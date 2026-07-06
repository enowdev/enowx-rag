# Agent instructions: install enowx-rag skill and MCP server

This project includes a per-project RAG memory MCP server (`enowx-rag`) and a skill.

When asked to install, set up, or configure RAG memory, first decide which of two paths applies:

- **Path A — Full setup from scratch:** enowx-rag is not installed yet. Do the full flow below (skill, build the Go server, choose embedder, install the MCP server in every tool, then onboard the project). Start at section 1.
- **Path B — Onboard a new project:** the enowx-rag MCP server is already installed and its tools are available (e.g. you can call `rag_create_project` / `rag_retrieve_context`). Skip the build/backend/tool-install steps and jump straight to section 5 (generate AGENTS.md + CLAUDE.md and create the project collection), plus install the skill (section 1). This is the common case for every project after the first.

To tell them apart: check whether the enowx-rag MCP tools are already available. If they are, use Path B. If they are not (or you are unsure and the binary isn't built), use Path A.

## 1. Install the skill

The skill lives at `skill/enowx-rag.md` in this repository. It is not Factory-Droid-only — install it into whatever skill directory your tool reads. The skill can be global (shared across projects) or project-local.

Pick the destination directory for the current tool. Common locations:

- **Factory Droid (global):** `~/.factory/skills/enowx-rag/skill.md`
- **Project-local skill:** `.agents/skills/enowx-rag/skill.md` or `.factory/skills/enowx-rag/skill.md` in the project root
- Any other skill folder your tool loads — do not assume `~/.factory`. Ask the user if you are unsure which directory their tool uses.

Copy the skill into the chosen directory, e.g. (global Factory Droid):

```bash
mkdir -p ~/.factory/skills/enowx-rag
cp /Users/enowdev/Project/enowx-rag/skill/enowx-rag.md ~/.factory/skills/enowx-rag/skill.md
```

Or project-local:

```bash
mkdir -p .agents/skills/enowx-rag
cp /Users/enowdev/Project/enowx-rag/skill/enowx-rag.md .agents/skills/enowx-rag/skill.md
```

If the tool requires a reload, restart the session or run any skill-refresh command available. After installing, follow the setup wizard in `skill/enowx-rag.md` whenever the user asks for RAG setup.

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

## 5. Onboard the target project (AGENTS.md + CLAUDE.md + collection)

This is the whole job for **Path B**, and the final step for Path A. To enable RAG memory for a specific project:

1. **Derive `PROJECT_ID`** from the project folder name or repo name (or ask the user).
2. **Create the collection.** Call `rag_create_project` with `PROJECT_ID`. Each project gets its own collection `project_<PROJECT_ID>`; multiple projects share the same MCP server but keep isolated memory. Safe to call if it already exists.
3. **Create or update the instruction files** in the target project root:
   - `AGENTS.md` — universal instructions for all AI agents.
   - `CLAUDE.md` — Claude-family specific instructions.

   **Important: merge, do not replace.** If the file already exists, append the RAG section below existing content separated by `---`. Use templates from `skill/templates/AGENTS.md` and `skill/templates/CLAUDE.md`, with `PROJECT_ID` filled in.
4. **Seed the collection.** Call `rag_index_project` on the project directory so the current files are indexed right away.

## 6. Always use RAG memory

Once installed, follow this workflow:

- **Before coding:** call `rag_retrieve_context` with the user's query and the project ID.
- **After coding:** call `rag_index` to save new knowledge, design decisions, gotchas, and reusable patterns.
- Each project has its own collection. Never mix project memories.

## Reference

Full setup guide: https://github.com/enowdev/enowx-rag/blob/main/README.md

Skill guide: https://github.com/enowdev/enowx-rag/blob/main/skill/enowx-rag.md

