# Project agent instructions

This project uses `enowx-rag` MCP server for per-project memory (RAG).

## Before you start coding

1. Call `rag_retrieve_context` with the user's task/query and the project ID `PROJECT_ID`.
2. Read the returned context. If it is empty or irrelevant, continue as normal.
3. If the context changes how you would approach the task, explain the relevant insights briefly.

## After you finish coding

1. Summarize what you changed and why.
2. Call `rag_index` with useful new facts, design decisions, gotchas, or patterns under project ID `PROJECT_ID`.
3. If files were added, changed, or deleted, call `rag_index_project` with the project directory to sync the codebase. This handles deletions automatically.
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
