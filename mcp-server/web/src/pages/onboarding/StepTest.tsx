import { useState } from 'react'
import { ChevronLeft, ChevronRight, Zap, CheckCircle2, XCircle } from 'lucide-react'
import { api, type SetupTestResponse } from '../../lib/api'
import type { DraftConfig } from './types'
import { draftToRequest } from './types'

interface TestResults {
  vectorStore: { ok: boolean; message: string; latency_ms: number } | null
  embedder: { ok: boolean; message: string; latency_ms: number } | null
}

interface StepTestProps {
  cfg: DraftConfig
  testResults: TestResults
  setTestResults: (r: TestResults) => void
  testPassed: boolean
  onBack: () => void
  onNext: () => void
}

export function StepTest({ cfg, testResults, setTestResults, testPassed, onBack, onNext }: StepTestProps) {
  const [testing, setTesting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const runTest = async () => {
    setTesting(true)
    setError(null)
    try {
      const resp: SetupTestResponse = await api.setupTest(draftToRequest(cfg))
      setTestResults({
        vectorStore: resp.vector_store,
        embedder: resp.embedder,
      })
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Test failed')
      setTestResults({ vectorStore: null, embedder: null })
    } finally {
      setTesting(false)
    }
  }

  const hasResults = testResults.vectorStore !== null || testResults.embedder !== null
  const passedCount = [testResults.vectorStore, testResults.embedder].filter((r) => r?.ok).length
  const totalCount = 2

  return (
    <div className="card">
      <div className="card-head">
        <h2>Test Connection</h2>
        <span className="step-badge mono">4 / 6</span>
        <span className="card-hint">POST /api/setup/test</span>
      </div>
      <div className="card-body">
        <p>Verify that your vector store and embedding provider are reachable. Click <b>Test Connection</b> to run per-component health checks.</p>

        <div style={{ marginBottom: 16 }}>
          <button className="btn primary" onClick={runTest} disabled={testing}>
            <Zap size={14} /> {testing ? 'Testing…' : 'Test Connection'}
          </button>
        </div>

        {error && (
          <div className="test-error">
            <XCircle size={16} />
            <span>{error}</span>
          </div>
        )}

        {/* Vector Store result */}
        {testResults.vectorStore && (
          <div className={`test-result ${testResults.vectorStore.ok ? '' : 'fail'}`}>
            <span
              className="status-dot"
              style={{ background: testResults.vectorStore.ok ? 'var(--good)' : 'var(--crit)' }}
            />
            <div className="test-info">
              <div className="comp-name">
                Vector Store <span className="mono">— {cfg.vectorStore}</span>
              </div>
              <div className="test-msg">{testResults.vectorStore.message}</div>
            </div>
            <span className={`latency ${testResults.vectorStore.ok ? 'ok' : ''}`}>
              {testResults.vectorStore.ok ? `${testResults.vectorStore.latency_ms}ms` : '—'}
            </span>
          </div>
        )}

        {/* Embedder result */}
        {testResults.embedder && (
          <div className={`test-result ${testResults.embedder.ok ? '' : 'fail'}`}>
            <span
              className="status-dot"
              style={{ background: testResults.embedder.ok ? 'var(--good)' : 'var(--crit)' }}
            />
            <div className="test-info">
              <div className="comp-name">
                Embedder <span className="mono">— {cfg.embedder === 'voyage' ? cfg.voyageModel : 'TEI'}</span>
              </div>
              <div className="test-msg">{testResults.embedder.message}</div>
            </div>
            <span className={`latency ${testResults.embedder.ok ? 'ok' : ''}`}>
              {testResults.embedder.ok ? `${testResults.embedder.latency_ms}ms` : '—'}
            </span>
          </div>
        )}

        {hasResults && !testPassed && (
          <div className="warn-box" style={{ marginTop: 14 }}>
            <XCircle size={16} className="warn-icon" />
            <div className="warn-text">
              <b>{passedCount} of {totalCount} components passed.</b> Fix the failing component and re-test,
              or proceed if the failing component is non-critical.
            </div>
          </div>
        )}

        {hasResults && testPassed && (
          <div className="success-box">
            <CheckCircle2 size={16} />
            <span>All components passed. You can proceed to the next step.</span>
          </div>
        )}
      </div>
      <div className="nav-buttons">
        <button className="btn" onClick={onBack}>
          <ChevronLeft size={14} /> Back
        </button>
        {hasResults && (
          <div className="gate-info">
            <span
              className="status-dot"
              style={{ background: testPassed ? 'var(--good)' : 'var(--warn)' }}
            />
            <span>{testPassed ? `${passedCount}/${totalCount} passed` : `${passedCount}/${totalCount} passed — proceed with override`}</span>
          </div>
        )}
        <div className="spacer" />
        <button
          className="btn primary"
          onClick={onNext}
          disabled={!hasResults}
        >
          {testPassed ? 'Next' : 'Proceed Anyway'} <ChevronRight size={14} />
        </button>
      </div>
    </div>
  )
}
