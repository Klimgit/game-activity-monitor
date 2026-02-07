import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { intervalsApi, sessionsApi } from '../api'
import type { ActivityInterval } from '../types/api'

const STATE_COLORS: Record<string, string> = {
  active_gameplay: 'bg-green-500/20 text-green-300 border-green-500/30',
  afk:             'bg-slate-500/20 text-slate-300 border-slate-500/30',
  menu:            'bg-blue-500/20  text-blue-300  border-blue-500/30',
  loading:         'bg-yellow-500/20 text-yellow-300 border-yellow-500/30',
}

function StateBadge({ state }: { state: string }) {
  const cls = STATE_COLORS[state] ?? 'bg-slate-700 text-slate-300 border-slate-600'
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium border ${cls}`}>
      {state.replace('_', ' ')}
    </span>
  )
}

function durationSec(start: string, end: string): string {
  const s = (new Date(end).getTime() - new Date(start).getTime()) / 1000
  if (s < 0 || Number.isNaN(s)) return '—'
  return `${s.toFixed(1)}s`
}

export default function Labels() {
  const [selectedSession, setSelectedSession] = useState<number | undefined>(undefined)

  const { data: sessions = [] } = useQuery({
    queryKey: ['sessions', {}],
    queryFn: () => sessionsApi.list(),
  })

  const {
    data: intervals = [],
    isLoading,
    isError,
    refetch,
  } = useQuery({
    queryKey: ['intervals', selectedSession],
    queryFn: () => intervalsApi.list(selectedSession),
  })

  const stateCounts = intervals.reduce<Record<string, number>>((acc, iv) => {
    acc[iv.state] = (acc[iv.state] ?? 0) + 1
    return acc
  }, {})

  const fmt = (ts: string) =>
    new Date(ts).toLocaleString('en', {
      month: 'short', day: 'numeric',
      hour: '2-digit', minute: '2-digit', second: '2-digit',
      hour12: false,
    })

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold text-white">Activity intervals</h1>
        <button
          type="button"
          onClick={() => refetch()}
          className="text-xs text-slate-400 hover:text-slate-200 transition-colors"
        >
          Refresh
        </button>
      </div>

      <div className="card">
        <label className="block text-xs text-slate-400 mb-1">Filter by session</label>
        <select
          className="input text-sm w-full md:w-72"
          value={selectedSession ?? ''}
          onChange={(e) =>
            setSelectedSession(e.target.value ? Number(e.target.value) : undefined)
          }
        >
          <option value="">— All sessions —</option>
          {sessions.map((s) => (
            <option key={s.id} value={s.id}>
              #{s.id} · {s.game_name || 'unnamed'} ·{' '}
              {new Date(s.session_start).toLocaleDateString()}
            </option>
          ))}
        </select>
      </div>

      {intervals.length > 0 && (
        <div className="flex flex-wrap gap-3">
          {Object.entries(stateCounts).map(([state, count]) => (
            <div key={state} className="card flex items-center gap-2 py-2 px-3">
              <StateBadge state={state} />
              <span className="text-slate-300 font-semibold">{count}</span>
              <span className="text-slate-500 text-xs">intervals</span>
            </div>
          ))}
        </div>
      )}

      <div className="card overflow-x-auto p-0">
        {isLoading && (
          <p className="text-slate-400 text-sm p-6">Loading intervals…</p>
        )}
        {isError && (
          <p className="text-red-400 text-sm p-6">Failed to load intervals. Check server connection.</p>
        )}
        {!isLoading && !isError && intervals.length === 0 && (
          <div className="p-8 text-center text-slate-500">
            <p className="text-lg mb-1">No intervals yet</p>
            <p className="text-sm">
              Use the desktop client: start/end hotkeys Ctrl+Shift+1…8 during a session, or POST /api/v1/intervals.
            </p>
          </div>
        )}
        {!isLoading && !isError && intervals.length > 0 && (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-slate-400 uppercase tracking-wide border-b border-slate-700">
                <th className="px-4 py-3">Start</th>
                <th className="px-4 py-3">End</th>
                <th className="px-4 py-3">Δ</th>
                <th className="px-4 py-3">State</th>
                <th className="px-4 py-3">Source</th>
                <th className="px-4 py-3">Session</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-700/40">
              {(intervals as ActivityInterval[]).map((iv) => (
                <tr key={iv.id} className="hover:bg-slate-800/50 transition-colors">
                  <td className="px-4 py-3 font-mono text-xs text-slate-300">{fmt(iv.start_at)}</td>
                  <td className="px-4 py-3 font-mono text-xs text-slate-300">{fmt(iv.end_at)}</td>
                  <td className="px-4 py-3 text-slate-400 text-xs">{durationSec(iv.start_at, iv.end_at)}</td>
                  <td className="px-4 py-3">
                    <StateBadge state={iv.state} />
                  </td>
                  <td className="px-4 py-3 text-slate-400 text-xs">{iv.source}</td>
                  <td className="px-4 py-3 text-slate-400 text-xs">#{iv.session_id}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="card">
        <h3 className="text-sm font-medium text-slate-300 mb-3">Dev hotkeys (desktop client)</h3>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-2 text-xs text-slate-400">
          {[
            ['Ctrl+Shift+1 / 2', 'active gameplay start / end'],
            ['Ctrl+Shift+3 / 4', 'AFK start / end'],
            ['Ctrl+Shift+5 / 6', 'menu start / end'],
            ['Ctrl+Shift+7 / 8', 'loading start / end'],
          ].map(([key, desc]) => (
            <div key={key} className="flex items-center gap-2">
              <kbd className="px-1.5 py-0.5 bg-slate-700 rounded text-slate-300 font-mono">{key}</kbd>
              <span>{desc}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
