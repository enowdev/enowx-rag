import { useState, useCallback, useEffect } from 'react'
import { Search, RotateCcw, Inbox, AlertCircle, Radio } from 'lucide-react'
import { api, type SearchResult } from '../lib/api'
import { useEvents } from '../lib/sse'

interface PlaygroundProps {
  activeProject: string
  sharedQuery?: string
  onSharedQueryChange?: (q: string) => void
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

export function Playground({ activeProject, sharedQuery, onSharedQueryChange }: PlaygroundProps) {
  const [query, setQuery] = useState(sharedQuery || '')
  const [results, setResults] = useState<SearchResult[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [hasSearched, setHasSearched] = useState(false)
  const [hybrid, setHybrid] = useState(false)
  const [rerank, setRerank] = useState(false)
  const [compress, setCompress] = useState(false)
  const [k, setK] = useState(5)
  const [recall, setRecall] = useState(40)
  const { events, connected } = useEvents()

  // Sync local query when sharedQuery changes (e.g., arriving from Overview)
  useEffect(() => {
    if (sharedQuery !== undefined && sharedQuery !== query) {
      setQuery(sharedQuery)
    }
  }, [sharedQuery]) // eslint-disable-line react-hooks/exhaustive-deps

  // Propagate query changes up to the shared state
  const handleQueryChange = useCallback((newQuery: string) => {
    setQuery(newQuery)
    onSharedQueryChange?.(newQuery)
  }, [onSharedQueryChange])

  const runSearch = useCallback(async () => {
    if (!activeProject || !query.trim()) return
    setLoading(true)
    setError('')
    setHasSearched(true)
    try {
      const resp = await api.search({
        project_id: activeProject,
        query,
        k,
        recall,
        hybrid,
        rerank,
        compress,
      })
      setResults(resp.results)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Search failed')
      setResults([])
    } finally {
      setLoading(false)
    }
  }, [activeProject, query, k, recall, hybrid, rerank, compress])

  // Format activity events for display
  const activityRows = events.slice(0, 8).map((ev) => {
    const time = new Date(ev.timestamp).toLocaleTimeString('en-US', { hour12: false })
    const isOk = ev.type === 'index_completed' || ev.type === 'query_executed' || ev.type === 'documents_indexed'
    const label = ev.type.replace(/_/g, ' ')
    let desc = ''
    if (ev.data) {
      const d = ev.data as Record<string, unknown>
      if (d.indexed !== undefined) desc = `${d.indexed} chunks · ${d.deleted || 0} deleted`
      else if (d.candidates !== undefined) desc = `${d.candidates} candidates`
      else if (d.directory) desc = String(d.directory)
      else if (d.count !== undefined) desc = `${d.count} points`
      else if (d.project_id) desc = String(d.project_id)
    }
    return { time, label, desc, isOk }
  })

  return (
    <>
      <div className="page-head">
        <h1>Playground</h1>
        <span className="id mono">project_{activeProject || '—'}</span>
      </div>

      <div className="cols pg-fill" style={{ gridTemplateColumns: '2fr 1fr' }}>
        {/* Main playground panel */}
        <section className="panel pg-panel" style={{ gridColumn: '1' }}>
          <div className="panel-head">
            <h2>Retrieval playground</h2>
            <span className="hint">top-{k} · {rerank ? 'reranked' : 'semantic'}</span>
          </div>
          <div className="panel-body pg-body">
            <div className="query-row">
              <input
                className="query-input"
                type="text"
                value={query}
                onChange={(e) => handleQueryChange(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && runSearch()}
                placeholder="Enter a retrieval query…"
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

            {error && (
              <div className="error-state" style={{ marginTop: '12px', display: 'flex', alignItems: 'center', gap: '8px' }}>
                <AlertCircle size={16} />
                {error}
              </div>
            )}

            {!hasSearched && !error && (
              <div className="empty-state" style={{ marginTop: '16px' }}>
                <Inbox size={28} />
                Run a query to see results
              </div>
            )}

            {hasSearched && results.length === 0 && !error && !loading && (
              <div className="empty-state" style={{ marginTop: '16px' }}>
                <AlertCircle size={28} />
                No results found
              </div>
            )}

            {results.length > 0 && (
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
            )}
          </div>
        </section>

        {/* Activity panel (SSE realtime) */}
        <section className="panel pg-panel" style={{ gridColumn: '2' }}>
          <div className="panel-head">
            <h2>Activity</h2>
            <span className="hint" style={{ display: 'flex', alignItems: 'center', gap: '5px' }}>
              <Radio size={11} style={{ color: connected ? 'var(--good)' : 'var(--text-faint)' }} />
              {connected ? 'live' : 'connecting'}
            </span>
          </div>
          <div className="panel-body pg-activity-body" style={{ padding: '12px 15px' }}>
            {activityRows.length === 0 ? (
              <div style={{ color: 'var(--text-faint)', fontSize: '12px', padding: '12px 0' }}>
                Waiting for events…
              </div>
            ) : (
              <div className="log" style={{ display: 'flex', flexDirection: 'column', gap: '7px' }}>
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
