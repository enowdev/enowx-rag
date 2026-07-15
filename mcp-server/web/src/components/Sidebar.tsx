import { useEffect, useState, useCallback } from 'react'
import { LayoutGrid, Search, List, Settings, ArrowRightLeft, BookOpen } from 'lucide-react'
import type { Page, ProjectInfo } from '../App'
import { api } from '../lib/api'
import { useEvents } from '../lib/sse'

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
  { label: 'Migration', page: 'migration', icon: ArrowRightLeft },
  { label: 'Docs', page: 'docs', icon: BookOpen },
  { label: 'Setup', page: 'setup', icon: Settings },
]

export function Sidebar({ page, onNavigate, projects, activeProject, onSelectProject, onProjectsLoaded }: SidebarProps) {
  const [localProjects, setLocalProjects] = useState<ProjectInfo[]>(projects)
  const { events } = useEvents()

  const fetchProjects = useCallback(() => {
    api.listProjects().then((stats) => {
      const projs: ProjectInfo[] = stats.map((s) => ({ projectID: s.project_id, chunkCount: s.chunk_count }))
      setLocalProjects(projs)
      onProjectsLoaded(projs)
    }).catch(() => {
      // API unavailable: show an empty project list rather than fabricated data.
      setLocalProjects([])
      onProjectsLoaded([])
    })
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    let cancelled = false
    api.listProjects().then((stats) => {
      if (cancelled) return
      const projs: ProjectInfo[] = stats.map((s) => ({ projectID: s.project_id, chunkCount: s.chunk_count }))
      setLocalProjects(projs)
      onProjectsLoaded(projs)
    }).catch(() => {
      if (cancelled) return
      // API unavailable: show an empty project list rather than fabricated data.
      setLocalProjects([])
      onProjectsLoaded([])
    })
    return () => { cancelled = true }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // VAL-CROSS-011, VAL-CROSS-012: Refresh project list when SSE events
  // indicate a change in projects or chunk counts.
  useEffect(() => {
    if (events.length === 0) return
    const latest = events[0]
    if (latest.type === 'index_completed' || latest.type === 'project_deleted' || latest.type === 'project_created' || latest.type === 'points_deleted' || latest.type === 'documents_indexed' || latest.type === 'migration_completed') {
      fetchProjects()
    }
  }, [events, fetchProjects])

  const displayProjects = localProjects.length > 0 ? localProjects : projects

  return (
    <aside className="sidebar">
      <div className="brand">
        <div className="brand-mark">
          <img className="brand-img brand-img-light" src="/icon-light-512.png" alt="enowx-rag" />
          <img className="brand-img brand-img-dark" src="/icon-dark-512.png" alt="" aria-hidden="true" />
        </div>
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
        {displayProjects.length === 0 ? (
          <div className="proj-empty">No projects indexed yet.</div>
        ) : (
          displayProjects.map((p) => (
            <div
              key={p.projectID}
              className={`proj ${activeProject === p.projectID ? 'active' : ''}`}
              onClick={() => onSelectProject(p.projectID)}
            >
              <span className="status-dot" style={{ background: 'var(--good)' }} />
              <span className="proj-name mono">{p.projectID}</span>
              <span className="count tnum">{p.chunkCount.toLocaleString()}</span>
            </div>
          ))
        )}
      </div>
    </aside>
  )
}
