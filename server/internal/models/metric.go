package models

import (
	"encoding/json"
	"time"
)

// EventType identifies the kind of raw input or system event.
type EventType string

const (
	EventMouseMove     EventType = "mouse_move"
	EventMouseClick    EventType = "mouse_click"
	EventKeyPress      EventType = "key_press"
	EventKeyRelease    EventType = "key_release"
	EventSystemMetrics EventType = "system_metrics"
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

// ClickPoint is used by the heatmap endpoint.
type ClickPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}
