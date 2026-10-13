import { Dashboard } from '../components/Dashboard'

export function LearningPage({ login }: { login?: string }) {
  return (
    <div className="page learning-page">
      <Dashboard login={login} />
    </div>
  )
}
