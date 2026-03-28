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
	WindowTitle   string  `json:"window_title,omitempty"`
}

// ActivityLabel carries a manual hotkey annotation to the server.
type ActivityLabel struct {
	SessionID *int64    `json:"session_id,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	// State: "active_gameplay" | "afk" | "menu" | "loading"
	State  string `json:"state"`
	Source string `json:"source"` // always "manual_hotkey" from the client
}

// MustMarshal serialises v to json.RawMessage, panicking only on programmer error.
func MustMarshal(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic("models.MustMarshal: " + err.Error())
	}
	return b
}
