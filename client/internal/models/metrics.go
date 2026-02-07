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
	// EventWindowMetrics is emitted by the client aggregator once per
	// aggregation window (default 10 s).  It contains pre-computed features
	// for mouse and keyboard activity and is stored server-side in the
	// session_windows hypertable for long-term ML training data.
	EventWindowMetrics EventType = "window_metrics"
)

// RawEvent is a single timestamped event ready to be sent to the server.
type RawEvent struct {
	UserID    int64           `json:"user_id"`
	SessionID *int64          `json:"session_id,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
	EventType EventType       `json:"event_type"`
	Data      json.RawMessage `json:"data"`
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
}

// WindowMetricsData is the payload for EventWindowMetrics.
// It summarises mouse and keyboard activity over one aggregation window so
// the server stores one compact row per window instead of hundreds of raw
// events.  system_metrics events are still forwarded individually for the
// real-time dashboard.
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
	// Hardware averages aggregated from system_metrics events in this window.
	// These are the primary ML features for detecting active gaming vs idle.
	CPUAvg     float64 `json:"cpu_avg"`
	CPUMax     float64 `json:"cpu_max"`
	MemAvg     float64 `json:"mem_avg"`
	GPUUtilAvg float64 `json:"gpu_util_avg"`
	GPUTempAvg float64 `json:"gpu_temp_avg"`
}

// MustMarshal serialises v to json.RawMessage, panicking only on programmer error.
func MustMarshal(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic("models.MustMarshal: " + err.Error())
	}
	return b
}
