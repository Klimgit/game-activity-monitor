import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { heatmapApi, sessionsApi } from '../api'
import type { ClickPoint, Session } from '../types/api'
import { format, parseISO } from 'date-fns'

// ── Canvas heatmap renderer ───────────────────────────────────────────────────

function renderHeatmap(canvas: HTMLCanvasElement, points: ClickPoint[]) {
  const ctx = canvas.getContext('2d')
  if (!ctx || points.length === 0) return

  ctx.clearRect(0, 0, canvas.width, canvas.height)

  // Compute coordinate range to scale points to the canvas.
  const maxX = Math.max(...points.map((p) => p.x), 1920)
  const maxY = Math.max(...points.map((p) => p.y), 1080)
  const scaleX = canvas.width / maxX
  const scaleY = canvas.height / maxY

  // Draw each click as a radial gradient; blending creates the heat effect.
  for (const p of points) {
    const cx = p.x * scaleX
    const cy = p.y * scaleY
    const radius = 28

    const gradient = ctx.createRadialGradient(cx, cy, 0, cx, cy, radius)
    gradient.addColorStop(0, 'rgba(255, 60, 0, 0.45)')
    gradient.addColorStop(0.4, 'rgba(255, 160, 0, 0.20)')
    gradient.addColorStop(1, 'rgba(0, 80, 255, 0)')

    ctx.beginPath()
    ctx.arc(cx, cy, radius, 0, Math.PI * 2)
    ctx.fillStyle = gradient
    ctx.fill()
  }
}

// ── Component ─────────────────────────────────────────────────────────────────

export default function Heatmap() {
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)

  const { data: sessions = [] } = useQuery({
    queryKey: ['sessions', {}],
    queryFn: () => sessionsApi.list(),
  })

  const { data: points = [], isLoading } = useQuery({
    queryKey: ['heatmap', selectedId],
    queryFn: () => heatmapApi.get(selectedId!),
    enabled: selectedId !== null,
  })

  // Re-render canvas whenever points or canvas size change.
  useEffect(() => {
    if (canvasRef.current && points.length > 0) {
      renderHeatmap(canvasRef.current, points)
    } else if (canvasRef.current) {
      const ctx = canvasRef.current.getContext('2d')
      ctx?.clearRect(0, 0, canvasRef.current.width, canvasRef.current.height)
    }
  }, [points])

  return (
    <div className="space-y-6">
      <h1 className="text-xl font-semibold text-white">Mouse Click Heatmap</h1>

      {/* Session selector */}
      <div className="card">
        <label className="block text-sm font-medium text-slate-300 mb-2">Select session</label>
        <select
          className="input max-w-md"
          value={selectedId ?? ''}
          onChange={(e) => setSelectedId(e.target.value ? Number(e.target.value) : null)}
        >
          <option value="">— choose a session —</option>
          {(sessions as Session[]).map((s) => (
            <option key={s.id} value={s.id}>
              {format(parseISO(s.session_start), 'dd MMM yyyy HH:mm')}{' '}
              {s.game_name ? `· ${s.game_name}` : ''}
            </option>
          ))}
        </select>
      </div>

      {/* Canvas */}
      <div className="card overflow-hidden p-0">
        {isLoading && (
          <div className="p-8 text-center text-slate-400">Loading heatmap…</div>
        )}

        {!isLoading && selectedId === null && (
          <div className="p-8 text-center text-slate-400">
            Select a session above to view the click heatmap.
          </div>
        )}

        {!isLoading && selectedId !== null && points.length === 0 && (
          <div className="p-8 text-center text-slate-400">
            No click data for this session. Make sure the client is running and recording events.
          </div>
        )}

        {points.length > 0 && (
          <div className="relative">
            <div className="flex items-center justify-between px-5 py-3 border-b border-slate-700">
              <span className="text-sm text-slate-400">{points.length.toLocaleString()} click events</span>
              <div className="flex items-center gap-3 text-xs text-slate-500">
                <span className="flex items-center gap-1">
                  <span className="w-3 h-3 rounded-full bg-blue-600 opacity-60" /> Low
                </span>
                <span className="flex items-center gap-1">
                  <span className="w-3 h-3 rounded-full bg-yellow-500 opacity-60" /> Medium
                </span>
                <span className="flex items-center gap-1">
                  <span className="w-3 h-3 rounded-full bg-red-500 opacity-60" /> High
                </span>
              </div>
            </div>
            {/* Dark screen background mimics the game display */}
            <div className="bg-slate-950 p-2">
              <canvas
                ref={canvasRef}
                width={960}
                height={540}
                className="w-full rounded"
              />
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
