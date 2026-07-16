import { useState } from 'react'
import { ChevronLeft, Check, AlertCircle, Loader2 } from 'lucide-react'
import { api } from '../../lib/api'
import type { DraftConfig } from './types'
import { draftToRequest } from './types'

interface StepDoneProps {
  cfg: DraftConfig
  onBack: () => void
  onComplete: () => void
}

export function StepDone({ cfg, onBack, onComplete }: StepDoneProps) {
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showConfirm, setShowConfirm] = useState(false)
  const [confirmedOverwrite, setConfirmedOverwrite] = useState(false)

  const handleFinish = async () => {
    // Check if config already exists (VAL-WIZ-035: don't overwrite without confirmation)
    if (showConfirm && !confirmedOverwrite) {
      return
    }

    setSaving(true)
    setError(null)
    try {
      await api.setupApply(draftToRequest(cfg))
      // Clear draft and step from localStorage
      localStorage.removeItem('wizard-draft')
      localStorage.removeItem('wizard-step')
      onComplete()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to save configuration')
    } finally {
      setSaving(false)
    }
  }

  const handleFinishClick = async () => {
    // Check if config exists first
    try {
      const status = await api.setupStatus()
      if (status.configured && !confirmedOverwrite) {
        setShowConfirm(true)
        return
      }
    } catch {
      // If we can't check, proceed
    }
    handleFinish()
  }

  return (
    <div className="card">
      <div className="card-head">
        <h2>Configuration Complete</h2>
        <span className="step-badge mono">7 / 7</span>
        <span className="card-hint">POST /api/setup/apply</span>
      </div>
      <div className="card-body done-body">
        <div className="done-icon">
          <Check size={28} strokeWidth={2.5} />
        </div>
        <div className="done-title">You are all set!</div>
        <div className="done-sub">
          Review your configuration below and click <b>Finish</b> to save and launch the dashboard.
        </div>

        <div className="summary-box">
          <div className="summary-row">
            <span className="sk">Vector Store</span>
            <span className="sv mono">{cfg.vectorStore}</span>
          </div>
          {cfg.vectorStore === 'pgvector' && (
            <div className="summary-row">
              <span className="sk">DSN</span>
              <span className="sv mono">{cfg.pgvectorDSN}</span>
            </div>
          )}
          {cfg.vectorStore === 'qdrant' && (
            <div className="summary-row">
              <span className="sk">URL</span>
              <span className="sv mono">{cfg.qdrantURL}</span>
            </div>
          )}
          {cfg.vectorStore === 'chroma' && (
            <div className="summary-row">
              <span className="sk">URL</span>
              <span className="sv mono">{cfg.chromaURL}</span>
            </div>
          )}
          <div className="summary-row">
            <span className="sk">Embedder</span>
            <span className="sv mono">{cfg.embedder}</span>
          </div>
          {cfg.embedder === 'voyage' && (
            <>
              <div className="summary-row">
                <span className="sk">Model</span>
                <span className="sv mono">{cfg.voyageModel} · {cfg.voyageDim}-dim</span>
              </div>
              <div className="summary-row">
                <span className="sk">API Key</span>
                <span className="sv mono">{cfg.voyageAPIKey ? '••••••••' + cfg.voyageAPIKey.slice(-4) : '—'}</span>
              </div>
            </>
          )}
          {cfg.embedder === 'tei' && (
            <div className="summary-row">
              <span className="sk">TEI URL</span>
              <span className="sv mono">{cfg.teiURL}</span>
            </div>
          )}
          {cfg.embedder === 'voyage' && (
            <div className="summary-row">
              <span className="sk">Reranker</span>
              <span className="sv mono">rerank-2.5</span>
            </div>
          )}
          <div className="summary-row">
            <span className="sk">Config path</span>
            <span className="sv mono">~/.enowx-rag/config.yaml</span>
          </div>
          <div className="summary-row">
            <span className="sk">Permissions</span>
            <span className="sv" style={{ color: 'var(--good)' }}>0600</span>
          </div>
        </div>

        <p style={{ fontSize: '12.5px', marginBottom: 0 }}>
          After saving, you will be redirected to the dashboard where you can index your first project and start using semantic search.
        </p>

        {error && (
          <div className="test-error" style={{ marginTop: 14 }}>
            <AlertCircle size={16} />
            <span>{error}</span>
          </div>
        )}

        {/* Confirmation dialog for existing config */}
        {showConfirm && !confirmedOverwrite && (
          <div className="confirm-dialog">
            <div className="confirm-content">
              <AlertCircle size={20} style={{ color: 'var(--warn)', flex: 'none' }} />
              <div>
                <div className="confirm-title">Configuration already exists</div>
                <div className="confirm-desc">
                  A config file already exists at <code className="mono">~/.enowx-rag/config.yaml</code>. Saving will
                  replace the existing configuration. Do you want to continue?
                </div>
              </div>
            </div>
            <div className="confirm-actions">
              <button className="btn" onClick={() => setShowConfirm(false)}>
                Cancel
              </button>
              <button
                className="btn primary"
                onClick={() => {
                  setConfirmedOverwrite(true)
                  handleFinish()
                }}
              >
                Replace Config
              </button>
            </div>
          </div>
        )}
      </div>
      <div className="nav-buttons">
        <button className="btn" onClick={onBack} disabled={saving}>
          <ChevronLeft size={14} /> Back
        </button>
        <div className="spacer" />
        <button
          className="btn primary"
          onClick={handleFinishClick}
          disabled={saving || (showConfirm && !confirmedOverwrite)}
        >
          {saving ? (
            <>
              <Loader2 size={14} className="spin" /> Saving…
            </>
          ) : (
            <>
              <Check size={14} /> Finish &amp; Launch
            </>
          )}
        </button>
      </div>
    </div>
  )
}
