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
3. Keep chunks focused and concise (one idea per chunk). Include metadata tags when helpful.

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
