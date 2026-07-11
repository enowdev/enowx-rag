import { useState, useCallback } from 'react'
import { Search, RotateCcw, Inbox, AlertCircle } from 'lucide-react'
import { api, type SearchResult } from '../lib/api'

interface PlaygroundProps {
  activeProject: string
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

export function Playground({ activeProject }: PlaygroundProps) {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<SearchResult[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [hasSearched, setHasSearched] = useState(false)
  const [hybrid, setHybrid] = useState(false)
  const [rerank, setRerank] = useState(false)
  const [compress, setCompress] = useState(false)
  const [k, setK] = useState(5)
  const [recall, setRecall] = useState(40)

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
      })
      setResults(resp.results)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Search failed')
      setResults([])
    } finally {
      setLoading(false)
    }
  }, [activeProject, query, k, recall, hybrid, rerank])

  return (
    <>
      <div className="page-head">
        <h1>Playground</h1>
        <span className="id mono">project_{activeProject || '—'}</span>
      </div>

      <section className="panel">
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
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && runSearch()}
              placeholder="Enter a retrieval query…"
            />
            <button className="btn primary" style={{ padding: '9px 14px' }} onClick={runSearch} disabled={loading}>
              {loading ? <RotateCcw size={14} strokeWidth={1.7} /> : <Search size={14} strokeWidth={1.7} />}
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

          {!hasSearched && !error && (
            <div className="empty-state" style={{ marginTop: '16px' }}>
              <Inbox size={28} />
              Run a query to see results
            </div>
          )}

          {hasSearched && results.length === 0 && !error && (
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
    </>
  )
}
