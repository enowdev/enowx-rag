# Project agent instructions

This project uses `enowx-rag` MCP server for per-project memory (RAG).

## Before you start coding

1. If the project has never been indexed (`rag_project_exists` returns false), call `rag_index_project` once with the project directory.
2. Call `rag_retrieve_context` with the user's task/query and the project ID `PROJECT_ID`.
3. Read the returned context. If it is empty or irrelevant, continue as normal.
4. If the context changes how you would approach the task, explain the relevant insights briefly.

## After you finish coding

1. Summarize what you changed and why.
2. Call `rag_index` with useful new facts, design decisions, gotchas, or patterns under project ID `PROJECT_ID` — code is synced automatically, but the reasons behind it are only saved if you save them.
3. You don't need to re-index files after each change: once a directory has been indexed with `rag_index_project`, the server re-syncs it automatically before searches (when the index is older than `RAG_AUTO_SYNC_INTERVAL`, default 60s). Call `rag_index_project` again only to force an immediate sync, or when auto-sync is turned off.
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
