import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import type { LearningSession } from '../types'
import { Icon } from './Icon'

function seconds(session: LearningSession, now: number) {
  const saved = session.elapsed_seconds ?? 0
  if (session.status !== 'active') return saved
  const activeSince = Date.parse(session.updated_at ?? session.started_at)
  return saved + (Number.isFinite(activeSince) ? Math.max(0, Math.floor((now - activeSince) / 1000)) : 0)
}
export function LearningTimer() {
  const queryClient = useQueryClient(); const active = useQuery({ queryKey: ['learning-active'], queryFn: api.getActiveLearning, staleTime: 10_000 }); const [now, setNow] = useState(() => Date.now())
  const action = useMutation({ mutationFn: ({ id, type }: { id: string; type: 'pause' | 'resume' | 'finish' }) => type === 'pause' ? api.pauseLearning(id) : type === 'resume' ? api.resumeLearning(id) : api.finishLearning(id), onSuccess: () => { void queryClient.invalidateQueries({ queryKey: ['learning-active'] }); void queryClient.invalidateQueries({ queryKey: ['learning-overview'] }); void queryClient.invalidateQueries({ queryKey: ['learning-activity'] }) } })
  useEffect(() => { if (active.data?.status !== 'active') return; const timer = window.setInterval(() => setNow(Date.now()), 1000); return () => window.clearInterval(timer) }, [active.data?.status])
  const session = active.data; const label = useMemo(() => session ? `${Math.floor(seconds(session, now) / 60).toString().padStart(2, '0')}:${(seconds(session, now) % 60).toString().padStart(2, '0')}` : '', [session, now])
  if (!session || active.isError) return null
  return <aside className="learning-timer" aria-label="Текущая учебная сессия"><span className="timer-dot" aria-hidden="true" /><strong>{label}</strong><span className="timer-title">Занятие</span><div className="timer-actions"><button className="icon-button" onClick={() => action.mutate({ id: session.id, type: session.status === 'active' ? 'pause' : 'resume' })} aria-label={session.status === 'active' ? 'Пауза' : 'Продолжить'}><Icon name={session.status === 'active' ? 'pause' : 'play'} /></button><button className="icon-button" onClick={() => action.mutate({ id: session.id, type: 'finish' })} aria-label="Завершить занятие"><Icon name="check" /></button></div></aside>
}
