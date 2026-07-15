package httpapi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// mcpServerName is the server key/name used across all client configs.
const mcpServerName = "enowx-rag"

// mcpFormat is the on-disk config format of a client.
type mcpFormat string

const (
	fmtMcpServers     mcpFormat = "json-mcpservers"   // { "mcpServers": { name: entry } }
	fmtContextServers mcpFormat = "json-contextservers" // Zed: { "context_servers": { name: entry } }
	fmtTOML           mcpFormat = "toml"              // Codex: [mcp_servers.name] + [.env]
	fmtYAMLList       mcpFormat = "yaml-list"         // Continue: mcpServers: [ {name, ...} ]
)

// mcpClient describes one supported MCP client target.
type mcpClient struct {
	ID          string
	Label       string
	Format      mcpFormat
	GlobalPath  string // ~-relative path to the global config file
	ProjectPath string // project-relative path (empty if none)
	// extra fields merged into the JSON entry for mcpServers-format clients.
	extraEntry map[string]any
}

// mcpClients is the registry of supported clients. Order drives UI display.
var mcpClients = []mcpClient{
	{ID: "claude-code", Label: "Claude Code", Format: fmtMcpServers, GlobalPath: "~/.claude.json", ProjectPath: ".mcp.json"},
	{ID: "claude-desktop", Label: "Claude Desktop", Format: fmtMcpServers, GlobalPath: "~/Library/Application Support/Claude/claude_desktop_config.json"},
	{ID: "cursor", Label: "Cursor", Format: fmtMcpServers, GlobalPath: "~/.cursor/mcp.json", ProjectPath: ".cursor/mcp.json", extraEntry: map[string]any{"type": "stdio"}},
	{ID: "cline", Label: "Cline", Format: fmtMcpServers, GlobalPath: "~/.cline/mcp.json", extraEntry: map[string]any{"disabled": false, "autoApprove": []any{}}},
	{ID: "windsurf", Label: "Windsurf", Format: fmtMcpServers, GlobalPath: "~/.codeium/windsurf/mcp_config.json"},
	{ID: "codex", Label: "Codex", Format: fmtTOML, GlobalPath: "~/.codex/config.toml"},
	{ID: "zed", Label: "Zed", Format: fmtContextServers, GlobalPath: "~/.config/zed/settings.json"},
	{ID: "continue", Label: "Continue", Format: fmtYAMLList, GlobalPath: "~/.continue/config.yaml"},
}

// mcpClientByID looks up a client by id.
func mcpClientByID(id string) (mcpClient, bool) {
	for _, c := range mcpClients {
		if c.ID == id {
			return c, true
		}
	}
	return mcpClient{}, false
}

// expandHome expands a leading ~ to the user's home directory.
func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// isInstalled reports whether the enowx-rag server is already present in this
// client's global config. It reads the config file and checks for the server
// name — a robust cross-format signal since the name is unique and appears as a
// key (JSON/TOML/context_servers) or a name field (YAML list). Returns false if
// the file doesn't exist.
func (c mcpClient) isInstalled() bool {
	data, err := os.ReadFile(expandHome(c.GlobalPath))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), mcpServerName)
}

// resolvePath returns the absolute config path for a client + scope. For
// project scope, projectDir is joined with the client's project-relative path.
func (c mcpClient) resolvePath(scope, projectDir string) (string, error) {
	if scope == "project" {
		if c.ProjectPath == "" {
			return "", fmt.Errorf("%s has no project-local config; use global scope", c.Label)
		}
		if projectDir == "" {
			return "", fmt.Errorf("project_dir is required for project scope")
		}
		return filepath.Join(projectDir, c.ProjectPath), nil
	}
	return expandHome(c.GlobalPath), nil
}

// mcpEntry is the command+env for the enowx-rag server, shared by all formats.
type mcpEntry struct {
	Command string
	Env     map[string]string
}

// orderedEnvKeys returns env keys in a stable order for deterministic output.
func (e mcpEntry) orderedEnvKeys() []string {
	keys := make([]string, 0, len(e.Env))
	for k := range e.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// --- Snippet generation (never writes; used for manual tab and "Other") ---

// snippet returns the file path and a ready-to-paste config snippet for a client.
func (c mcpClient) snippet(entry mcpEntry) (path, content string) {
	path = c.GlobalPath
	switch c.Format {
	case fmtTOML:
		content = tomlSection(entry)
	case fmtYAMLList:
		content = yamlListSnippet(entry)
	case fmtContextServers:
		content = jsonSnippet("context_servers", entry, c.extraEntry, true)
	default:
		content = jsonSnippet("mcpServers", entry, c.extraEntry, false)
	}
	return path, content
}

func jsonEntryMap(entry mcpEntry, extra map[string]any, withArgs bool) map[string]any {
	m := map[string]any{"command": entry.Command}
	if withArgs {
		m["args"] = []any{}
	}
	m["env"] = envToAny(entry.Env)
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func envToAny(env map[string]string) map[string]any {
	out := make(map[string]any, len(env))
	for k, v := range env {
		out[k] = v
	}
	return out
}

func jsonSnippet(rootKey string, entry mcpEntry, extra map[string]any, withArgs bool) string {
	root := map[string]any{rootKey: map[string]any{mcpServerName: jsonEntryMap(entry, extra, withArgs)}}
	b, _ := json.MarshalIndent(root, "", "  ")
	return string(b)
}

func tomlSection(entry mcpEntry) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "[mcp_servers.%s]\n", mcpServerName)
	fmt.Fprintf(&sb, "command = %q\n\n", entry.Command)
	fmt.Fprintf(&sb, "[mcp_servers.%s.env]\n", mcpServerName)
	for _, k := range entry.orderedEnvKeys() {
		fmt.Fprintf(&sb, "%s = %q\n", k, entry.Env[k])
	}
	return sb.String()
}

