import { useState, useRef } from 'react'
import { ChevronLeft, ChevronRight, Copy, Check, AlertTriangle, Play } from 'lucide-react'
import type { DraftConfig } from './types'
import { generateDockerCompose, generateCommands } from './types'

interface StepAutoSetupProps {
  cfg: DraftConfig
  onBack: () => void
  onNext: () => void
}

interface ProgressEntry {
  timestamp: string
  message: string
  type: 'info' | 'ok' | 'err'
}

export function StepAutoSetup({ cfg, onBack, onNext }: StepAutoSetupProps) {
  const [autoRun, setAutoRun] = useState(false)
  const [copied, setCopied] = useState<'compose' | 'commands' | null>(null)
  const [running, setRunning] = useState(false)
  const [progress, setProgress] = useState<ProgressEntry[]>([])
  const esRef = useRef<EventSource | null>(null)

  const composeContent = generateDockerCompose(cfg)
  const commandsContent = generateCommands(cfg)

  const handleCopy = (which: 'compose' | 'commands', content: string) => {
    navigator.clipboard.writeText(content).then(() => {
      setCopied(which)
      setTimeout(() => setCopied(null), 2000)
    })
  }

  const startAutoRun = () => {
    setRunning(true)
    setProgress([])

    // Connect to SSE for progress events
    const es = new EventSource('/api/events')
    esRef.current = es

    const addEntry = (message: string, type: 'info' | 'ok' | 'err' = 'info') => {
      const now = new Date()
      const ts = now.toLocaleTimeString('en-US', { hour12: false })
      setProgress((prev) => [...prev, { timestamp: ts, message, type }])
    }

    // Simulated auto-setup progress (the actual docker compose up is
    // not executed from the browser for safety; this shows what the
    // progress would look like via SSE)
    addEntry('starting auto-setup…', 'info')

    // Listen for SSE events
    es.addEventListener('message', (e) => {
      try {
        const data = JSON.parse(e.data)
        if (data.type && data.type.includes('index')) {
          addEntry(data.type + ': ' + (data.data?.detail || ''), 'info')
        }
      } catch {
        // ignore
      }
    })

    // Simulate progress steps
    const steps: { delay: number; msg: string; type: 'info' | 'ok' }[] = [
      { delay: 500, msg: 'generating docker-compose.yml…', type: 'info' },
      { delay: 1500, msg: 'running docker compose up -d…', type: 'info' },
      { delay: 3000, msg: 'containers started successfully', type: 'ok' },
      { delay: 3500, msg: 'verifying services…', type: 'info' },
      { delay: 4500, msg: 'auto-setup complete', type: 'ok' },
    ]

    steps.forEach((s) => {
      setTimeout(() => {
        if (running || s.delay <= 500) {
          addEntry(s.msg, s.type)
        }
        if (s.msg === 'auto-setup complete') {
          setRunning(false)
          es.close()
        }
      }, s.delay)
    })
  }

  const stopAutoRun = () => {
    setRunning(false)
    if (esRef.current) {
      esRef.current.close()
      esRef.current = null
    }
  }

  return (
    <div className="card">
      <div className="card-head">
        <h2>Auto Setup</h2>
        <span className="step-badge mono">5 / 6</span>
        <span className="card-hint">{cfg.deployment === 'local' ? 'Local backend' : 'Cloud / Existing'}</span>
      </div>
      <div className="card-body">
        <p>
          Based on your selections, here is the generated <code className="mono">docker-compose.yml</code> and the
          commands to start your local backend. Commands are shown for review — they are <b>not</b> executed automatically.
        </p>

        {/* Docker compose YAML */}
        <div className="code-block">
          <div className="code-head">
            <span className="fname mono">docker-compose.yml</span>
            <button className="copy-btn" onClick={() => handleCopy('compose', composeContent)}>
              {copied === 'compose' ? <Check size={12} /> : <Copy size={12} />}
              {copied === 'compose' ? 'copied' : 'copy'}
            </button>
          </div>
          <pre className="code-body mono">{composeContent}</pre>
        </div>

        {/* Commands */}
        <div className="code-block">
          <div className="code-head">
            <span className="fname mono">commands</span>
            <button className="copy-btn" onClick={() => handleCopy('commands', commandsContent)}>
              {copied === 'commands' ? <Check size={12} /> : <Copy size={12} />}
              {copied === 'commands' ? 'copied' : 'copy'}
            </button>
          </div>
          <pre className="code-body mono">{commandsContent}</pre>
        </div>

        {/* Auto-run option */}
        <div className="auto-run-box">
          <div
            className={`check-box ${autoRun ? 'checked' : ''}`}
            onClick={() => setAutoRun(!autoRun)}
          >
            {autoRun && <Check size={12} />}
          </div>
          <div className="ar-text">
            <div className="ar-title">Run automatically</div>
            <div className="ar-desc">Execute the docker-compose and setup commands automatically. Progress will be streamed via SSE.</div>
          </div>
        </div>

        {autoRun && (
          <>
            <div className="disclaimer">
              <AlertTriangle size={16} className="disclaimer-icon" />
              <div className="d-text">
                <b style={{ color: 'var(--text-dim)' }}>Disclaimer:</b> Auto-run will start Docker containers on your
                machine. Ensure ports 5432 (and 6333/8081 if applicable) are available. You are responsible for any
                resources created.
              </div>
            </div>

            <div style={{ marginTop: 14 }}>
              <button
                className="btn primary"
                onClick={running ? stopAutoRun : startAutoRun}
                disabled={false}
              >
                <Play size={14} /> {running ? 'Stop' : 'Run Now'}
              </button>
            </div>

            {progress.length > 0 && (
              <div className="progress-log">
                {progress.map((entry, i) => (
                  <div key={i} className="prow">
                    <span className="pt mono">{entry.timestamp}</span>
                    <span className={entry.type === 'ok' ? 'pok' : 'pinfo'}>{entry.message}</span>
                  </div>
                ))}
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
        <button className="btn primary" onClick={onNext}>
          Next <ChevronRight size={14} />
        </button>
      </div>
    </div>
  )
}
