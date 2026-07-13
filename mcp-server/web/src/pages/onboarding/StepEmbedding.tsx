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
    id: 'openai',
    name: 'OpenAI-compatible',
    desc: 'Any /v1/embeddings API — OpenAI, Together, Jina, Mistral, a local Ollama, LiteLLM, etc. Set the base URL and model.',
    meta: 'cloud or local · custom base URL',
  },
  {
    id: 'tei',
    name: 'TEI (self-hosted)',
    desc: 'Text Embeddings Inference server — run any local model (BGE, GTE, E5, nomic…) via Docker. The wizard just needs its URL.',
    meta: 'self-hosted · any local model · :8081',
  },
]

export function StepEmbedding({ cfg, updateCfg, onBack, onNext }: StepEmbeddingProps) {
  const [revealKey, setRevealKey] = useState(false)
  const canProceed = cfg.embedder === 'voyage' || cfg.embedder === 'tei' ||
    (cfg.embedder === 'openai' && cfg.openaiModel.trim() !== '' && cfg.openaiBaseURL.trim() !== '')

  return (
    <div className="card">
      <div className="card-head">
        <h2>Choose an Embedding Provider</h2>
        <span className="step-badge mono">3 / 7</span>
      </div>
      <div className="card-body">
        <p>Select the service that will generate vector embeddings from your text. Voyage AI is recommended for production quality; OpenAI-compatible covers most other APIs and local Ollama; TEI runs any local model.</p>

        <div className="cards cards-3">
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

        {cfg.embedder === 'openai' && (
          <>
            <div className="field">
              <label>Base URL</label>
              <input
                className="input mono"
                type="text"
                value={cfg.openaiBaseURL}
                onChange={(e) => updateCfg({ openaiBaseURL: e.target.value })}
                placeholder="https://api.openai.com/v1"
              />
              <div className="field-hint">
                The provider's <code className="mono">/v1</code> endpoint. Examples: OpenAI, Together,
                Jina, or a local Ollama at <code className="mono">http://localhost:11434/v1</code>.
              </div>
            </div>

            <div className="field">
              <label>API Key <span className="mono" style={{ color: 'var(--text-faint)' }}>(leave empty for local/no-auth)</span></label>
              <div className="input-wrapper">
                <Key size={15} strokeWidth={1.5} className="input-icon" />
                <input
                  className="input mono with-icon"
                  type={revealKey ? 'text' : 'password'}
                  value={cfg.openaiAPIKey}
                  onChange={(e) => updateCfg({ openaiAPIKey: e.target.value })}
                  placeholder="sk-… (or empty for a local endpoint)"
                />
                <button className="reveal-btn" onClick={() => setRevealKey(!revealKey)} title={revealKey ? 'Hide' : 'Reveal'}>
                  {revealKey ? <EyeOff size={16} /> : <Eye size={16} />}
                </button>
              </div>
            </div>

            <div className="field-row">
              <div className="field">
                <label>Model</label>
                <input
                  className="input mono"
                  type="text"
                  value={cfg.openaiModel}
                  onChange={(e) => updateCfg({ openaiModel: e.target.value })}
                  placeholder="text-embedding-3-small"
                />
              </div>
              <div className="field">
                <label>Dimension <span className="mono" style={{ color: 'var(--text-faint)' }}>(0 = auto)</span></label>
                <input
                  className="input mono"
                  type="number"
                  value={cfg.openaiDim}
                  onChange={(e) => updateCfg({ openaiDim: parseInt(e.target.value) || 0 })}
                  min={0}
                  max={4096}
                  step={128}
                />
              </div>
            </div>

            <div className="warn-box">
              <AlertTriangle size={16} className="warn-icon" />
              <div className="warn-text">
                <b>Re-index required.</b> Changing the model or dimension after indexing needs a full
                re-index — vectors of different models/dimensions can't share a collection.
              </div>
            </div>
          </>
        )}

        {cfg.embedder === 'tei' && (
          <>
            <div className="field">
              <label>TEI Server URL</label>
              <input
                className="input mono"
                type="text"
                value={cfg.teiURL}
                onChange={(e) => updateCfg({ teiURL: e.target.value })}
                placeholder="http://localhost:8081"
              />
              <div className="field-hint">
                TEI is a server, not a model — run it with any embedding model you like (BGE, GTE, E5,
                nomic-embed, …). Start it via Docker, then point this URL at it.
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
