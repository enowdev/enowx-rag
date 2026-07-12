import { Moon, Sun } from 'lucide-react'
import type { Page } from '../App'

interface TopbarProps {
  theme: 'light' | 'dark'
  onToggleTheme: () => void
  activeProject: string
  page: Page
}

const pageLabels: Record<Page, string> = {
  overview: 'Overview',
  playground: 'Playground',
  chunks: 'Chunks',
  setup: 'Setup',
}

export function Topbar({ theme, onToggleTheme, activeProject, page }: TopbarProps) {
  return (
    <div className="topbar">
      <div className="crumb">
        <span>Projects</span>
        <span className="sep">/</span>
        <b className="mono">{activeProject || '—'}</b>
        <span className="sep">/</span>
        <span>{pageLabels[page]}</span>
      </div>
      <div className="topbar-spacer" />
      <button className="icon-btn" onClick={onToggleTheme} title="Toggle theme">
        {theme === 'dark' ? <Sun size={15} strokeWidth={1.7} /> : <Moon size={15} strokeWidth={1.7} />}
      </button>
    </div>
  )
}
