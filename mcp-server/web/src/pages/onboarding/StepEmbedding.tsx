import { useState } from 'react'
import { ChevronLeft, ChevronRight, Check, Eye, EyeOff, Key, AlertTriangle } from 'lucide-react'
import type { DraftConfig, EmbedderProvider } from './types'

interface StepEmbeddingProps {
  cfg: DraftConfig
  updateCfg: (patch: Partial<DraftConfig>) => void
  onBack: () => void
  onNext: () => void
}

const embedders: {
  id: EmbedderProvider
  name: string
  desc: string
  meta: string
}[] = [
  {
    id: 'voyage',
    name: 'Voyage AI',
    desc: 'Cloud API with state-of-the-art embeddings. voyage-4 model offers 200M free tokens.',
    meta: 'cloud · 1024-dim · rerank-2.5',
  },
  {
    id: 'tei',
    name: 'TEI',
    desc: 'Text Embeddings Inference server. Self-hosted, runs locally via Docker for zero-cost inference.',
    meta: 'self-hosted · Docker · :8081',
  },
]

export function StepEmbedding({ cfg, updateCfg, onBack, onNext }: StepEmbeddingProps) {
  const [revealKey, setRevealKey] = useState(false)
  const canProceed = cfg.embedder !== '' &&
    (cfg.embedder === 'tei' || cfg.embedder === 'voyage')

  return (
    <div className="card">
      <div className="card-head">
        <h2>Choose an Embedding Provider</h2>
        <span className="step-badge mono">3 / 6</span>
      </div>
      <div className="card-body">
        <p>Select the service that will generate vector embeddings from your text. Voyage AI is recommended for production quality.</p>

        <div className="cards cards-2">
          {embedders.map((p) => (
            <div
              key={p.id}
              className={`pcard ${cfg.embedder === p.id ? 'selected' : ''}`}
              onClick={() => updateCfg({ embedder: p.id })}
            >
              {cfg.embedder === p.id && <Check className="pcard-check" size={16} />}
              <div className="pname">{p.name}</div>
              <div className="pdesc">{p.desc}</div>
              <div className="pmeta mono">{p.meta}</div>
            </div>
          ))}
        </div>

        {cfg.embedder === 'voyage' && (
          <>
            <div className="field">
              <label>API Key</label>
              <div className="input-wrapper">
                <Key size={15} strokeWidth={1.5} className="input-icon" />
                <input
                  className="input mono with-icon"
                  type={revealKey ? 'text' : 'password'}
                  value={cfg.voyageAPIKey}
                  onChange={(e) => updateCfg({ voyageAPIKey: e.target.value })}
                  placeholder="Enter your Voyage AI API key"
                />
                <button
                  className="reveal-btn"
                  onClick={() => setRevealKey(!revealKey)}
                  title={revealKey ? 'Hide' : 'Reveal'}
                >
                  {revealKey ? <EyeOff size={16} /> : <Eye size={16} />}
                </button>
              </div>
            </div>

            <div className="field-row">
              <div className="field">
                <label>Model</label>
                <select
                  className="select-box mono"
                  value={cfg.voyageModel}
                  onChange={(e) => updateCfg({ voyageModel: e.target.value })}
                >
                  <option value="voyage-4">voyage-4</option>
                  <option value="voyage-3">voyage-3</option>
                  <option value="voyage-3-lite">voyage-3-lite</option>
                  <option value="voyage-2">voyage-2</option>
                </select>
              </div>
              <div className="field">
                <label>Dimension</label>
                <input
                  className="input mono"
                  type="number"
                  value={cfg.voyageDim}
                  onChange={(e) => updateCfg({ voyageDim: parseInt(e.target.value) || 1024 })}
                  min={256}
                  max={4096}
                  step={256}
                />
              </div>
            </div>

            <div className="warn-box">
              <AlertTriangle size={16} className="warn-icon" />
              <div className="warn-text">
                <b>Re-index required.</b> Changing the embedding model or dimension after data has been
                indexed will require a full re-index. Existing vectors with a different model/dimension
                cannot be mixed in the same collection.
              </div>
            </div>
          </>
        )}

        {cfg.embedder === 'tei' && (
          <>
            <div className="field" style={{ marginBottom: 0 }}>
              <label>TEI Server URL</label>
              <input
                className="input mono"
                type="text"
                value={cfg.teiURL}
                onChange={(e) => updateCfg({ teiURL: e.target.value })}
                placeholder="http://localhost:8081"
              />
            </div>

            <div className="warn-box">
              <AlertTriangle size={16} className="warn-icon" />
              <div className="warn-text">
                <b>Re-index required.</b> Changing the embedding model or dimension after data has been
                indexed will require a full re-index. Existing vectors with a different model/dimension
                cannot be mixed in the same collection.
              </div>
            </div>
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
