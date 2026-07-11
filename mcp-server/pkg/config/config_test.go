package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// helperSetEnv sets env vars and returns a cleanup function.
func helperSetEnv(t *testing.T, envs map[string]string) func() {
	t.Helper()
	saved := make(map[string]string)
	for k, v := range envs {
		saved[k] = os.Getenv(k)
		os.Setenv(k, v)
	}
	return func() {
		for k, v := range saved {
			os.Unsetenv(k)
			if v != "" {
				os.Setenv(k, v)
			}
		}
	}
}

// helperClearEnv unsets all RAG_ env vars and returns a cleanup function.
func helperClearEnv(t *testing.T) func() {
	t.Helper()
	keys := []string{
		"RAG_VECTOR_STORE", "RAG_EMBEDDER",
		"RAG_QDRANT_URL", "RAG_QDRANT_API_KEY",
		"RAG_CHROMA_URL",
		"RAG_PGVECTOR_DSN",
		"RAG_TEI_URL",
		"RAG_VOYAGE_API_KEY", "RAG_VOYAGE_MODEL", "RAG_VECTOR_DIM",
		"RAG_RERANKER_MODEL",
	}
	saved := make(map[string]string)
	for _, k := range keys {
		saved[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	return func() {
		for k, v := range saved {
			if v != "" {
				os.Setenv(k, v)
			}
		}
	}
}

// --- Path ---

func TestPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir: %v", err)
	}
	got := Path()
	want := filepath.Join(home, ".enowx-rag", "config.yaml")
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

// --- Load: nonexistent file returns error ---

func TestLoad_NonexistentFile(t *testing.T) {
	cleanup := helperClearEnv(t)
	defer cleanup()

	// Point HOME to a temp dir with no config file.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should return error when config file does not exist")
	}
}

// --- Load: valid YAML populates fields ---

func TestLoad_ValidYAML(t *testing.T) {
	cleanup := helperClearEnv(t)
	defer cleanup()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Write a valid config.yaml.
	configDir := filepath.Join(tmpDir, ".enowx-rag")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	yamlContent := `vector_store: pgvector
embedder: voyage
voyage:
  api_key: test-key-123
  model: voyage-4
  dim: 1024
pgvector_dsn: "postgresql://enowdev@localhost:5432/enowxrag"
qdrant_url: "http://localhost:6333"
qdrant_api_key: ""
chroma_url: "http://localhost:8000"
tei_url: "http://localhost:8081"
reranker_model: "rerank-2.5"
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.VectorStore != "pgvector" {
		t.Errorf("VectorStore = %q, want %q", cfg.VectorStore, "pgvector")
	}
	if cfg.Embedder != "voyage" {
		t.Errorf("Embedder = %q, want %q", cfg.Embedder, "voyage")
	}
	if cfg.Voyage.APIKey != "test-key-123" {
		t.Errorf("Voyage.APIKey = %q, want %q", cfg.Voyage.APIKey, "test-key-123")
	}
	if cfg.Voyage.Model != "voyage-4" {
		t.Errorf("Voyage.Model = %q, want %q", cfg.Voyage.Model, "voyage-4")
	}
	if cfg.Voyage.Dim != 1024 {
		t.Errorf("Voyage.Dim = %d, want %d", cfg.Voyage.Dim, 1024)
	}
	if cfg.PGVectorDSN != "postgresql://enowdev@localhost:5432/enowxrag" {
		t.Errorf("PGVectorDSN = %q, want %q", cfg.PGVectorDSN, "postgresql://enowdev@localhost:5432/enowxrag")
	}
	if cfg.QdrantURL != "http://localhost:6333" {
		t.Errorf("QdrantURL = %q, want %q", cfg.QdrantURL, "http://localhost:6333")
	}
	if cfg.RerankerModel != "rerank-2.5" {
		t.Errorf("RerankerModel = %q, want %q", cfg.RerankerModel, "rerank-2.5")
	}
}

// --- Load: invalid YAML returns error ---

func TestLoad_InvalidYAML(t *testing.T) {
	cleanup := helperClearEnv(t)
	defer cleanup()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	configDir := filepath.Join(tmpDir, ".enowx-rag")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Invalid YAML: broken mapping
	yamlContent := `vector_store: pgvector
  bad_indent: value
embedder: voyage
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should return error for invalid YAML")
	}
}

// --- Save: writes file with chmod 0600, creates directory ---

func TestSave_CreatesDirectoryAndFile(t *testing.T) {
	cleanup := helperClearEnv(t)
	defer cleanup()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := &Config{
		VectorStore: "pgvector",
		Embedder:    "voyage",
		Voyage: VoyageConfig{
			APIKey: "secret-key",
			Model:  "voyage-4",
			Dim:    1024,
		},
		PGVectorDSN: "postgresql://enowdev@localhost:5432/enowxrag",
		QdrantURL:   "http://localhost:6333",
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Verify file exists.
	configPath := Path()
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	// Verify permissions are 0600.
	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Errorf("file mode = %o, want 0600", mode)
	}

	// Verify directory was created.
	dir := filepath.Dir(configPath)
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("config directory not created: %v", err)
	}
}

