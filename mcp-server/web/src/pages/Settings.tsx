import { useEffect, useState } from 'react'
import { Eye, EyeOff, Save, Key, RefreshCw, Copy, Check, AlertTriangle } from 'lucide-react'
import { api } from '../lib/api'

export function Settings() {
  const [masked, setMasked] = useState<Record<string, unknown> | null>(null)
  const [revealed, setRevealed] = useState<Record<string, string> | null>(null)
  const [error, setError] = useState('')
  const [saveMsg, setSaveMsg] = useState('')
  const [newToken, setNewToken] = useState('')
  const [copiedToken, setCopiedToken] = useState(false)
  const [tokenEnvOverride, setTokenEnvOverride] = useState(false)

  // Editable fields (empty = leave unchanged; a value = overwrite).
  const [voyageKey, setVoyageKey] = useState('')
  const [openaiKey, setOpenaiKey] = useState('')
  const [qdrantKey, setQdrantKey] = useState('')

  const load = () => {
    api.configMasked().then(setMasked).catch((e) => setError(e instanceof Error ? e.message : 'Failed to load config'))
  }
  useEffect(load, [])

  const reveal = async () => {
    try {
      setRevealed(await api.configReveal())
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Reveal failed (requires localhost or admin token)')
    }
  }

  const save = async () => {
    setSaveMsg('')
    setError('')
    const patch: Record<string, string> = {}
    if (voyageKey) patch.voyage_api_key = voyageKey
    if (openaiKey) patch.openai_api_key = openaiKey
    if (qdrantKey) patch.qdrant_api_key = qdrantKey
    if (Object.keys(patch).length === 0) {
      setSaveMsg('Nothing to save — enter a new value to change a key.')
      return
    }
    try {
      await api.configUpdate(patch)
      setSaveMsg('Saved to ~/.enowx-rag/config.yaml (0600).')
      setVoyageKey(''); setOpenaiKey(''); setQdrantKey('')
      setRevealed(null)
      load()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Save failed')
    }
  }

  const generate = async () => {
    setError('')
    try {
      const r = await api.genToken()
      setNewToken(r.token)
      setTokenEnvOverride(r.env_override)
      load()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Generate failed')
    }
  }

  const copyToken = () => {
    navigator.clipboard.writeText(newToken).then(() => {
      setCopiedToken(true)
      setTimeout(() => setCopiedToken(false), 2000)
    })
  }

  const m = (k: string) => (masked?.[k] as string) || '—'
  const rev = (k: string) => revealed?.[k]

  return (
    <>
      <div className="page-head">
        <h1>Settings</h1>
        <span className="id mono">API keys · admin token</span>
      </div>

      <div className="cols" style={{ gridTemplateColumns: '1fr 1fr' }}>
        {/* API keys */}
        <section className="panel">
          <div className="panel-head">
            <h2>API keys</h2>
            <button className="btn" style={{ padding: '5px 10px' }} onClick={reveal}>
              <Eye size={13} /> Reveal
            </button>
          </div>
          <div className="panel-body">
            {error && <div className="error-state" style={{ marginBottom: 12 }}>{error}</div>}

            <KeyRow label="Voyage API key" masked={m('voyage_api_key')} revealed={rev('voyage_api_key')}
              value={voyageKey} onChange={setVoyageKey} placeholder="new Voyage key…" />
            <KeyRow label="OpenAI API key" masked={m('openai_api_key')} revealed={rev('openai_api_key')}
              value={openaiKey} onChange={setOpenaiKey} placeholder="new OpenAI key…" />
            <KeyRow label="Qdrant API key" masked={m('qdrant_api_key')} revealed={rev('qdrant_api_key')}
              value={qdrantKey} onChange={setQdrantKey} placeholder="new Qdrant key…" />

            <button className="btn primary" onClick={save} style={{ marginTop: 6 }}>
              <Save size={14} /> Save changes
            </button>
            {saveMsg && <div className="reindex-msg" style={{ marginTop: 10 }}>{saveMsg}</div>}
            <div className="field-hint" style={{ marginTop: 10 }}>
              Leave a field blank to keep the existing key. New values are written to
              <code className="mono"> ~/.enowx-rag/config.yaml</code> (0600). Re-index is not needed for
              key changes, but changing the embedding <b>model/dimension</b> is.
            </div>
          </div>
        </section>

        {/* Admin token */}
        <section className="panel">
          <div className="panel-head">
            <h2>Admin token</h2>
            <span className="hint mono">{(masked?.admin_token_set as boolean) ? 'set' : 'not set'}</span>
          </div>
          <div className="panel-body">
            <p style={{ color: 'var(--text-dim)', fontSize: 12.5, marginTop: 0 }}>
              Gates <code className="mono">/api/*</code> and <code className="mono">/mcp</code> with a bearer
              token. Required when exposing the daemon publicly. Generate one here (stored in config), or set
              <code className="mono"> RAG_ADMIN_TOKEN</code> in the environment (env takes precedence).
            </p>

            <button className="btn primary" onClick={generate}>
              <RefreshCw size={14} /> Generate new token
            </button>

            {newToken && (
              <div style={{ marginTop: 14 }}>
                <div className="code-block">
                  <div className="code-head">
                    <span className="fname mono">new admin token (copy now)</span>
                    <button className="copy-btn" onClick={copyToken}>
                      {copiedToken ? <Check size={12} /> : <Copy size={12} />}
                      {copiedToken ? 'copied' : 'copy'}
                    </button>
                  </div>
                  <pre className="code-body mono">{newToken}</pre>
                </div>
                {tokenEnvOverride && (
                  <div className="warn-box" style={{ marginTop: 10 }}>
                    <AlertTriangle size={16} className="warn-icon" />
                    <div className="warn-text">
                      <b>RAG_ADMIN_TOKEN is set in the environment</b> and takes precedence over this saved
                      token at runtime. Unset it to use the generated one.
                    </div>
                  </div>
                )}
              </div>
            )}
          </div>
        </section>
      </div>
    </>
  )
}

function KeyRow({ label, masked, revealed, value, onChange, placeholder }: {
  label: string; masked: string; revealed?: string; value: string; onChange: (v: string) => void; placeholder: string
}) {
  const [show, setShow] = useState(false)
  return (
    <div className="field">
      <label>{label}</label>
      <div className="stat-line" style={{ padding: '2px 0 8px' }}>
        <span className="k">Current</span>
        <span className="v mono" style={{ wordBreak: 'break-all' }}>
          {revealed !== undefined ? (revealed || '(empty)') : masked}
        </span>
      </div>
      <div className="input-wrapper">
        <Key size={15} strokeWidth={1.5} className="input-icon" />
        <input className="input mono with-icon" type={show ? 'text' : 'password'} value={value}
          onChange={(e) => onChange(e.target.value)} placeholder={placeholder} />
        <button className="reveal-btn" onClick={() => setShow(!show)}>
          {show ? <EyeOff size={16} /> : <Eye size={16} />}
        </button>
      </div>
    </div>
  )
}
