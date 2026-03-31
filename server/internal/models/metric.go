package models

import (
	"encoding/json"
	"time"
)

type EventType string

const (
	EventMouseClick    EventType = "mouse_click"
	EventWindowMetrics EventType = "window_metrics"
)

type RawEvent struct {
	UserID    int64           `json:"user_id"              db:"user_id"`
	SessionID *int64          `json:"session_id,omitempty" db:"session_id"`
	Timestamp time.Time       `json:"timestamp"            db:"timestamp"`
	EventType EventType       `json:"event_type"           db:"event_type"`
	Data      json.RawMessage `json:"data"                 db:"data"`
}

type MouseMoveData struct {
	X            int     `json:"x"`
	Y            int     `json:"y"`
	Speed        float64 `json:"speed"`
	Acceleration float64 `json:"acceleration"`
}

type MouseClickData struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Button string `json:"button"`
}

type KeyEventData struct {
	Key    string `json:"key"`
	HoldMs int    `json:"hold_ms,omitempty"`
}

type SystemMetricsData struct {
	CPUPercent            float64 `json:"cpu_percent"`
	MemPercent            float64 `json:"mem_percent"`
	GPUPercent            float64 `json:"gpu_percent,omitempty"`
	GPUTempC              float64 `json:"gpu_temp_c,omitempty"`
	GPUMemUsedMB          int64   `json:"gpu_mem_used_mb,omitempty"`
	ActiveProcess         string  `json:"active_process,omitempty"`
	ForegroundWindowTitle string  `json:"foreground_window_title,omitempty"`
}

type WindowMetricsData struct {
	WindowStart           time.Time `json:"window_start"`
	WindowEnd             time.Time `json:"window_end"`
	DurationS             float64   `json:"duration_s"`
	MouseMoves            int       `json:"mouse_moves"`
	MouseClicks           int       `json:"mouse_clicks"`
	SpeedAvg              float64   `json:"speed_avg"`
	SpeedMax              float64   `json:"speed_max"`
	Keystrokes            int       `json:"keystrokes"`
	KeyHoldAvgMs          float64   `json:"key_hold_avg_ms"`
	KeyPressIntervalAvgMs float64   `json:"key_press_interval_avg_ms"`
	KeyW                  int       `json:"key_w"`
	KeyA                  int       `json:"key_a"`
	KeyS                  int       `json:"key_s"`
	KeyD                  int       `json:"key_d"`
	ActiveProcess         string    `json:"active_process,omitempty"`
	ForegroundWindowTitle string    `json:"foreground_window_title,omitempty"`
	CursorAccelAvg        float64   `json:"cursor_accel_avg"`
	CursorAccelMax        float64   `json:"cursor_accel_max"`
	CPUAvg                float64   `json:"cpu_avg"`
	CPUMax                float64   `json:"cpu_max"`
	MemAvg                float64   `json:"mem_avg"`
	GPUUtilAvg            float64   `json:"gpu_util_avg"`
	GPUTempAvg            float64   `json:"gpu_temp_avg"`
}

type ClickPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}
