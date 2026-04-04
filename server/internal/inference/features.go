package inference

import (
	"strings"

	"game-activity-monitor/server/internal/models"
	"game-activity-monitor/server/internal/titlematch"
)

func WindowFeatureRow(w *models.WindowMetricsData, gameName string) map[string]interface{} {
	gn := strings.TrimSpace(gameName)
	tms := titleMatchNumeric(gn, w.ForegroundWindowTitle)

	row := map[string]interface{}{
		"duration_s":                w.DurationS,
		"mouse_moves":               float64(w.MouseMoves),
		"mouse_clicks":              float64(w.MouseClicks),
		"speed_avg":                 w.SpeedAvg,
		"speed_max":                 w.SpeedMax,
		"keystrokes":                float64(w.Keystrokes),
		"key_hold_avg_ms":           w.KeyHoldAvgMs,
		"key_press_interval_avg_ms": w.KeyPressIntervalAvgMs,
		"key_w":                     float64(w.KeyW),
		"key_a":                     float64(w.KeyA),
		"key_s":                     float64(w.KeyS),
		"key_d":                     float64(w.KeyD),
		"cursor_accel_avg":          w.CursorAccelAvg,
		"cursor_accel_max":          w.CursorAccelMax,
		"cpu_avg":                   w.CPUAvg,
		"cpu_max":                   w.CPUMax,
		"mem_avg":                   w.MemAvg,
		"gpu_util_avg":              w.GPUUtilAvg,
		"gpu_temp_avg":              w.GPUTempAvg,
		"gpu_mem_avg_mb":            w.GPUMemAvgMB,
		"title_match_score":         tms,
		"active_process":            w.ActiveProcess,
		"foreground_window_title":   w.ForegroundWindowTitle,
		"game_name":                 gn,
	}
	return row
}

func titleMatchNumeric(gameName, windowTitle string) float64 {
	if strings.TrimSpace(gameName) == "" {
		return 0
	}
	s := titlematch.TitleMatchScore(gameName, windowTitle)
	if s < 0 {
		return 0
	}
	return s
}
