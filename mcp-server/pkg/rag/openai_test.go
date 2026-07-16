package rag

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeOpenAIURL(t *testing.T) {
	cases := map[string]string{
		"":                                  "https://api.openai.com/v1/embeddings",
		"https://api.openai.com/v1":         "https://api.openai.com/v1/embeddings",
		"https://api.openai.com/v1/":        "https://api.openai.com/v1/embeddings",
		"http://localhost:11434/v1":         "http://localhost:11434/v1/embeddings",
		"https://x.ai/v1/embeddings":        "https://x.ai/v1/embeddings",
	}
	for in, want := range cases {
		if got := normalizeOpenAIURL(in); got != want {
			t.Errorf("normalizeOpenAIURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestOpenAIEmbed verifies request shape (input, model, dimensions) and decoding.
func TestOpenAIEmbed(t *testing.T) {
	var body map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
		w.Write([]byte(`{"data":[{"index":0,"embedding":[0.1,0.2,0.3]},{"index":1,"embedding":[0.4,0.5,0.6]}],"usage":{"total_tokens":7}}`))
	}))
	defer srv.Close()

	c := NewOpenAIEmbeddingClient("sk-test", "text-embedding-3-small", srv.URL+"/v1", 3)
	vecs, err := c.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != 2 || len(vecs[0]) != 3 {
		t.Fatalf("unexpected vecs shape: %v", vecs)
	}
	if auth != "Bearer sk-test" {
		t.Errorf("auth = %q, want Bearer sk-test", auth)
	}
	if body["model"] != "text-embedding-3-small" {
		t.Errorf("model = %v", body["model"])
	}
	if d, _ := body["dimensions"].(float64); int(d) != 3 {
		t.Errorf("dimensions = %v, want 3", body["dimensions"])
	}
	if c.TokensUsed() != 7 {
		t.Errorf("TokensUsed = %d, want 7", c.TokensUsed())
	}
	if c.ModelName() != "text-embedding-3-small" {
		t.Errorf("ModelName = %q", c.ModelName())
	}
}

// TestOpenAIDimensionsOmittedWhenZero verifies dimensions is not sent when Dim=0.
func TestOpenAIDimensionsOmittedWhenZero(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
		w.Write([]byte(`{"data":[{"index":0,"embedding":[0.1,0.2]}],"usage":{"total_tokens":1}}`))
	}))
	defer srv.Close()

	c := NewOpenAIEmbeddingClient("k", "nomic-embed-text", srv.URL+"/v1", 0)
	if _, err := c.Embed(context.Background(), []string{"x"}); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if _, present := body["dimensions"]; present {
		t.Error("dimensions should be omitted when Dim=0")
	}
}

// TestOpenAIVectorSizeProbe verifies VectorSize probes when Dim is 0.
func TestOpenAIVectorSizeProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"index":0,"embedding":[0,0,0,0,0]}],"usage":{"total_tokens":1}}`))
	}))
	defer srv.Close()

	c := NewOpenAIEmbeddingClient("k", "m", srv.URL+"/v1", 0)
	if got := c.VectorSize(); got != 5 {
		t.Errorf("VectorSize probe = %d, want 5", got)
	}
}

// TestOpenAINoAuthHeaderWhenKeyEmpty verifies local providers without a key work.
func TestOpenAINoAuthHeaderWhenKeyEmpty(t *testing.T) {
	var hadAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadAuth = r.Header["Authorization"]
		w.Write([]byte(`{"data":[{"index":0,"embedding":[1]}],"usage":{}}`))
	}))
	defer srv.Close()

	c := NewOpenAIEmbeddingClient("", "m", srv.URL+"/v1", 1)
	c.Embed(context.Background(), []string{"x"})
	if hadAuth {
		t.Error("Authorization header should be absent when API key is empty")
	}
}
