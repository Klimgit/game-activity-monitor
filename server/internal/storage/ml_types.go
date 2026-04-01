package storage

import "time"

// NormalizeWindowTime aligns event/DB timestamps for map keys and joins (Postgres timestamptz resolution).
func NormalizeWindowTime(t time.Time) time.Time {
	return t.UTC().Truncate(time.Microsecond)
}

// WindowKey identifies one row in session_windows (matches raw window_metrics event timestamp + session).
type WindowKey struct {
	Time      time.Time
	SessionID int64
}

// WindowMetricsSummary aggregates session_windows for a dashboard date range.
type WindowMetricsSummary struct {
	MLPlaytimeSeconds map[string]int64 `json:"ml_playtime_seconds"`
	WindowCount       int64            `json:"window_count"`
	TotalDurationS    float64          `json:"total_duration_s"`
	TotalMouseMoves   int64            `json:"total_mouse_moves"`
	TotalKeystrokes   int64            `json:"total_keystrokes"`
}
