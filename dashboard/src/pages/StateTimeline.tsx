import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { intervalsApi, predictionsApi, sessionsApi } from '../api'
import type { ActivityInterval, PredictedWindow } from '../types/api'

const STATE_BG: Record<string, string> = {
  active_gameplay: 'bg-emerald-500/80',
  afk: 'bg-slate-500/80',
  menu: 'bg-blue-500/80',
  loading: 'bg-amber-500/80',
}

function StateBadge({ state }: { state: string }) {
  const cls = STATE_BG[state] ?? 'bg-slate-600/80'
  return (
    <span className={`inline-flex px-2 py-0.5 rounded text-xs font-medium text-white ${cls}`}>
      {state.replace(/_/g, ' ')}
    </span>
  )
}

type Segment = { start: number; end: number; state: string }

function buildSegments(
  intervals: Pick<ActivityInterval, 'start_at' | 'end_at' | 'state'>[],
): Segment[] {
  return intervals.map((iv) => ({
    start: new Date(iv.start_at).getTime(),
    end: new Date(iv.end_at).getTime(),
    state: iv.state,
  }))
}

function predSegments(preds: Pick<PredictedWindow, 'window_start' | 'window_end' | 'predicted_state'>[]): Segment[] {
  return preds.map((p) => ({
    start: new Date(p.window_start).getTime(),
    end: new Date(p.window_end).getTime(),
    state: p.predicted_state,
  }))
}

function TimelineStrip({
  title,
  segments,
  t0,
  t1,
}: {
  title: string
  segments: Segment[]
  t0: number
  t1: number
}) {
  const span = Math.max(1, t1 - t0)
  return (
    <div className="space-y-1">
      <div className="text-xs text-slate-400">{title}</div>
      <div className="relative h-10 w-full rounded-md bg-slate-800/80 overflow-hidden">
        {segments.map((s, i) => {
          const left = ((s.start - t0) / span) * 100
          const width = ((s.end - s.start) / span) * 100
          if (width <= 0) return null
          return (
            <div
              key={i}
              className={`absolute top-0 bottom-0 ${STATE_BG[s.state] ?? 'bg-slate-600'} hover:opacity-90 transition-opacity`}
              style={{ left: `${left}%`, width: `${width}%` }}
              title={`${s.state}`}
            />
          )
        })}
      </div>
    </div>
  )
}

export default function StateTimeline() {
  const [selectedSession, setSelectedSession] = useState<number | undefined>(undefined)

  const { data: sessions = [] } = useQuery({
    queryKey: ['sessions', {}],
    queryFn: () => sessionsApi.list(),
  })

  const { data: intervals = [], isLoading: loadIv } = useQuery({
    queryKey: ['intervals', selectedSession],
    queryFn: () => intervalsApi.list(selectedSession),
    enabled: selectedSession != null,
  })

  const { data: predictions = [], isLoading: loadPr } = useQuery({
    queryKey: ['predictions', selectedSession],
    queryFn: () => predictionsApi.list(selectedSession),
    enabled: selectedSession != null,
  })

  const { t0, t1, labelSeg, predSeg } = useMemo(() => {
    const ivSeg = buildSegments(intervals as ActivityInterval[])
    const prSeg = predSegments(predictions)
    const all = [...ivSeg, ...prSeg]
    if (all.length === 0) {
      return { t0: 0, t1: 0, labelSeg: [] as Segment[], predSeg: [] as Segment[] }
    }
    let mn = Infinity
    let mx = -Infinity
    for (const s of all) {
      mn = Math.min(mn, s.start)
      mx = Math.max(mx, s.end)
    }
    return {
      t0: mn,
      t1: mx,
      labelSeg: ivSeg,
      predSeg: prSeg,
    }
  }, [intervals, predictions])

  const sessionMeta = sessions.find((s) => s.id === selectedSession)

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-xl font-semibold text-white">State timeline</h1>
        <p className="text-sm text-slate-400 mt-1">
          Ground-truth labels (hotkeys) and model predictions per time window. Predictions are written by your inference
          pipeline via <code className="text-slate-300">POST /api/v1/predictions/batch</code>.
        </p>
      </div>

      <div className="card">
        <label className="block text-xs text-slate-400 mb-1">Session</label>
        <select
          className="input text-sm w-full md:w-96"
          value={selectedSession ?? ''}
          onChange={(e) => setSelectedSession(e.target.value ? Number(e.target.value) : undefined)}
        >
          <option value="">— Select a session —</option>
          {sessions.map((s) => (
            <option key={s.id} value={s.id}>
              #{s.id} · {s.game_name || 'game'} · {new Date(s.session_start).toLocaleString()}
            </option>
          ))}
        </select>
        {sessionMeta && (
          <p className="text-xs text-slate-500 mt-2">
            Session {sessionMeta.id} · {sessionMeta.game_name}
          </p>
        )}
      </div>

      {selectedSession == null && (
        <p className="text-slate-500 text-sm">Choose a session to see labels and predictions over time.</p>
      )}

      {selectedSession != null && (loadIv || loadPr) && (
        <p className="text-slate-400 text-sm">Loading…</p>
      )}

      {selectedSession != null && !loadIv && !loadPr && t1 <= t0 && (
        <div className="card text-slate-400 text-sm">
          No intervals or predictions in range for this session. Add labels with the desktop client, and ingest
          predictions from your model.
        </div>
      )}

      {selectedSession != null && !loadIv && !loadPr && t1 > t0 && (
        <div className="card space-y-6">
          <TimelineStrip title="Labels (ground truth)" segments={labelSeg} t0={t0} t1={t1} />
          <TimelineStrip title="Predictions (model)" segments={predSeg} t0={t0} t1={t1} />

          <div className="flex flex-wrap gap-3 text-xs text-slate-500">
            <span>Legend:</span>
            {['active_gameplay', 'afk', 'menu', 'loading'].map((st) => (
              <span key={st} className="inline-flex items-center gap-1">
                <StateBadge state={st} />
              </span>
            ))}
          </div>

          {predictions.length > 0 && predictions[0]?.model_version && (
            <p className="text-xs text-slate-500">
              Model version: <span className="text-slate-400">{predictions[0].model_version}</span>
              {predictions.some((p) => p.confidence != null) && ' · hover segments for state (confidence in API JSON)'}
            </p>
          )}
        </div>
      )}
    </div>
  )
}
