export interface User {
  id: number
  email: string
  created_at: string
  updated_at: string
}

export interface AuthResponse {
  token: string
  user: User
}

export interface Session {
  id: number
  user_id: number
  session_start: string
  session_end?: string
  game_name: string
  total_duration: number   // seconds
  active_duration: number  // seconds
  afk_duration: number     // seconds
  activity_score: number   // 0..1
  state: string
  created_at: string
  updated_at: string
}

export type EventType =
  | 'mouse_move'
  | 'mouse_click'
  | 'key_press'
  | 'key_release'
  | 'system_metrics'

export interface RawEvent {
  user_id: number
  session_id?: number
  timestamp: string
  event_type: EventType
  data: Record<string, unknown>
}

export interface SystemMetricsData {
  cpu_percent?: number
  mem_percent?: number
  gpu_percent?: number
  gpu_temp_c?: number
  gpu_mem_used_mb?: number
  active_process?: string
  window_title?: string
}

export interface MouseMoveData {
  x: number
  y: number
  speed: number
}

export interface MouseClickData {
  x: number
  y: number
  button: string
}

export interface KeyEventData {
  key: string
  hold_ms?: number
}

export interface ActivityLabel {
  id: number
  user_id: number
  session_id?: number
  timestamp: string
  state: string  // active_gameplay | afk | menu | loading
  source: string // manual_hotkey | auto_detected
  created_at: string
}

export interface ClickPoint {
  x: number
  y: number
}

export interface SessionFilters {
  from?: string   // YYYY-MM-DD
  to?: string     // YYYY-MM-DD
  game?: string
}
