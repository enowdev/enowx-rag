import { useEffect, useState } from 'react'
import { Copy, Check } from 'lucide-react'
import { api } from '../lib/api'

// Minimal markdown renderer for the docs sections: headings, paragraphs, list
// items, GitHub-style tables, fenced/indented code, and inline `code` / **bold**.
// Avoids a markdown dependency for known, controlled content.
function inline(text: string, keyBase: string): React.ReactNode {
  const parts = text.split(/(`[^`]+`|\*\*[^*]+\*\*)/g)
  return parts.map((p, i) => {
    if (p.startsWith('`') && p.endsWith('`')) return <code key={`${keyBase}-${i}`} className="mono">{p.slice(1, -1)}</code>
    if (p.startsWith('**') && p.endsWith('**')) return <b key={`${keyBase}-${i}`}>{p.slice(2, -2)}</b>
    return <span key={`${keyBase}-${i}`}>{p}</span>
  })
}

function renderMarkdown(md: string): React.ReactNode {
  const lines = md.split('\n')
  const out: React.ReactNode[] = []
  let i = 0
  let key = 0
  while (i < lines.length) {
    const line = lines[i]

    // Indented code line (4 spaces) → collect a block.
    if (/^ {4}\S/.test(line)) {
      const buf: string[] = []
      while (i < lines.length && (/^ {4}/.test(lines[i]) || lines[i].trim() === '')) {
        buf.push(lines[i].replace(/^ {4}/, ''))
        i++
      }
      out.push(<pre key={key++} className="code-body mono docs-endpoint">{buf.join('\n').replace(/\n+$/, '')}</pre>)
      continue
    }

    // Table: header row starting with | followed by a |---| separator.
    if (line.trim().startsWith('|') && i + 1 < lines.length && /^\s*\|[\s:|-]+\|\s*$/.test(lines[i + 1])) {
      const rows: string[][] = []
      const header = line.split('|').slice(1, -1).map((c) => c.trim())
      i += 2 // skip header + separator
      while (i < lines.length && lines[i].trim().startsWith('|')) {
        rows.push(lines[i].split('|').slice(1, -1).map((c) => c.trim()))
        i++
      }
      out.push(
        <div key={key++} className="docs-table-wrap">
          <table className="docs-table">
            <thead><tr>{header.map((h, hi) => <th key={hi}>{inline(h, `th${key}-${hi}`)}</th>)}</tr></thead>
            <tbody>{rows.map((r, ri) => <tr key={ri}>{r.map((c, ci) => <td key={ci}>{inline(c, `td${key}-${ri}-${ci}`)}</td>)}</tr>)}</tbody>
          </table>
        </div>,
      )
      continue
    }

    if (line.startsWith('## ')) out.push(<h2 key={key++} className="docs-h2">{inline(line.slice(3), `h2${key}`)}</h2>)
    else if (line.startsWith('# ')) out.push(<h1 key={key++} className="docs-h1">{inline(line.slice(2), `h1${key}`)}</h1>)
    else if (line.startsWith('- ')) out.push(<li key={key++} className="docs-li">{inline(line.slice(2), `li${key}`)}</li>)
    else if (/^(GET|POST|DELETE|PUT) /.test(line.trim())) out.push(<pre key={key++} className="code-body mono docs-endpoint">{line.trim()}</pre>)
    else if (line.trim() === '') out.push(<div key={key++} style={{ height: 8 }} />)
    else out.push(<p key={key++} className="docs-p">{inline(line, `p${key}`)}</p>)
    i++
  }
  return out
}

export function Docs() {
  const [sections, setSections] = useState<{ id: string; title: string }[]>([])
  const [active, setActive] = useState('overview')
  const [content, setContent] = useState('')
  const [error, setError] = useState('')
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    api.docsList().then((s) => {
      setSections(s)
      if (s.length > 0 && !s.find((x) => x.id === active)) setActive(s[0].id)
    }).catch((e) => setError(e instanceof Error ? e.message : 'Failed to load docs'))
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    setError('')
    setContent('')
    api.docsSection(active).then(setContent).catch((e) => setError(e instanceof Error ? e.message : 'Failed to load section'))
  }, [active])

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
        <span className="id mono">{sections.find((s) => s.id === active)?.title || ''}</span>
      </div>

      <div className="docs-layout">
        {/* Section nav */}
        <nav className="docs-nav">
          {sections.map((s) => (
            <div
              key={s.id}
              className={`docs-nav-item ${active === s.id ? 'active' : ''}`}
              onClick={() => setActive(s.id)}
            >
              {s.title}
            </div>
          ))}
        </nav>

        {/* Content */}
        <section className="panel docs-content-panel">
          <div className="panel-body docs-body">
            {active === 'agent-setup' && (
              <div className="code-block" style={{ marginBottom: 18 }}>
                <div className="code-head">
                  <span className="fname mono">copy this prompt into your agent</span>
                  <button className="copy-btn" onClick={copyPrompt}>
                    {copied ? <Check size={12} /> : <Copy size={12} />}
                    {copied ? 'copied' : 'copy'}
                  </button>
                </div>
                <pre className="code-body mono">{agentPrompt}</pre>
              </div>
            )}
            {error ? (
              <div className="error-state">{error}</div>
            ) : content ? (
              renderMarkdown(content)
            ) : (
              <div className="panel-empty">Loading…</div>
            )}
          </div>
        </section>
      </div>
    </>
  )
}
