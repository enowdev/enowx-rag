import { useEffect, useMemo, useState } from 'react'
import { Play, CheckCircle2, XCircle, Trash2, Key, Eye, EyeOff } from 'lucide-react'
import type { ProjectInfo } from '../App'
import { api, type MigrateRequest } from '../lib/api'
import { useEvents } from '../lib/sse'

interface MigrationProps {
  activeProject: string
  projects: ProjectInfo[]
}

type Store = 'qdrant' | 'pgvector' | 'chroma'
type Embedder = 'voyage' | 'openai' | 'tei'

export function Migration({ activeProject, projects }: MigrationProps) {
  const [source, setSource] = useState(activeProject || '')
  const [dest, setDest] = useState('')
  const [store, setStore] = useState<Store>('qdrant')
  const [embedder, setEmbedder] = useState<Embedder>('voyage')
  const [revealKey, setRevealKey] = useState(false)
  const [running, setRunning] = useState(false)
  const [error, setError] = useState('')
  const [finished, setFinished] = useState<'ok' | 'fail' | null>(null)
  const [deleteMsg, setDeleteMsg] = useState('')

  // Target connection/embedder fields (defaults mirror the wizard).
  const [qdrantURL, setQdrantURL] = useState('http://localhost:6333')
  const [qdrantKey, setQdrantKey] = useState('')
  const [chromaURL, setChromaURL] = useState('http://localhost:8000')
  const [pgDSN, setPgDSN] = useState('postgresql://enowdev@localhost:5432/enowxrag')
  const [pgTable, setPgTable] = useState('project_memory')
  const [voyageKey, setVoyageKey] = useState('')
  const [voyageModel, setVoyageModel] = useState('voyage-4')
  const [voyageDim, setVoyageDim] = useState(1024)
  const [openaiKey, setOpenaiKey] = useState('')
  const [openaiModel, setOpenaiModel] = useState('text-embedding-3-small')
  const [openaiBase, setOpenaiBase] = useState('https://api.openai.com/v1')
  const [openaiDim, setOpenaiDim] = useState(0)
  const [teiURL, setTeiURL] = useState('http://localhost:8081')

  const { events } = useEvents()

  useEffect(() => {
    if (activeProject && !source) setSource(activeProject)
  }, [activeProject]) // eslint-disable-line react-hooks/exhaustive-deps

  // Default destination name once a source is chosen.
  useEffect(() => {
    if (source && !dest) {
      const tag = embedder === 'voyage' ? voyageModel : embedder === 'openai' ? openaiModel.replace(/[^a-z0-9]+/gi, '-') : 'tei'
      setDest(`${source}-${tag}`)
    }
  }, [source]) // eslint-disable-line react-hooks/exhaustive-deps

  // Live progress from SSE.
  const progress = useMemo(() => {
    const p = events.find((e) => e.type === 'migration_progress')
    return p?.data ? (p.data as { percent: number; done: number; total: number }) : null
  }, [events])

  useEffect(() => {
    if (events.length === 0) return
    const latest = events[0]
    if (latest.type === 'migration_completed') {
      setRunning(false)
      setFinished('ok')
    } else if (latest.type === 'migration_failed') {
      setRunning(false)
      setFinished('fail')
      setError((latest.data as { error?: string })?.error || 'Migration failed')
    }
  }, [events])

  const runMigration = async () => {
    if (!source || !dest) return
    setError('')
    setFinished(null)
    setDeleteMsg('')
    setRunning(true)
    const req: MigrateRequest = {
      source_project: source,
      dest_project: dest,
      vector_store: store,
      embedder,
      qdrant_url: store === 'qdrant' ? qdrantURL : undefined,
      qdrant_api_key: store === 'qdrant' && qdrantKey ? qdrantKey : undefined,
      chroma_url: store === 'chroma' ? chromaURL : undefined,
      pgvector_dsn: store === 'pgvector' ? pgDSN : undefined,
      pgvector_table: store === 'pgvector' ? pgTable : undefined,
      voyage_api_key: embedder === 'voyage' ? voyageKey : undefined,
      voyage_model: embedder === 'voyage' ? voyageModel : undefined,
      voyage_dim: embedder === 'voyage' ? voyageDim : undefined,
      openai_api_key: embedder === 'openai' ? openaiKey : undefined,
      openai_model: embedder === 'openai' ? openaiModel : undefined,
      openai_base_url: embedder === 'openai' ? openaiBase : undefined,
      openai_dim: embedder === 'openai' ? openaiDim : undefined,
      tei_url: embedder === 'tei' ? teiURL : undefined,
    }
    try {
      await api.migrate(req)
      // 202: progress + completion arrive over SSE.
    } catch (e) {
      setRunning(false)
      setError(e instanceof Error ? e.message : 'Failed to start migration')
    }
  }

  const deleteSource = async () => {
    if (!window.confirm(`Delete the source project "${source}"? This cannot be undone.`)) return
    try {
      await api.deleteProject(source)
      setDeleteMsg(`Source project "${source}" deleted.`)
    } catch (e) {
      setDeleteMsg(`Delete failed: ${e instanceof Error ? e.message : 'unknown error'}`)
    }
  }

  const pct = progress?.percent ?? (finished === 'ok' ? 100 : 0)

  return (
    <>
      <div className="page-head">
        <h1>Migration</h1>
        <span className="id mono">re-embed · move · change dimension</span>
      </div>

      <div className="cols" style={{ gridTemplateColumns: '1fr 1fr' }}>
        {/* Source + target config */}
        <section className="panel">
          <div className="panel-head"><h2>Migrate a project</h2></div>
          <div className="panel-body">
            <p style={{ color: 'var(--text-dim)', fontSize: 12.5, marginTop: 0 }}>
              Re-embeds a project's stored text into a new destination — use it to change embedding
              model/dimension or move to another vector store. Raw vectors aren't copied (they're
              model-specific); the text is re-embedded by the destination.
            </p>

            <div className="field">
              <label>Source project</label>
              <select className="select-box mono" value={source} onChange={(e) => setSource(e.target.value)}>
                <option value="">— select —</option>
                {projects.map((p) => (
                  <option key={p.projectID} value={p.projectID}>{p.projectID} ({p.chunkCount})</option>
                ))}
              </select>
            </div>

            <div className="field">
              <label>Destination project name</label>
              <input className="input mono" value={dest} onChange={(e) => setDest(e.target.value)} placeholder="e.g. myproject-voyage" />
            </div>

            <div className="field-row">
              <div className="field">
                <label>Destination vector store</label>
                <select className="select-box mono" value={store} onChange={(e) => setStore(e.target.value as Store)}>
                  <option value="qdrant">qdrant</option>
                  <option value="pgvector">pgvector</option>
                  <option value="chroma">chroma</option>
                </select>
              </div>
              <div className="field">
                <label>Destination embedder</label>
                <select className="select-box mono" value={embedder} onChange={(e) => setEmbedder(e.target.value as Embedder)}>
                  <option value="voyage">voyage</option>
                  <option value="openai">openai-compatible</option>
                  <option value="tei">tei</option>
                </select>
              </div>
            </div>

            {/* Store fields */}
            {store === 'qdrant' && (
              <>
                <div className="field"><label>Qdrant URL</label>
                  <input className="input mono" value={qdrantURL} onChange={(e) => setQdrantURL(e.target.value)} /></div>
                <div className="field"><label>Qdrant API key (empty for local)</label>
                  <input className="input mono" value={qdrantKey} onChange={(e) => setQdrantKey(e.target.value)} placeholder="for Qdrant Cloud" /></div>
              </>
            )}
            {store === 'chroma' && (
              <div className="field"><label>Chroma URL</label>
                <input className="input mono" value={chromaURL} onChange={(e) => setChromaURL(e.target.value)} /></div>
            )}
            {store === 'pgvector' && (
              <>
                <div className="field"><label>pgvector DSN</label>
                  <input className="input mono" value={pgDSN} onChange={(e) => setPgDSN(e.target.value)} /></div>
                <div className="field"><label>Table (use a new name to change dimension)</label>
                  <input className="input mono" value={pgTable} onChange={(e) => setPgTable(e.target.value)} />
                  <div className="field-hint">pgvector stores all projects in one fixed-dimension table. To migrate to a different dimension, write to a NEW table name.</div>
                </div>
              </>
            )}

            {/* Embedder fields */}
            {embedder === 'voyage' && (
              <>
                <div className="field"><label>Voyage API key</label>
                  <div className="input-wrapper">
                    <Key size={15} className="input-icon" />
                    <input className="input mono with-icon" type={revealKey ? 'text' : 'password'} value={voyageKey} onChange={(e) => setVoyageKey(e.target.value)} placeholder="pa-…" />
                    <button className="reveal-btn" onClick={() => setRevealKey(!revealKey)}>{revealKey ? <EyeOff size={16} /> : <Eye size={16} />}</button>
                  </div>
                </div>
                <div className="field-row">
                  <div className="field"><label>Model</label><input className="input mono" value={voyageModel} onChange={(e) => setVoyageModel(e.target.value)} /></div>
                  <div className="field"><label>Dimension</label><input className="input mono" type="number" value={voyageDim} onChange={(e) => setVoyageDim(parseInt(e.target.value) || 1024)} /></div>
                </div>
              </>
            )}
            {embedder === 'openai' && (
              <>
                <div className="field"><label>Base URL</label><input className="input mono" value={openaiBase} onChange={(e) => setOpenaiBase(e.target.value)} /></div>
                <div className="field"><label>API key (empty for local)</label>
                  <div className="input-wrapper">
                    <Key size={15} className="input-icon" />
                    <input className="input mono with-icon" type={revealKey ? 'text' : 'password'} value={openaiKey} onChange={(e) => setOpenaiKey(e.target.value)} placeholder="sk-…" />
                    <button className="reveal-btn" onClick={() => setRevealKey(!revealKey)}>{revealKey ? <EyeOff size={16} /> : <Eye size={16} />}</button>
                  </div>
                </div>
                <div className="field-row">
                  <div className="field"><label>Model</label><input className="input mono" value={openaiModel} onChange={(e) => setOpenaiModel(e.target.value)} /></div>
                  <div className="field"><label>Dimension (0=auto)</label><input className="input mono" type="number" value={openaiDim} onChange={(e) => setOpenaiDim(parseInt(e.target.value) || 0)} /></div>
                </div>
              </>
            )}
            {embedder === 'tei' && (
              <div className="field"><label>TEI URL</label><input className="input mono" value={teiURL} onChange={(e) => setTeiURL(e.target.value)} /></div>
            )}

            <button className="btn primary" onClick={runMigration} disabled={running || !source || !dest} style={{ marginTop: 8 }}>
              <Play size={14} /> {running ? 'Migrating…' : 'Start migration'}
            </button>
            {error && <div className="error-state" style={{ marginTop: 12 }}>{error}</div>}
          </div>
        </section>

        {/* Progress + result */}
        <section className="panel">
          <div className="panel-head"><h2>Progress</h2><span className="hint mono">live</span></div>
          <div className="panel-body">
            {!running && finished === null && (
              <div className="panel-empty">Configure a migration and press Start.</div>
            )}
            {(running || finished) && (
              <>
                <div className="stat-line"><span className="k">Source</span><span className="v mono">{source}</span></div>
                <div className="stat-line"><span className="k">Destination</span><span className="v mono">{dest}</span></div>
                {progress && (
                  <div className="stat-line"><span className="k">Documents</span><span className="v tnum">{progress.done} / {progress.total}</span></div>
                )}
                <div style={{ marginTop: 14 }}>
                  <span className="bar" style={{ display: 'block', height: 8 }}>
                    <i style={{ width: `${pct}%`, background: finished === 'fail' ? 'var(--crit)' : 'var(--accent)' }} />
                  </span>
                  <div className="sub mono" style={{ marginTop: 6 }}>{pct}%</div>
                </div>

                {finished === 'ok' && (
                  <>
                    <div className="success-box" style={{ marginTop: 14 }}>
                      <CheckCircle2 size={16} />
                      <span>Migration complete. Verify "{dest}" in the Playground, then optionally remove the source.</span>
                    </div>
                    <button className="btn" onClick={deleteSource} style={{ marginTop: 12 }}>
                      <Trash2 size={14} /> Delete source project "{source}"
                    </button>
                    {deleteMsg && <div className="reindex-msg" style={{ marginTop: 10 }}>{deleteMsg}</div>}
                  </>
                )}
                {finished === 'fail' && (
                  <div className="test-error" style={{ marginTop: 14 }}>
                    <XCircle size={16} /><span>{error || 'Migration failed'}</span>
                  </div>
                )}
              </>
            )}
          </div>
        </section>
      </div>
    </>
  )
}
