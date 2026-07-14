package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/enowdev/enowx-rag/pkg/rag"
)

// EXPERIMENTAL connectors below: built from vendor API docs, mock-tested only.
// Not verified against live vendor accounts. See package doc.

var httpClient = &http.Client{Timeout: 120 * time.Second}

func doJSON(ctx context.Context, method, url string, headers map[string]string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s returned %d: %s", method, url, resp.StatusCode, string(b))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func metaString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func stringMeta(m map[string]any) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

// --- Pinecone (EXPERIMENTAL) ---
// Uses the data-plane list + fetch. Text is expected in metadata[textField].

type pineconeExporter struct {
	url       string // index host, e.g. https://idx-xxxx.svc.env.pinecone.io
	apiKey    string
	index     string
	textField string
}

func (p *pineconeExporter) ExportPoints(ctx context.Context, _ string) ([]rag.Document, error) {
	base := strings.TrimRight(p.url, "/")
	headers := map[string]string{"Api-Key": p.apiKey}

	var docs []rag.Document
	paginationToken := ""
	for {
		listURL := base + "/vectors/list?limit=100"
		if paginationToken != "" {
			listURL += "&paginationToken=" + paginationToken
		}
		var listResp struct {
			Vectors []struct {
				ID string `json:"id"`
			} `json:"vectors"`
			Pagination struct {
				Next string `json:"next"`
			} `json:"pagination"`
		}
		if err := doJSON(ctx, http.MethodGet, listURL, headers, nil, &listResp); err != nil {
			return nil, fmt.Errorf("pinecone list: %w", err)
		}
		if len(listResp.Vectors) == 0 {
			break
		}
		ids := make([]string, len(listResp.Vectors))
		fetchURL := base + "/vectors/fetch?"
		for i, v := range listResp.Vectors {
			ids[i] = v.ID
			if i > 0 {
				fetchURL += "&"
			}
			fetchURL += "ids=" + v.ID
		}
		var fetchResp struct {
			Vectors map[string]struct {
				ID       string         `json:"id"`
				Metadata map[string]any `json:"metadata"`
			} `json:"vectors"`
		}
		if err := doJSON(ctx, http.MethodGet, fetchURL, headers, nil, &fetchResp); err != nil {
			return nil, fmt.Errorf("pinecone fetch: %w", err)
		}
		for _, v := range fetchResp.Vectors {
			docs = append(docs, rag.Document{
				ID:      v.ID,
				Content: metaString(v.Metadata, p.textField),
				Meta:    stringMeta(v.Metadata),
			})
		}
		if listResp.Pagination.Next == "" {
			break
		}
		paginationToken = listResp.Pagination.Next
	}
	return docs, nil
}

// --- Weaviate (EXPERIMENTAL) ---
// Uses GraphQL to fetch objects of a class, reading the text property + _additional.id.

type weaviateExporter struct {
	url       string
	apiKey    string
	class     string
	textField string
}

func (wv *weaviateExporter) ExportPoints(ctx context.Context, _ string) ([]rag.Document, error) {
	base := strings.TrimRight(wv.url, "/")
	headers := map[string]string{}
	if wv.apiKey != "" {
		headers["Authorization"] = "Bearer " + wv.apiKey
	}
	// GraphQL Get with a generous limit; a production connector would cursor via
	// `after`. This experimental version fetches up to 10k objects.
	query := fmt.Sprintf(`{ Get { %s(limit: 10000) { %s _additional { id } } } }`, wv.class, wv.textField)
	var resp struct {
		Data map[string]map[string][]map[string]any `json:"data"`
	}
	if err := doJSON(ctx, http.MethodPost, base+"/v1/graphql", headers, map[string]any{"query": query}, &resp); err != nil {
		return nil, fmt.Errorf("weaviate graphql: %w", err)
	}
	var docs []rag.Document
	for _, objs := range resp.Data["Get"] {
		for _, o := range objs {
			id := ""
			if add, ok := o["_additional"].(map[string]any); ok {
				id = metaString(add, "id")
			}
			docs = append(docs, rag.Document{
				ID:      id,
				Content: metaString(o, wv.textField),
				Meta:    stringMeta(o),
			})
		}
	}
	return docs, nil
}

// --- Chroma Cloud (EXPERIMENTAL) ---
// Same /get shape as the local Chroma provider, but against a cloud host + auth.

type chromaCloudExporter struct {
	url        string
	apiKey     string
	collection string
	textField  string
}

func (c *chromaCloudExporter) ExportPoints(ctx context.Context, _ string) ([]rag.Document, error) {
	base := strings.TrimRight(c.url, "/")
	headers := map[string]string{}
	if c.apiKey != "" {
		headers["Authorization"] = "Bearer " + c.apiKey
	}
	var resp struct {
		IDs       [][]string         `json:"ids"`
		Documents [][]string         `json:"documents"`
		Metadatas [][]map[string]any `json:"metadatas"`
	}
	body := map[string]any{"include": []string{"metadatas", "documents"}}
	if err := doJSON(ctx, http.MethodPost, base+"/api/v1/collections/"+c.collection+"/get", headers, body, &resp); err != nil {
		return nil, fmt.Errorf("chroma cloud get: %w", err)
	}
	var docs []rag.Document
	for bi, batch := range resp.IDs {
		for pi, id := range batch {
			content := ""
			if bi < len(resp.Documents) && pi < len(resp.Documents[bi]) {
				content = resp.Documents[bi][pi]
			}
			meta := map[string]string{}
			if bi < len(resp.Metadatas) && pi < len(resp.Metadatas[bi]) {
				meta = stringMeta(resp.Metadatas[bi][pi])
				// Prefer the configured text field from metadata if documents empty.
				if content == "" {
					content = metaString(resp.Metadatas[bi][pi], c.textField)
				}
			}
			docs = append(docs, rag.Document{ID: id, Content: content, Meta: meta})
		}
	}
	return docs, nil
}
