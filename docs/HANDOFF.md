# HANDOFF — enowx-rag

> Untuk agent (Claude Code / Droid / Cursor / dll) yang melanjutkan pekerjaan di project ini.
> Baca ini **dulu**, lalu [`PLANNING.md`](./PLANNING.md) untuk spesifikasi implementasi.
> Terakhir diperbarui: 2026-07-11.

---

## 1. Apa ini

`enowx-rag` = **per-project RAG memory MCP server** (Go, stdio transport). Tiap project punya collection vektor terisolasi (`project_<PROJECT_ID>`) sehingga LLM bisa meng-index konteks codebase dan meretrieve-nya cepat.

**Sedang dikembangkan** menjadi platform RAG dengan UI (dashboard + retrieval playground + onboarding wizard) — lihat `PLANNING.md`. Kode UI/HTTP **belum ada**; yang ada sekarang murni MCP server.

- Repo: https://github.com/enowdev/enowx-rag
- Lokal: `/Users/enowdev/Project/enowx-rag`
- Bahasa: Go 1.26 (module `github.com/enowdev/enowx-rag`)
- Binary: `mcp-server/mcp-server` (hasil `go build ./cmd/mcp-server`)

---

## 2. PENTING — project ID untuk RAG project ini

Saat memakai RAG memory untuk **project enowx-rag sendiri**, gunakan:

```
PROJECT_ID = enowx-rag
```

Collection: `project_enowx-rag`. Sudah ada isinya (17 file, 76 chunk per 2026-07-11).

---

## 3. Cara pakai RAG memory (WAJIB, tiap sesi)

Project ini memakai MCP server-nya sendiri untuk memori. Alur wajib:

### Sebelum coding / menjawab pertanyaan arsitektur
Panggil MCP tool:
```
rag_retrieve_context(project_id="enowx-rag", query="<pertanyaan/topik user>")
```
Baca konteks yang kembali. Kalau relevan, pakai untuk membentuk jawaban/rencana. Kalau kosong/tak relevan, lanjut normal.

### Setelah menyelesaikan pekerjaan
1. Ringkas apa yang diubah.
2. Sinkronkan file ke RAG:
   ```
   rag_index_project(project_id="enowx-rag", directory="/Users/enowdev/Project/enowx-rag")
   ```
   Ini menangani file baru, edit, dan penghapusan otomatis.
3. Simpan fakta/keputusan penting yang tak terlihat dari kode:
   ```
   rag_index(project_id="enowx-rag", documents=[{content:"...", meta:{type:"decision"}}])
   ```

### Tag metadata yang dipakai
`type:architecture`, `type:decision`, `type:api`, `type:bugfix`, `type:howto`, `type:snippet`. Jaga chunk pendek & satu ide per chunk.

---

## 4. MCP tools yang tersedia

| Tool | Fungsi | Argumen utama |
|---|---|---|
| `rag_create_project` | Buat collection project (aman dipanggil ulang) | `project_id` |
| `rag_delete_project` | Hapus collection + semua memori | `project_id` |
| `rag_index` | Index dokumen manual (fakta/keputusan) | `project_id`, `documents[]` |
| `rag_index_project` | Scan direktori & auto-index semua file kode/teks (handle insert + delete) | `project_id`, `directory` |
| `rag_semantic_search` | Semantic search, kembalikan chunk + skor | `project_id`, `query`, `limit` |
| `rag_retrieve_context` | String konteks ringkas untuk LLM (gabungan top chunk) | `project_id`, `query`, `limit` |

Catatan implementasi:
- `rag_index_project` skip `node_modules`, `.git`, `vendor`, `dist`, `build`, dll (lihat `defaultIgnores` di `pkg/indexer/indexer.go`), skip file > 500KB, chunkSize 1500 char.
- Stale reconcile per `source_dir` (base name direktori) — meng-index dua direktori berbeda ke project sama **tidak** saling menghapus.

---

## 5. Skill

Skill (`skill/enowx-rag.md`, ~20KB) adalah panduan setup/onboarding yang bisa dipasang ke tool apa pun.

- **Terpasang di:** `~/.factory/skills/enowx-rag/skill.md` (Factory Droid global).
- **Install ulang / ke tool lain:**
  ```bash
  mkdir -p ~/.factory/skills/enowx-rag
  cp /Users/enowdev/Project/enowx-rag/skill/enowx-rag.md ~/.factory/skills/enowx-rag/skill.md
  ```
  Untuk tool lain, taruh di direktori skill tool tersebut (mis. `.agents/skills/enowx-rag/skill.md`). Jangan asumsikan `~/.factory` untuk tool non-Droid.
- Skill berisi dua path: **Option A** (setup penuh dari nol) & **Option B** (onboard project baru saat MCP sudah terpasang). Detail lengkap di `README.md`.

---

## 6. Setup MCP server (kalau belum jalan)

### Config MCP (Voyage — direkomendasikan)
Claude Code (`~/.claude.json` atau `.mcp.json`):
```json
{
  "mcpServers": {
    "enowx-rag": {
      "command": "/Users/enowdev/Project/enowx-rag/mcp-server/mcp-server",
      "env": {
        "RAG_VECTOR_STORE": "qdrant",
        "RAG_QDRANT_URL": "http://localhost:6333",
        "RAG_VOYAGE_API_KEY": "your-voyage-api-key",
        "RAG_VOYAGE_MODEL": "voyage-4"
      }
    }
  }
}
```

