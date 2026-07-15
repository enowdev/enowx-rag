package httpapi

import (
	"fmt"
	"net/http"
	"os"
)

// SetupDocs handles GET /api/docs/setup. It returns markdown instructions that
// an AI agent reads and follows to set up enowx-rag for a project: probe what's
// already installed, then install only the missing pieces (MCP, skill, AGENTS.md).
// The short copy-paste prompt in the UI points the agent here.
func (h *Handlers) SetupDocs(w http.ResponseWriter, r *http.Request) {
	// The server's own base URL isn't reliably knowable; use the request host.
	base := "http://" + r.Host
	if r.TLS != nil {
		base = "https://" + r.Host
	}
	exe, _ := os.Executable()

	doc := fmt.Sprintf(`# enowx-rag agent setup

Set up enowx-rag (per-project RAG memory) for the current project. Do the steps
below in order. Skip any step that the probe reports as already done. All calls
go to the enowx-rag server at %s.

## 1. Probe what already exists

GET %s/api/setup/probe?client=<CLIENT_ID>&dir=<ABS_PROJECT_DIR>

Response:
- mcp: { "<client>": true|false }  — is the enowx-rag MCP server in that client's config
- skill: { installed: bool, dir }  — is the skill installed
- agents_md: { exists, has_block } — does the project's AGENTS.md have the enowx-rag block

Pick CLIENT_ID from: claude-code, claude-desktop, cursor, cline, windsurf, codex, zed, continue.

## 2. Install the MCP server (skip if mcp[client] is true)

POST %s/api/setup/install-mcp  { "client_id": "<CLIENT_ID>", "scope": "global" }

This merges the enowx-rag server into the client's config (backing up the
original). For a manual snippet instead: GET %s/api/setup/mcp-snippet?client_id=<CLIENT_ID>.

The MCP server binary is: %s

## 3. Install the skill (skip if skill.installed is true)

Skills are supported by some clients only (e.g. Claude Code, Factory). Get the
exact commands: GET %s/api/setup/skill-guide — then run them (they copy the
skill markdown into the client's skills directory).

## 4. Write the project's AGENTS.md (skip if agents_md.has_block is true)

POST %s/api/setup/write-agents-md  { "dir": "<ABS_PROJECT_DIR>", "project_id": "<PROJECT_ID>" }

This merges an enowx-rag section into AGENTS.md idempotently (markers
<!-- enowx-rag:start --> ... <!-- enowx-rag:end -->), preserving existing content.

## 5. Index the project (optional but recommended)

POST %s/api/projects/<PROJECT_ID>/reindex  { "directory": "<ABS_PROJECT_DIR>" }

## Notes
- Use an absolute project directory for dir.
- PROJECT_ID is a short slug for this project (e.g. the repo name).
- Endpoints that write files require the request to originate from localhost (or a valid RAG_ADMIN_TOKEN).
`, base, base, base, base, exe, base, base, base)

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(doc))
}
