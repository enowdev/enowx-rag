import { useEffect, useState } from 'react'
import { ChevronRight, ChevronLeft } from 'lucide-react'

interface StepWelcomeProps {
  onNext: () => void
}

interface EnvCheck {
  label: string
  status: 'ok' | 'fail' | 'checking' | 'unknown'
  detail: string
}

export function StepWelcome({ onNext }: StepWelcomeProps) {
  const [envChecks, setEnvChecks] = useState<EnvCheck[]>([
    { label: 'Docker', status: 'checking', detail: 'checking…' },
    { label: 'PostgreSQL (:5432)', status: 'checking', detail: 'checking…' },
    { label: 'Qdrant (:6333)', status: 'checking', detail: 'checking…' },
    { label: 'TEI Embedder (:8081)', status: 'checking', detail: 'checking…' },
  ])

  useEffect(() => {
    // Check ports by attempting to reach the setup/status endpoint
    // and doing lightweight port checks. Since the SPA can't do raw
    // TCP, we use a heuristic: if the API is up, we know PostgreSQL is
    // available (the server itself uses it). For Docker and other
    // services, we check connectivity indirectly.
    const checks: Promise<EnvCheck>[] = [
      checkDocker(),
      checkPostgres(),
      checkPort('Qdrant (:6333)', 6333, 'http://localhost:6333/healthz'),
      checkPort('TEI Embedder (:8081)', 8081, 'http://localhost:8081/health'),
    ]

    Promise.all(checks).then(setEnvChecks)
  }, [])

  return (
    <div className="card">
      <div className="card-head">
        <h2>Welcome to enowx-rag</h2>
        <span className="step-badge mono">1 / 7</span>
        <span className="card-hint">First-run setup</span>
      </div>
      <div className="card-body">
        <p>
          This wizard will guide you through configuring your RAG memory server. You will set up a{' '}
          <b>vector store</b>, choose an <b>embedding provider</b>, <b>test</b> connectivity,
          optionally run <b>auto-setup</b> for local backends, and then start indexing your projects.
        </p>

        <div className="field-label">Environment detection</div>
        <div className="env-grid">
          {envChecks.map((item) => (
            <div key={item.label} className="env-item">
              <span
                className="status-dot"
                style={{
                  background:
                    item.status === 'ok' ? 'var(--good)' : item.status === 'fail' ? 'var(--text-faint)' : 'var(--warn)',
                }}
              />
              <span className="env-label">{item.label}</span>
              <span className={`env-val mono ${item.status === 'ok' ? 'ok' : ''}`}>{item.detail}</span>
            </div>
          ))}
        </div>

        <p className="welcome-explain">
          The wizard will configure: (1) vector store selection, (2) embedding provider, (3) connection
          test, (4) optional auto-setup for local Docker backends, (5) save configuration to{' '}
          <code className="mono">~/.enowx-rag/config.yaml</code>.
        </p>
      </div>
      <div className="nav-buttons">
        <button className="btn ghost" disabled>
          <ChevronLeft size={14} /> Back
        </button>
        <div className="spacer" />
        <button className="btn primary" onClick={onNext}>
          Get Started <ChevronRight size={14} />
        </button>
      </div>
    </div>
  )
}

// Docker and PostgreSQL cannot be probed from a browser (no raw TCP, no shell).
// Rather than report a misleading "available", we say so honestly and point the
// user at the CLI (`enowx-rag setup --run`), which verifies them for real.
async function checkDocker(): Promise<EnvCheck> {
  return { label: 'Docker', status: 'unknown', detail: 'verify via CLI' }
}

async function checkPostgres(): Promise<EnvCheck> {
  return { label: 'PostgreSQL (:5432)', status: 'unknown', detail: 'verify via CLI' }
}

// checkPort attempts a real reachability check for HTTP services (Qdrant/TEI
// expose health endpoints). no-cors resolves opaquely when the port is open.
async function checkPort(label: string, _port: number, url: string): Promise<EnvCheck> {
  try {
    const resp = await fetch(url, { signal: AbortSignal.timeout(3000), mode: 'no-cors' })
    if (resp.type === 'opaque' || resp.ok) {
      return { label, status: 'ok', detail: `:${_port} — reachable` }
    }
    return { label, status: 'fail', detail: `:${_port} — not reachable` }
  } catch {
    return { label, status: 'fail', detail: `:${_port} — not reachable` }
  }
}
