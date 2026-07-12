# Contributing to enowx-rag

Thanks for your interest in improving **enowx-rag** — a per-project RAG memory
skill and MCP server for AI coding agents. Contributions of all sizes are
welcome, from typo fixes to new vector-store providers.

## Ways to contribute

- **Report a bug** — open an issue with steps to reproduce, expected vs actual
  behavior, and your config (vector store, embedder, OS).
- **Suggest a feature** — open an issue describing the use case before writing
  code, so we can agree on the approach.
- **Pick up a `good first issue`** — issues labeled this way are scoped for
  newcomers and have context in the description.
- **Improve docs** — the README, skill guide, and `docs/` are all fair game.

## Development setup

The server is a Go module under `mcp-server/` with an embedded React SPA under
`mcp-server/web/`.

```bash
# Prerequisites: Go 1.26+, Node 20+, (optional) Docker for local Qdrant/TEI

git clone https://github.com/enowdev/enowx-rag.git
cd enowx-rag/mcp-server

# Build everything (frontend + Go binary)
make build          # from repo root: builds web/dist then the binary

# Run the test suite
go test ./...
```

To run the server with the web dashboard locally:

```bash
# Local backend (no API key needed)
docker compose up -d qdrant tei-embedding
RAG_VECTOR_STORE=qdrant RAG_EMBEDDER=tei \
  RAG_QDRANT_URL=http://localhost:6333 RAG_TEI_URL=http://localhost:8081 \
  ./enowx-rag --serve

# then open http://localhost:7777
```

See the [README](README.md) for the full list of environment variables.

## Pull request checklist

Before opening a PR, please make sure:

- [ ] `go build ./...` and `go test ./...` pass under `mcp-server/`
- [ ] `npm run build` passes under `mcp-server/web/` (if you touched the SPA)
- [ ] New behavior has a test that locks it in
- [ ] Commit messages are descriptive (we follow `type: summary`, e.g.
      `fix:`, `feat:`, `docs:`, `test:`)
- [ ] You've run `go vet ./...`

Keep PRs focused — one logical change per PR is much easier to review.

## Code style

- **Go**: standard `gofmt`; match the surrounding code's naming and comment
  density. The `rag.Provider` interface is the contract for vector stores —
  new backends implement it (and optionally `HybridSearcher`, `Reranker`).
- **TypeScript/React**: follow the existing component structure under
  `web/src/`. Keep the true-black flat design tokens in `styles/tokens.css`.

## Reporting security issues

Please **do not** open a public issue for security vulnerabilities. See
[SECURITY.md](SECURITY.md) for how to report them privately.

## Code of Conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md). By
participating, you agree to uphold it.

## License

By contributing, you agree that your contributions will be licensed under the
[Apache License 2.0](LICENSE) that covers this project.
