import type { Analytics } from '../types'

export function AnalyticsSummary({ analytics, isLoading, isError }: { analytics?: Analytics; isLoading: boolean; isError: boolean }) {
  if (isLoading) return <section className="analytics-summary analytics-loading" aria-label="Аналитика" role="status">Загрузка аналитики…</section>
  if (isError || !analytics) return <section className="analytics-summary analytics-error" aria-label="Аналитика"><strong>Сводка временно недоступна</strong><span>Карты по-прежнему можно открывать и редактировать.</span></section>
  const active = analytics.roadmaps.filter((item) => item.total > 0 && item.remaining > 0)
  const meaningful = analytics.roadmaps.filter((item) => item.total > 0 && item.remaining <= 0)
  const empty = analytics.roadmaps.filter((item) => item.total <= 0)
  const roadmaps = [...active, ...meaningful, ...empty].slice(0, 3)
  return (
    <section className="analytics-summary" aria-labelledby="analytics-title">
      <div className="analytics-heading"><div><p className="eyebrow">Сводка</p><h2 id="analytics-title">Прогресс</h2></div><span className="analytics-percent">{analytics.percent}%</span></div>
      <div className="analytics-metrics">
        <div><strong>{analytics.total}</strong><span>Всего тем</span></div>
        <div><strong>{analytics.completed}</strong><span>Изучено</span></div>
        <div><strong>{analytics.remaining}</strong><span>Осталось</span></div>
      </div>
      <div className="analytics-track" role="progressbar" aria-label={`Общий прогресс ${analytics.percent}%`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.max(0, Math.min(100, analytics.percent))}><span style={{ width: `${Math.max(0, Math.min(100, analytics.percent))}%` }} /></div>
      {roadmaps.length > 0 && <div className="analytics-roadmaps">{roadmaps.map((item) => <div className="analytics-roadmap" key={item.roadmap_id || item.title}><div><strong>{item.title || 'Карта'}</strong><span>{item.completed} из {item.total}</span></div><div className="analytics-roadmap-track"><span style={{ width: `${Math.max(0, Math.min(100, item.percent))}%` }} /></div></div>)}</div>}
    </section>
  )
}
