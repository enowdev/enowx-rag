import { useState, useCallback } from 'react'
import { Sidebar } from './components/Sidebar'
import { Topbar } from './components/Topbar'
import { Overview } from './pages/Overview'
import { Playground } from './pages/Playground'
import { Chunks } from './pages/Chunks'
import { Setup } from './pages/Setup'
import { useTheme } from './lib/useTheme'

export type Page = 'overview' | 'playground' | 'chunks' | 'setup'

export interface ProjectInfo {
  projectID: string
  chunkCount: number
}

function App() {
  const { theme, toggleTheme } = useTheme()
  const [page, setPage] = useState<Page>('overview')
  const [projects, setProjects] = useState<ProjectInfo[]>([])
  const [activeProject, setActiveProject] = useState<string>('')

  const handleSelectProject = useCallback((id: string) => {
    setActiveProject(id)
    setPage('overview')
  }, [])

  const handleNavigate = useCallback((p: Page) => {
    setPage(p)
  }, [])

  const handleProjectsLoaded = useCallback((projs: ProjectInfo[]) => {
    setProjects(projs)
    if (!activeProject && projs.length > 0) {
      setActiveProject(projs[0].projectID)
    }
  }, [activeProject])

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
          {page === 'overview' && <Overview activeProject={activeProject} onNavigate={handleNavigate} />}
          {page === 'playground' && <Playground activeProject={activeProject} />}
          {page === 'chunks' && <Chunks activeProject={activeProject} />}
          {page === 'setup' && <Setup />}
        </div>
      </div>
    </div>
  )
}

export default App
