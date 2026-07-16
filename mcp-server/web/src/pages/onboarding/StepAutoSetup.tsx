import { useState } from 'react'
import { ChevronLeft, ChevronRight, Copy, Check, Terminal, CheckCircle2 } from 'lucide-react'
import type { DraftConfig } from './types'
import { generateDockerCompose, generateCommands, localServices } from './types'

interface StepAutoSetupProps {
  cfg: DraftConfig
  onBack: () => void
  onNext: () => void
}

export function StepAutoSetup({ cfg, onBack, onNext }: StepAutoSetupProps) {
  const [copied, setCopied] = useState<'compose' | 'commands' | null>(null)

  const services = localServices(cfg)
  const needsDocker = services.length > 0
  const composeContent = generateDockerCompose(cfg)
  const commandsContent = generateCommands(cfg)

  const handleCopy = (which: 'compose' | 'commands', content: string) => {
    navigator.clipboard.writeText(content).then(() => {
      setCopied(which)
      setTimeout(() => setCopied(null), 2000)
    })
  }

  return (
    <div className="card">
      <div className="card-head">
        <h2>Local Backend</h2>
        <span className="step-badge mono">5 / 7</span>
        <span className="card-hint">{needsDocker ? `Docker: ${services.join(', ')}` : 'Nothing to run locally'}</span>
      </div>
      <div className="card-body">
        {!needsDocker ? (
          <>
            <p>
              Your vector store and embedder are all <b>cloud / API / remote</b> — nothing needs to run
              on this machine via Docker. You can continue.
            </p>
            <div className="cli-hint">
              <CheckCircle2 size={16} className="cli-hint-icon" style={{ color: 'var(--good)' }} />
              <div className="d-text">
                No local backend required. (A hosted Qdrant and the Voyage embedding API both run in the
                cloud.) If you later want to self-host, go back and point the vector store or embedder at
                a <code className="mono">localhost</code> URL.
              </div>
            </div>
          </>
        ) : (
          <>
            <p>
              These components run locally via Docker: <b>{services.join(', ')}</b>. Copy the
              <code className="mono"> docker-compose.yml</code> and commands below and run them yourself in a
              terminal — they are <b>not</b> executed automatically.
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

            {/* Run from the CLI (never executed from the browser) */}
            <div className="cli-hint">
              <Terminal size={16} className="cli-hint-icon" />
              <div className="d-text">
                To start the backend, run the commands above yourself, or let the CLI do it:
                <pre className="code-body mono" style={{ marginTop: 8 }}>enowx-rag setup --run</pre>
                This writes the compose file and runs <code className="mono">docker compose up -d</code> in your
                terminal. The dashboard never runs Docker for you.
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
        <button className="btn primary" onClick={onNext}>
          Next <ChevronRight size={14} />
        </button>
      </div>
    </div>
  )
}
