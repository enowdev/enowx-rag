import { useState, useCallback, useEffect } from 'react'
import { Search, RefreshCw, RotateCcw } from 'lucide-react'
import type { Page } from '../App'
import { api, type SearchResult } from '../lib/api'
import { useEvents } from '../lib/sse'

interface OverviewProps {
  activeProject: string
  onNavigate: (p: Page) => void
  onNavigateWithQuery?: (p: Page, query: string) => void
  sharedQuery?: string
  onSharedQueryChange?: (q: string) => void
  onProjectsUpdated?: (projs: { projectID: string; chunkCount: number }[]) => void
}

// Mock data for fallback when API is unavailable
const mockResults: SearchResult[] = [
  {
    id: '1',
    content: '// List only points belonging to this source_dir so that\n// indexing a different directory into the same project\n// doesn\'t wipe the first. Reconcile against currentSet.',
    score: 0.912,
    meta: { source_file: 'indexer.go', source_dir: 'pkg/indexer', chunk_index: '3', content_hash: 'a1b2c3d4', embed_model: 'voyage-4', type: 'architecture' },
  },
  {
    id: '2',
    content: 'DELETE FROM project_memory\nWHERE project_id = $1 AND id = ANY($2)\n// stale ids resolved from ListPoints() by source_dir',
    score: 0.874,
    meta: { source_file: 'pgvector.go', source_dir: 'pkg/rag', chunk_index: '5', content_hash: 'e5f6g7h8', embed_model: 'voyage-4', type: 'snippet' },
  },
  {
    id: '3',
    content: 'SELECT id, metadata->>\'source_file\' FROM project_memory\nWHERE project_id = $1 AND metadata->>\'source_dir\' = $2',
    score: 0.661,
    meta: { source_file: 'pgvector.go', source_dir: 'pkg/rag', chunk_index: '4', content_hash: 'i9j0k1l2', embed_model: 'voyage-4', type: 'snippet' },
  },
  {
    id: '4',
    content: '// DeletePoints removes specific points by ID from the\n// project collection.',
    score: 0.402,
    meta: { source_file: 'provider.go', source_dir: 'pkg/rag', chunk_index: '2', content_hash: 'm3n4o5p6', embed_model: 'voyage-4', type: 'api' },
  },
]

function scoreColor(score: number): string {
  if (score >= 0.75) return 'var(--good)'
  if (score >= 0.5) return 'var(--warn)'
  return 'var(--text-faint)'
}

function scoreBarColor(score: number): string {
  if (score >= 0.75) return 'var(--good)'
  if (score >= 0.5) return 'var(--warn)'
  return 'var(--border-strong)'
}

function highlightSnippet(content: string, query: string): React.ReactNode {
  if (!query.trim()) return content
  const terms = query.toLowerCase().split(/\s+/).filter((t) => t.length > 1)
  if (terms.length === 0) return content
  const regex = new RegExp(`(${terms.map((t) => t.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|')})`, 'gi')
  const parts = content.split(regex)
  return parts.map((part, i) =>
    regex.test(part) ? <mark key={i}>{part}</mark> : part,
  )
}

