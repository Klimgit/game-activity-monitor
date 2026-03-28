package models

import (
	"encoding/json"
	"time"
)

// EventType identifies the kind of raw input or system event.
// The server is a generic event store — it stores all event types as-is in
// raw_input_events.  Only the two types below require special server-side
// handling (mirroring to dedicated long-term tables).
type EventType string

const (
	// EventMouseClick is mirrored to click_events (90-day retention) for
	// per-session heatmap queries.
	EventMouseClick EventType = "mouse_click"
	// EventWindowMetrics is mirrored to session_windows (1-year retention)
	// as the primary ML training dataset.
	EventWindowMetrics EventType = "window_metrics"
)

// RawEvent is a single timestamped event stored in the hypertable.
type RawEvent struct {
	UserID    int64           `json:"user_id"              db:"user_id"`
	SessionID *int64          `json:"session_id,omitempty" db:"session_id"`
	Timestamp time.Time       `json:"timestamp"            db:"timestamp"`
	EventType EventType       `json:"event_type"           db:"event_type"`
	Data      json.RawMessage `json:"data"                 db:"data"`
}

// ---- typed payloads for the Data field ----

type MouseMoveData struct {
	X     int     `json:"x"`
	Y     int     `json:"y"`
	Speed float64 `json:"speed"` // px/s
}

type MouseClickData struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Button string `json:"button"` // "left" | "right" | "middle"
}

type KeyEventData struct {
	Key    string `json:"key"`
	HoldMs int    `json:"hold_ms,omitempty"`
}

type SystemMetricsData struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemPercent    float64 `json:"mem_percent"`
	GPUPercent    float64 `json:"gpu_percent,omitempty"`
	GPUTempC      float64 `json:"gpu_temp_c,omitempty"`
	GPUMemUsedMB  int64   `json:"gpu_mem_used_mb,omitempty"`
	ActiveProcess string  `json:"active_process,omitempty"`
	WindowTitle   string  `json:"window_title,omitempty"`
}

// WindowMetricsData is the JSON payload inside a window_metrics RawEvent.
type WindowMetricsData struct {
	WindowStart   time.Time `json:"window_start"`
	WindowEnd     time.Time `json:"window_end"`
	DurationS     float64   `json:"duration_s"`
	MouseMoves    int       `json:"mouse_moves"`
	MouseClicks   int       `json:"mouse_clicks"`
	SpeedAvg      float64   `json:"speed_avg"`
	SpeedMax      float64   `json:"speed_max"`
	Keystrokes    int       `json:"keystrokes"`
	KeyHoldAvgMs  float64   `json:"key_hold_avg_ms"`
	ActiveProcess string    `json:"active_process,omitempty"`
	CPUAvg        float64   `json:"cpu_avg"`
	CPUMax        float64   `json:"cpu_max"`
	MemAvg        float64   `json:"mem_avg"`
	GPUUtilAvg    float64   `json:"gpu_util_avg"`
	GPUTempAvg    float64   `json:"gpu_temp_avg"`
}

// ClickPoint is used by the heatmap endpoint.
type ClickPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}
