// Package config handles reading, writing, and resolving RAG configuration
// from three sources with a strict priority: environment variables > config
// file (~/.enowx-rag/config.yaml) > built-in defaults.
//
// The config file uses YAML and is written with chmod 0600 because it may
// contain API keys.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

// VoyageConfig holds Voyage AI embedding settings.
type VoyageConfig struct {
	APIKey string `yaml:"api_key"`
	Model  string `yaml:"model"`
	Dim    int    `yaml:"dim"`
}

// Config holds all configurable settings for the RAG server. Fields use
// yaml struct tags so the struct can be marshalled/unmarshalled directly.
type Config struct {
	VectorStore   string       `yaml:"vector_store"`
	Embedder      string       `yaml:"embedder"`
	Voyage        VoyageConfig `yaml:"voyage"`
	PGVectorDSN   string       `yaml:"pgvector_dsn"`
	QdrantURL     string       `yaml:"qdrant_url"`
	QdrantAPIKey  string       `yaml:"qdrant_api_key"`
	ChromaURL     string       `yaml:"chroma_url"`
	TEIURL        string       `yaml:"tei_url"`
	RerankerModel string       `yaml:"reranker_model"`
}

// Default returns a Config populated with built-in default values. These are
// the lowest-priority values, used when neither an env var nor a config file
// provides a setting.
func Default() *Config {
	return &Config{
		VectorStore: "qdrant",
		Embedder:    "voyage",
		Voyage: VoyageConfig{
			Model: "voyage-4",
			Dim:   1024,
		},
		QdrantURL: "http://localhost:6333",
		ChromaURL: "http://localhost:8000",
		TEIURL:    "http://localhost:8081",
	}
}

// Path returns the absolute path to the config file: ~/.enowx-rag/config.yaml.
func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fall back to a relative path; this should not happen in practice.
		return filepath.Join(".enowx-rag", "config.yaml")
	}
	return filepath.Join(home, ".enowx-rag", "config.yaml")
}

// Load reads the config file from Path() and returns a populated *Config.
// If the file does not exist, Load returns an error (this triggers the
// onboarding wizard in HTTP mode). Env var overrides are applied on top of
// the file values.
func Load() (*Config, error) {
	data, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found at %s: %w", Path(), err)
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config yaml: %w", err)
	}

	// Apply env var overrides on top of file values.
	applyEnvOverrides(&cfg)

	return &cfg, nil
}

// Save writes the config to Path() as YAML with file permissions 0600.
// It creates the parent directory if it does not exist.
func Save(c *Config) error {
	dir := filepath.Dir(Path())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config yaml: %w", err)
	}

	if err := os.WriteFile(Path(), data, 0600); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	// os.WriteFile only applies the mode when creating a new file. When
	// overwriting an existing file the permissions are NOT changed, so we
	// call os.Chmod to guarantee 0600 on every save.
	if err := os.Chmod(Path(), 0600); err != nil {
		return fmt.Errorf("chmod config file: %w", err)
	}

	return nil
}

// Resolve implements the full config priority: env var > config file > default.
// It tries to load the config file; if the file doesn't exist, it starts from
// defaults. Then env var overrides are applied on top.
func Resolve() (*Config, error) {
	cfg, err := Load()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No config file: start from defaults, then apply env overrides.
			cfg = Default()
			applyEnvOverrides(cfg)
		} else {
			return nil, err
		}
	}

	return cfg, nil
}

// applyEnvOverrides mutates the Config in place, setting fields from
// environment variables when those variables are set and non-empty.
// Environment variables always take precedence over file/default values.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("RAG_VECTOR_STORE"); v != "" {
		cfg.VectorStore = v
	}
	if v := os.Getenv("RAG_EMBEDDER"); v != "" {
		cfg.Embedder = v
	}
	if v := os.Getenv("RAG_QDRANT_URL"); v != "" {
		cfg.QdrantURL = v
	}
	if v := os.Getenv("RAG_QDRANT_API_KEY"); v != "" {
		cfg.QdrantAPIKey = v
	}
	if v := os.Getenv("RAG_CHROMA_URL"); v != "" {
		cfg.ChromaURL = v
	}
	if v := os.Getenv("RAG_PGVECTOR_DSN"); v != "" {
		cfg.PGVectorDSN = v
	}
	if v := os.Getenv("RAG_TEI_URL"); v != "" {
		cfg.TEIURL = v
	}
	if v := os.Getenv("RAG_VOYAGE_API_KEY"); v != "" {
		cfg.Voyage.APIKey = v
	}
	if v := os.Getenv("RAG_VOYAGE_MODEL"); v != "" {
		cfg.Voyage.Model = v
	}
	if v := os.Getenv("RAG_RERANKER_MODEL"); v != "" {
		cfg.RerankerModel = v
	}
	if v := os.Getenv("RAG_VECTOR_DIM"); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d > 0 {
			cfg.Voyage.Dim = d
		}
	}
}
