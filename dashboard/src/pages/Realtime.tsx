import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
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
import type { SystemMetricsData, WindowMetricsData } from '../types/api'

// ── types ─────────────────────────────────────────────────────────────────────

interface ChartPoint {
  time: string
  cpu: number
  mem: number
  gpu: number
  mouseActivity: number   // mouse_moves from window_metrics (or clicks as fallback)
  keyActivity: number     // keystrokes from window_metrics
  speedAvg: number        // avg mouse speed px/s from window_metrics
}

// ── helpers ───────────────────────────────────────────────────────────────────

const avg = (arr: number[]) =>
  arr.length === 0 ? 0 : arr.reduce((s, v) => s + v, 0) / arr.length

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
      mouseActivity: totalMoves,
      keyActivity: totalKeys,
      speedAvg: Math.round(avg(speedValues)),
    }

    setHistory((prev) => [...prev.slice(-59), newPoint])
    setSessionTime((t) => t + 2) // approximate: +2s per poll
  }, [dataUpdatedAt]) // eslint-disable-line react-hooks/exhaustive-deps

  const latest = history.at(-1)

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
        <StatCard label="Mouse moves / window" value={latest?.mouseActivity ?? 0} color="text-yellow-400" />
        <StatCard label="Keystrokes / window" value={latest?.keyActivity ?? 0} color="text-purple-400" />
        <StatCard label="Avg mouse speed" value={latest?.speedAvg ?? 0} unit="px/s" color="text-cyan-400" />
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
      </div>

      {/* ── Hotkeys reference ─────────────────────────────────────────────── */}
      <div className="card">
        <h3 className="text-sm font-medium text-slate-300 mb-3">Client Hotkeys</h3>
        <div className="grid grid-cols-2 md:grid-cols-3 gap-2 text-xs text-slate-400">
          {[
            ['Ctrl+Shift+S', 'Start session'],
            ['Ctrl+Shift+E', 'End session'],
            ['Ctrl+Shift+A', 'Mark: active gameplay'],
            ['Ctrl+Shift+F', 'Mark: AFK'],
            ['Ctrl+Shift+M', 'Mark: in menu'],
            ['Ctrl+Shift+L', 'Mark: loading'],
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
