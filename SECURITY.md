# Security Policy

## Reporting a vulnerability

We take the security of enowx-rag seriously. If you discover a vulnerability,
please report it **privately** — do not open a public issue.

**How to report:**

- Use GitHub's [private vulnerability reporting](https://github.com/enowdev/enowx-rag/security/advisories/new)
  (Security → Advisories → Report a vulnerability), or
- Email the maintainer at the address listed on the
  [GitHub profile](https://github.com/enowdev).

Please include:

- A description of the vulnerability and its impact
- Steps to reproduce (proof-of-concept if possible)
- Affected version / commit
- Your suggested remediation, if any

## What to expect

- We aim to acknowledge your report within **3 business days**.
- We will keep you informed as we investigate and work on a fix.
- Once a fix is released, we will credit you in the advisory unless you prefer
  to remain anonymous.

## Scope

This project is an MCP server and skill that handles:

- API keys for embedding/reranking providers (e.g. Voyage AI) via environment
  variables
- An optional admin token (`RAG_ADMIN_TOKEN`) protecting the HTTP `/api/*`
  endpoints
- Content indexed from user codebases, stored in a vector database

Areas of particular interest for security reports include: authentication
bypass on the HTTP API, cross-project data leakage, SSRF/injection in the
vector-store or embedding requests, and secret exposure in logs or responses.

## Hardening notes for operators

- Set `RAG_ADMIN_TOKEN` when exposing the `--serve` HTTP API beyond localhost.
- The SSE event stream (`/api/events`) is same-origin only unless you set
  `RAG_CORS_ORIGIN` — leave it unset in single-origin deployments.
- Never commit `RAG_VOYAGE_API_KEY` or other secrets; pass them via the
  environment or your MCP client config.
