import { useState } from 'react'
import type { SessionFilters } from '../types/api'

export default function Export() {
  const [filters, setFilters] = useState<SessionFilters>({})

  const buildQuery = () => {
    const params = new URLSearchParams()
    if (filters.from) params.set('from', filters.from)
    if (filters.to) params.set('to', filters.to)
    if (filters.game) params.set('game', filters.game)
    return params.toString()
  }

  const download = (format: 'csv' | 'json') => {
    const token = localStorage.getItem('token')
    const query = buildQuery()
    const url = `/api/v1/export/${format}${query ? `?${query}` : ''}`

    // Use a hidden anchor with Authorization header via fetch → blob URL.
    fetch(url, { headers: { Authorization: `Bearer ${token}` } })
      .then(async (res) => {
        if (!res.ok) {
          const body = await res.json().catch(() => ({ error: 'unknown error' }))
          alert(`Export failed: ${(body as { error: string }).error}`)
          return
        }
        const blob = await res.blob()
        const blobUrl = URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = blobUrl
        const from = filters.from ?? 'all'
        const to = filters.to ?? 'all'
        a.download = `game-activity-${from}-${to}.${format}`
        a.click()
        URL.revokeObjectURL(blobUrl)
      })
      .catch((err: unknown) => alert(`Network error: ${String(err)}`))
  }

  return (
    <div className="space-y-6">
      <h1 className="text-xl font-semibold text-white">Export Data</h1>

      {/* Notice */}
      <div className="card border-yellow-500/30 bg-yellow-500/5">
        <div className="flex gap-3">
          <span className="text-yellow-400 text-lg">⚠️</span>
          <div>
            <p className="text-sm font-medium text-yellow-400">Export endpoints not yet implemented</p>
            <p className="text-sm text-slate-400 mt-1">
              The server currently returns 501. The UI is ready — add the CSV/JSON generation
              logic to <code className="text-slate-300">server/internal/api/handlers/export.go</code>.
            </p>
          </div>
        </div>
      </div>

      {/* Filters */}
      <div className="card">
        <h2 className="text-sm font-medium text-slate-300 mb-4">Filter</h2>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div>
            <label className="block text-xs text-slate-400 mb-1">From</label>
            <input
              type="date"
              className="input text-sm"
              value={filters.from ?? ''}
              onChange={(e) => setFilters((f) => ({ ...f, from: e.target.value || undefined }))}
            />
          </div>
          <div>
            <label className="block text-xs text-slate-400 mb-1">To</label>
            <input
              type="date"
              className="input text-sm"
              value={filters.to ?? ''}
              onChange={(e) => setFilters((f) => ({ ...f, to: e.target.value || undefined }))}
            />
          </div>
          <div>
            <label className="block text-xs text-slate-400 mb-1">Game</label>
            <input
              type="text"
              className="input text-sm"
              placeholder="All games"
              value={filters.game ?? ''}
              onChange={(e) => setFilters((f) => ({ ...f, game: e.target.value || undefined }))}
            />
          </div>
        </div>
      </div>

      {/* Export buttons */}
      <div className="card">
        <h2 className="text-sm font-medium text-slate-300 mb-4">Download</h2>
        <div className="flex flex-wrap gap-4">
          <button onClick={() => download('csv')} className="btn-primary flex items-center gap-2">
            <span>📄</span> Export CSV
          </button>
          <button onClick={() => download('json')} className="btn-secondary flex items-center gap-2">
            <span>📋</span> Export JSON
          </button>
        </div>
        <p className="mt-3 text-xs text-slate-500">
          Exports include session summaries with durations, activity scores, and labelled states.
        </p>
      </div>

      {/* Schema reference */}
      <div className="card">
        <h2 className="text-sm font-medium text-slate-300 mb-3">CSV Column Reference</h2>
        <div className="overflow-x-auto">
          <table className="text-xs w-full">
            <thead>
              <tr className="text-slate-400 border-b border-slate-700">
                <th className="text-left py-2 pr-4">Column</th>
                <th className="text-left py-2">Description</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-700/40 text-slate-300">
              {[
                ['session_id', 'Unique session identifier'],
                ['session_start', 'ISO 8601 timestamp'],
                ['session_end', 'ISO 8601 timestamp (empty if still active)'],
                ['game_name', 'Detected game or process name'],
                ['total_duration_s', 'Total session time in seconds'],
                ['active_duration_s', 'Time with active input in seconds'],
                ['afk_duration_s', 'AFK time in seconds'],
                ['activity_score', 'Activity ratio 0.0–1.0'],
                ['state', 'Final session state'],
              ].map(([col, desc]) => (
                <tr key={col}>
                  <td className="py-2 pr-4 font-mono text-blue-300">{col}</td>
                  <td className="py-2 text-slate-400">{desc}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
