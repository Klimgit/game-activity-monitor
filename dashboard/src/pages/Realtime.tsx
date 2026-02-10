import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { format, subDays } from 'date-fns'
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from 'recharts'
import { metricsApi } from '../api'
import type { SystemMetricsData, WindowMetricsData, WindowMetricsSummary } from '../types/api'

// ── types ─────────────────────────────────────────────────────────────────────

interface ChartPoint {
  time: string
  cpu: number
  mem: number
  gpu: number
  gpuTemp: number
  gpuMemMb: number
  mouseActivity: number   // mouse_moves from window_metrics (or clicks as fallback)
  keyActivity: number     // keystrokes from window_metrics
  speedAvg: number        // avg mouse speed px/s from window_metrics
  mlStateRank: number     // 0 unknown; 1 loading … 4 active_gameplay
}

// ── helpers ───────────────────────────────────────────────────────────────────

const avg = (arr: number[]) =>
  arr.length === 0 ? 0 : arr.reduce((s, v) => s + v, 0) / arr.length

const ML_STATE_RANK: Record<string, number> = {
  loading: 1,
  menu: 2,
  afk: 3,
  active_gameplay: 4,
}

function mlStateRank(state: string | undefined): number {
  if (!state) return 0
  return ML_STATE_RANK[state] ?? 0
}

function fmtDuration(sec: number) {
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = sec % 60
  return [h, m, s].map((v) => String(v).padStart(2, '0')).join(':')
}

// ── sub-components ────────────────────────────────────────────────────────────

interface StatCardProps {
  label: string
  value: string | number
  unit?: string
  color: string
}

function StatCard({ label, value, unit, color }: StatCardProps) {
  return (
    <div className="card flex flex-col gap-1">
      <span className="text-xs uppercase tracking-wide text-slate-400">{label}</span>
      <span className={`text-3xl font-bold ${color}`}>
        {value}
        {unit && <span className="text-base font-normal text-slate-400 ml-1">{unit}</span>}
      </span>
    </div>
  )
}

interface ChartCardProps {
  title: string
  data: ChartPoint[]
  dataKey: keyof ChartPoint
  color: string
  domain?: [number, number]
  unit?: string
}

