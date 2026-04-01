import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { format, parseISO } from 'date-fns'
import { sessionsApi } from '../api'
import type { Session, SessionFilters } from '../types/api'

function fmtDuration(sec: number) {
  if (sec <= 0) return '—'
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  if (h > 0) return `${h}h ${m}m`
  return `${m}m ${sec % 60}s`
}

function mlSec(m: Record<string, number> | undefined, key: string) {
  if (!m) return 0
  return Math.round(m[key] ?? 0)
}

function ActivityBadge({ score }: { score: number }) {
  const pct = Math.round(score * 100)
  const color =
    pct >= 70 ? 'bg-green-500/20 text-green-400' :
    pct >= 40 ? 'bg-yellow-500/20 text-yellow-400' :
               'bg-red-500/20 text-red-400'
  return (
    <span className={`badge ${color}`}>{pct}%</span>
  )
}

function StateBadge({ state }: { state: string }) {
  const color =
    state === 'active'  ? 'bg-blue-500/20 text-blue-400' :
    state === 'ended'   ? 'bg-slate-500/20 text-slate-400' :
                          'bg-purple-500/20 text-purple-400'
  return <span className={`badge ${color}`}>{state}</span>
}

function GameNameCell({ session }: { session: Session }) {
  const qc = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(session.game_name)

  useEffect(() => {
    if (!editing) setDraft(session.game_name)
  }, [session.game_name, session.id, editing])

  const mut = useMutation({
    mutationFn: (game_name: string) => sessionsApi.patch(session.id, { game_name }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['sessions'] })
      setEditing(false)
    },
  })

  if (editing) {
    return (
      <td className="px-4 py-3 align-top">
        <div className="flex flex-wrap items-center gap-2">
          <input
            type="text"
            className="input text-sm py-1 max-w-[220px]"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Escape') {
                setEditing(false)
                setDraft(session.game_name)
              }
              if (e.key === 'Enter') mut.mutate(draft.trim())
            }}
            autoFocus
          />
          <button
            type="button"
            className="btn-primary text-xs py-1 px-2"
            disabled={mut.isPending}
            onClick={() => mut.mutate(draft.trim())}
          >
            Save
          </button>
          <button
            type="button"
            className="btn-secondary text-xs py-1 px-2"
            disabled={mut.isPending}
            onClick={() => {
              setEditing(false)
              setDraft(session.game_name)
            }}
          >
            Cancel
          </button>
        </div>
        {mut.isError && (
          <p className="text-red-400 text-xs mt-1">Could not save — try again.</p>
        )}
      </td>
    )
  }

  return (
    <td className="px-4 py-3 text-slate-200 font-medium">
      <div className="flex items-center gap-2 flex-wrap">
        <span>{session.game_name || <span className="text-slate-500 italic">unknown</span>}</span>
        <button
          type="button"
          className="text-xs text-sky-400 hover:text-sky-300 underline-offset-2 hover:underline"
          onClick={() => setEditing(true)}
        >
          Edit
        </button>
      </div>
    </td>
  )
}

