import { useState, useCallback, useEffect } from 'react'
import { Sun, Moon } from 'lucide-react'
import { STEPS, STEP_LABELS, defaultDraft, type Step, type DraftConfig } from './types'
import { StepWelcome } from './StepWelcome'
import { StepVectorStore } from './StepVectorStore'
import { StepEmbedding } from './StepEmbedding'
import { StepTest } from './StepTest'
import { StepAutoSetup } from './StepAutoSetup'
import { StepInstall } from './StepInstall'
import { StepDone } from './StepDone'

interface WizardProps {
  onComplete: () => void
  theme: 'light' | 'dark'
  onToggleTheme: () => void
}

export function Wizard({ onComplete, theme, onToggleTheme }: WizardProps) {
  const [step, setStep] = useState<Step>(loadStep)
  const [cfg, setCfg] = useState<DraftConfig>(loadDraft)
  const [testResults, setTestResults] = useState<{
    vectorStore: { ok: boolean; message: string; latency_ms: number } | null
    embedder: { ok: boolean; message: string; latency_ms: number } | null
  }>({ vectorStore: null, embedder: null })

  // Persist full draft config to localStorage on every change (VAL-WIZ-033).
  useEffect(() => {
    try {
      localStorage.setItem('wizard-draft', JSON.stringify(cfg))
    } catch {
      // ignore
    }
  }, [cfg])

  // Persist step position to localStorage on every change (VAL-WIZ-033).
  useEffect(() => {
    try {
      localStorage.setItem('wizard-step', step)
    } catch {
      // ignore
    }
  }, [step])

  const stepIndex = STEPS.indexOf(step)

  const next = useCallback(() => {
    if (stepIndex < STEPS.length - 1) {
      setStep(STEPS[stepIndex + 1])
    }
  }, [stepIndex])

  const back = useCallback(() => {
    if (stepIndex > 0) {
      setStep(STEPS[stepIndex - 1])
    }
  }, [stepIndex])

  const updateCfg = useCallback((patch: Partial<DraftConfig>) => {
    setCfg((prev) => ({ ...prev, ...patch }))
  }, [])

  // Check if test step can proceed: vector store and embedder must pass.
  const testPassed =
    testResults.vectorStore?.ok === true && testResults.embedder?.ok === true

  return (
    <div className="wizard-shell">
      {/* Top bar */}
      <div className="wizard-topbar">
        <div className="brand">
          <div className="brand-mark">
            <img className="brand-img brand-img-light" src="/icon-light-512.png" alt="enowx-rag" />
            <img className="brand-img brand-img-dark" src="/icon-dark-512.png" alt="" aria-hidden="true" />
          </div>
          <div className="brand-name">enowx<span>·rag</span></div>
        </div>
        <span className="wizard-subtitle">Setup Wizard</span>
        <div className="topbar-spacer" />
        <button className="icon-btn" onClick={onToggleTheme} title="Toggle theme">
          {theme === 'dark' ? <Sun size={15} strokeWidth={1.7} /> : <Moon size={15} strokeWidth={1.7} />}
        </button>
      </div>

      <div className="wizard-container">
        {/* Step indicator */}
        <div className="stepper">
          {STEPS.map((s, i) => (
            <div key={s} className={`step-group ${i === stepIndex ? 'current' : ''} ${i < stepIndex ? 'completed' : ''}`}>
              {i > 0 && <div className={`step-connector ${i <= stepIndex ? 'completed' : ''}`} />}
              <div className="step-item">
                <div className="step-circle">{i + 1}</div>
                <span className="step-label">{STEP_LABELS[s]}</span>
              </div>
            </div>
          ))}
        </div>

        {/* Step content */}
        {step === 'welcome' && (
          <StepWelcome onNext={next} />
        )}
        {step === 'vector' && (
          <StepVectorStore
            cfg={cfg}
            updateCfg={updateCfg}
            onBack={back}
            onNext={next}
          />
        )}
        {step === 'embedding' && (
          <StepEmbedding
            cfg={cfg}
            updateCfg={updateCfg}
            onBack={back}
            onNext={next}
          />
        )}
        {step === 'test' && (
          <StepTest
            cfg={cfg}
            testResults={testResults}
            setTestResults={setTestResults}
            testPassed={testPassed}
            onBack={back}
            onNext={next}
          />
        )}
        {step === 'setup' && (
          <StepAutoSetup
            cfg={cfg}
            onBack={back}
            onNext={next}
          />
        )}
        {step === 'install' && (
          <StepInstall
            onBack={back}
            onNext={next}
          />
        )}
        {step === 'done' && (
          <StepDone
            cfg={cfg}
            onBack={back}
            onComplete={onComplete}
          />
        )}
      </div>
    </div>
  )
}

function loadStep(): Step {
  try {
    const stored = localStorage.getItem('wizard-step')
    if (stored && (STEPS as readonly string[]).includes(stored)) {
      return stored as Step
    }
  } catch {
    // ignore
  }
  return 'welcome'
}

function loadDraft(): DraftConfig {
  try {
    const stored = localStorage.getItem('wizard-draft')
    if (stored) {
      return { ...defaultDraft, ...JSON.parse(stored) }
    }
  } catch {
    // ignore
  }
  return defaultDraft
}
