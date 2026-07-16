import { useState, useEffect, useCallback } from 'react'
import { Inbox, Trash2, ChevronLeft, ChevronRight, Filter } from 'lucide-react'
import { api, type PointInfo } from '../lib/api'

interface ChunksProps {
  activeProject: string
}

export function Chunks({ activeProject }: ChunksProps) {
  const [points, setPoints] = useState<PointInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState('')
  const [appliedFilter, setAppliedFilter] = useState('')
  const [offset, setOffset] = useState(0)
  const [deletingId, setDeletingId] = useState<string | null>(null)
  const limit = 20

  const fetchPoints = useCallback(async () => {
    if (!activeProject) return
    setLoading(true)
    try {
      const data = await api.listPoints(activeProject, {
        source_file: appliedFilter || undefined,
        offset,
        limit,
      })
      setPoints(data)
    } catch {
      setPoints([])
    } finally {
      setLoading(false)
    }
  }, [activeProject, appliedFilter, offset])

  useEffect(() => {
    fetchPoints()
  }, [fetchPoints])

  const handleDelete = useCallback(async (pointId: string) => {
    if (!activeProject) return
    setDeletingId(pointId)
    try {
      await api.deletePoint(activeProject, pointId)
      setPoints((prev) => prev.filter((p) => p.id !== pointId))
    } catch {
      // Re-fetch on error to restore state
      fetchPoints()
    } finally {
      setDeletingId(null)
    }
  }, [activeProject, fetchPoints])

  const applyFilter = useCallback(() => {
    setAppliedFilter(filter)
    setOffset(0)
  }, [filter])

  const clearFilter = useCallback(() => {
    setFilter('')
    setAppliedFilter('')
    setOffset(0)
  }, [])

  return (
    <>
      <div className="page-head">
        <h1>Chunks</h1>
        <span className="id mono">project_{activeProject || '—'}</span>
      </div>

      <section className="panel">
        <div className="panel-head">
          <h2>All chunks</h2>
          <span className="hint">
            {appliedFilter ? `filtered: ${appliedFilter}` : `${points.length} shown`}
          </span>
        </div>
        <div className="panel-body">
          {/* Filter bar */}
          <div style={{ display: 'flex', gap: '9px', marginBottom: '12px', alignItems: 'center' }}>
            <Filter size={14} style={{ color: 'var(--text-faint)', flex: 'none' }} />
            <input
              className="query-input"
              type="text"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && applyFilter()}
              placeholder="Filter by source file…"
              style={{ flex: '1' }}
            />
            {filter !== appliedFilter && (
              <button className="btn" onClick={applyFilter} style={{ padding: '7px 12px' }}>
                Apply
              </button>
            )}
            {appliedFilter && (
              <button className="btn" onClick={clearFilter} style={{ padding: '7px 12px' }}>
                Clear
              </button>
            )}
          </div>

          {loading && <div style={{ color: 'var(--text-faint)', fontSize: '13px' }}>Loading…</div>}

          {!loading && points.length === 0 && (
            <div className="empty-state">
              <Inbox size={28} />
              No chunks found
            </div>
          )}

          {points.length > 0 && (
            <div className="chunk-list">
              {points.map((p) => (
                <div className="chunk-row" key={p.id}>
                  <div className="chunk-info">
                    <div className="chunk-header">
                      <span className="fname mono">{p.source_file || p.id}</span>
                      {p.chunk_index && (
                        <span className="chunk-idx mono">chunk {p.chunk_index}</span>
                      )}
                      <span className="status-dot" style={{ background: 'var(--good)' }} />
                    </div>
                    {p.content && (
                      <div className="chunk-preview mono">{p.content}</div>
                    )}
                    <div className="chunk-meta">
                      {p.content_hash && (
                        <span className="tag mono">hash: {p.content_hash}</span>
                      )}
                      {p.chunk_version && (
                        <span className="tag mono">{p.chunk_version}</span>
                      )}
                      <span className="tag mono">{p.id}</span>
                    </div>
                  </div>
                  <button
                    className="btn chunk-delete"
                    onClick={() => handleDelete(p.id)}
                    disabled={deletingId === p.id}
                    title="Delete chunk"
                    style={{ padding: '6px 8px', flex: 'none' }}
                  >
                    <Trash2 size={13} strokeWidth={1.7} />
                  </button>
                </div>
              ))}
            </div>
          )}

          {/* Pagination */}
          {points.length > 0 && (
            <div className="pagination">
              <button
                className="btn"
                disabled={offset === 0}
                onClick={() => setOffset(Math.max(0, offset - limit))}
              >
                <ChevronLeft size={14} strokeWidth={1.7} />
                Previous
              </button>
              <span className="mono page-info">
                {offset + 1}–{offset + points.length}
              </span>
              <button
                className="btn"
                disabled={points.length < limit}
                onClick={() => setOffset(offset + limit)}
              >
                Next
                <ChevronRight size={14} strokeWidth={1.7} />
              </button>
            </div>
          )}
        </div>
      </section>
    </>
  )
}
