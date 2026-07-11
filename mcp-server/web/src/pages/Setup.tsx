import { useEffect, useState } from 'react'
import { CheckCircle2, AlertCircle, Loader2 } from 'lucide-react'
import { api, type SetupStatus as SetupStatusType } from '../lib/api'
import { Wizard } from './onboarding/Wizard'
import { useTheme } from '../lib/useTheme'

export function Setup() {
  const { theme, toggleTheme } = useTheme()
  const [status, setStatus] = useState<SetupStatusType | null>(null)
  const [showWizard, setShowWizard] = useState(false)

  useEffect(() => {
    api.setupStatus().then(setStatus).catch(() => setStatus(null))
  }, [])

  if (showWizard) {
    return (
      <Wizard
        onComplete={() => {
          setShowWizard(false)
          // Reload status after wizard completes
          api.setupStatus().then(setStatus).catch(() => {})
        }}
        theme={theme}
        onToggleTheme={toggleTheme}
      />
    )
  }

  return (
    <>
      <div className="page-head">
        <h1>Setup</h1>
        <span className="id mono">onboarding wizard</span>
      </div>

      <section className="panel">
        <div className="panel-head">
          <h2>Configuration status</h2>
        </div>
        <div className="panel-body">
          {status === null ? (
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: 'var(--text-faint)', fontSize: '13px' }}>
              <Loader2 size={16} className="spin" />
              Checking configuration…
            </div>
          ) : status.configured ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: 'var(--good)', fontSize: '13px' }}>
                <CheckCircle2 size={16} />
                Configured — config file exists at ~/.enowx-rag/config.yaml
              </div>
              <button className="btn primary" onClick={() => setShowWizard(true)}>
                Reconfigure
              </button>
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: 'var(--warn)', fontSize: '13px' }}>
                <AlertCircle size={16} />
                Not configured — run the onboarding wizard to set up.
              </div>
              <button className="btn primary" onClick={() => setShowWizard(true)}>
                Start Wizard
              </button>
            </div>
          )}
        </div>
      </section>
    </>
  )
}