// --- Save: writes valid YAML content ---

func TestSave_WritesValidYAML(t *testing.T) {
	cleanup := helperClearEnv(t)
	defer cleanup()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := &Config{
		VectorStore: "qdrant",
		Embedder:    "voyage",
		Voyage: VoyageConfig{
			APIKey: "key-abc",
			Model:  "voyage-4",
			Dim:    1024,
		},
		QdrantURL: "http://localhost:6333",
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Read back the file and verify content is valid YAML by loading it.
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() after Save() error: %v", err)
	}
	if loaded.VectorStore != "qdrant" {
		t.Errorf("loaded VectorStore = %q, want %q", loaded.VectorStore, "qdrant")
	}
	if loaded.Voyage.APIKey != "key-abc" {
		t.Errorf("loaded Voyage.APIKey = %q, want %q", loaded.Voyage.APIKey, "key-abc")
	}
}

// --- Round-trip: Save then Load returns identical config ---

func TestRoundTrip_SaveThenLoad(t *testing.T) {
	cleanup := helperClearEnv(t)
	defer cleanup()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	original := &Config{
		VectorStore:   "pgvector",
		Embedder:      "voyage",
		Voyage:        VoyageConfig{APIKey: "rt-key", Model: "voyage-4", Dim: 1024},
		PGVectorDSN:   "postgresql://enowdev@localhost:5432/enowxrag",
		QdrantURL:     "http://localhost:6333",
		QdrantAPIKey:  "qdrant-secret",
		ChromaURL:     "http://localhost:8000",
		TEIURL:        "http://localhost:8081",
		RerankerModel: "rerank-2.5",
	}

	if err := Save(original); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if !reflect.DeepEqual(original, loaded) {
		t.Errorf("round-trip mismatch:\n  original = %+v\n  loaded   = %+v", original, loaded)
	}
}

// --- Env var override priority: env > file > default ---

