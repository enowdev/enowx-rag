import { useState, useCallback, useEffect } from 'react'
import { Sidebar } from './components/Sidebar'
import { Topbar } from './components/Topbar'
import { Overview } from './pages/Overview'
import { Migration } from './pages/Migration'
import { Docs } from './pages/Docs'
import { Playground } from './pages/Playground'
import { Chunks } from './pages/Chunks'
import { Setup } from './pages/Setup'
import { Wizard } from './pages/onboarding/Wizard'
import { useTheme } from './lib/useTheme'
import { api } from './lib/api'

export type Page = 'overview' | 'playground' | 'chunks' | 'migration' | 'docs' | 'setup'

export interface ProjectInfo {
  projectID: string
  chunkCount: number
}

type AppState = 'checking' | 'wizard' | 'dashboard'

function App() {
  const { theme, toggleTheme } = useTheme()
  const [appState, setAppState] = useState<AppState>('checking')
  const [page, setPage] = useState<Page>('overview')
  const [projects, setProjects] = useState<ProjectInfo[]>([])
  const [activeProject, setActiveProject] = useState<string>('')
  // Shared query state: VAL-CROSS-010 requires that the query entered in the
  // Overview playground persists when navigating to the full Playground page.
  const [sharedQuery, setSharedQuery] = useState('')

  // First-run detection: check if config exists on load.
  // If no config, show wizard. If config exists, show dashboard.
  useEffect(() => {
    let cancelled = false
    api.setupStatus()
      .then((status) => {
        if (cancelled) return
        setAppState(status.configured ? 'dashboard' : 'wizard')
      })
      .catch(() => {
        if (cancelled) return
        // If we can't reach the API, default to dashboard
        // (the server might not have setup endpoints, or it's a dev issue)
        setAppState('dashboard')
      })
    return () => { cancelled = true }
  }, [])

  const handleWizardComplete = useCallback(() => {
    setAppState('dashboard')
    setPage('overview')
  }, [])

  const handleSelectProject = useCallback((id: string) => {
    setActiveProject(id)
    setPage('overview')
  }, [])

  const handleNavigate = useCallback((p: Page) => {
    setPage(p)
  }, [])

  // VAL-CROSS-010: Navigate to Playground while preserving the query from
  // the Overview playground.
  const handleNavigateWithQuery = useCallback((p: Page, query: string) => {
    setSharedQuery(query)
    setPage(p)
  }, [])

  const handleProjectsLoaded = useCallback((projs: ProjectInfo[]) => {
    setProjects(projs)
    if (!activeProject && projs.length > 0) {
      setActiveProject(projs[0].projectID)
    }
  }, [activeProject])

  // Show wizard on first run (no config)
  if (appState === 'wizard') {
    return <Wizard onComplete={handleWizardComplete} theme={theme} onToggleTheme={toggleTheme} />
  }

  // Loading state
  if (appState === 'checking') {
    return (
      <div className="app-loading">
        <div className="brand-mark mono">e</div>
        <span>Loading…</span>
      </div>
    )
  }

  return (
    <div className="app">
      <Sidebar
        page={page}
        onNavigate={handleNavigate}
        projects={projects}
        activeProject={activeProject}
        onSelectProject={handleSelectProject}
        onProjectsLoaded={handleProjectsLoaded}
      />
      <div className="main">
        <Topbar theme={theme} onToggleTheme={toggleTheme} activeProject={activeProject} page={page} />
        <div className="content">
          {page === 'overview' && <Overview activeProject={activeProject} onNavigate={handleNavigate} onNavigateWithQuery={handleNavigateWithQuery} sharedQuery={sharedQuery} onSharedQueryChange={setSharedQuery} onProjectsUpdated={setProjects} />}
          {page === 'playground' && <Playground activeProject={activeProject} sharedQuery={sharedQuery} onSharedQueryChange={setSharedQuery} />}
          {page === 'chunks' && <Chunks activeProject={activeProject} />}
          {page === 'migration' && <Migration activeProject={activeProject} projects={projects} />}
          {page === 'docs' && <Docs />}
          {page === 'setup' && <Setup />}
        </div>
      </div>
    </div>
  )
}

export default App
