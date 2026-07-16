import { ChevronLeft, ChevronRight, Check, Database } from 'lucide-react'
import type { DraftConfig, VectorStoreProvider } from './types'

interface StepVectorStoreProps {
  cfg: DraftConfig
  updateCfg: (patch: Partial<DraftConfig>) => void
  onBack: () => void
  onNext: () => void
}

const providers: {
  id: VectorStoreProvider
  name: string
  desc: string
  meta: string
}[] = [
  {
    id: 'pgvector',
    name: 'pgvector',
    desc: 'PostgreSQL extension for vector similarity search. Supports hybrid search via tsvector + RRF fusion.',
    meta: 'hybrid · GIN index · tsvector',
  },
  {
    id: 'qdrant',
    name: 'Qdrant',
    desc: 'Dedicated vector search engine with high performance and rich filtering capabilities.',
    meta: 'payload filter · HNSW',
  },
  {
    id: 'chroma',
    name: 'Chroma',
    desc: 'Lightweight, open-source embedding database. Simple setup for development and testing.',
    meta: 'where filter · collection',
  },
]

export function StepVectorStore({ cfg, updateCfg, onBack, onNext }: StepVectorStoreProps) {
  const canProceed = cfg.vectorStore !== ''

  const getEndpointLabel = () => {
    switch (cfg.vectorStore) {
      case 'pgvector': return 'Connection string / DSN'
      case 'qdrant': return 'Qdrant URL'
      case 'chroma': return 'Chroma URL'
      default: return 'Endpoint'
    }
  }

  const getEndpointValue = () => {
    switch (cfg.vectorStore) {
      case 'pgvector': return cfg.pgvectorDSN
      case 'qdrant': return cfg.qdrantURL
      case 'chroma': return cfg.chromaURL
      default: return ''
    }
  }

  const setEndpointValue = (val: string) => {
    switch (cfg.vectorStore) {
      case 'pgvector': updateCfg({ pgvectorDSN: val }); break
      case 'qdrant': updateCfg({ qdrantURL: val }); break
      case 'chroma': updateCfg({ chromaURL: val }); break
    }
  }

  return (
    <div className="card">
      <div className="card-head">
        <h2>Choose a Vector Store</h2>
        <span className="step-badge mono">2 / 7</span>
      </div>
      <div className="card-body">
        <p>Select the database that will store your vector embeddings. Each option supports full semantic search and metadata filtering.</p>

        <div className="cards cards-3">
          {providers.map((p) => (
            <div
              key={p.id}
              className={`pcard ${cfg.vectorStore === p.id ? 'selected' : ''}`}
              onClick={() => updateCfg({ vectorStore: p.id })}
            >
              {cfg.vectorStore === p.id && <Check className="pcard-check" size={16} />}
              <div className="pcard-icon">
                <Database size={18} strokeWidth={1.5} />
              </div>
              <div className="pname">{p.name}</div>
              <div className="pdesc">{p.desc}</div>
              <div className="pmeta mono">{p.meta}</div>
            </div>
          ))}
        </div>

        {cfg.vectorStore && (
          <>
            <div className="field" style={{ marginBottom: 0 }}>
              <label>{getEndpointLabel()}</label>
              <input
                className="input mono"
                type="text"
                value={getEndpointValue()}
                onChange={(e) => setEndpointValue(e.target.value)}
                placeholder="Enter endpoint or DSN"
              />
            </div>

            {cfg.vectorStore === 'qdrant' && (
              <div className="field" style={{ marginTop: 14, marginBottom: 0 }}>
                <label>Qdrant API Key (optional, for cloud)</label>
                <input
                  className="input mono"
                  type="password"
                  value={cfg.qdrantAPIKey}
                  onChange={(e) => updateCfg({ qdrantAPIKey: e.target.value })}
                  placeholder="Enter API key if using Qdrant Cloud"
                />
              </div>
            )}
          </>
        )}
      </div>
      <div className="nav-buttons">
        <button className="btn" onClick={onBack}>
          <ChevronLeft size={14} /> Back
        </button>
        <div className="spacer" />
        <button className="btn primary" onClick={onNext} disabled={!canProceed}>
          Next <ChevronRight size={14} />
        </button>
      </div>
    </div>
  )
}
