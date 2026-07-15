import { useEffect, useState } from 'react'
import { Copy, Check, BookOpen } from 'lucide-react'

// Minimal markdown renderer for the setup docs: headings, paragraphs, list
// items, and inline `code`. Avoids a markdown dependency for a small, known
// document. Not a general-purpose renderer.
function renderMarkdown(md: string): React.ReactNode {
  const lines = md.split('\n')
  const out: React.ReactNode[] = []
  let key = 0
  const inline = (text: string): React.ReactNode => {
    // Split on `code` spans and bold **text**.
    const parts = text.split(/(`[^`]+`|\*\*[^*]+\*\*)/g)
    return parts.map((p, i) => {
      if (p.startsWith('`') && p.endsWith('`')) return <code key={i} className="mono">{p.slice(1, -1)}</code>
      if (p.startsWith('**') && p.endsWith('**')) return <b key={i}>{p.slice(2, -2)}</b>
      return <span key={i}>{p}</span>
    })
  }
  for (const line of lines) {
    if (line.startsWith('## ')) out.push(<h2 key={key++} className="docs-h2">{inline(line.slice(3))}</h2>)
    else if (line.startsWith('# ')) out.push(<h1 key={key++} className="docs-h1">{inline(line.slice(2))}</h1>)
    else if (line.startsWith('- ')) out.push(<li key={key++} className="docs-li">{inline(line.slice(2))}</li>)
    else if (/^(GET|POST|DELETE|PUT) /.test(line.trim())) out.push(<pre key={key++} className="code-body mono docs-endpoint">{line.trim()}</pre>)
    else if (line.trim() === '') out.push(<div key={key++} style={{ height: 8 }} />)
    else out.push(<p key={key++} className="docs-p">{inline(line)}</p>)
  }
  return out
}

export function Docs() {
  const [md, setMd] = useState('')
  const [error, setError] = useState('')
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    fetch('/api/docs/setup')
      .then((r) => (r.ok ? r.text() : Promise.reject(new Error(`HTTP ${r.status}`))))
      .then(setMd)
      .catch((e) => setError(e instanceof Error ? e.message : 'Failed to load docs'))
  }, [])

  const docsURL = `${window.location.origin}/api/docs/setup`
  const agentPrompt =
    `Set up enowx-rag (per-project RAG memory) for this project. ` +
    `First read the setup instructions at ${docsURL} and follow them exactly: ` +
    `probe what's already installed, then install only the missing pieces ` +
    `(MCP server for my client, the skill, and the AGENTS.md block). ` +
    `Use my client id (e.g. claude-code) and this project's absolute directory. Skip anything already set up.`

  const copyPrompt = () => {
    navigator.clipboard.writeText(agentPrompt).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  return (
    <>
      <div className="page-head">
        <h1>Docs</h1>
        <span className="id mono">agent setup</span>
      </div>

      <div className="cols" style={{ gridTemplateColumns: '1fr' }}>
        {/* Copy-paste agent prompt */}
        <section className="panel">
          <div className="panel-head">
            <h2>Set up with an AI agent</h2>
            <span className="hint">one prompt</span>
          </div>
          <div className="panel-body">
            <p style={{ color: 'var(--text-dim)', fontSize: 12.5, marginTop: 0 }}>
              Paste this into your AI coding agent. It reads the instructions below and installs only
              what's missing (skips MCP / skill / AGENTS.md that already exist).
            </p>
            <div className="code-block">
              <div className="code-head">
                <span className="fname mono">setup prompt</span>
                <button className="copy-btn" onClick={copyPrompt}>
                  {copied ? <Check size={12} /> : <Copy size={12} />}
                  {copied ? 'copied' : 'copy'}
                </button>
              </div>
              <pre className="code-body mono">{agentPrompt}</pre>
            </div>
          </div>
        </section>

        {/* Rendered docs */}
        <section className="panel">
          <div className="panel-head">
            <h2><BookOpen size={15} strokeWidth={1.7} style={{ verticalAlign: '-2px', marginRight: 6 }} />Setup instructions</h2>
            <span className="hint mono">/api/docs/setup</span>
          </div>
          <div className="panel-body docs-body">
            {error ? (
              <div className="error-state">{error}</div>
            ) : md ? (
              renderMarkdown(md)
            ) : (
              <div className="panel-empty">Loading…</div>
            )}
          </div>
        </section>
      </div>
    </>
  )
}
