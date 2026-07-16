# enowx-rag — Planning: Advanced RAG Platform + UI

> Status: rencana implementasi (belum dikerjakan). Dokumen ini adalah spesifikasi untuk agent yang akan mengimplementasikan.
> Terakhir diperbarui: 2026-07-11.
> Baca juga: [`HANDOFF.md`](./HANDOFF.md) (cara pakai skill/MCP/RAG untuk project ini) dan [`mockups/dashboard.html`](./mockups/dashboard.html) (referensi visual UI yang sudah di-approve).

Rencana menaikkan `enowx-rag` dari MCP server (stdio, headless) menjadi **platform RAG self-host open-source** dengan UI: dashboard, retrieval playground, dan **onboarding wizard** (pilih embedding + vector DB, local/cloud, set endpoint, auto-setup) — **tanpa membuang** MCP server yang sudah ada.

---

## Daftar isi

1. [Keputusan terkunci](#0-keputusan-yang-sudah-dikunci)
2. [Kondisi existing (peta kode)](#1-kondisi-existing-peta-kode-nyata)
3. [Arsitektur target](#2-arsitektur-target)
4. [Struktur folder target](#3-struktur-folder-target)
5. [Fase 0 — Refactor & fix fondasi (dengan kode)](#4-fase-0--refactor--fix-fondasi)
6. [Fase 1 — HTTP API (dengan kode)](#5-fase-1--http-api-layer)
7. [Fase 2 — Onboarding Wizard (dengan kode)](#6-fase-2--onboarding-wizard)
8. [Fase 3 — Dashboard & Playground (dengan kode)](#7-fase-3--dashboard--playground)
9. [Fase 4 — Kualitas RAG: rerank + hybrid (dengan kode)](#8-fase-4--kualitas-rag)
10. [Fase 5 — Polish & distribusi](#9-fase-5--polish--distribusi)
11. [Kriteria UI (design tokens)](#10-kriteria-ui--design-tokens)
12. [Risiko & gotcha](#11-risiko--gotcha)
13. [Status pengerjaan](#12-status-pengerjaan)

---

## 0. Keputusan yang sudah dikunci

| Aspek | Keputusan | Alasan |
|---|---|---|
| Target | Open-source self-host showcase | Orang bisa self-host; fokus DX & kemudahan deploy |
| Onboarding | Wizard: pilih embedding + vector DB, local/cloud, set endpoint, auto-setup | Nilai jual utama |
| Embedding default | Voyage-4 @ 1024-dim (pluggable) | Terbukti hemat (53.9M token → 10+ project besar) & akurat |
| Vector store (UI penuh) | pgvector direkomendasikan; tetap multi-provider | SQL bebas → dashboard & hybrid gampang |
| UI stack | React + Vite + Tailwind + shadcn/ui (baseColor neutral) + framer-motion (hemat) + lucide-react | Selaras ekosistem React user; di-embed ke Go binary |
| Distribusi | **1 binary** — SPA build → `embed.FS` → `go build` | Sesuai filosofi existing |
| Realtime | SSE (Server-Sent Events) | One-way cukup; native `net/http` |
| Desain | **True-black flat, zero gradient** (lihat §10) | Selera user; dev-tool |

---

## 1. Kondisi existing (peta kode nyata)

Semua path relatif ke `mcp-server/`.

| File | Isi | Catatan penting |
|---|---|---|
| `cmd/mcp-server/main.go` | Entry point, `Config`, `buildProvider()`, 6 MCP tool | Transport **stdio saja**. `main()` panggil `server.Run(ctx, &mcp.StdioTransport{})` |
| `pkg/rag/provider.go` | `Provider` interface + `EmbeddingClient` interface + `Document`/`Result`/`PointInfo` | Interface bersih, titik extend utama |
| `pkg/rag/pgvector.go` | `PGVectorProvider` — single table `project_memory`, HNSW cosine | `SemanticSearch` pakai `Embed` (document type), **belum** `EmbedQuery` |
| `pkg/rag/qdrant.go` | `QdrantProvider` — REST, per-project collection, optional API key | `vectorDim` dari `embedder.VectorSize()` |
| `pkg/rag/chroma.go` | `ChromaProvider` | — |
| `pkg/rag/voyage.go` | `VoyageEmbeddingClient` — batch 128, `input_type` document/query | ⚠️ `VectorSize()` **hardcoded return 1024**; `EmbedQuery` ada tapi tak dipakai; **tak kirim `output_dimension`** |
| `pkg/rag/tei.go` | `TEIEmbeddingClient` — batch 32, `/embed`, `VectorSize()` via `/info` | Fallback lokal |
| `pkg/indexer/indexer.go` | `Indexer.IndexProject()` — walk dir, chunk, incremental sync via `source_dir`/`source_file` | chunkSize default 1500 (dari main.go), stale reconcile per `source_dir` |
| `docker-compose.yml` | qdrant + tei-embedding + mcp-server | TEI model `BAAI/bge-small-en-v1.5` (384-dim) |

### Provider interface (existing — JANGAN diubah signature-nya, hanya di-extend)

```go
// pkg/rag/provider.go
type Provider interface {
	CreateCollection(ctx context.Context, projectID string) error
	DeleteCollection(ctx context.Context, projectID string) error
	Index(ctx context.Context, projectID string, docs []Document) error
	SemanticSearch(ctx context.Context, projectID, query string, limit int) ([]Result, error)
	Embed(ctx context.Context, text string) ([]float32, error)
	DeletePoints(ctx context.Context, projectID string, pointIDs []string) error
	ListPointIDs(ctx context.Context, projectID string, metaFilter map[string]string) ([]string, error)
	ListPoints(ctx context.Context, projectID string, metaFilter map[string]string) ([]PointInfo, error)
	Close() error
}
```

### 6 MCP tool existing
`rag_create_project`, `rag_delete_project`, `rag_index`, `rag_semantic_search`, `rag_retrieve_context`, `rag_index_project`.

### Gap ke visi
- ❌ Tidak ada HTTP server / API / UI.
- ❌ Tidak ada reranker.
- ❌ Tidak ada hybrid search (pgvector belum pakai `tsvector`).
- ⚠️ `Voyage.VectorSize()` hardcoded 1024; `EmbedQuery` (input_type=query) belum dipakai di `SemanticSearch`; `output_dimension` (Matryoshka) tak dikirim.
- ❌ Metadata versioning (`embed_model`, `embed_dim`, `chunk_version`, `content_hash`) belum ada.
- ❌ Config cuma env var; belum ada config file/wizard.

---

## 2. Arsitektur target

```
┌─────────────────────────────────────────────┐
│              pkg/rag core (Go)               │
│  Provider iface · Embedder · Indexer · Rerank │
└───────────────┬──────────────┬───────────────┘
                │              │
      ┌─────────▼───┐   ┌──────▼──────────────┐
      │ MCP (stdio) │   │  HTTP server (chi)  │
      │  (existing) │   │  REST + SSE + embed │
      └─────────────┘   └──────┬──────────────┘
                               │  embed.FS
                        ┌──────▼──────┐
                        │  React SPA  │  (Vite build → dist → embedded)
                        │ wizard·dash │
                        └─────────────┘
```

**Prinsip:** MCP dan HTTP dua-duanya adapter tipis di atas core yang sama. `go build` tetap hasilkan **1 binary**.

**Mode jalan:**
- `enowx-rag` (default) → stdio MCP (seperti sekarang, tak berubah).
- `enowx-rag --serve [--addr :7777]` → HTTP + UI.

---

## 3. Struktur folder target

```
mcp-server/
├── cmd/
│   └── mcp-server/
│       └── main.go            # tambah flag --serve; pisah runStdio() / runHTTP()
├── pkg/
│   ├── rag/                   # (existing) provider + embedder
│   │   ├── provider.go
│   │   ├── pgvector.go        # + hybrid (tsvector + RRF)
│   │   ├── qdrant.go
│   │   ├── chroma.go
│   │   ├── voyage.go          # fix VectorSize/EmbedQuery/output_dimension
│   │   ├── tei.go
│   │   └── rerank.go          # BARU: Voyage reranker client
│   ├── indexer/               # (existing) + content_hash + versioning
│   │   └── indexer.go
│   ├── config/                # BARU: load/save ~/.enowx-rag/config.yaml
│   │   └── config.go
│   ├── core/                  # BARU: service layer dipakai MCP & HTTP
│   │   └── service.go
│   └── httpapi/               # BARU: chi router, SSE, embed FS
│       ├── server.go
│       ├── handlers.go
│       ├── sse.go
│       └── setup.go           # wizard endpoints
├── web/                       # BARU: React SPA (Vite)
│   ├── package.json
│   ├── vite.config.ts
│   ├── index.html
│   ├── src/
│   │   ├── main.tsx
│   │   ├── App.tsx
│   │   ├── lib/api.ts
│   │   ├── lib/sse.ts
│   │   ├── styles/tokens.css  # design token true-black flat (dari §10)
│   │   ├── components/ui/     # shadcn components
│   │   ├── pages/Overview.tsx
│   │   ├── pages/Playground.tsx
│   │   ├── pages/Chunks.tsx
│   │   └── pages/onboarding/  # wizard steps
│   └── dist/                  # hasil build → di-embed
├── web/embed.go               # BARU: //go:embed dist
├── Makefile                   # BARU: web-build → go-build
├── docker-compose.yml
└── Dockerfile
```

---

## 4. Fase 0 — Refactor & fix fondasi

Prasyarat semua fase lain. Murah, dampak besar.

### 4.1 Fix Voyage: Matryoshka + input_type=query

**Masalah** (`pkg/rag/voyage.go`):
- `VectorSize()` hardcoded `return 1024` → tak bisa ganti dimensi.
- `EmbedQuery` (input_type=query) sudah ada tapi tak dipanggil oleh provider.
- `output_dimension` tak dikirim ke API.

**Perbaikan** — tambah field `Dim` dan kirim `output_dimension`:

```go
// pkg/rag/voyage.go
type VoyageEmbeddingClient struct {
	APIKey string
	Model  string
	Dim    int          // BARU: 256/512/1024/2048 (Matryoshka). 0 = default model.
	client *http.Client
}

func NewVoyageEmbeddingClient(apiKey, model string, dim int) *VoyageEmbeddingClient {
	if model == "" {
		model = "voyage-4"
	}
	if dim == 0 {
		dim = 1024 // sweet spot untuk voyage-4 + HNSW pgvector
	}
	return &VoyageEmbeddingClient{
		APIKey: apiKey, Model: model, Dim: dim,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

type voyageRequest struct {
	Input           []string `json:"input"`
	Model           string   `json:"model"`
	InputType       string   `json:"input_type,omitempty"`
	OutputDimension int      `json:"output_dimension,omitempty"` // BARU
}

func (c *VoyageEmbeddingClient) embedBatch(ctx context.Context, texts []string, inputType string) ([][]float32, error) {
	body, _ := json.Marshal(voyageRequest{
		Input:           texts,
		Model:           c.Model,
		InputType:       inputType,
		OutputDimension: c.Dim, // BARU
	})
	// ... sisanya sama
}

// VectorSize sekarang mengembalikan dimensi yang benar.
func (c *VoyageEmbeddingClient) VectorSize() int {
	if c.Dim > 0 {
		return c.Dim
	}
	return 1024
}
```

**Pakai `EmbedQuery` di provider.** Tambah interface opsional lalu deteksi di `SemanticSearch`:

```go
// pkg/rag/provider.go — tambah interface opsional
type QueryEmbedder interface {
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
}
```

```go
// pkg/rag/pgvector.go — di SemanticSearch, ganti embed query:
func (p *PGVectorProvider) queryVector(ctx context.Context, query string) ([]float32, error) {
	if qe, ok := p.embedder.(QueryEmbedder); ok {
		return qe.EmbedQuery(ctx, query) // input_type=query → retrieval lebih akurat
	}
	vecs, err := p.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}
```

**Update `main.go`** — teruskan `RAG_VECTOR_DIM` ke konstruktor:

```go
// cmd/mcp-server/main.go, di buildProvider()
case "voyage":
	if cfg.VoyageAPIKey == "" {
		return nil, fmt.Errorf("RAG_VOYAGE_API_KEY is required for voyage embedder")
	}
	embedder = rag.NewVoyageEmbeddingClient(cfg.VoyageAPIKey, cfg.VoyageModel, cfg.VectorDim)
```

⚠️ **Konsekuensi:** ganti dimensi = ganti ukuran vektor = **re-index penuh** & tabel/collection baru. Simpan `embed_dim` di metadata (lihat 4.2).

### 4.2 Metadata versioning (buka re-index selektif)

Di `pkg/indexer/indexer.go`, saat membuat `Document`, tambah metadata:

```go
import "crypto/sha256"
import "encoding/hex"

func contentHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8]) // 16 hex chars cukup
}

// di dalam loop chunk:
docs = append(docs, rag.Document{
	ID:      docID,
	Content: fmt.Sprintf("File: %s\n\n%s", relPath, chunk),
	Meta: map[string]string{
		"source_file":   relPath,
		"source_dir":    sourceDir,
		"chunk_index":   fmt.Sprintf("%d", i),
		"content_hash":  contentHash(chunk),      // BARU: skip re-embed jika sama
		"chunk_version": "v2",                    // BARU: bump saat strategi chunk berubah
		// embed_model & embed_dim diisi di provider.Index (tahu model aktif)
	},
})
```

Di `provider.Index`, tambahkan `embed_model` & `embed_dim` ke tiap metadata sebelum simpan. Ini memungkinkan UI menampilkan model/dim per chunk dan re-index selektif hanya chunk yang `content_hash`-nya berubah.

### 4.3 Ekstrak service layer (`pkg/core`)

Supaya MCP dan HTTP tak duplikasi logika:

```go
// pkg/core/service.go
package core

type Service struct {
	provider rag.Provider
	reranker rag.Reranker // nil kalau tak dikonfigurasi
	indexer  *indexer.Indexer
	events   *EventBus     // untuk SSE
}

func New(provider rag.Provider, reranker rag.Reranker) *Service { /* ... */ }

// Dipakai MCP tool DAN HTTP handler:
func (s *Service) Search(ctx context.Context, projectID, query string, opts SearchOpts) ([]rag.Result, error)
func (s *Service) IndexProject(ctx context.Context, projectID, dir string) (*indexer.SyncResult, error)
func (s *Service) ListProjects(ctx context.Context) ([]ProjectStat, error)
```

MCP tool di `main.go` kemudian jadi wrapper tipis di atas `Service`.

---

## 5. Fase 1 — HTTP API layer

### 5.1 Flag & mode di main.go

```go
// cmd/mcp-server/main.go
func main() {
	serve := flag.Bool("serve", false, "run HTTP+UI server instead of stdio MCP")
	addr := flag.String("addr", ":7777", "HTTP listen address")
	flag.Parse()

	cfg := loadConfig()
	svc := buildService(cfg) // wrap provider + reranker + indexer

	if *serve {
		runHTTP(svc, *addr)   // httpapi.NewServer(svc).Run(addr)
		return
	}
	runStdio(svc)             // MCP seperti sekarang
}
```

### 5.2 Router (chi) + embed SPA

```go
// pkg/httpapi/server.go
package httpapi

import (
	"net/http"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewServer(svc *core.Service, ui fs.FS) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger, middleware.Recoverer)

	r.Route("/api", func(r chi.Router) {
		r.Get("/projects", h.listProjects)
		r.Get("/projects/{id}", h.getProject)
		r.Get("/projects/{id}/points", h.listPoints)
		r.Post("/projects/{id}/reindex", h.reindex)
		r.Delete("/projects/{id}", h.deleteProject)
		r.Post("/search", h.search)
		r.Get("/stats", h.stats)
		r.Get("/events", h.sse)           // SSE stream
		// wizard (Fase 2):
		r.Post("/setup/test", h.setupTest)
		r.Post("/setup/apply", h.setupApply)
		r.Get("/setup/status", h.setupStatus)
	})

	// SPA fallback: serve embedded dist, index.html untuk route non-file
	r.Handle("/*", spaHandler(ui))
	return r
}
```

```go
// web/embed.go
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
```

### 5.3 Handler search (contoh, dengan opsi hybrid/rerank)

```go
// pkg/httpapi/handlers.go
type searchReq struct {
	ProjectID string `json:"project_id"`
	Query     string `json:"query"`
	K         int    `json:"k"`
	Recall    int    `json:"recall"`
	Hybrid    bool   `json:"hybrid"`
	Rerank    bool   `json:"rerank"`
}

func (h *Handlers) search(w http.ResponseWriter, r *http.Request) {
	var req searchReq
	json.NewDecoder(r.Body).Decode(&req)
	res, err := h.svc.Search(r.Context(), req.ProjectID, req.Query, core.SearchOpts{
		K: req.K, Recall: req.Recall, Hybrid: req.Hybrid, Rerank: req.Rerank,
	})
	if err != nil { writeErr(w, 500, err); return }
	writeJSON(w, map[string]any{"results": res})
}
```

### 5.4 SSE (realtime progress)

```go
// pkg/httpapi/sse.go
func (h *Handlers) sse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher := w.(http.Flusher)

	ch := h.svc.Subscribe() // channel dari EventBus
	defer h.svc.Unsubscribe(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			b, _ := json.Marshal(ev)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, b)
			flusher.Flush()
		}
	}
}
```

Frontend konsumsi: `new EventSource("/api/events")`.

---

## 6. Fase 2 — Onboarding Wizard

**JANGAN LUPA — ini nilai jual utama.** Dijalankan saat first-run (config belum ada) atau via menu Setup.

### 6.1 Config file

```go
// pkg/config/config.go
package config

type Config struct {
	VectorStore string `yaml:"vector_store"` // pgvector|qdrant|chroma
	Embedder    string `yaml:"embedder"`     // voyage|tei|openai
	Voyage      struct {
		APIKey string `yaml:"api_key"`
		Model  string `yaml:"model"`
		Dim    int    `yaml:"dim"`
	} `yaml:"voyage"`
	PGVectorDSN string `yaml:"pgvector_dsn"`
	QdrantURL   string `yaml:"qdrant_url"`
	// ...
}

func Path() string { // ~/.enowx-rag/config.yaml
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".enowx-rag", "config.yaml")
}
func Load() (*Config, error) { /* baca yaml; error jika belum ada → trigger wizard */ }
func Save(c *Config) error   { /* mkdir + tulis yaml, chmod 0600 (ada API key) */ }
```

Prioritas config: **env var > config.yaml > default** (env var menang, biar MCP existing tak berubah perilaku).

### 6.2 Endpoint test connection

```go
// pkg/httpapi/setup.go
type testReq struct {
	VectorStore string `json:"vector_store"`
	Embedder    string `json:"embedder"`
	// endpoints + keys...
}
type testResult struct {
	VectorStore compCheck `json:"vector_store"`
	Embedder    compCheck `json:"embedder"`
}
type compCheck struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"` // actionable: "Qdrant unreachable at localhost:6333 — run `docker compose up -d qdrant`"
	Latency int    `json:"latency_ms"`
}

func (h *Handlers) setupTest(w http.ResponseWriter, r *http.Request) {
	// coba connect vector store (health check) + coba embed 1 kata via embedder
	// kembalikan per-komponen hijau/merah + pesan
}
```

### 6.3 Flow wizard (React, 6 step)

```
web/src/pages/onboarding/
├── Wizard.tsx        # stepper state machine
├── StepWelcome.tsx   # deteksi env (Docker? port kepakai?)
├── StepVectorStore.tsx  # kartu pgvector/qdrant/chroma + local/cloud + endpoint
├── StepEmbedding.tsx    # kartu voyage/tei/openai + key + model + DIMENSI
├── StepTest.tsx      # tombol Test → POST /api/setup/test → hijau/merah per komponen
├── StepAutoSetup.tsx # generate compose ATAU jalankan (SSE progress)
└── StepDone.tsx      # POST /api/setup/apply → simpan → ke dashboard
```

Contoh state machine (ringkas):

```tsx
// Wizard.tsx
const STEPS = ["welcome","vector","embedding","test","setup","done"] as const;
type Step = typeof STEPS[number];

export function Wizard() {
  const [step, setStep] = useState<Step>("welcome");
  const [cfg, setCfg] = useState<DraftConfig>(defaultDraft);
  const next = () => setStep(STEPS[STEPS.indexOf(step)+1]);
  const back = () => setStep(STEPS[STEPS.indexOf(step)-1]);
  // render step sesuai `step`, pass cfg + setCfg + next/back
}
```

### 6.4 Catatan desain wizard
- Setiap step bisa mundur; draft state disimpan.
- Field endpoint/DSN/key pakai **mono**; tombol reveal untuk key.
- **Peringatan penting** di StepEmbedding: "Ganti model/dimensi mengunci collection — perlu re-index penuh."
- Auto-setup default: **generate `docker-compose.yml` + tampilkan command** (aman). Opsi "jalankan otomatis" dengan disclaimer.
- Empty/error state jelas & actionable.
- **Mockup wizard: TODO** `docs/mockups/onboarding.html` (belum dibuat).

---

## 7. Fase 3 — Dashboard & Playground

Referensi visual: **`docs/mockups/dashboard.html`** (sudah di-approve). Layout: sidebar fixed + bento grid full-width.

### 7.1 Peta halaman
- **Overview** (`pages/Overview.tsx`) — KPI row (chunks, embedding, latency+sparkline, token usage) + Retrieval playground + Index status + Chunk distribution + Retrieval breakdown + Recent files + Activity (bento grid seperti mockup).
- **Playground** (`pages/Playground.tsx`) — versi full dari playground: query → chunk + skor + highlight + toggle Hybrid/Rerank/Compress.
- **Chunks** (`pages/Chunks.tsx`) — browse/filter/hapus chunk per file.
- **Setup** — masuk ke Wizard.

### 7.2 Client API

```ts
// web/src/lib/api.ts
export async function search(body: SearchReq): Promise<SearchRes> {
  const r = await fetch("/api/search", {
    method: "POST", headers: {"content-type":"application/json"},
    body: JSON.stringify(body),
  });
  if (!r.ok) throw new Error((await r.json()).error);
  return r.json();
}
export async function listProjects(): Promise<ProjectStat[]> {
  return (await fetch("/api/projects")).json();
}
```

### 7.3 SSE hook (realtime activity)

```ts
// web/src/lib/sse.ts
export function useEvents(onEvent: (e: RagEvent) => void) {
  useEffect(() => {
    const es = new EventSource("/api/events");
    es.onmessage = (m) => onEvent(JSON.parse(m.data));
    return () => es.close();
  }, []);
}
```

---

## 8. Fase 4 — Kualitas RAG

Ini yang bikin "advance". Dampak > UI apapun.

### 8.1 Reranker (Voyage rerank-2.5)

```go
// pkg/rag/rerank.go
package rag

type Reranker interface {
	Rerank(ctx context.Context, query string, docs []string, topK int) ([]RerankHit, error)
}
type RerankHit struct {
	Index int     `json:"index"` // index di slice docs asli
	Score float64 `json:"relevance_score"`
}

type VoyageReranker struct {
	APIKey string
	Model  string // "rerank-2.5"
	client *http.Client
}

const voyageRerankURL = "https://api.voyageai.com/v1/rerank"

func (r *VoyageReranker) Rerank(ctx context.Context, query string, docs []string, topK int) ([]RerankHit, error) {
	body, _ := json.Marshal(map[string]any{
		"query": query, "documents": docs, "model": r.Model, "top_k": topK,
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", voyageRerankURL, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+r.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	var out struct{ Data []RerankHit `json:"data"` }
	json.NewDecoder(resp.Body).Decode(&out)
	return out.Data, nil
}
```

Alur di `core.Service.Search`: retrieval recall besar (mis. 40) → rerank → top-K kecil (mis. 4).

```go
// pkg/core/service.go
func (s *Service) Search(ctx context.Context, projectID, query string, o SearchOpts) ([]rag.Result, error) {
	recall := o.Recall; if recall == 0 { recall = 40 }
	k := o.K; if k == 0 { k = 5 }

	cands, err := s.provider.SemanticSearch(ctx, projectID, query, recall)
	if err != nil { return nil, err }

	if o.Rerank && s.reranker != nil && len(cands) > 0 {
		docs := make([]string, len(cands))
		for i, c := range cands { docs[i] = c.Content }
		hits, err := s.reranker.Rerank(ctx, query, docs, k)
		if err == nil {
			out := make([]rag.Result, 0, len(hits))
			for _, h := range hits {
				r := cands[h.Index]; r.Score = h.Score
				out = append(out, r)
			}
			return out, nil
		}
		// fallback ke urutan semantic kalau rerank gagal
	}
	if len(cands) > k { cands = cands[:k] }
	return cands, nil
}
```

### 8.2 Hybrid search (pgvector: dense + tsvector + RRF)

Tambah kolom tsvector + index di `ensureTable`:

```sql
-- pkg/rag/pgvector.go, ensureTable()
ALTER TABLE project_memory ADD COLUMN IF NOT EXISTS content_tsv tsvector
  GENERATED ALWAYS AS (to_tsvector('simple', content)) STORED;
CREATE INDEX IF NOT EXISTS idx_project_memory_tsv
  ON project_memory USING GIN (content_tsv);
```

Query hybrid dengan Reciprocal Rank Fusion (RRF):

```sql
-- dense + lexical, gabung via RRF (k=60 konvensi)
WITH dense AS (
  SELECT id, content, metadata,
         ROW_NUMBER() OVER (ORDER BY embedding <=> $1::vector) AS rank
  FROM project_memory WHERE project_id = $3
  ORDER BY embedding <=> $1::vector LIMIT $4
),
lexical AS (
  SELECT id, content, metadata,
         ROW_NUMBER() OVER (ORDER BY ts_rank(content_tsv, plainto_tsquery('simple',$2)) DESC) AS rank
  FROM project_memory
  WHERE project_id = $3 AND content_tsv @@ plainto_tsquery('simple',$2)
  LIMIT $4
)
SELECT COALESCE(d.id,l.id) AS id,
       COALESCE(d.content,l.content) AS content,
       COALESCE(d.metadata,l.metadata) AS metadata,
       (COALESCE(1.0/(60+d.rank),0) + COALESCE(1.0/(60+l.rank),0)) AS score
FROM dense d FULL OUTER JOIN lexical l USING (id)
ORDER BY score DESC LIMIT $5;
```

> Untuk korpus kode (nama fungsi/API eksak), lexical sering menang — hybrid menutup gap yang dense saja lewatkan.

### 8.3 Contextual compression (opsional, hemat token)
Setelah rerank, potong kalimat non-relevan tiap chunk sebelum dikirim ke LLM. Bisa heuristik (skor per kalimat) atau LLM-based. Toggle "Compress" di UI.

---

## 9. Fase 5 — Polish & distribusi

### 9.1 Makefile (build pipeline 1 binary)

```makefile
# Makefile
.PHONY: web build

web:
	cd web && npm ci && npm run build   # hasil ke web/dist

build: web
	cd mcp-server && go build -o enowx-rag ./cmd/mcp-server

dev-web:
	cd web && npm run dev               # vite dev server proxy ke :7777
```

### 9.2 Lain-lain
- `docker-compose.yml` all-in-one (sudah ada, tambah service UI opsional).
- README + GIF wizard, one-command deploy.
- Auth ringan opsional (single admin token via `RAG_ADMIN_TOKEN`) bila di-expose ke internet.

---

## 10. Kriteria UI — design tokens

**Arah: true-black flat, zero gradient.** Tervalidasi lewat `docs/mockups/dashboard.html`.

Prinsip:
- Dark bg **`#000000`**, surface `#0a0a0a` — hitam pekat, bukan biru-kehitaman.
- **Zero gradient.** Semua warna solid. Kedalaman lewat **hairline border `1px` + beda surface**, bukan shadow/glow/glass.
- Light & dark **sederajat** (palet zinc netral); toggle stamp `data-theme`.
- **1 accent solid** (violet) hanya untuk aksi & endpoint. **Skor pakai warna semantik terpisah** (hijau/amber/abu).
- **Mono** untuk semua data teknis (path, ID, skor, DSN).
- Data-first, density tinggi. Sidebar fixed, konten full-width (bento grid).
- **NO** glass/nebula/gradient/Aceternity/Magic UI.

Token (salin ke `web/src/styles/tokens.css`):

```css
:root { /* light */
  --bg:#ffffff; --surface:#fafafa; --surface-2:#f4f4f5;
  --border:#e4e4e7; --border-strong:#d4d4d8;
  --text:#09090b; --text-dim:#52525b; --text-faint:#a1a1aa;
  --accent:#6d4aff; --accent-fg:#ffffff;
  --good:#1a7f37; --warn:#9a6700; --crit:#cf222e;
}
:root[data-theme="dark"], @media (prefers-color-scheme: dark) { /* dark */
  --bg:#000000; --surface:#0a0a0a; --surface-2:#101012;
  --border:#1e1e21; --border-strong:#2a2a2e;
  --text:#fafafa; --text-dim:#a1a1aa; --text-faint:#6b6b73;
  --accent:#7c5cff; --accent-fg:#ffffff;
  --good:#3fb950; --warn:#d29922; --crit:#f85149;
}
```

Font: UI = Inter/Geist; data/mono = JetBrains Mono / Geist Mono. **Bukan** Rubik/Nunito (itu punya RobloxKit — beda produk).

---

## 11. Risiko & gotcha

- ⚠️ **Ganti embedding model/dimensi = re-index penuh.** Vektor beda dimensi/model tak boleh dicampur dalam satu collection. Wizard harus mengunci & memperingatkan.
- ⚠️ **Auto-setup Docker dari binary** kompleks/rawan (izin, port bentrok). Default: generate compose + tampilkan command, bukan eksekusi paksa.
- ⚠️ **Multi-provider × wizard** melipatgandakan matrix testing. Utamakan **pgvector** untuk UI penuh (hybrid butuh tsvector — hanya pgvector); provider lain "supported tapi basic".
- ⚠️ **Env var menang atas config.yaml** — jaga agar MCP stdio existing tak berubah perilaku saat file config muncul.
- ⚠️ **HNSW pgvector**: butuh index (jangan seq scan). Untuk irit storage pertimbangkan `halfvec(1024)`. Voyage output normalized → cosine.
- ⚠️ **Build pipeline**: `web/dist` harus ada sebelum `go build` (embed.FS gagal kalau folder kosong). Makefile `build` depend `web`.
- ⚠️ **Selera desain ≠ RobloxKit**: RobloxKit glass+gradient+rose; enowx-rag true-black flat zero gradient. Reuse ekosistem React saja, bukan look-nya.
- ⚠️ **Indexer chunkSize 1500** dipilih agar chunk + prefix "File:" muat di limit token TEI (~512 tok, ~3 char/token). Kalau pindah ke Voyage penuh, bisa dinaikkan.

---

## 12. Status pengerjaan

- [x] Project ter-index ke RAG (`project_enowx-rag`, 17 file, 76 chunk).
- [x] Skill terpasang (`~/.factory/skills/enowx-rag/skill.md`).
- [x] AGENTS.md & CLAUDE.md sudah ada di root.
- [x] Kriteria UI dikunci (true-black flat) + mockup dashboard di-approve (`docs/mockups/dashboard.html`).
- [x] Planning detail dengan contoh kode (dokumen ini).
- [x] HANDOFF.md untuk agent lain.
- [ ] Mockup onboarding wizard (`docs/mockups/onboarding.html`).
- [ ] Fase 0 — fix Voyage + metadata versioning + service layer.
- [ ] Fase 1 — HTTP API + SSE + embed FS.
- [ ] Fase 2 — Onboarding wizard.
- [ ] Fase 3 — Dashboard & playground.
- [ ] Fase 4 — Reranker + hybrid.
- [ ] Fase 5 — Polish & distribusi.

**Urutan rekomendasi:** Fase 0 → 4 (kualitas dulu, murah & berdampak) → 1 → 3 (biar bisa lihat hasil) → 2 (wizard, paling makan waktu) → 5.