func TestEnvVarOverridePriority(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Write a config file with specific values.
	configDir := filepath.Join(tmpDir, ".enowx-rag")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	yamlContent := `vector_store: pgvector
embedder: voyage
voyage:
  api_key: file-key
  model: voyage-4
  dim: 1024
pgvector_dsn: "postgresql://enowdev@localhost:5432/enowxrag"
qdrant_url: "http://localhost:6333"
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("env_var_wins_over_file", func(t *testing.T) {
		cleanup := helperSetEnv(t, map[string]string{
			"RAG_VECTOR_STORE":   "qdrant",
			"RAG_VOYAGE_API_KEY": "env-key",
		})
		defer cleanup()

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}

		// Env var should override file value.
		if cfg.VectorStore != "qdrant" {
			t.Errorf("VectorStore = %q, want %q (env override)", cfg.VectorStore, "qdrant")
		}
		if cfg.Voyage.APIKey != "env-key" {
			t.Errorf("Voyage.APIKey = %q, want %q (env override)", cfg.Voyage.APIKey, "env-key")
		}
		// File-only value should still be present.
		if cfg.Voyage.Model != "voyage-4" {
			t.Errorf("Voyage.Model = %q, want %q (from file)", cfg.Voyage.Model, "voyage-4")
		}
	})

	t.Run("file_value_used_when_env_not_set", func(t *testing.T) {
		cleanup := helperClearEnv(t)
		defer cleanup()

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}

		// File values should be used.
		if cfg.VectorStore != "pgvector" {
			t.Errorf("VectorStore = %q, want %q (from file)", cfg.VectorStore, "pgvector")
		}
		if cfg.Voyage.APIKey != "file-key" {
			t.Errorf("Voyage.APIKey = %q, want %q (from file)", cfg.Voyage.APIKey, "file-key")
		}
	})
}

// --- Defaults: used when neither env nor file ---

func TestDefaults_WhenNeitherEnvNorFile(t *testing.T) {
	cleanup := helperClearEnv(t)
	defer cleanup()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// No config file exists, no env vars set.
	// Load() returns error, but Default() should return default config.
	cfg := Default()

	if cfg.VectorStore != "qdrant" {
		t.Errorf("default VectorStore = %q, want %q", cfg.VectorStore, "qdrant")
	}
	if cfg.Embedder != "voyage" {
		t.Errorf("default Embedder = %q, want %q", cfg.Embedder, "voyage")
	}
	if cfg.Voyage.Model != "voyage-4" {
		t.Errorf("default Voyage.Model = %q, want %q", cfg.Voyage.Model, "voyage-4")
	}
	if cfg.QdrantURL != "http://localhost:6333" {
		t.Errorf("default QdrantURL = %q, want %q", cfg.QdrantURL, "http://localhost:6333")
	}
}

// --- Resolve: full priority resolution (env > file > default) ---

func TestResolve_FullPriority(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Write config file.
	configDir := filepath.Join(tmpDir, ".enowx-rag")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	yamlContent := `vector_store: pgvector
embedder: voyage
voyage:
  api_key: file-key
  model: voyage-4
  dim: 1024
pgvector_dsn: "postgresql://file-dsn"
qdrant_url: "http://file-qdrant:6333"
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("env_overrides_file", func(t *testing.T) {
		cleanup := helperSetEnv(t, map[string]string{
			"RAG_VECTOR_STORE":   "chroma",
			"RAG_PGVECTOR_DSN":   "postgresql://env-dsn",
			"RAG_VOYAGE_API_KEY": "env-key",
		})
		defer cleanup()

		cfg, err := Resolve()
		if err != nil {
			t.Fatalf("Resolve() error: %v", err)
		}

		if cfg.VectorStore != "chroma" {
			t.Errorf("VectorStore = %q, want %q (env)", cfg.VectorStore, "chroma")
		}
		if cfg.PGVectorDSN != "postgresql://env-dsn" {
			t.Errorf("PGVectorDSN = %q, want %q (env)", cfg.PGVectorDSN, "postgresql://env-dsn")
		}
		if cfg.Voyage.APIKey != "env-key" {
			t.Errorf("Voyage.APIKey = %q, want %q (env)", cfg.Voyage.APIKey, "env-key")
		}
		// File-only values preserved.
		if cfg.QdrantURL != "http://file-qdrant:6333" {
			t.Errorf("QdrantURL = %q, want %q (file)", cfg.QdrantURL, "http://file-qdrant:6333")
		}
	})

	t.Run("file_used_when_no_env", func(t *testing.T) {
		cleanup := helperClearEnv(t)
		defer cleanup()

		cfg, err := Resolve()
		if err != nil {
			t.Fatalf("Resolve() error: %v", err)
		}

		if cfg.VectorStore != "pgvector" {
			t.Errorf("VectorStore = %q, want %q (file)", cfg.VectorStore, "pgvector")
		}
		if cfg.PGVectorDSN != "postgresql://file-dsn" {
			t.Errorf("PGVectorDSN = %q, want %q (file)", cfg.PGVectorDSN, "postgresql://file-dsn")
		}
		if cfg.Voyage.APIKey != "file-key" {
			t.Errorf("Voyage.APIKey = %q, want %q (file)", cfg.Voyage.APIKey, "file-key")
		}
	})

	t.Run("defaults_when_no_file_no_env", func(t *testing.T) {
		cleanup := helperClearEnv(t)
		defer cleanup()

		// Remove config file.
		os.Remove(filepath.Join(tmpDir, ".enowx-rag", "config.yaml"))

		cfg, err := Resolve()
		if err != nil {
			t.Fatalf("Resolve() error: %v", err)
		}

		if cfg.VectorStore != "qdrant" {
			t.Errorf("VectorStore = %q, want %q (default)", cfg.VectorStore, "qdrant")
		}
		if cfg.Embedder != "voyage" {
			t.Errorf("Embedder = %q, want %q (default)", cfg.Embedder, "voyage")
		}
		if cfg.QdrantURL != "http://localhost:6333" {
			t.Errorf("QdrantURL = %q, want %q (default)", cfg.QdrantURL, "http://localhost:6333")
		}
	})
}

// --- Resolve: VectorDim env var override ---

func TestResolve_VectorDimEnvOverride(t *testing.T) {
	cleanup := helperClearEnv(t)
	defer cleanup()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Set RAG_VECTOR_DIM env var.
	dimCleanup := helperSetEnv(t, map[string]string{
		"RAG_VECTOR_DIM": "512",
	})
	defer dimCleanup()

	cfg, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	if cfg.Voyage.Dim != 512 {
		t.Errorf("Voyage.Dim = %d, want 512 (env override)", cfg.Voyage.Dim)
	}
}

// --- Save: overwrites existing file ---

func TestSave_OverwritesExisting(t *testing.T) {
	cleanup := helperClearEnv(t)
	defer cleanup()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Save first config.
	cfg1 := &Config{
		VectorStore: "qdrant",
		Embedder:    "voyage",
		Voyage:      VoyageConfig{APIKey: "first-key", Model: "voyage-4", Dim: 1024},
	}
	if err := Save(cfg1); err != nil {
		t.Fatalf("Save() first: %v", err)
	}

	// Save second config with different values.
	cfg2 := &Config{
		VectorStore: "pgvector",
		Embedder:    "voyage",
		Voyage:      VoyageConfig{APIKey: "second-key", Model: "voyage-4", Dim: 1024},
		PGVectorDSN: "postgresql://enowdev@localhost:5432/enowxrag",
	}
	if err := Save(cfg2); err != nil {
		t.Fatalf("Save() second: %v", err)
	}

	// Load and verify second config.
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if loaded.VectorStore != "pgvector" {
		t.Errorf("VectorStore = %q, want %q", loaded.VectorStore, "pgvector")
	}
	if loaded.Voyage.APIKey != "second-key" {
		t.Errorf("Voyage.APIKey = %q, want %q", loaded.Voyage.APIKey, "second-key")
	}
}