export function Overview({ activeProject, onNavigate, onNavigateWithQuery, sharedQuery, onSharedQueryChange, onProjectsUpdated }: OverviewProps) {
  const [query, setQuery] = useState(sharedQuery || 'how does pgvector handle stale chunks')
  const [results, setResults] = useState<SearchResult[]>(mockResults)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [hybrid, setHybrid] = useState(true)
  const [rerank, setRerank] = useState(true)
  const [compress, setCompress] = useState(false)
  const [k, setK] = useState(4)
  const [recall, setRecall] = useState(40)
  const [stats, setStats] = useState<{ totalProjects: number; totalChunks: number; embedModel: string } | null>(null)
  const { events } = useEvents()

  // Sync local query when sharedQuery changes (e.g., when arriving from Playground)
  useEffect(() => {
    if (sharedQuery && sharedQuery !== query) {
      setQuery(sharedQuery)
    }
  }, [sharedQuery]) // eslint-disable-line react-hooks/exhaustive-deps

  // Propagate query changes up to the shared state
  const handleQueryChange = useCallback((newQuery: string) => {
    setQuery(newQuery)
    onSharedQueryChange?.(newQuery)
  }, [onSharedQueryChange])

  // Fetch stats on mount, when activeProject changes, and when index_completed
  // or project_deleted SSE events arrive (VAL-CROSS-011, VAL-CROSS-012).
  const refreshStats = useCallback(() => {
    api.stats().then((s) => {
      setStats({ totalProjects: s.total_projects, totalChunks: s.total_chunks, embedModel: s.embed_model })
      // Also update the parent's project list so the sidebar stays in sync
      onProjectsUpdated?.(s.projects.map((p) => ({ projectID: p.project_id, chunkCount: p.chunk_count })))
    }).catch(() => {
      setStats({ totalProjects: 9, totalChunks: 76, embedModel: 'voyage-4' })
    })
  }, [onProjectsUpdated])

  useEffect(() => {
    refreshStats()
  }, [activeProject, refreshStats])

  // Listen for SSE events that should trigger a stats refresh
  useEffect(() => {
    if (events.length === 0) return
    const latest = events[0]
    if (latest.type === 'index_completed' || latest.type === 'project_deleted' || latest.type === 'points_deleted' || latest.type === 'documents_indexed') {
      refreshStats()
    }
  }, [events, refreshStats])

  const runSearch = useCallback(async () => {
    if (!activeProject || !query.trim()) return
    setLoading(true)
    setError('')
    try {
      const resp = await api.search({
        project_id: activeProject,
        query,
        k,
        recall,
        hybrid,
        rerank,
      })
      setResults(resp.results.length > 0 ? resp.results : [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Search failed')
      // Keep mock results visible for UI demo
    } finally {
      setLoading(false)
    }
  }, [activeProject, query, k, recall, hybrid, rerank])

  const handleReindex = useCallback(async () => {
    if (!activeProject) return
    try {
      await api.reindex(activeProject, `/Project/${activeProject}`)
    } catch {
      // API may not be available
    }
  }, [activeProject])

  // Format activity events for display
  const activityRows = events.slice(0, 6).map((ev) => {
    const time = new Date(ev.timestamp).toLocaleTimeString('en-US', { hour12: false })
    const isOk = ev.type === 'index_completed' || ev.type === 'query_executed'
    const label = ev.type.replace(/_/g, ' ')
    let desc = ''
    if (ev.data) {
      const d = ev.data as Record<string, unknown>
      if (d.indexed !== undefined) desc = `${d.indexed} chunks · ${d.deleted || 0} deleted`
      else if (d.candidates !== undefined) desc = `${d.candidates} candidates`
      else if (d.directory) desc = String(d.directory)
    }
    return { time, label, desc, isOk }
  })

  // Fallback activity if no SSE events
  const displayActivity = activityRows.length > 0 ? activityRows : [
    { time: '12:19:41', label: '✓ synced', desc: '76 chunks · 0 deleted', isOk: true },
    { time: '12:14:02', label: '✓ query', desc: 'latency 213ms · k=4', isOk: true },
    { time: '12:19:38', label: 'embed', desc: '17 files → voyage-4', isOk: false },
    { time: '12:13:55', label: 'rerank', desc: '40 → 4 · rerank-2.5', isOk: false },
    { time: '12:19:37', label: 'scan', desc: '/Project/enowx-rag', isOk: false },
    { time: '12:09:20', label: '✓ query', desc: 'latency 198ms · k=4', isOk: true },
  ]

  return (
    <>
      {/* Page head */}
      <div className="page-head">
        <h1>Overview</h1>
        <span className="id mono">project_{activeProject || '—'}</span>
        <div className="head-actions">
          <button className="btn" onClick={handleReindex}>
            <RefreshCw size={14} strokeWidth={1.7} />
            Re-index
          </button>
          <button className="btn primary" onClick={() => onNavigateWithQuery ? onNavigateWithQuery('playground', query) : onNavigate('playground')}>
            <Search size={14} strokeWidth={1.7} />
            New query
          </button>
        </div>
      </div>

      {/* KPIs */}
      <div className="kpis">
        <div className="kpi">
          <div className="label">Chunks</div>
          <div className="val tnum">{stats?.totalChunks ?? '—'}</div>
          <div className="sub">across 17 files</div>
        </div>
        <div className="kpi">
          <div className="label">Embedding</div>
          <div className="val mono" style={{ fontSize: '18px', marginTop: '11px' }}>
            {stats?.embedModel ?? 'voyage-4'}
          </div>
          <div className="sub mono">1024-dim · cosine</div>
        </div>
        <div className="kpi">
          <div className="label">Avg. query latency</div>
          <div className="val tnum">213<small> ms</small></div>
          <svg className="spark" viewBox="0 0 120 30" preserveAspectRatio="none">
            <polyline
              fill="none"
              stroke="var(--accent)"
              strokeWidth="1.5"
              points="0,22 12,18 24,20 36,12 48,15 60,9 72,14 84,7 96,11 108,6 120,10"
            />
            <circle cx="120" cy="10" r="2.2" fill="var(--accent)" />
          </svg>
        </div>
        <div className="kpi">
          <div className="label">Tokens used</div>
          <div className="val tnum">53.9<small> M</small></div>
          <div className="token-meter">
            <div className="meter-track">
              <i style={{ width: '27%' }} />
            </div>
            <div className="meter-cap">
              <span>26.96%</span>
              <span className="mono">200M free</span>
            </div>
          </div>
        </div>
      </div>

      {/* Bento grid */}
      <div className="cols">
        {/* Retrieval playground */}
        <section className="panel g-play">
          <div className="panel-head">
            <h2>Retrieval playground</h2>
            <span className="hint">top-{k} · {rerank ? 'reranked' : 'semantic'}</span>
          </div>
          <div className="panel-body">
            <div className="query-row">
              <input
                className="query-input"
                type="text"
                value={query}
                onChange={(e) => handleQueryChange(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && runSearch()}
                placeholder="Enter a query…"
              />
              <button className="btn primary" style={{ padding: '9px 14px' }} onClick={runSearch} disabled={loading}>
                {loading ? <RotateCcw size={14} strokeWidth={1.7} className="spin" /> : <Search size={14} strokeWidth={1.7} />}
                Run
              </button>
            </div>

            <div className="toolbar">
              <span className={`toggle ${hybrid ? 'on' : ''}`} onClick={() => setHybrid(!hybrid)}>
                <span className="switch" />
                Hybrid
              </span>
              <span className={`toggle ${rerank ? 'on' : ''}`} onClick={() => setRerank(!rerank)}>
                <span className="switch" />
                Rerank
              </span>
              <span className={`toggle ${compress ? 'on' : ''}`} onClick={() => setCompress(!compress)}>
                <span className="switch" />
                Compress
              </span>
              <span className="chip" onClick={() => setK((prev) => (prev >= 10 ? 1 : prev + 1))}>
                k = <b>{k}</b>
              </span>
              <span className="chip" onClick={() => setRecall((prev) => (prev >= 100 ? 10 : prev + 10))}>
                recall <b>{recall}</b>
              </span>
            </div>

            {error && <div className="error-state" style={{ marginTop: '12px' }}>{error}</div>}

            <div className="results">
              {results.map((res, i) => {
                const meta = res.meta || {}
                const fileName = meta.source_file || 'unknown'
                const tag = meta.type || 'snippet'
                return (
                  <div className="res" key={res.id || i}>
                    <div className="score">
                      <span className="num" style={{ color: scoreColor(res.score) }}>
                        {res.score.toFixed(3)}
                      </span>
                      <span className="bar">
                        <i style={{ width: `${Math.round(res.score * 100)}%`, background: scoreBarColor(res.score) }} />
                      </span>
                    </div>
                    <div className="res-main">
                      <div className="res-file">
                        <span className="path mono">
                          <b>{fileName}</b>
                          {meta.chunk_index ? ` · chunk ${meta.chunk_index}` : ''}
                        </span>
                        <span className="tag">{tag}</span>
                      </div>
                      <div className="res-snippet">{highlightSnippet(res.content, query)}</div>
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        </section>

        {/* Index status */}
        <section className="panel g-status">
          <div className="panel-head">
            <h2>Index status</h2>
            <span className="hint mono" style={{ color: 'var(--good)' }}>● synced</span>
          </div>
          <div className="panel-body" style={{ padding: '12px 15px' }}>
            <div className="stat-line"><span className="k">Files scanned</span><span className="v tnum">17</span></div>
            <div className="stat-line"><span className="k">Chunks indexed</span><span className="v tnum">{stats?.totalChunks ?? 76}</span></div>
            <div className="stat-line"><span className="k">Points deleted</span><span className="v tnum">0</span></div>
            <div className="stat-line"><span className="k">Chunk version</span><span className="v">v2 · 1500c</span></div>
            <div className="stat-line"><span className="k">Last sync</span><span className="v">just now</span></div>
          </div>
        </section>

        {/* Retrieval breakdown */}
        <section className="panel g-breakdown">
          <div className="panel-head">
            <h2>Retrieval breakdown</h2>
            <span className="hint mono">this query</span>
          </div>
          <div className="panel-body" style={{ padding: '13px 15px' }}>
            <div className="compo">
              <i style={{ width: hybrid ? '58%' : '100%', background: 'var(--accent)' }} />
              {hybrid && <i style={{ width: '42%', background: 'var(--good)' }} />}
            </div>
            <div className="compo-legend">
              <div className="lrow">
                <span className="sw" style={{ background: 'var(--accent)' }} />
                Dense (vector)
                <span className="lval">{hybrid ? '58%' : '100%'}</span>
              </div>
              {hybrid && (
                <div className="lrow">
                  <span className="sw" style={{ background: 'var(--good)' }} />
                  Lexical (tsvector)
                  <span className="lval">42%</span>
                </div>
              )}
              {rerank && (
                <div className="lrow">
                  <span className="sw" style={{ background: 'var(--border-strong)' }} />
                  Reranker moved
                  <span className="lval">2 ↑</span>
                </div>
              )}
            </div>
          </div>
        </section>

        {/* Chunk distribution */}
        <section className="panel g-dist">
          <div className="panel-head">
            <h2>Chunk distribution</h2>
            <span className="hint">chunks per file</span>
          </div>
          <div className="panel-body" style={{ padding: '14px 15px' }}>
            <div className="dist" style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '9px 24px' }}>
              {[
                { name: 'README.md', count: 12, pct: 100 },
                { name: 'pkg/rag/qdrant.go', count: 7, pct: 58 },
                { name: 'cmd/mcp-server/main.go', count: 9, pct: 75 },
                { name: 'pkg/indexer/indexer.go', count: 6, pct: 50 },
                { name: 'pkg/rag/pgvector.go', count: 8, pct: 67 },
                { name: 'pkg/rag/chroma.go', count: 5, pct: 42 },
              ].map((f) => (
                <div className="dist-row" key={f.name}>
                  <span className="dname">{f.name}</span>
                  <span className="dcount">{f.count}</span>
                  <span className="dist-bar">
                    <i style={{ width: `${f.pct}%` }} />
                  </span>
                </div>
              ))}
            </div>
          </div>
        </section>

        {/* Recent files */}
        <section className="panel g-files">
          <div className="panel-head">
            <h2>Recent files</h2>
          </div>
          <div className="panel-body" style={{ padding: '6px 15px 12px' }}>
            <div className="files">
              {[
                { name: 'pkg/rag/pgvector.go', chunks: 8, status: 'good' },
                { name: 'pkg/indexer/indexer.go', chunks: 6, status: 'good' },
                { name: 'cmd/mcp-server/main.go', chunks: 9, status: 'good' },
                { name: 'pkg/rag/voyage.go', chunks: 4, status: 'good' },
                { name: 'pkg/rag/tei.go', chunks: 3, status: 'good' },
                { name: 'README.md', chunks: 12, status: 'warn' },
                { name: 'skill/enowx-rag.md', chunks: 7, status: 'good' },
              ].map((f) => (
                <div className="file-row" key={f.name}>
                  <span className="status-dot" style={{ background: `var(--${f.status})` }} />
                  <span className="fname">{f.name}</span>
                  <span className="fmeta">{f.chunks} ch</span>
                </div>
              ))}
            </div>
          </div>
        </section>

        {/* Activity */}
        <section className="panel g-activity">
          <div className="panel-head">
            <h2>Activity</h2>
            <span className="hint">live</span>
          </div>
          <div className="panel-body" style={{ padding: '12px 15px' }}>
            <div className="log" style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '7px 24px' }}>
              {displayActivity.map((row, i) => (
                <div className="row" key={i}>
                  <span className="t">{row.time}</span>
                  <span className={row.isOk ? 'ok' : ''}>{row.label}</span>
                  <span>{row.desc}</span>
                </div>
              ))}
            </div>
          </div>
        </section>
      </div>
    </>
  )
}
