package models

import (
	"encoding/json"
	"time"
)

type EventType string

const (
	EventMouseMove     EventType = "mouse_move"
	EventMouseClick    EventType = "mouse_click"
	EventKeyPress      EventType = "key_press"
	EventKeyRelease    EventType = "key_release"
	EventSystemMetrics EventType = "system_metrics"
	EventWindowMetrics EventType = "window_metrics"
)

type RawEvent struct {
	UserID    int64           `json:"user_id"`
	SessionID *int64          `json:"session_id,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
	EventType EventType       `json:"event_type"`
	Data      json.RawMessage `json:"data"`
}

type MouseMoveData struct {
	X     int     `json:"x"`
	Y     int     `json:"y"`
	Speed float64 `json:"speed"`
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
	CPUPercent    float64 `json:"cpu_percent"`
	MemPercent    float64 `json:"mem_percent"`
	GPUPercent    float64 `json:"gpu_percent,omitempty"`
	GPUTempC      float64 `json:"gpu_temp_c,omitempty"`
	GPUMemUsedMB  int64   `json:"gpu_mem_used_mb,omitempty"`
	ActiveProcess string  `json:"active_process,omitempty"`
}

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

func MustMarshal(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic("models.MustMarshal: " + err.Error())
	}
	return b
}