### Build binary
```bash
cd /Users/enowdev/Project/enowx-rag/mcp-server
go build ./cmd/mcp-server
```

### Backend lokal (kalau tak pakai Voyage/Qdrant cloud)
```bash
cd /Users/enowdev/Project/enowx-rag/mcp-server
docker compose up -d qdrant tei-embedding
# verify
curl -f http://localhost:6333/healthz
curl -f http://localhost:8081/health
```

Format config untuk tool lain (Cursor, Zed, Windsurf, Codex, dll) ada lengkap di `README.md` §4.

---

## 7. Environment variables

| Variable | Default | Keterangan |
|---|---|---|
| `RAG_VECTOR_STORE` | `qdrant` | `qdrant` \| `chroma` \| `pgvector` |
| `RAG_EMBEDDER` | `voyage` | `voyage` \| `tei` (fallback ke `tei` jika `RAG_VOYAGE_API_KEY` kosong) |
| `RAG_QDRANT_URL` | `http://localhost:6333` | Qdrant REST URL |
| `RAG_QDRANT_API_KEY` | — | opsional, Qdrant cloud |
| `RAG_CHROMA_URL` | `http://localhost:8000` | Chroma REST URL |
| `RAG_PGVECTOR_DSN` | — | Postgres connection string |
| `RAG_TEI_URL` | `http://localhost:8081` | TEI URL |
| `RAG_VOYAGE_API_KEY` | — | wajib saat `RAG_EMBEDDER=voyage` |
| `RAG_VOYAGE_MODEL` | `voyage-4` | model Voyage |
| `RAG_VECTOR_DIM` | (dari model) | override dimensi vektor |

---

## 8. Peta kode (di mana mengubah apa)

Semua path relatif ke `mcp-server/`.

| Ingin ubah… | Ke file… |
|---|---|
| Tool MCP / entry point / config env | `cmd/mcp-server/main.go` |
| Kontrak provider/embedder | `pkg/rag/provider.go` |
| Logika pgvector (+hybrid nanti) | `pkg/rag/pgvector.go` |
| Logika Qdrant | `pkg/rag/qdrant.go` |
| Embedding Voyage (fix dim/query nanti) | `pkg/rag/voyage.go` |
| Embedding TEI lokal | `pkg/rag/tei.go` |
| Scan direktori / chunking / incremental sync | `pkg/indexer/indexer.go` |
| Backend lokal (Qdrant+TEI) | `docker-compose.yml` |

**Gotcha kode yang sudah diketahui** (detail + fix di `PLANNING.md` §4):
- `voyage.go`: `VectorSize()` hardcoded 1024; `EmbedQuery` (input_type=query) ada tapi belum dipakai `SemanticSearch`; `output_dimension` tak dikirim.
- Belum ada metadata versioning (`embed_model`/`embed_dim`/`content_hash`).

---

## 9. Konvensi & aturan project

- **Jangan ubah signature `Provider` interface** — hanya extend (tambah interface opsional seperti `QueryEmbedder`/`Reranker`).
- **MCP stdio harus tetap jalan tanpa perubahan perilaku.** Fitur baru (HTTP/UI) di belakang flag `--serve`. Env var menang atas config file.
- **1 binary.** UI di-embed via `embed.FS`. Build: `web/dist` dulu, baru `go build` (Makefile `build` depend `web`).
- **Embedding tak boleh dicampur** antar model/dimensi dalam satu collection → ganti = re-index penuh.
- **Log ke stderr, bukan stdout** di mode stdio (stdout dipakai protokol MCP). Lihat `fmt.Fprintf(os.Stderr, ...)` di `main.go`.
- **Desain UI true-black flat, zero gradient** — jangan reuse look RobloxKit (glass/gradient). Token di `PLANNING.md` §10.

---

## 10. Build & test

```bash
cd /Users/enowdev/Project/enowx-rag/mcp-server
go build ./cmd/mcp-server        # build
go vet ./...                     # static check
go test ./...                    # test (jika ada)
```

Git: branch default `main`, remote `origin` = GitHub `enowdev/enowx-rag`. Commit/push **hanya jika diminta user**.

---

## 11. Status saat ini & langkah berikutnya

Sudah beres:
- [x] Project ter-index ke RAG (`project_enowx-rag`).
- [x] Skill terpasang (`~/.factory/skills/enowx-rag/skill.md`).
- [x] `AGENTS.md` + `CLAUDE.md` di root.
- [x] Planning detail: `docs/PLANNING.md`.
- [x] Mockup dashboard (approved): `docs/mockups/dashboard.html`.

Belum:
- [ ] Mockup onboarding wizard (`docs/mockups/onboarding.html`).
- [ ] Implementasi Fase 0–5 (lihat `PLANNING.md`).

**Mulai dari mana:** `PLANNING.md` §4 (Fase 0 — fix Voyage + metadata + service layer). Itu murah, berdampak besar, dan prasyarat semua fase lain. Urutan rekomendasi: Fase 0 → 4 → 1 → 3 → 2 → 5.

---

## 12. Dokumen terkait

- [`PLANNING.md`](./PLANNING.md) — spesifikasi implementasi lengkap + contoh kode.
- [`mockups/dashboard.html`](./mockups/dashboard.html) — referensi visual UI (approved).
- [`../README.md`](../README.md) — setup guide lengkap semua tool.
- [`../skill/enowx-rag.md`](../skill/enowx-rag.md) — skill (template AGENTS.md/CLAUDE.md).
- [`../AGENTS.md`](../AGENTS.md) — panduan install universal.
