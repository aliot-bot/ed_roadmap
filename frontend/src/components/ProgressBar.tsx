import type { Progress } from '../types'

export function ProgressBar({ progress, compact = false }: { progress?: Progress; compact?: boolean }) {
  if (!progress || progress.total === 0 || progress.percent === null) {
    return <span className="progress-empty">Нет тем</span>
  }
  const percent = Math.max(0, Math.min(100, Math.round(progress.percent)))
  return (
    <div className={`progress ${compact ? 'compact' : ''}`}>
      <div className="progress-meta">
        <span>{progress.completed} из {progress.total}</span>
        <strong>{percent}%</strong>
      </div>
      <div className="progress-track" role="progressbar" aria-label={`Изучено ${progress.completed} из ${progress.total}, ${percent}%`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent}>
        <span style={{ width: `${percent}%` }} />
      </div>
    </div>
  )
}
