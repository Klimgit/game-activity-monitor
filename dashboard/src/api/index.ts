import client from './client'
import type {
  AuthResponse,
  ClickPoint,
  RawEvent,
  Session,
  SessionFilters,
  ActivityLabel,
} from '../types/api'

// ── Auth ──────────────────────────────────────────────────────────────────────

export const authApi = {
  login: async (email: string, password: string): Promise<AuthResponse> => {
    const { data } = await client.post<AuthResponse>('/auth/login', { email, password })
    return data
  },
  register: async (email: string, password: string): Promise<AuthResponse> => {
    const { data } = await client.post<AuthResponse>('/auth/register', { email, password })
    return data
  },
}

// ── Metrics ───────────────────────────────────────────────────────────────────

export const metricsApi = {
  /** Returns raw events from the last `seconds` seconds (max 300). */
  getRecent: async (seconds = 30): Promise<RawEvent[]> => {
    const { data } = await client.get<RawEvent[]>(`/metrics/recent?seconds=${seconds}`)
    return data
  },
}

// ── Sessions ──────────────────────────────────────────────────────────────────

export const sessionsApi = {
  list: async (filters: SessionFilters = {}): Promise<Session[]> => {
    const params = new URLSearchParams()
    if (filters.from) params.set('from', filters.from)
    if (filters.to) params.set('to', filters.to)
    if (filters.game) params.set('game', filters.game)
    const { data } = await client.get<Session[]>(`/sessions?${params}`)
    return data
  },

  get: async (id: number): Promise<Session> => {
    const { data } = await client.get<Session>(`/sessions/${id}`)
    return data
  },

  start: async (gameName: string): Promise<Session> => {
    const { data } = await client.post<Session>('/sessions/start', { game_name: gameName })
    return data
  },

  end: async (
    id: number,
    payload: {
      total_duration: number
      active_duration: number
      afk_duration: number
      activity_score: number
    },
  ): Promise<Session> => {
    const { data } = await client.post<Session>(`/sessions/${id}/end`, payload)
    return data
  },
}

// ── Labels ────────────────────────────────────────────────────────────────────

export const labelsApi = {
  list: async (sessionId?: number): Promise<ActivityLabel[]> => {
    const params = sessionId ? `?session_id=${sessionId}` : ''
    const { data } = await client.get<ActivityLabel[]>(`/labels${params}`)
    return data
  },

  create: async (payload: {
    session_id?: number
    timestamp: string
    state: string
    source: string
  }): Promise<ActivityLabel> => {
    const { data } = await client.post<ActivityLabel>('/labels', payload)
    return data
  },
}

// ── Heatmap ───────────────────────────────────────────────────────────────────

export const heatmapApi = {
  get: async (sessionId: number): Promise<ClickPoint[]> => {
    const { data } = await client.get<ClickPoint[]>(`/heatmap/${sessionId}`)
    return data
  },
}
