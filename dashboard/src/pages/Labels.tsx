import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { labelsApi, sessionsApi } from '../api'
import type { ActivityLabel } from '../types/api'

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

export default function Labels() {
  const [selectedSession, setSelectedSession] = useState<number | undefined>(undefined)

  // ── data ──────────────────────────────────────────────────────────────────
  const { data: sessions = [] } = useQuery({
    queryKey: ['sessions', {}],
    queryFn: () => sessionsApi.list(),
  })

  const {
    data: labels = [],
    isLoading,
    isError,
    refetch,
  } = useQuery({
    queryKey: ['labels', selectedSession],
    queryFn: () => labelsApi.list(selectedSession),
  })

  // ── helpers ───────────────────────────────────────────────────────────────
  const stateCounts = labels.reduce<Record<string, number>>((acc, l) => {
    acc[l.state] = (acc[l.state] ?? 0) + 1
    return acc
  }, {})

  const fmt = (ts: string) =>
    new Date(ts).toLocaleString('en', {
      month: 'short', day: 'numeric',
      hour: '2-digit', minute: '2-digit', second: '2-digit',
      hour12: false,
    })

  // ── render ────────────────────────────────────────────────────────────────
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold text-white">Activity Labels</h1>
        <button
          onClick={() => refetch()}
          className="text-xs text-slate-400 hover:text-slate-200 transition-colors"
        >
          Refresh
        </button>
      </div>

      {/* ── Session filter ──────────────────────────────────────────────── */}
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

      {/* ── Summary badges ──────────────────────────────────────────────── */}
      {labels.length > 0 && (
        <div className="flex flex-wrap gap-3">
          {Object.entries(stateCounts).map(([state, count]) => (
            <div key={state} className="card flex items-center gap-2 py-2 px-3">
              <StateBadge state={state} />
              <span className="text-slate-300 font-semibold">{count}</span>
              <span className="text-slate-500 text-xs">labels</span>
            </div>
          ))}
        </div>
      )}

      {/* ── Table ───────────────────────────────────────────────────────── */}
      <div className="card overflow-x-auto p-0">
        {isLoading && (
          <p className="text-slate-400 text-sm p-6">Loading labels…</p>
        )}
        {isError && (
          <p className="text-red-400 text-sm p-6">Failed to load labels. Check server connection.</p>
        )}
        {!isLoading && !isError && labels.length === 0 && (
          <div className="p-8 text-center text-slate-500">
            <p className="text-lg mb-1">No labels yet</p>
            <p className="text-sm">
              Use the client hotkeys (Ctrl+Shift+A/F/M/L) during a session to annotate activity.
            </p>
          </div>
        )}
        {!isLoading && !isError && labels.length > 0 && (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-slate-400 uppercase tracking-wide border-b border-slate-700">
                <th className="px-4 py-3">Time</th>
                <th className="px-4 py-3">State</th>
                <th className="px-4 py-3">Source</th>
                <th className="px-4 py-3">Session</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-700/40">
              {(labels as ActivityLabel[]).map((l) => (
                <tr key={l.id} className="hover:bg-slate-800/50 transition-colors">
                  <td className="px-4 py-3 font-mono text-xs text-slate-300">
                    {fmt(l.timestamp)}
                  </td>
                  <td className="px-4 py-3">
                    <StateBadge state={l.state} />
                  </td>
                  <td className="px-4 py-3 text-slate-400 text-xs">{l.source}</td>
                  <td className="px-4 py-3 text-slate-400 text-xs">
                    {l.session_id != null ? `#${l.session_id}` : '—'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* ── Hotkey reference ────────────────────────────────────────────── */}
      <div className="card">
        <h3 className="text-sm font-medium text-slate-300 mb-3">Label Hotkeys</h3>
        <div className="grid grid-cols-2 md:grid-cols-4 gap-2 text-xs text-slate-400">
          {[
            ['Ctrl+Shift+A', 'active gameplay'],
            ['Ctrl+Shift+F', 'AFK'],
            ['Ctrl+Shift+M', 'in menu'],
            ['Ctrl+Shift+L', 'loading'],
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
