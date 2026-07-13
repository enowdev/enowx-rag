package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/enowdev/enowx-rag/pkg/config"
)

// mcpServerEntry builds the command + env for the enowx-rag MCP server from the
// currently saved config. The command is this binary's own path so the client
// launches the exact server the user configured.
func mcpServerEntry() (mcpEntry, error) {
	exe, err := os.Executable()
	if err != nil {
		return mcpEntry{}, fmt.Errorf("resolve binary path: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return mcpEntry{}, fmt.Errorf("load config: %w", err)
	}

	env := map[string]string{
		"RAG_VECTOR_STORE": cfg.VectorStore,
		"RAG_EMBEDDER":     cfg.Embedder,
	}
	switch cfg.VectorStore {
	case "qdrant":
		env["RAG_QDRANT_URL"] = cfg.QdrantURL
		if cfg.QdrantAPIKey != "" {
			env["RAG_QDRANT_API_KEY"] = cfg.QdrantAPIKey
		}
	case "chroma":
		env["RAG_CHROMA_URL"] = cfg.ChromaURL
	case "pgvector":
		if cfg.PGVectorDSN != "" {
			env["RAG_PGVECTOR_DSN"] = cfg.PGVectorDSN
		}
	}
	switch cfg.Embedder {
	case "voyage":
		if cfg.Voyage.APIKey != "" {
			env["RAG_VOYAGE_API_KEY"] = cfg.Voyage.APIKey
		}
		if cfg.Voyage.Model != "" {
			env["RAG_VOYAGE_MODEL"] = cfg.Voyage.Model
		}
	case "tei":
		env["RAG_TEI_URL"] = cfg.TEIURL
	}

	return mcpEntry{Command: exe, Env: env}, nil
}

// SetupInstallMCP handles POST /api/setup/install-mcp.
// It writes (merging, with a .bak backup) the enowx-rag MCP server into the
// selected client's config file. Body: {client_id, scope, project_dir?}.
func (h *Handlers) SetupInstallMCP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientID   string `json:"client_id"`
		Scope      string `json:"scope"`       // "global" (default) or "project"
		ProjectDir string `json:"project_dir"` // required for scope=project
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	client, ok := mcpClientByID(req.ClientID)
	if !ok {
		writeErr(w, http.StatusBadRequest, "unknown client_id")
		return
	}
	scope := req.Scope
	if scope == "" {
		scope = "global"
	}
	path, err := client.resolvePath(scope, req.ProjectDir)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	entry, err := mcpServerEntry()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	backedUp, err := client.install(path, entry)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Sprintf("install failed: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "installed",
		"client":    client.Label,
		"path":      path,
		"backed_up": backedUp,
	})
}

// SetupMCPSnippet handles GET /api/setup/mcp-snippet?client_id=...
// It returns the config file path and a ready-to-paste snippet, without writing
// anything. Used for the manual tab and unknown ("Other") clients.
func (h *Handlers) SetupMCPSnippet(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("client_id")
	client, ok := mcpClientByID(id)
	if !ok {
		writeErr(w, http.StatusBadRequest, "unknown client_id")
		return
	}
	entry, err := mcpServerEntry()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	path, content := client.snippet(entry)
	writeJSON(w, http.StatusOK, map[string]any{
		"client":  client.Label,
		"path":    path,
		"format":  string(client.Format),
		"content": content,
	})
}

// SetupClients handles GET /api/setup/clients.
// Returns the list of supported clients (id, label, scopes) for the wizard UI.
func (h *Handlers) SetupClients(w http.ResponseWriter, r *http.Request) {
	type clientInfo struct {
		ID           string `json:"id"`
		Label        string `json:"label"`
		Format       string `json:"format"`
		HasProject   bool   `json:"has_project"`
		GlobalPath   string `json:"global_path"`
	}
	out := make([]clientInfo, 0, len(mcpClients))
	for _, c := range mcpClients {
		out = append(out, clientInfo{
			ID:         c.ID,
			Label:      c.Label,
			Format:     string(c.Format),
			HasProject: c.ProjectPath != "",
			GlobalPath: c.GlobalPath,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// SetupSkillGuide handles GET /api/setup/skill-guide.
// Returns manual install instructions for the skill (command + target paths).
// The skill is not auto-installed because only some clients support skills.
func (h *Handlers) SetupSkillGuide(w http.ResponseWriter, r *http.Request) {
	type target struct {
		Client string `json:"client"`
		Dir    string `json:"dir"`
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"note":        "Skills are supported by some clients only (e.g. Claude Code, Factory). Install manually into your client's skills directory.",
		"source_file": "skill/enowx-rag.md (in the enowx-rag repo)",
		"targets": []target{
			{Client: "Claude Code", Dir: "~/.claude/skills/enowx-rag/"},
			{Client: "Factory", Dir: "~/.factory/skills/enowx-rag/"},
		},
		"commands": []string{
			"mkdir -p ~/.claude/skills/enowx-rag",
			"cp /path/to/enowx-rag/skill/enowx-rag.md ~/.claude/skills/enowx-rag/skill.md",
		},
	})
}
