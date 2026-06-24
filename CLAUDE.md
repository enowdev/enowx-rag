# Claude instructions for this project

This project is `enowx-rag`, a per-project RAG memory MCP server.

## Setup command

If the user asks to install the skill or MCP server, run:

```bash
mkdir -p ~/.factory/skills/enowx-rag
cp /Users/enowdev/Project/enowx-rag/skill/enowx-rag.md ~/.factory/skills/enowx-rag/skill.md
cd /Users/enowdev/Project/enowx-rag/mcp-server
go build ./cmd/mcp-server
```

For local backend:

```bash
cd /Users/enowdev/Project/enowx-rag/mcp-server
docker compose up -d qdrant tei-embedding
```

## MCP server config

Add this to the user's MCP client config (e.g. `~/.claude/config.json` or `claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_EMBEDDER": "tei",
        "RAG_QDRANT_URL": "http://localhost:6333",
        "RAG_TEI_URL": "http://localhost:8081"
      }
    }
  }
}
```

## Always use RAG memory

- Before coding: call `rag_retrieve_context` with project ID `enowx-rag` and the user's query.
- After coding: call `rag_index` to save new facts, design decisions, and patterns under project ID `enowx-rag`.

Keep index chunks concise and tag metadata such as `type:architecture`, `type:decision`, `type:howto`, or `type:snippet`.

## Reference

- Setup guide: https://github.com/enowdev/enowx-rag/blob/main/README.md
- Skill guide: https://github.com/enowdev/enowx-rag/blob/main/skill/enowx-rag.md
- Agent install guide: https://github.com/enowdev/enowx-rag/blob/main/AGENTS.md
