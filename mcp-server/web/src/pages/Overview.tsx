import { useState, useCallback, useEffect } from 'react'
import { Search, RefreshCw, RotateCcw } from 'lucide-react'
import type { Page } from '../App'
import { api, type SearchResult, type MetricsResponse, type PointInfo } from '../lib/api'
import { useEvents } from '../lib/sse'

interface OverviewProps {
  activeProject: string
  onNavigate: (p: Page) => void
  onNavigateWithQuery?: (p: Page, query: string) => void
  sharedQuery?: string
  onSharedQueryChange?: (q: string) => void
  onProjectsUpdated?: (projs: { projectID: string; chunkCount: number }[]) => void
}

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

// fileAgg aggregates chunk counts per source_file from the project's points.
interface FileAgg {
  name: string
  count: number
}

function aggregateFiles(points: PointInfo[]): FileAgg[] {
  const counts = new Map<string, number>()
  for (const p of points) {
    const f = p.source_file || '(unknown)'
    counts.set(f, (counts.get(f) || 0) + 1)
  }
  return Array.from(counts.entries())
    .map(([name, count]) => ({ name, count }))
    .sort((a, b) => b.count - a.count)
}

function formatTokens(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'k'
  return String(n)
}

export function Overview({ activeProject, onNavigate, onNavigateWithQuery, sharedQuery, onSharedQueryChange, onProjectsUpdated }: OverviewProps) {
  const [query, setQuery] = useState(sharedQuery || '')
  const [results, setResults] = useState<SearchResult[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [hybrid, setHybrid] = useState(true)
  const [rerank, setRerank] = useState(true)
  const [compress, setCompress] = useState(false)
  const [k, setK] = useState(4)
  const [recall, setRecall] = useState(40)
  const [stats, setStats] = useState<{ totalChunks: number; embedModel: string } | null>(null)
  const [metrics, setMetrics] = useState<MetricsResponse | null>(null)
  const [points, setPoints] = useState<PointInfo[]>([])
  const [reindexMsg, setReindexMsg] = useState('')
  const { events } = useEvents()

  useEffect(() => {
    if (sharedQuery && sharedQuery !== query) setQuery(sharedQuery)
  }, [sharedQuery]) // eslint-disable-line react-hooks/exhaustive-deps

  const handleQueryChange = useCallback((newQuery: string) => {
    setQuery(newQuery)
    onSharedQueryChange?.(newQuery)
  }, [onSharedQueryChange])

  const refreshStats = useCallback(() => {
    api.stats().then((s) => {
      setStats({ totalChunks: s.total_chunks, embedModel: s.embed_model })
      onProjectsUpdated?.(s.projects.map((p) => ({ projectID: p.project_id, chunkCount: p.chunk_count })))
    }).catch(() => setStats(null))
  }, [onProjectsUpdated])

  const refreshMetrics = useCallback(() => {
    api.metrics().then(setMetrics).catch(() => setMetrics(null))
  }, [])

  const refreshPoints = useCallback(() => {
    if (!activeProject) {
      setPoints([])
      return
    }
    api.listPoints(activeProject).then(setPoints).catch(() => setPoints([]))
  }, [activeProject])

  useEffect(() => {
    refreshStats()
    refreshMetrics()
    refreshPoints()
  }, [activeProject, refreshStats, refreshMetrics, refreshPoints])

  // Refresh derived data on relevant SSE events.
  useEffect(() => {
    if (events.length === 0) return
    const latest = events[0]
    if (['index_completed', 'project_deleted', 'points_deleted', 'documents_indexed'].includes(latest.type)) {
      refreshStats()
      refreshPoints()
    }
    if (latest.type === 'query_executed') {
      refreshMetrics()
    }
  }, [events, refreshStats, refreshPoints, refreshMetrics])

  const runSearch = useCallback(async () => {
    if (!activeProject || !query.trim()) return
    setLoading(true)
    setError('')
    try {
      const resp = await api.search({ project_id: activeProject, query, k, recall, hybrid, rerank, compress })
      setResults(resp.results)
      refreshMetrics()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Search failed')
      setResults([])
    } finally {
      setLoading(false)
    }
  }, [activeProject, query, k, recall, hybrid, rerank, compress, refreshMetrics])

  const handleReindex = useCallback(async () => {
    if (!activeProject) return
    const directory = window.prompt(
      `Directory to (re)index for "${activeProject}" (absolute path):`,
    )
    if (!directory) return
    setReindexMsg('Re-indexing…')
    try {
      const r = await api.reindex(activeProject, directory)
      setReindexMsg(`Indexed ${r.chunks_indexed} chunks (${r.files_scanned} files, ${r.skipped} unchanged, ${r.points_deleted} removed).`)
      refreshStats()
      refreshPoints()
    } catch (err) {
      setReindexMsg(`Re-index failed: ${err instanceof Error ? err.message : 'unknown error'}`)
    }
  }, [activeProject, refreshStats, refreshPoints])

  const fileAggs = aggregateFiles(points)
  const uniqueFiles = fileAggs.length
  const maxChunks = fileAggs.length > 0 ? fileAggs[0].count : 0

  // Activity rows from live SSE events (no synthetic fallback).
  const activityRows = events.slice(0, 8).map((ev) => {
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

  const comp = metrics?.last_query
  const hasComposition = !!comp && (comp.dense_count > 0 || comp.lexical_count > 0)

  return (
    <>
      {/* Page head */}
      <div className="page-head">
        <h1>Overview</h1>
        <span className="id mono">project_{activeProject || '—'}</span>
        <div className="head-actions">
          <button className="btn" onClick={handleReindex} disabled={!activeProject}>
            <RefreshCw size={14} strokeWidth={1.7} />
            Re-index
          </button>
          <button className="btn primary" onClick={() => onNavigateWithQuery ? onNavigateWithQuery('playground', query) : onNavigate('playground')}>
            <Search size={14} strokeWidth={1.7} />
            New query
          </button>
        </div>
      </div>
      {reindexMsg && <div className="reindex-msg">{reindexMsg}</div>}

      {/* KPIs */}
      <div className="kpis">
        <div className="kpi">
          <div className="label">Chunks</div>
          <div className="val tnum">{stats?.totalChunks ?? '—'}</div>
          <div className="sub">{uniqueFiles > 0 ? `across ${uniqueFiles} file${uniqueFiles === 1 ? '' : 's'}` : 'no files indexed'}</div>
        </div>
        <div className="kpi">
          <div className="label">Embedding</div>
          <div className="val mono" style={{ fontSize: '18px', marginTop: '11px' }}>
            {stats?.embedModel ?? '—'}
          </div>
          <div className="sub mono">{metrics?.backend ? `${metrics.backend}${metrics.persistent ? ' · persistent' : ''}` : ''}</div>
        </div>
        <div className="kpi">
          <div className="label">Avg. query latency</div>
          {metrics && metrics.query_count > 0 ? (
            <>
              <div className="val tnum">{Math.round(metrics.avg_latency_ms)}<small> ms</small></div>
              <div className="sub mono">p50 {Math.round(metrics.p50_latency_ms)} · p95 {Math.round(metrics.p95_latency_ms)} · {metrics.query_count} q</div>
            </>
          ) : (
            <><div className="val tnum">—</div><div className="sub">no queries yet</div></>
          )}
        </div>
        <div className="kpi">
          <div className="label">Tokens used</div>
          {metrics && metrics.tokens_total > 0 ? (
            <>
              <div className="val tnum">{formatTokens(metrics.tokens_total)}</div>
              <div className="sub mono">{formatTokens(metrics.tokens_embed)} embed · {formatTokens(metrics.tokens_rerank)} rerank</div>
            </>
          ) : (
            <><div className="val tnum">—</div><div className="sub">not tracked yet</div></>
          )}
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
              <button className="btn primary" style={{ padding: '9px 14px' }} onClick={runSearch} disabled={loading || !activeProject}>
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
              {results.length === 0 && !loading && !error && (
                <div className="results-empty">Run a query to see results.</div>
              )}
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
            <span className="hint mono" style={{ color: stats ? 'var(--good)' : 'var(--text-faint)' }}>
              {stats ? '● synced' : '○ no data'}
            </span>
          </div>
          <div className="panel-body" style={{ padding: '12px 15px' }}>
            <div className="stat-line"><span className="k">Files indexed</span><span className="v tnum">{uniqueFiles}</span></div>
            <div className="stat-line"><span className="k">Chunks indexed</span><span className="v tnum">{stats?.totalChunks ?? '—'}</span></div>
            <div className="stat-line"><span className="k">Backend</span><span className="v mono">{metrics?.backend ?? '—'}</span></div>
            <div className="stat-line"><span className="k">Persistence</span><span className="v">{metrics ? (metrics.persistent ? 'durable' : 'in-memory') : '—'}</span></div>
          </div>
        </section>

        {/* Retrieval breakdown */}
        <section className="panel g-breakdown">
          <div className="panel-head">
            <h2>Retrieval breakdown</h2>
            <span className="hint mono">last query</span>
          </div>
          <div className="panel-body" style={{ padding: '13px 15px' }}>
            {hasComposition ? (
              <>
                <div className="compo">
                  <i style={{ width: `${(comp!.dense_count / (comp!.dense_count + comp!.lexical_count)) * 100}%`, background: 'var(--accent)' }} />
                  <i style={{ width: `${(comp!.lexical_count / (comp!.dense_count + comp!.lexical_count)) * 100}%`, background: 'var(--good)' }} />
                </div>
                <div className="compo-legend">
                  <div className="lrow">
                    <span className="sw" style={{ background: 'var(--accent)' }} />
                    Dense (vector)
                    <span className="lval">{comp!.dense_count}</span>
                  </div>
                  <div className="lrow">
                    <span className="sw" style={{ background: 'var(--good)' }} />
                    Lexical (tsvector)
                    <span className="lval">{comp!.lexical_count}</span>
                  </div>
                  {comp!.reranked && (
                    <div className="lrow">
                      <span className="sw" style={{ background: 'var(--border-strong)' }} />
                      Reranker moved
                      <span className="lval">{comp!.rerank_moved}</span>
                    </div>
                  )}
                </div>
              </>
            ) : (
              <div className="panel-empty">
                {comp
                  ? 'Dense-only retrieval — enable hybrid on a pgvector backend for a dense/lexical breakdown.'
                  : 'No query yet.'}
              </div>
            )}
          </div>
        </section>

        {/* Chunk distribution */}
        <section className="panel g-dist">
          <div className="panel-head">
            <h2>Chunk distribution</h2>
            <span className="hint">chunks per file</span>
          </div>
          <div className="panel-body" style={{ padding: '14px 15px' }}>
            {fileAggs.length === 0 ? (
              <div className="panel-empty">No chunks indexed for this project.</div>
            ) : (
              <div className="dist" style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '9px 24px' }}>
                {fileAggs.slice(0, 8).map((f) => (
                  <div className="dist-row" key={f.name}>
                    <span className="dname">{f.name}</span>
                    <span className="dcount">{f.count}</span>
                    <span className="dist-bar">
                      <i style={{ width: `${maxChunks > 0 ? (f.count / maxChunks) * 100 : 0}%` }} />
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>
        </section>

        {/* Recent files */}
        <section className="panel g-files">
          <div className="panel-head">
            <h2>Files</h2>
          </div>
          <div className="panel-body" style={{ padding: '6px 15px 12px' }}>
            {fileAggs.length === 0 ? (
              <div className="panel-empty">—</div>
            ) : (
              <div className="files">
                {fileAggs.slice(0, 8).map((f) => (
                  <div className="file-row" key={f.name}>
                    <span className="status-dot" style={{ background: 'var(--good)' }} />
                    <span className="fname">{f.name}</span>
                    <span className="fmeta">{f.count} ch</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        </section>

        {/* Activity */}
        <section className="panel g-activity">
          <div className="panel-head">
            <h2>Activity</h2>
            <span className="hint">live</span>
          </div>
          <div className="panel-body" style={{ padding: '12px 15px' }}>
            {activityRows.length === 0 ? (
              <div className="panel-empty">No activity yet — index a project or run a query.</div>
            ) : (
              <div className="log" style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '7px 24px' }}>
                {activityRows.map((row, i) => (
                  <div className="row" key={i}>
                    <span className="t">{row.time}</span>
                    <span className={row.isOk ? 'ok' : ''}>{row.label}</span>
                    <span>{row.desc}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        </section>
      </div>
    </>
  )
}
