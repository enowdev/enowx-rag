import { useEffect, useState } from 'react'
import { CheckCircle2, AlertCircle } from 'lucide-react'
import { api, type SetupStatus as SetupStatusType } from '../lib/api'

export function Setup() {
  const [status, setStatus] = useState<SetupStatusType | null>(null)

  useEffect(() => {
    api.setupStatus().then(setStatus).catch(() => setStatus(null))
  }, [])

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
            <div style={{ color: 'var(--text-faint)', fontSize: '13px' }}>
              Checking configuration…
            </div>
          ) : status.configured ? (
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: 'var(--good)', fontSize: '13px' }}>
              <CheckCircle2 size={16} />
              Configured — config file exists at ~/.enowx-rag/config.yaml
            </div>
          ) : (
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: 'var(--warn)', fontSize: '13px' }}>
              <AlertCircle size={16} />
              Not configured — run the onboarding wizard to set up.
            </div>
          )}
        </div>
      </section>
    </>
  )
}