func yamlListSnippet(entry mcpEntry) string {
	var sb strings.Builder
	sb.WriteString("mcpServers:\n")
	fmt.Fprintf(&sb, "  - name: %s\n", mcpServerName)
	fmt.Fprintf(&sb, "    command: %s\n", entry.Command)
	sb.WriteString("    env:\n")
	for _, k := range entry.orderedEnvKeys() {
		fmt.Fprintf(&sb, "      %s: %s\n", k, entry.Env[k])
	}
	return sb.String()
}

// --- Install (merge-not-replace, with backup) ---

// install writes the enowx-rag server into the client's config at path,
// merging into any existing content and backing up the original to path.bak.
// Returns whether a backup was created.
func (c mcpClient) install(path string, entry mcpEntry) (backedUp bool, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create config dir: %w", err)
	}

	existing, readErr := os.ReadFile(path)
	if readErr == nil && len(existing) > 0 {
		// Back up the original before modifying someone else's config file.
		if err := os.WriteFile(path+".bak", existing, 0o600); err != nil {
			return false, fmt.Errorf("write backup: %w", err)
		}
		backedUp = true
	} else if readErr != nil && !os.IsNotExist(readErr) {
		return false, fmt.Errorf("read existing config: %w", readErr)
	}

	var out []byte
	switch c.Format {
	case fmtTOML:
		out, err = mergeTOML(existing, entry)
	case fmtYAMLList:
		out, err = mergeYAMLList(existing, entry)
	case fmtContextServers:
		out, err = mergeJSONMap(existing, "context_servers", jsonEntryMap(entry, c.extraEntry, true))
	default:
		out, err = mergeJSONMap(existing, "mcpServers", jsonEntryMap(entry, c.extraEntry, false))
	}
	if err != nil {
		return backedUp, err
	}

	if err := os.WriteFile(path, out, 0o600); err != nil {
		return backedUp, fmt.Errorf("write config: %w", err)
	}
	return backedUp, nil
}

// mergeJSONMap merges the server entry into rootKey's object without disturbing
// other servers or top-level keys.
func mergeJSONMap(existing []byte, rootKey string, entry map[string]any) ([]byte, error) {
	root := map[string]any{}
	if len(strings.TrimSpace(string(existing))) > 0 {
		if err := json.Unmarshal(existing, &root); err != nil {
			return nil, fmt.Errorf("existing config is not valid JSON: %w", err)
		}
	}
	servers, _ := root[rootKey].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers[mcpServerName] = entry
	root[rootKey] = servers
	return json.MarshalIndent(root, "", "  ")
}

// mergeYAMLList merges the entry into Continue's mcpServers list, replacing any
// existing item whose name matches.
func mergeYAMLList(existing []byte, entry mcpEntry) ([]byte, error) {
	root := map[string]any{}
	if len(strings.TrimSpace(string(existing))) > 0 {
		if err := yaml.Unmarshal(existing, &root); err != nil {
			return nil, fmt.Errorf("existing config is not valid YAML: %w", err)
		}
	}
	item := map[string]any{
		"name":    mcpServerName,
		"command": entry.Command,
		"env":     envToAny(entry.Env),
	}
	list, _ := root["mcpServers"].([]any)
	replaced := false
	for i, it := range list {
		if m, ok := it.(map[string]any); ok {
			if name, _ := m["name"].(string); name == mcpServerName {
				list[i] = item
				replaced = true
				break
			}
		}
	}
	if !replaced {
		list = append(list, item)
	}
	root["mcpServers"] = list
	return yaml.Marshal(root)
}

// mergeTOML merges the enowx-rag server section into an existing TOML file.
// It preserves all lines except the previous [mcp_servers.enowx-rag*] sections,
// which are replaced. This avoids a TOML parser dependency; the format we write
// is fixed and simple.
func mergeTOML(existing []byte, entry mcpEntry) ([]byte, error) {
	section := tomlSection(entry)
	if len(strings.TrimSpace(string(existing))) == 0 {
		return []byte(section), nil
	}

	// Strip any existing enowx-rag sections (header + body until next section).
	lines := strings.Split(string(existing), "\n")
	var kept []string
	skipping := false
	prefix := fmt.Sprintf("[mcp_servers.%s", mcpServerName)
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if strings.HasPrefix(trimmed, "[") {
			// A new section header decides whether we skip it.
			skipping = strings.HasPrefix(trimmed, prefix)
		}
		if !skipping {
			kept = append(kept, ln)
		}
	}
	body := strings.TrimRight(strings.Join(kept, "\n"), "\n")
	if body != "" {
		body += "\n\n"
	}
	return []byte(body + section), nil
}
