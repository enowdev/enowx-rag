import { useEffect, useState } from 'react'
import { LayoutGrid, Search, List, Settings } from 'lucide-react'
import type { Page, ProjectInfo } from '../App'
import { api } from '../lib/api'

interface SidebarProps {
  page: Page
  onNavigate: (p: Page) => void
  projects: ProjectInfo[]
  activeProject: string
  onSelectProject: (id: string) => void
  onProjectsLoaded: (projs: ProjectInfo[]) => void
}

const navItems: { label: string; page: Page; icon: typeof LayoutGrid }[] = [
  { label: 'Overview', page: 'overview', icon: LayoutGrid },
  { label: 'Playground', page: 'playground', icon: Search },
  { label: 'Chunks', page: 'chunks', icon: List },
  { label: 'Setup', page: 'setup', icon: Settings },
]

export function Sidebar({ page, onNavigate, projects, activeProject, onSelectProject, onProjectsLoaded }: SidebarProps) {
  const [localProjects, setLocalProjects] = useState<ProjectInfo[]>(projects)

  useEffect(() => {
    let cancelled = false
    api.listProjects().then((stats) => {
      if (cancelled) return
      const projs: ProjectInfo[] = stats.map((s) => ({ projectID: s.project_id, chunkCount: s.chunk_count }))
      setLocalProjects(projs)
      onProjectsLoaded(projs)
    }).catch(() => {
      // API not available yet, use mock data for UI rendering
      if (localProjects.length === 0) {
        const mock: ProjectInfo[] = [
          { projectID: 'enowx-rag', chunkCount: 76 },
          { projectID: 'robloxkit', chunkCount: 2140 },
          { projectID: 'enowxreality', chunkCount: 1883 },
          { projectID: 'reality-client-rs', chunkCount: 642 },
          { projectID: 'antaresban', chunkCount: 508 },
          { projectID: 'pixelify', chunkCount: 431 },
          { projectID: 'enowxai', chunkCount: 377 },
          { projectID: 'enowx-discord', chunkCount: 294 },
          { projectID: 'reality-auto-login', chunkCount: 210 },
        ]
        setLocalProjects(mock)
        onProjectsLoaded(mock)
      }
    })
    return () => { cancelled = true }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const displayProjects = localProjects.length > 0 ? localProjects : projects

  return (
    <aside className="sidebar">
      <div className="brand">
        <div className="brand-mark mono">e</div>
        <div className="brand-name">enowx<span>·rag</span></div>
      </div>

      {navItems.map((item) => {
        const Icon = item.icon
        return (
          <div
            key={item.page}
            className={`nav-item ${page === item.page ? 'active' : ''}`}
            onClick={() => onNavigate(item.page)}
          >
            <Icon size={15} strokeWidth={1.6} />
            {item.label}
          </div>
        )
      })}

      <div className="nav-label">Projects</div>
      <div className="proj-list">
        {displayProjects.map((p) => (
          <div
            key={p.projectID}
            className={`proj ${activeProject === p.projectID ? 'active' : ''}`}
            onClick={() => onSelectProject(p.projectID)}
          >
            <span className="status-dot" style={{ background: 'var(--good)' }} />
            <span className="proj-name mono">{p.projectID}</span>
            <span className="count tnum">{p.chunkCount.toLocaleString()}</span>
          </div>
        ))}
      </div>

      <div className="sidebar-foot">
        <div className="nav-item">
          <Settings size={15} strokeWidth={1.6} />
          Settings
        </div>
      </div>
    </aside>
  )
}
