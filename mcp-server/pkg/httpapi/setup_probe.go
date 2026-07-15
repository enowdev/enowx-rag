package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// agentsMarkerStart / agentsMarkerEnd delimit the enowx-rag block inside a
// project's AGENTS.md so it can be merged idempotently without touching the
// user's own content.
const (
	agentsMarkerStart = "<!-- enowx-rag:start -->"
	agentsMarkerEnd   = "<!-- enowx-rag:end -->"
)

// skillDirs are the known per-client skill directories. Only some clients have
// a skill system; the skill is "installed" if present in any of them.
var skillDirs = []string{"~/.claude/skills/enowx-rag", "~/.factory/skills/enowx-rag"}

// SetupProbe handles GET /api/setup/probe?client=<id>&dir=<project-dir>.
// It reports what is already set up so an agent can skip finished steps:
//   - mcp: whether the enowx-rag server is in the client's config (per client,
//     or all clients when `client` is omitted)
//   - skill: whether the skill is installed in a known skill directory
//   - agents_md: whether the project's AGENTS.md exists and already contains the
//     enowx-rag block
func (h *Handlers) SetupProbe(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client")
	dir := r.URL.Query().Get("dir")

	// MCP status: one client, or a map of all clients.
	mcp := map[string]bool{}
	if clientID != "" {
		if c, ok := mcpClientByID(clientID); ok {
			mcp[c.ID] = c.isInstalled()
		}
	} else {
		for _, c := range mcpClients {
			mcp[c.ID] = c.isInstalled()
		}
	}

	// Skill status.
	skillInstalled := false
	skillDir := ""
	for _, d := range skillDirs {
		p := expandHome(d)
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			skillInstalled = true
			skillDir = p
			break
		}
	}

	// AGENTS.md status for the given project directory.
	agentsExists := false
	agentsHasBlock := false
	agentsPath := ""
	if dir != "" {
		agentsPath = filepath.Join(expandHome(dir), "AGENTS.md")
		if data, err := os.ReadFile(agentsPath); err == nil {
			agentsExists = true
			agentsHasBlock = strings.Contains(string(data), agentsMarkerStart)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"mcp": mcp,
		"skill": map[string]any{
			"installed": skillInstalled,
			"dir":       skillDir,
		},
		"agents_md": map[string]any{
			"path":      agentsPath,
			"exists":    agentsExists,
			"has_block": agentsHasBlock,
		},
	})
}

// agentsBlock builds the enowx-rag AGENTS.md block for a project, wrapped in the
// idempotent markers so it can be merged in and out cleanly.
func agentsBlock(projectID string) string {
	return fmt.Sprintf(`%s
## enowx-rag memory (project: %s)

This project uses the enowx-rag MCP server for per-project RAG memory.

- **Before coding**: call `+"`rag_retrieve_context`"+` with project ID `+"`%s`"+` and the user's query; use any relevant context.
- **After coding**: call `+"`rag_index`"+` with new facts/decisions/gotchas, then `+"`rag_index_project`"+` with the project directory to sync file changes. Keep chunks focused; tag with `+"`type:architecture|decision|api|bugfix|howto|snippet`"+`.
- Each project has its own collection; do not mix project memories.
%s`, agentsMarkerStart, projectID, projectID, agentsMarkerEnd)
}

// SetupWriteAgentsMD handles POST /api/setup/write-agents-md.
// Body: {dir, project_id}. It merges the enowx-rag block into the project's
// AGENTS.md idempotently: if the file has the markers, the block is replaced;
// if the file exists without markers, the block is appended; if the file does
// not exist, it is created with the block. The user's own content is preserved.
func (h *Handlers) SetupWriteAgentsMD(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Dir       string `json:"dir"`
		ProjectID string `json:"project_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Dir == "" || req.ProjectID == "" {
		writeErr(w, http.StatusBadRequest, "dir and project_id are required")
		return
	}
	path := filepath.Join(expandHome(req.Dir), "AGENTS.md")
	block := agentsBlock(req.ProjectID)

	existing, err := os.ReadFile(path)
	var out string
	action := ""
	switch {
	case err != nil && os.IsNotExist(err):
		out = "# Agent instructions\n\n" + block + "\n"
		action = "created"
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "read AGENTS.md: "+err.Error())
		return
	default:
		content := string(existing)
		if i := strings.Index(content, agentsMarkerStart); i >= 0 {
			// Replace the existing block (idempotent update).
			j := strings.Index(content, agentsMarkerEnd)
			if j < 0 || j < i {
				writeErr(w, http.StatusConflict, "AGENTS.md has a start marker without a matching end marker; fix it manually")
				return
			}
			out = content[:i] + block + content[j+len(agentsMarkerEnd):]
			action = "updated"
		} else {
			// Append the block, preserving the user's content.
			sep := "\n\n"
			if strings.HasSuffix(content, "\n") {
				sep = "\n"
			}
			out = content + sep + block + "\n"
			action = "appended"
		}
	}

	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		writeErr(w, http.StatusInternalServerError, "write AGENTS.md: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": action, "path": path})
}
