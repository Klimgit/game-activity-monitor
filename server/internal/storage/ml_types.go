package storage

import "time"

func NormalizeWindowTime(t time.Time) time.Time {
	return t.UTC().Truncate(time.Microsecond)
}

type WindowKey struct {
	Time      time.Time
	SessionID int64
}

type WindowMetricsSummary struct {
	MLPlaytimeSeconds map[string]int64 `json:"ml_playtime_seconds"`
	WindowCount       int64            `json:"window_count"`
	TotalDurationS    float64          `json:"total_duration_s"`
	TotalMouseMoves   int64            `json:"total_mouse_moves"`
	TotalKeystrokes   int64            `json:"total_keystrokes"`
}
