import { useState, useEffect, useCallback } from 'react'
import { Inbox } from 'lucide-react'
import { api, type PointInfo } from '../lib/api'

interface ChunksProps {
  activeProject: string
}

export function Chunks({ activeProject }: ChunksProps) {
  const [points, setPoints] = useState<PointInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState('')
  const [offset, setOffset] = useState(0)
  const limit = 20

  const fetchPoints = useCallback(async () => {
    if (!activeProject) return
    setLoading(true)
    try {
      const data = await api.listPoints(activeProject, {
        source_file: filter || undefined,
        offset,
        limit,
      })
      setPoints(data)
    } catch {
      setPoints([])
    } finally {
      setLoading(false)
    }
  }, [activeProject, filter, offset])

  useEffect(() => {
    fetchPoints()
  }, [fetchPoints])

  return (
    <>
      <div className="page-head">
        <h1>Chunks</h1>
        <span className="id mono">project_{activeProject || '—'}</span>
      </div>

      <section className="panel">
        <div className="panel-head">
          <h2>All chunks</h2>
          <span className="hint">{points.length} shown</span>
        </div>
        <div className="panel-body">
          <div style={{ display: 'flex', gap: '9px', marginBottom: '12px' }}>
            <input
              className="query-input"
              type="text"
              value={filter}
              onChange={(e) => { setFilter(e.target.value); setOffset(0) }}
              placeholder="Filter by source file…"
            />
          </div>

          {loading && <div style={{ color: 'var(--text-faint)', fontSize: '13px' }}>Loading…</div>}

          {!loading && points.length === 0 && (
            <div className="empty-state">
              <Inbox size={28} />
              No chunks found
            </div>
          )}

          {points.length > 0 && (
            <div className="files">
              {points.map((p) => (
                <div className="file-row" key={p.id}>
                  <span className="status-dot" style={{ background: 'var(--good)' }} />
                  <span className="fname">{p.source_file || p.id}</span>
                  <span className="fmeta">{p.chunk_version || 'v2'}</span>
                </div>
              ))}
            </div>
          )}

          {points.length > 0 && (
            <div style={{ display: 'flex', gap: '8px', marginTop: '12px', justifyContent: 'center' }}>
              <button
                className="btn"
                disabled={offset === 0}
                onClick={() => setOffset(Math.max(0, offset - limit))}
              >
                Previous
              </button>
              <span className="mono" style={{ color: 'var(--text-faint)', fontSize: '12px', alignSelf: 'center' }}>
                {offset + 1}–{offset + points.length}
              </span>
              <button
                className="btn"
                disabled={points.length < limit}
                onClick={() => setOffset(offset + limit)}
              >
                Next
              </button>
            </div>
          )}
        </div>
      </section>
    </>
  )
}
