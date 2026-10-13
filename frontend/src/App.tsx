import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, api } from './api'
import { Icon } from './components/Icon'
import { ThemeToggle } from './components/ThemeToggle'
import { LoginPage } from './pages/LoginPage'
import { RoadmapPage } from './pages/RoadmapPage'
import { RoadmapsPage } from './pages/RoadmapsPage'
import { LearningPage } from './pages/LearningPage'
import { LearningTimer } from './components/LearningTimer'
import { readRoute, type Route } from './route'

export function App() {
  const queryClient = useQueryClient()
  const [route, setRoute] = useState(readRoute)
  const session = useQuery({ queryKey: ['me'], queryFn: api.me, retry: false })
  const logout = useMutation({
    mutationFn: api.logout,
    onSettled: () => {
      queryClient.clear()
      window.history.replaceState({}, '', '/')
      setRoute({ page: 'roadmaps' })
    },
  })

  useEffect(() => {
    const onPopState = () => setRoute(readRoute())
    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  }, [])

  useEffect(() => {
    window.scrollTo(0, 0)
  }, [route])

  const navigate = (next: Route) => {
    const path = next.page === 'roadmaps' ? '/' : next.page === 'learning' ? '/learning' : `/roadmaps/${encodeURIComponent(next.id)}`
    window.history.pushState({}, '', path)
    setRoute(next)
  }

  if (session.isPending) return <FullPageStatus label="Открываем вашу карту…" />
  if (session.isError && session.error instanceof ApiError && session.error.status === 401) {
    return <LoginPage onSuccess={() => void session.refetch()} />
  }
  if (session.isError) {
    return <FullPageStatus label="Не удалось связаться с сервером" action="Повторить" onAction={() => void session.refetch()} />
  }

  return (
    <div className="app-shell">
      <header className="app-header">
        <button className="brand" onClick={() => navigate({ page: 'roadmaps' })} aria-label="Перейти к списку карт">
          <span className="brand-mark" aria-hidden="true"><Icon name="map" /></span>
          <span className="brand-copy"><strong>Карты знаний</strong><small>Личное пространство</small></span>
        </button>
        <div className="account-menu">
          <span className="account-avatar" aria-hidden="true">{session.data?.login?.slice(0, 1).toUpperCase() || 'U'}</span>
          <span className="account-name">{session.data?.login}</span>
        </div>
      </header>
      <aside className="app-rail" aria-label="Основная навигация">
        <button className={`rail-button ${route.page === 'roadmaps' ? 'active' : ''}`} onClick={() => navigate({ page: 'roadmaps' })} aria-label="Мои карты"><Icon name="grid" /></button>
        <button className={`rail-button ${route.page === 'learning' ? 'active' : ''}`} onClick={() => navigate({ page: 'learning' })} aria-label="Обзор обучения"><Icon name="sparkles" /></button>
        <button className={`rail-button ${route.page === 'roadmap' ? 'active' : ''}`} disabled={route.page !== 'roadmap'} aria-label="Открытая карта"><Icon name="graph" /></button>
        <span className="rail-spacer" />
        <ThemeToggle />
        <button className="icon-button rail-button" onClick={() => logout.mutate()} disabled={logout.isPending} aria-label="Выйти"><Icon name="logout" /></button>
      </aside>
      <main className="app-main">
        {route.page === 'roadmaps' ? (
          <RoadmapsPage onOpen={(id) => navigate({ page: 'roadmap', id })} />
        ) : route.page === 'learning' ? (
          <LearningPage login={session.data?.display_name || session.data?.login} />
        ) : (
          <RoadmapPage roadmapId={route.id} onBack={() => navigate({ page: 'roadmaps' })} />
        )}
      </main>
      <LearningTimer />
    </div>
  )
}

function FullPageStatus({ label, action, onAction }: { label: string; action?: string; onAction?: () => void }) {
  return (
    <main className="full-page-status" role="status">
      <span className="brand-mark large" aria-hidden="true"><Icon name="map" /></span>
      <p>{label}</p>
      {action && <button className="button secondary" onClick={onAction}>{action}</button>}
    </main>
  )
}
