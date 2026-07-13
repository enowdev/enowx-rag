package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func testEntry() mcpEntry {
	return mcpEntry{
		Command: "/usr/local/bin/enowx-rag",
		Env: map[string]string{
			"RAG_VECTOR_STORE":   "qdrant",
			"RAG_QDRANT_URL":     "http://localhost:6333",
			"RAG_VOYAGE_API_KEY": "secret",
		},
	}
}

// TestMergeJSONPreservesOtherServers verifies installing into an existing
// mcpServers config keeps other servers intact.
func TestMergeJSONPreservesOtherServers(t *testing.T) {
	existing := []byte(`{"mcpServers":{"other":{"command":"/bin/other"}},"someTopLevel":true}`)
	out, err := mergeJSONMap(existing, "mcpServers", jsonEntryMap(testEntry(), nil, false))
	if err != nil {
		t.Fatalf("mergeJSONMap: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("result not valid JSON: %v", err)
	}
	servers := root["mcpServers"].(map[string]any)
	if _, ok := servers["other"]; !ok {
		t.Error("existing 'other' server was dropped")
	}
	if _, ok := servers["enowx-rag"]; !ok {
		t.Error("enowx-rag server was not added")
	}
	if root["someTopLevel"] != true {
		t.Error("top-level key was dropped")
	}
}

// TestMergeJSONInvalidExisting returns a clear error, not a silent overwrite.
func TestMergeJSONInvalidExisting(t *testing.T) {
	_, err := mergeJSONMap([]byte("not json"), "mcpServers", jsonEntryMap(testEntry(), nil, false))
	if err == nil {
		t.Fatal("expected error for invalid existing JSON")
	}
}

// TestZedContextServers verifies Zed uses context_servers and args:[].
func TestZedContextServers(t *testing.T) {
	c, _ := mcpClientByID("zed")
	out, err := mergeJSONMap(nil, "context_servers", jsonEntryMap(testEntry(), c.extraEntry, true))
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	var root map[string]any
	json.Unmarshal(out, &root)
	cs, ok := root["context_servers"].(map[string]any)
	if !ok {
		t.Fatal("expected context_servers key")
	}
	entry := cs["enowx-rag"].(map[string]any)
	if _, ok := entry["args"]; !ok {
		t.Error("Zed entry must include args")
	}
}

// TestTOMLMerge verifies Codex TOML: writes section, and replaces on re-install
// without duplicating or dropping other sections.
func TestTOMLMerge(t *testing.T) {
	out, err := mergeTOML(nil, testEntry())
	if err != nil {
		t.Fatalf("mergeTOML fresh: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "[mcp_servers.enowx-rag]") || !strings.Contains(s, "[mcp_servers.enowx-rag.env]") {
		t.Errorf("TOML missing expected sections:\n%s", s)
	}

	// Existing file with another server + our old section.
	existing := []byte("[mcp_servers.other]\ncommand = \"/bin/other\"\n\n[mcp_servers.enowx-rag]\ncommand = \"/old\"\n")
	out2, err := mergeTOML(existing, testEntry())
	if err != nil {
		t.Fatalf("mergeTOML existing: %v", err)
	}
	s2 := string(out2)
	if !strings.Contains(s2, "[mcp_servers.other]") {
		t.Error("other TOML server was dropped")
	}
	if strings.Count(s2, "[mcp_servers.enowx-rag]") != 1 {
		t.Errorf("enowx-rag section should appear exactly once:\n%s", s2)
	}
	if strings.Contains(s2, "/old") {
		t.Error("old command should have been replaced")
	}
}

// TestYAMLListMerge verifies Continue's list format and replace-by-name.
func TestYAMLListMerge(t *testing.T) {
	existing := []byte("mcpServers:\n  - name: other\n    command: /bin/other\n")
	out, err := mergeYAMLList(existing, testEntry())
	if err != nil {
		t.Fatalf("mergeYAMLList: %v", err)
	}
	var root map[string]any
	if err := yaml.Unmarshal(out, &root); err != nil {
		t.Fatalf("result not valid YAML: %v", err)
	}
	list := root["mcpServers"].([]any)
	if len(list) != 2 {
		t.Fatalf("expected 2 items (other + enowx-rag), got %d", len(list))
	}
	names := map[string]bool{}
	for _, it := range list {
		names[it.(map[string]any)["name"].(string)] = true
	}
	if !names["other"] || !names["enowx-rag"] {
		t.Errorf("expected both servers, got %v", names)
	}
}

// TestInstallCreatesBackup verifies install backs up an existing file.
func TestInstallCreatesBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	os.WriteFile(path, []byte(`{"mcpServers":{"other":{"command":"/x"}}}`), 0o600)

	c, _ := mcpClientByID("cursor")
	backedUp, err := c.install(path, testEntry())
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !backedUp {
		t.Error("expected backup to be created")
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf(".bak file not found: %v", err)
	}
	// Cursor must include type:stdio.
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"type": "stdio"`) {
		t.Error("Cursor entry should include type:stdio")
	}
}

// TestInstallNoBackupWhenNew verifies no backup for a fresh file.
func TestInstallNoBackupWhenNew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "mcp.json")
	c, _ := mcpClientByID("claude-code")
	backedUp, err := c.install(path, testEntry())
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if backedUp {
		t.Error("should not back up a nonexistent file")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("config not written: %v", err)
	}
}
