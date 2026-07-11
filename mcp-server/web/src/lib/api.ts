// API client: typed fetch wrappers for all REST endpoints.

export interface ProjectStat {
  project_id: string
  chunk_count: number
}

export interface PointInfo {
  id: string
  source_file?: string
  content_hash?: string
  chunk_version?: string
  doc_id?: string
  content?: string
  chunk_index?: string
}

export interface SearchResult {
  id: string
  content: string
  score: number
  meta?: Record<string, string>
}

export interface SearchResponse {
  results: SearchResult[]
}

export interface SearchRequest {
  project_id: string
  query: string
  k?: number
  recall?: number
  hybrid?: boolean
  rerank?: boolean
}

export interface StatsResponse {
  total_projects: number
  total_chunks: number
  embed_model: string
  projects: ProjectStat[]
}

export interface ReindexResponse {
  status: string
  project_id: string
  chunks_indexed: number
  points_deleted: number
  files_scanned: number
  skipped: number
  stale_error: string
}

export interface SetupStatus {
  configured: boolean
}

export interface SetupTestComponent {
  ok: boolean
  message: string
  latency_ms: number
}

export interface SetupTestResponse {
  vector_store: SetupTestComponent
  embedder: SetupTestComponent
}

export interface SetupApplyRequest {
  vector_store: string
  embedder: string
  voyage_api_key?: string
  voyage_model?: string
  voyage_dim?: number
  pgvector_dsn?: string
  qdrant_url?: string
  qdrant_api_key?: string
  chroma_url?: string
  tei_url?: string
}

const API_BASE = '/api'

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(url, init)
  if (!resp.ok) {
    let msg = `HTTP ${resp.status}`
    try {
      const body = await resp.json()
      if (body.error) msg = body.error
    } catch {
      // not JSON
    }
    throw new Error(msg)
  }
  return resp.json() as Promise<T>
}

export const api = {
  listProjects: () => fetchJSON<ProjectStat[]>(`${API_BASE}/projects`),

  getProject: (id: string) => fetchJSON<ProjectStat>(`${API_BASE}/projects/${encodeURIComponent(id)}`),

  listPoints: (id: string, params?: { source_file?: string; offset?: number; limit?: number }) => {
    const qs = new URLSearchParams()
    if (params?.source_file) qs.set('source_file', params.source_file)
    if (params?.offset !== undefined) qs.set('offset', String(params.offset))
    if (params?.limit !== undefined) qs.set('limit', String(params.limit))
    const q = qs.toString()
    return fetchJSON<PointInfo[]>(`${API_BASE}/projects/${encodeURIComponent(id)}/points${q ? '?' + q : ''}`)
  },

  deletePoint: (id: string, pointId: string) =>
    fetchJSON<{ status: string; project_id: string; point_id: string }>(
      `${API_BASE}/projects/${encodeURIComponent(id)}/points/${encodeURIComponent(pointId)}`,
      { method: 'DELETE' },
    ),

  reindex: (id: string, directory: string) =>
    fetchJSON<ReindexResponse>(`${API_BASE}/projects/${encodeURIComponent(id)}/reindex`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ directory }),
    }),

  deleteProject: (id: string) =>
    fetchJSON<{ status: string; project_id: string }>(`${API_BASE}/projects/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),

  search: (req: SearchRequest) =>
    fetchJSON<SearchResponse>(`${API_BASE}/search`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    }),

  stats: () => fetchJSON<StatsResponse>(`${API_BASE}/stats`),

  setupStatus: () => fetchJSON<SetupStatus>(`${API_BASE}/setup/status`),

  setupTest: (config: SetupApplyRequest) =>
    fetchJSON<SetupTestResponse>(`${API_BASE}/setup/test`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(config),
    }),

  setupApply: (config: SetupApplyRequest) =>
    fetchJSON<{ status: string }>(`${API_BASE}/setup/apply`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(config),
    }),
}
