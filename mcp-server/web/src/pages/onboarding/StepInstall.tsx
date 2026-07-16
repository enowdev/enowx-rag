import { useEffect, useState } from 'react'
import { ChevronLeft, ChevronRight, Copy, Check, Download, Terminal, CheckCircle2, XCircle } from 'lucide-react'
import { api, type McpClient, type McpSnippetResponse, type SkillGuideResponse } from '../../lib/api'

interface StepInstallProps {
  onBack: () => void
  onNext: () => void
}

type Mode = 'auto' | 'manual'

export function StepInstall({ onBack, onNext }: StepInstallProps) {
  const [clients, setClients] = useState<McpClient[]>([])
  const [selected, setSelected] = useState<string>('')
  const [scope, setScope] = useState<'global' | 'project'>('global')
  const [mode, setMode] = useState<Mode>('auto')
  // Connection: local stdio (spawns the binary) vs remote daemon (url + token).
  const [conn, setConn] = useState<'local' | 'remote'>('local')
  const [remoteURL, setRemoteURL] = useState('')
  const [remoteToken, setRemoteToken] = useState('')
  const [installing, setInstalling] = useState(false)
  const [result, setResult] = useState<{ ok: boolean; message: string } | null>(null)
  const [snippet, setSnippet] = useState<McpSnippetResponse | null>(null)
  const [skill, setSkill] = useState<SkillGuideResponse | null>(null)
  const [copied, setCopied] = useState<string | null>(null)

  useEffect(() => {
    api.mcpClients().then((c) => {
      setClients(c)
      if (c.length > 0) setSelected(c[0].id)
    }).catch(() => setClients([]))
    api.skillGuide().then(setSkill).catch(() => setSkill(null))
  }, [])

  const selectedClient = clients.find((c) => c.id === selected)

  const remoteOpts = () => conn === 'remote'
    ? { mode: 'remote', remote_url: remoteURL, token: remoteToken || undefined }
    : undefined

  // Load the manual snippet whenever the manual tab is shown or inputs change.
  useEffect(() => {
    if (mode === 'manual' && selected && selected !== 'other') {
      api.mcpSnippet(selected, remoteOpts()).then(setSnippet).catch(() => setSnippet(null))
    }
  }, [mode, selected, conn, remoteURL, remoteToken]) // eslint-disable-line react-hooks/exhaustive-deps

  const copy = (key: string, text: string) => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(key)
      setTimeout(() => setCopied(null), 2000)
    })
  }

  // Short prompt an agent runs: it reads the setup docs from the API, then does
  // only the missing steps. Uses the current origin so it works wherever hosted.
  const docsURL = `${window.location.origin}/api/docs/setup`
  const agentPrompt =
    `Set up enowx-rag (per-project RAG memory) for this project. ` +
    `First read the setup instructions at ${docsURL} and follow them exactly: ` +
    `probe what's already installed, then install only the missing pieces ` +
    `(MCP server for my client, the skill, and the AGENTS.md block). ` +
    `Use my client id (e.g. claude-code) and this project's absolute directory. Skip anything already set up.`

  const runInstall = async () => {
    if (!selected) return
    if (conn === 'remote' && !remoteURL) {
      setResult({ ok: false, message: 'Enter the daemon URL (e.g. https://host/mcp) for a remote install.' })
      return
    }
    setInstalling(true)
    setResult(null)
    try {
      const r = await api.installMcp({
        client_id: selected, scope,
        ...(conn === 'remote' ? { mode: 'remote', remote_url: remoteURL, token: remoteToken || undefined } : {}),
      })
      setResult({
        ok: true,
        message: `Installed into ${r.path}${r.backed_up ? ' (existing config backed up to .bak)' : ''}.`,
      })
    } catch (e) {
      setResult({ ok: false, message: e instanceof Error ? e.message : 'Install failed' })
    } finally {
      setInstalling(false)
    }
  }

  return (
    <div className="card">
      <div className="card-head">
        <h2>Install MCP Server</h2>
        <span className="step-badge mono">6 / 7</span>
        <span className="card-hint">Connect enowx-rag to your AI tool</span>
      </div>
      <div className="card-body">
        <p>Pick your MCP client. enowx-rag can write the config for you, or show a snippet to paste manually.</p>

        {/* Client picker */}
        <div className="field-label">Client</div>
        <div className="cards cards-3">
          {clients.map((c) => (
            <div
              key={c.id}
              className={`pcard ${selected === c.id ? 'selected' : ''}`}
              onClick={() => { setSelected(c.id); setResult(null) }}
            >
              <div className="pcard-title">{c.label}</div>
              <div className="pcard-desc mono">{c.format.replace('json-', '').replace('yaml-list', 'yaml')}</div>
            </div>
          ))}
          <div
            className={`pcard ${selected === 'other' ? 'selected' : ''}`}
            onClick={() => { setSelected('other'); setMode('manual'); setResult(null) }}
          >
            <div className="pcard-title">Other</div>
            <div className="pcard-desc">Manual snippet</div>
          </div>
        </div>

        {/* Connection: local stdio vs remote daemon */}
        <div className="toolbar" style={{ marginTop: 14 }}>
          <span className={`toggle ${conn === 'local' ? 'on' : ''}`} onClick={() => setConn('local')}>
            <span className="switch" /> Local (stdio)
          </span>
          <span className={`toggle ${conn === 'remote' ? 'on' : ''}`} onClick={() => setConn('remote')}>
            <span className="switch" /> Remote daemon
          </span>
        </div>
        {conn === 'remote' && (
          <div style={{ marginTop: 10 }}>
            <div className="field-row">
              <div className="field"><label>Daemon URL</label>
                <input className="input mono" value={remoteURL} onChange={(e) => setRemoteURL(e.target.value)} placeholder="https://rag.example.com/mcp" /></div>
              <div className="field"><label>Token (RAG_ADMIN_TOKEN)</label>
                <input className="input mono" type="password" value={remoteToken} onChange={(e) => setRemoteToken(e.target.value)} placeholder="Bearer token, if set" /></div>
            </div>
            <div className="field-hint">Connect to an enowx-rag daemon (`enowx-rag --serve`) over HTTP instead of spawning a local binary.</div>
          </div>
        )}

        {selected && selected !== 'other' && (
          <>
            {/* Mode toggle */}
            <div className="toolbar" style={{ marginTop: 14 }}>
              <span className={`toggle ${mode === 'auto' ? 'on' : ''}`} onClick={() => setMode('auto')}>
                <span className="switch" /> Install automatically
              </span>
              <span className={`toggle ${mode === 'manual' ? 'on' : ''}`} onClick={() => setMode('manual')}>
                <span className="switch" /> Show snippet
              </span>
              {selectedClient?.has_project && mode === 'auto' && (
                <span className="chip" onClick={() => setScope(scope === 'global' ? 'project' : 'global')}>
                  scope <b>{scope}</b>
                </span>
              )}
            </div>

            {mode === 'auto' ? (
              <div style={{ marginTop: 14 }}>
                <div className="cli-hint" style={{ marginBottom: 12 }}>
                  <Download size={16} className="cli-hint-icon" />
                  <div className="d-text">
                    Writes <code className="mono">{selectedClient?.global_path}</code>, merging into any
                    existing config (your other MCP servers are preserved; the original is backed up to
                    <code className="mono"> .bak</code>).
                  </div>
                </div>
                <button className="btn primary" onClick={runInstall} disabled={installing}>
                  <Download size={14} /> {installing ? 'Installing…' : `Install to ${selectedClient?.label}`}
                </button>
                {result && (
                  <div className={`test-result ${result.ok ? '' : 'fail'}`} style={{ marginTop: 14 }}>
                    <span className="status-dot" style={{ background: result.ok ? 'var(--good)' : 'var(--crit)' }} />
                    <div className="test-info">
                      <div className="comp-name">{result.ok ? 'Installed' : 'Install failed'}</div>
                      <div className="test-msg">{result.message}</div>
                    </div>
                    {result.ok ? <CheckCircle2 size={16} style={{ color: 'var(--good)' }} /> : <XCircle size={16} style={{ color: 'var(--crit)' }} />}
                  </div>
                )}
              </div>
            ) : (
              <div style={{ marginTop: 14 }}>
                {snippet ? (
                  <div className="code-block">
                    <div className="code-head">
                      <span className="fname mono">{snippet.path}</span>
                      <button className="copy-btn" onClick={() => copy('snippet', snippet.content)}>
                        {copied === 'snippet' ? <Check size={12} /> : <Copy size={12} />}
                        {copied === 'snippet' ? 'copied' : 'copy'}
                      </button>
                    </div>
                    <pre className="code-body mono">{snippet.content}</pre>
                  </div>
                ) : (
                  <div className="panel-empty">Loading snippet…</div>
                )}
              </div>
            )}
          </>
        )}

        {selected === 'other' && (
          <div style={{ marginTop: 14 }}>
            <p className="welcome-explain">
              For a client not listed above, add an MCP server named <code className="mono">enowx-rag</code>{' '}
              that runs the enowx-rag binary. Use one of the snippets from a listed client with a similar
              format as a template, adapting the file path and key for your tool.
            </p>
          </div>
        )}

        {/* Skill install (manual) */}
        {skill && (
          <div style={{ marginTop: 22 }}>
            <div className="field-label">Skill (optional)</div>
            <div className="cli-hint">
              <Terminal size={16} className="cli-hint-icon" />
              <div className="d-text">
                {skill.note}
                <pre className="code-body mono" style={{ marginTop: 8 }}>{skill.commands.join('\n')}</pre>
                <button className="copy-btn" onClick={() => copy('skill', skill.commands.join('\n'))} style={{ marginTop: 6 }}>
                  {copied === 'skill' ? <Check size={12} /> : <Copy size={12} />}
                  {copied === 'skill' ? 'copied' : 'copy commands'}
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Agent setup: one prompt an AI agent runs to set up MCP + skill + AGENTS.md */}
        <div style={{ marginTop: 22 }}>
          <div className="field-label">Or: set up with an AI agent</div>
          <p style={{ color: 'var(--text-dim)', fontSize: 12.5, marginTop: 0 }}>
            Paste this prompt into your AI coding agent. It reads the setup docs and installs only
            what's missing (skips MCP/skill/AGENTS.md that already exist).
          </p>
          <div className="code-block">
            <div className="code-head">
              <span className="fname mono">setup prompt</span>
              <button className="copy-btn" onClick={() => copy('prompt', agentPrompt)}>
                {copied === 'prompt' ? <Check size={12} /> : <Copy size={12} />}
                {copied === 'prompt' ? 'copied' : 'copy'}
              </button>
            </div>
            <pre className="code-body mono">{agentPrompt}</pre>
          </div>
        </div>
      </div>
      <div className="nav-buttons">
        <button className="btn" onClick={onBack}>
          <ChevronLeft size={14} /> Back
        </button>
        <div className="spacer" />
        <button className="btn primary" onClick={onNext}>
          Next <ChevronRight size={14} />
        </button>
      </div>
    </div>
  )
}
