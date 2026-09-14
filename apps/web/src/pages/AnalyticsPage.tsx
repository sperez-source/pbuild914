import { useQuery } from '@tanstack/react-query'
import { Card, Spinner } from '@meridian/ui-kit'
import { useAuth } from '../context/AuthContext'
import './AnalyticsPage.css'

export default function AnalyticsPage() {
  const { client } = useAuth()

  const { data, isLoading, error } = useQuery({
    queryKey: ['analytics'],
    queryFn: () => client.getAnalyticsOverview(),
  })

  if (isLoading) return <Spinner />
  if (error) return <p className="error">Failed to load analytics</p>
  if (!data) return null

  const statusEntries = Object.entries(data.tasks_by_status)

  return (
    <div className="analytics-page">
      <h1>Analytics Overview</h1>

      <div className="analytics-grid">
        <Card>
          <div className="stat-value">{data.total_projects}</div>
          <div className="stat-label">Projects</div>
        </Card>
        <Card>
          <div className="stat-value">{data.total_tasks}</div>
          <div className="stat-label">Tasks</div>
        </Card>
        <Card>
          <div className="stat-value">{data.total_comments}</div>
          <div className="stat-label">Comments</div>
        </Card>
        <Card>
          <div className="stat-value">{data.total_teams}</div>
          <div className="stat-label">Teams</div>
        </Card>
      </div>

      <Card className="analytics-chart">
        <h2>Tasks by Status</h2>
        <div className="bar-chart">
          {statusEntries.map(([status, count]) => {
            const max = Math.max(...statusEntries.map(([, c]) => c), 1)
            const pct = (count / max) * 100
            return (
              <div key={status} className="bar-row">
                <span className="bar-label">{status.replace('_', ' ')}</span>
                <div className="bar-track">
                  <div className={`bar-fill bar-${status}`} style={{ width: `${pct}%` }} />
                </div>
                <span className="bar-count">{count}</span>
              </div>
            )
          })}
        </div>
      </Card>
    </div>
  )
}