package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/enowdev/enowx-rag/pkg/config"
)

// maskSecret returns a masked view of a secret: first 4 + last 3 chars, middle
// as dots. Empty stays empty; short secrets are fully dotted.
func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "••••"
	}
	return s[:4] + "••••" + s[len(s)-3:]
}

// currentConfig loads the saved config, tolerating a missing file.
func currentConfig() *config.Config {
	cfg, err := config.Load()
	if err != nil {
		return config.Default()
	}
	return cfg
}

// SetupConfig handles GET /api/setup/config — the current configuration with
// secrets masked (never returns full keys). Safe for display.
func (h *Handlers) SetupConfig(w http.ResponseWriter, r *http.Request) {
	cfg := currentConfig()
	writeJSON(w, http.StatusOK, map[string]any{
		"vector_store":     cfg.VectorStore,
		"embedder":         cfg.Embedder,
		"qdrant_url":       cfg.QdrantURL,
		"qdrant_api_key":   maskSecret(cfg.QdrantAPIKey),
		"chroma_url":       cfg.ChromaURL,
		"pgvector_dsn":     cfg.PGVectorDSN,
		"tei_url":          cfg.TEIURL,
		"voyage_model":     cfg.Voyage.Model,
		"voyage_dim":       cfg.Voyage.Dim,
		"voyage_api_key":   maskSecret(cfg.Voyage.APIKey),
		"openai_base_url":  cfg.OpenAI.BaseURL,
		"openai_model":     cfg.OpenAI.Model,
		"openai_api_key":   maskSecret(cfg.OpenAI.APIKey),
		"reranker_model":   cfg.RerankerModel,
		"admin_token_set":  config.EffectiveAdminToken() != "",
		"admin_token":      maskSecret(cfg.AdminToken),
	})
}

// SetupConfigReveal handles GET /api/setup/config/reveal — the FULL secrets.
// Gated by LocalOrAdminMiddleware (localhost or a valid admin token) so it is
// not open to anonymous callers on an exposed daemon.
func (h *Handlers) SetupConfigReveal(w http.ResponseWriter, r *http.Request) {
	cfg := currentConfig()
	writeJSON(w, http.StatusOK, map[string]any{
		"voyage_api_key": cfg.Voyage.APIKey,
		"qdrant_api_key": cfg.QdrantAPIKey,
		"openai_api_key": cfg.OpenAI.APIKey,
		"pgvector_dsn":   cfg.PGVectorDSN,
		"admin_token":    cfg.AdminToken,
	})
}

// SetupConfigUpdate handles POST /api/setup/config — partial update of secrets/
// settings, merged into the saved config. Only non-empty fields overwrite; a
// field set to the sentinel "" is left unchanged (send the new value to change).
// Gated (writes config.yaml with secrets).
func (h *Handlers) SetupConfigUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		VoyageAPIKey *string `json:"voyage_api_key"`
		VoyageModel  *string `json:"voyage_model"`
		QdrantAPIKey *string `json:"qdrant_api_key"`
		OpenAIAPIKey *string `json:"openai_api_key"`
		OpenAIModel  *string `json:"openai_model"`
		OpenAIBaseURL *string `json:"openai_base_url"`
		AdminToken   *string `json:"admin_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	cfg := currentConfig()
	if req.VoyageAPIKey != nil {
		cfg.Voyage.APIKey = *req.VoyageAPIKey
	}
	if req.VoyageModel != nil {
		cfg.Voyage.Model = *req.VoyageModel
	}
	if req.QdrantAPIKey != nil {
		cfg.QdrantAPIKey = *req.QdrantAPIKey
	}
	if req.OpenAIAPIKey != nil {
		cfg.OpenAI.APIKey = *req.OpenAIAPIKey
	}
	if req.OpenAIModel != nil {
		cfg.OpenAI.Model = *req.OpenAIModel
	}
	if req.OpenAIBaseURL != nil {
		cfg.OpenAI.BaseURL = *req.OpenAIBaseURL
	}
	if req.AdminToken != nil {
		cfg.AdminToken = *req.AdminToken
	}
	if err := config.Save(cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, "save config: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "saved"})
}

// SetupGenToken handles POST /api/setup/gen-token — generate a strong random
// admin token, save it to config.yaml, and return it ONCE (so the caller can
// copy it). Gated. The env var still takes precedence at runtime if set.
func (h *Handlers) SetupGenToken(w http.ResponseWriter, r *http.Request) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		writeErr(w, http.StatusInternalServerError, "generate token: "+err.Error())
		return
	}
	token := hex.EncodeToString(buf)
	cfg := currentConfig()
	cfg.AdminToken = token
	if err := config.Save(cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, "save config: "+err.Error())
		return
	}
	envOverride := config.EffectiveAdminToken() != token // true if env var shadows it
	writeJSON(w, http.StatusOK, map[string]any{
		"token":            token,
		"saved":            true,
		"env_override":     envOverride,
		"note":             "Copy this now — it is stored in config.yaml (0600). If RAG_ADMIN_TOKEN is set in the environment, that value takes precedence at runtime.",
	})
}