export default function Sessions() {
  const [filters, setFilters] = useState<SessionFilters>({})
  const [applied, setApplied] = useState<SessionFilters>({})

  const { data: sessions = [], isLoading, refetch } = useQuery({
    queryKey: ['sessions', applied],
    queryFn: () => sessionsApi.list(applied),
  })

  const handleApply = () => setApplied({ ...filters })
  const handleReset = () => {
    setFilters({})
    setApplied({})
  }

  const totalActiveTime = sessions.reduce((s: number, sess: Session) => s + sess.active_duration, 0)
  const totalTime = sessions.reduce((s: number, sess: Session) => s + sess.total_duration, 0)
  const totalMlActive = sessions.reduce(
    (s: number, sess: Session) => s + mlSec(sess.ml_playtime_seconds, 'active_gameplay'),
    0,
  )

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold text-white">Sessions</h1>
        <button onClick={() => refetch()} className="btn-secondary text-sm">
          Refresh
        </button>
      </div>

      {/* ── Filters ──────────────────────────────────────────────────────── */}
      <div className="card">
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
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
          <div className="flex items-end gap-2">
            <button onClick={handleApply} className="btn-primary text-sm flex-1">Apply</button>
            <button onClick={handleReset} className="btn-secondary text-sm">Reset</button>
          </div>
        </div>
      </div>

      {/* ── Summary ──────────────────────────────────────────────────────── */}
      {sessions.length > 0 && (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          <div className="card text-center">
            <p className="text-2xl font-bold text-white">{sessions.length}</p>
            <p className="text-xs text-slate-400 mt-1">Sessions</p>
          </div>
          <div className="card text-center">
            <p className="text-2xl font-bold text-white">{fmtDuration(totalTime)}</p>
            <p className="text-xs text-slate-400 mt-1">Total time (client)</p>
          </div>
          <div className="card text-center">
            <p className="text-2xl font-bold text-green-400">{fmtDuration(totalActiveTime)}</p>
            <p className="text-xs text-slate-400 mt-1">Active (client)</p>
          </div>
          <div className="card text-center">
            <p className="text-2xl font-bold text-emerald-300">{fmtDuration(totalMlActive)}</p>
            <p className="text-xs text-slate-400 mt-1">Active gameplay (ML)</p>
          </div>
        </div>
      )}

      {/* ── Table ────────────────────────────────────────────────────────── */}
      <div className="card overflow-hidden p-0">
        {isLoading ? (
          <div className="p-8 text-center text-slate-400">Loading…</div>
        ) : sessions.length === 0 ? (
          <div className="p-8 text-center text-slate-400">
            No sessions found. Start the desktop client and use the tray menu → Session → Start session.
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-slate-700 text-left">
                  {[
                    'Date',
                    'Game',
                    'Start',
                    'Duration',
                    'Active',
                    'AFK',
                    'ML play',
                    'ML afk',
                    'ML menu',
                    'ML load',
                    'Activity',
                    'State',
                  ].map((h) => (
                    <th
                      key={h}
                      className="px-4 py-3 text-xs font-medium text-slate-400 uppercase tracking-wide"
                    >
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-700/50">
                {sessions.map((s: Session) => (
                  <tr key={s.id} className="hover:bg-slate-700/30 transition-colors">
                    <td className="px-4 py-3 text-slate-300 whitespace-nowrap">
                      {format(parseISO(s.session_start), 'dd MMM yyyy')}
                    </td>
                    <GameNameCell session={s} />
                    <td className="px-4 py-3 text-slate-300 whitespace-nowrap">
                      {format(parseISO(s.session_start), 'HH:mm:ss')}
                    </td>
                    <td className="px-4 py-3 text-slate-300">{fmtDuration(s.total_duration)}</td>
                    <td className="px-4 py-3 text-green-400">{fmtDuration(s.active_duration)}</td>
                    <td className="px-4 py-3 text-yellow-400">{fmtDuration(s.afk_duration)}</td>
                    <td className="px-4 py-3 text-emerald-400 text-xs whitespace-nowrap">
                      {fmtDuration(mlSec(s.ml_playtime_seconds, 'active_gameplay'))}
                    </td>
                    <td className="px-4 py-3 text-amber-400/90 text-xs whitespace-nowrap">
                      {fmtDuration(mlSec(s.ml_playtime_seconds, 'afk'))}
                    </td>
                    <td className="px-4 py-3 text-sky-400/90 text-xs whitespace-nowrap">
                      {fmtDuration(mlSec(s.ml_playtime_seconds, 'menu'))}
                    </td>
                    <td className="px-4 py-3 text-violet-400/90 text-xs whitespace-nowrap">
                      {fmtDuration(mlSec(s.ml_playtime_seconds, 'loading'))}
                    </td>
                    <td className="px-4 py-3">
                      <ActivityBadge score={s.activity_score} />
                    </td>
                    <td className="px-4 py-3">
                      <StateBadge state={s.state} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