function ChartCard({ title, data, dataKey, color, domain, unit }: ChartCardProps) {
  return (
    <div className="card">
      <h3 className="text-sm font-medium text-slate-300 mb-3">{title}</h3>
      <ResponsiveContainer width="100%" height={160}>
        <LineChart data={data} margin={{ top: 4, right: 8, bottom: 0, left: -10 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#334155" />
          <XAxis
            dataKey="time"
            tick={{ fill: '#94a3b8', fontSize: 10 }}
            interval="preserveStartEnd"
          />
          <YAxis
            domain={domain ?? ['auto', 'auto']}
            tick={{ fill: '#94a3b8', fontSize: 10 }}
            unit={unit}
          />
          <Tooltip
            contentStyle={{ backgroundColor: '#1e293b', border: '1px solid #334155', borderRadius: 8 }}
            labelStyle={{ color: '#94a3b8' }}
            itemStyle={{ color: color }}
          />
          <Line
            type="monotone"
            dataKey={dataKey}
            stroke={color}
            strokeWidth={2}
            dot={false}
            isAnimationActive={false}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}

// ── main component ────────────────────────────────────────────────────────────

export default function Realtime() {
  const [history, setHistory] = useState<ChartPoint[]>([])
  const [activeProcess, setActiveProcess] = useState('—')
  const [sessionTime, setSessionTime] = useState(0)
  const [latestMlLabel, setLatestMlLabel] = useState<string | null>(null)

  const [rangeFrom, setRangeFrom] = useState(() => format(subDays(new Date(), 7), 'yyyy-MM-dd'))
  const [rangeTo, setRangeTo] = useState(() => format(new Date(), 'yyyy-MM-dd'))
  const [rangeSummary, setRangeSummary] = useState<WindowMetricsSummary | null>(null)
  const [rangeError, setRangeError] = useState<string | null>(null)
  const [rangeLoading, setRangeLoading] = useState(false)

  const { data: events = [], dataUpdatedAt } = useQuery({
    queryKey: ['metrics', 'recent'],
    queryFn: () => metricsApi.getRecent(30),
    refetchInterval: 2000,
    staleTime: 0,
  })

  // Aggregate incoming events into a new chart point every poll.
  useEffect(() => {
    if (dataUpdatedAt === 0) return

    const sysEvents = events.filter((e) => e.event_type === 'system_metrics')
    // window_metrics events carry pre-aggregated mouse/keyboard stats.
    // Individual mouse_move / key_press events are no longer forwarded by the
    // client aggregator, so we use window_metrics as the activity source.
    const winEvents = events.filter((e) => e.event_type === 'window_metrics')

    const cpuValues = sysEvents
      .map((e) => (e.data as SystemMetricsData).cpu_percent ?? 0)
      .filter((v) => v > 0)
    const memValues = sysEvents
      .map((e) => (e.data as SystemMetricsData).mem_percent ?? 0)
      .filter((v) => v > 0)
    const gpuValues = sysEvents
      .map((e) => (e.data as SystemMetricsData).gpu_percent ?? 0)
      .filter((v) => v > 0)
    const gpuTemps = sysEvents
      .map((e) => (e.data as SystemMetricsData).gpu_temp_c ?? 0)
      .filter((v) => v > 0)
    const gpuMems = sysEvents
      .map((e) => (e.data as SystemMetricsData).gpu_mem_used_mb ?? 0)
      .filter((v) => v > 0)

    const processes = sysEvents
      .map((e) => (e.data as SystemMetricsData).active_process)
      .filter((v): v is string => Boolean(v))
    if (processes.length > 0) {
      setActiveProcess(processes.at(-1)!)
    }

    // Sum mouse/keyboard counts across all window_metrics in this poll window.
    const winData = winEvents.map((e) => e.data as unknown as WindowMetricsData)
    const totalMoves = winData.reduce((s, d) => s + (d.mouse_moves ?? 0), 0)
    const totalKeys  = winData.reduce((s, d) => s + (d.keystrokes  ?? 0), 0)
    const speedValues = winData.map((d) => d.speed_avg ?? 0).filter((v) => v > 0)

    const lastWin = winData.at(-1)
    const ml = lastWin?.ml_predicted_state
    if (ml) setLatestMlLabel(ml)
    const mlRank = mlStateRank(ml)

    const newPoint: ChartPoint = {
      time: new Date().toLocaleTimeString('en', {
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hour12: false,
      }),
      cpu: Math.round(avg(cpuValues) * 10) / 10,
      mem: Math.round(avg(memValues) * 10) / 10,
      gpu: Math.round(avg(gpuValues) * 10) / 10,
      gpuTemp: Math.round(avg(gpuTemps) * 10) / 10,
      gpuMemMb: gpuMems.length ? Math.round(avg(gpuMems)) : 0,
      mouseActivity: totalMoves,
      keyActivity: totalKeys,
      speedAvg: Math.round(avg(speedValues)),
      mlStateRank: mlRank,
    }

    setHistory((prev) => [...prev.slice(-59), newPoint])
    setSessionTime((t) => t + 2) // approximate: +2s per poll
  }, [dataUpdatedAt]) // eslint-disable-line react-hooks/exhaustive-deps

  const latest = history.at(-1)

  async function loadRangeSummary() {
    setRangeError(null)
    setRangeLoading(true)
    try {
      const s = await metricsApi.getWindowsSummary(rangeFrom, rangeTo)
      setRangeSummary(s)
    } catch {
      setRangeError('Could not load summary.')
      setRangeSummary(null)
    } finally {
      setRangeLoading(false)
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold text-white">Live Monitor</h1>
        <div className="flex items-center gap-2 text-sm text-slate-400">
          <span className="w-2 h-2 rounded-full bg-green-500 animate-pulse" />
          Polling every 2s
        </div>
      </div>

      {/* ── Stat cards ───────────────────────────────────────────────────── */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard label="CPU" value={latest?.cpu ?? 0} unit="%" color="text-blue-400" />
        <StatCard label="Memory" value={latest?.mem ?? 0} unit="%" color="text-green-400" />
        <StatCard label="GPU" value={latest?.gpu ?? 0} unit="%" color="text-orange-400" />
        <StatCard label="GPU temp" value={latest?.gpuTemp ?? 0} unit="°C" color="text-orange-300" />
        <StatCard label="GPU mem used" value={latest?.gpuMemMb ?? 0} unit="MB" color="text-amber-300" />
        <StatCard label="Mouse moves / window" value={latest?.mouseActivity ?? 0} color="text-yellow-400" />
        <StatCard label="Keystrokes / window" value={latest?.keyActivity ?? 0} color="text-purple-400" />
        <StatCard label="Avg mouse speed" value={latest?.speedAvg ?? 0} unit="px/s" color="text-cyan-400" />
        <StatCard
          label="ML state (latest window)"
          value={latestMlLabel ?? '—'}
          color="text-emerald-400"
        />
      </div>

      {/* ── Info row ─────────────────────────────────────────────────────── */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="card flex items-center gap-3">
          <span className="text-slate-400 text-sm">Active process:</span>
          <span className="font-mono text-blue-300">{activeProcess}</span>
        </div>
        <div className="card flex items-center gap-3">
          <span className="text-slate-400 text-sm">Monitor uptime:</span>
          <span className="font-mono text-slate-200">{fmtDuration(sessionTime)}</span>
        </div>
      </div>

      {/* ── Charts ───────────────────────────────────────────────────────── */}
      <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
        <ChartCard title="CPU Usage" data={history} dataKey="cpu" color="#60a5fa" domain={[0, 100]} unit="%" />
        <ChartCard title="Memory Usage" data={history} dataKey="mem" color="#4ade80" domain={[0, 100]} unit="%" />
        <ChartCard title="GPU Utilization" data={history} dataKey="gpu" color="#fb923c" domain={[0, 100]} unit="%" />
        <ChartCard title="Mouse Moves / Window" data={history} dataKey="mouseActivity" color="#facc15" />
        <ChartCard title="Keystrokes / Window" data={history} dataKey="keyActivity" color="#c084fc" />
        <ChartCard title="Avg Mouse Speed" data={history} dataKey="speedAvg" color="#22d3ee" unit="px/s" />
        <ChartCard
          title="ML state rank (1=loading … 4=active)"
          data={history}
          dataKey="mlStateRank"
          color="#34d399"
          domain={[0, 4]}
        />
      </div>

      {/* ── Period summary (session_windows) ─────────────────────────────── */}
      <div className="card space-y-4">
        <h3 className="text-sm font-medium text-slate-300">Metrics for a selected period</h3>
        <p className="text-xs text-slate-500">
          Aggregates stored windows (including ML playtime). Uses UTC calendar days.
        </p>
        <div className="flex flex-wrap gap-3 items-end">
          <div>
            <label className="block text-xs text-slate-400 mb-1">From</label>
            <input
              type="date"
              className="input text-sm"
              value={rangeFrom}
              onChange={(e) => setRangeFrom(e.target.value)}
            />
          </div>
          <div>
            <label className="block text-xs text-slate-400 mb-1">To</label>
            <input
              type="date"
              className="input text-sm"
              value={rangeTo}
              onChange={(e) => setRangeTo(e.target.value)}
            />
          </div>
          <button type="button" className="btn-primary text-sm" disabled={rangeLoading} onClick={loadRangeSummary}>
            {rangeLoading ? 'Loading…' : 'Load summary'}
          </button>
        </div>
        {rangeError && <p className="text-red-400 text-sm">{rangeError}</p>}
        {rangeSummary && (
          <div className="grid grid-cols-2 md:grid-cols-4 gap-3 text-sm">
            <div className="rounded-lg bg-slate-800/80 p-3">
              <p className="text-slate-400 text-xs">Windows</p>
              <p className="text-xl font-semibold text-white">{rangeSummary.window_count}</p>
            </div>
            <div className="rounded-lg bg-slate-800/80 p-3">
              <p className="text-slate-400 text-xs">Total duration (sum)</p>
              <p className="text-xl font-semibold text-slate-200">
                {Math.round(rangeSummary.total_duration_s)}s
              </p>
            </div>
            <div className="rounded-lg bg-slate-800/80 p-3">
              <p className="text-slate-400 text-xs">Mouse moves</p>
              <p className="text-xl font-semibold text-yellow-400">{rangeSummary.total_mouse_moves}</p>
            </div>
            <div className="rounded-lg bg-slate-800/80 p-3">
              <p className="text-slate-400 text-xs">Keystrokes</p>
              <p className="text-xl font-semibold text-purple-400">{rangeSummary.total_keystrokes}</p>
            </div>
            <div className="col-span-2 md:col-span-4 rounded-lg bg-slate-800/80 p-3">
              <p className="text-slate-400 text-xs mb-2">ML playtime by state (seconds)</p>
              <div className="flex flex-wrap gap-4 text-slate-200">
                {['active_gameplay', 'afk', 'menu', 'loading'].map((k) => (
                  <span key={k}>
                    <span className="text-slate-500">{k}:</span>{' '}
                    {Math.round(rangeSummary.ml_playtime_seconds[k] ?? 0)}s
                  </span>
                ))}
              </div>
            </div>
          </div>
        )}
      </div>

      {/* ── Client tray menu ──────────────────────────────────────────────── */}
      <div className="card">
        <h3 className="text-sm font-medium text-slate-300 mb-3">Desktop client (system tray)</h3>
        <p className="text-xs text-slate-400">
          Use the tray icon menu: <strong className="text-slate-300">Session</strong> for start/end session,{' '}
          <strong className="text-slate-300">Activity labels</strong> for interval marks (one open at a time).
        </p>
      </div>
    </div>
  )
}
