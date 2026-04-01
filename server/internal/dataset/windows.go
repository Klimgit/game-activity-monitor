package dataset

import (
	"context"
	"encoding/csv"
	"io"
	"sort"
	"strconv"
	"time"

	"game-activity-monitor/server/internal/models"
	"game-activity-monitor/server/internal/storage"
)

type windowKey struct {
	sid int64
	t   time.Time
}

func WriteDatasetWindowsCSV(ctx context.Context, w io.Writer, st storage.Storage, userID int64, from, to time.Time, sessionID *int64, trainingOnly, includeHeader bool) error {
	intervals, err := st.ListActivityIntervals(ctx, userID, sessionID, from, to)
	if err != nil {
		return err
	}
	bySession := make(map[int64][]*models.ActivityInterval)
	for _, iv := range intervals {
		bySession[iv.SessionID] = append(bySession[iv.SessionID], iv)
	}

	rows, err := st.SessionWindowsForUser(ctx, userID, from, to, sessionID)
	if err != nil {
		return err
	}

	order := make([]windowKey, 0, len(rows))
	for _, r := range rows {
		if !r.SessionID.Valid {
			continue
		}
		order = append(order, windowKey{sid: r.SessionID.Int64, t: r.WindowStart})
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].sid != order[j].sid {
			return order[i].sid < order[j].sid
		}
		return order[i].t.Before(order[j].t)
	})
	idxMap := make(map[windowKey]int)
	var cur int64 = -1
	var n int
	for _, k := range order {
		if k.sid != cur {
			cur = k.sid
			n = 0
		}
		idxMap[k] = n
		n++
	}

	cw := csv.NewWriter(w)
	// Column order matches session_windows feature columns (then label), for parity with DB exports.
	header := []string{
		"user_id", "session_id", "window_index", "window_start", "window_end", "game_name",
		"duration_s", "mouse_moves", "mouse_clicks", "speed_avg", "speed_max",
		"keystrokes", "key_hold_avg_ms", "active_process",
		"cpu_avg", "cpu_max", "mem_avg", "gpu_util_avg", "gpu_temp_avg",
		"key_press_interval_avg_ms", "key_w", "key_a", "key_s", "key_d",
		"cursor_accel_avg", "cursor_accel_max", "foreground_window_title", "title_match_score",
		"gpu_mem_avg_mb", "label",
	}
	if includeHeader {
		if err := cw.Write(header); err != nil {
			return err
		}
	}

	for _, r := range rows {
		if !r.SessionID.Valid {
			continue
		}
		sid := r.SessionID.Int64
		ivs := bySession[sid]
		lbl, ok := majorityLabel(r.WindowStart, r.WindowEnd, ivs, sid)
		if trainingOnly && !ok {
			continue
		}
		if !ok {
			lbl = ""
		}
		widx := strconv.Itoa(idxMap[windowKey{sid: sid, t: r.WindowStart}])
		ssid := strconv.FormatInt(sid, 10)
		row := []string{
			strconv.FormatInt(r.UserID, 10),
			ssid,
			widx,
			r.WindowStart.UTC().Format(time.RFC3339Nano),
			r.WindowEnd.UTC().Format(time.RFC3339Nano),
			r.GameName,
			strconv.FormatFloat(r.DurationS, 'f', 4, 64),
			strconv.Itoa(r.MouseMoves),
			strconv.Itoa(r.MouseClicks),
			strconv.FormatFloat(r.SpeedAvg, 'f', 6, 64),
			strconv.FormatFloat(r.SpeedMax, 'f', 6, 64),
			strconv.Itoa(r.Keystrokes),
			strconv.FormatFloat(r.KeyHoldAvgMs, 'f', 4, 64),
			r.ActiveProcess,
			strconv.FormatFloat(r.CPUAvg, 'f', 4, 64),
			strconv.FormatFloat(r.CPUMax, 'f', 4, 64),
			strconv.FormatFloat(r.MemAvg, 'f', 4, 64),
			strconv.FormatFloat(r.GPUUtilAvg, 'f', 4, 64),
			strconv.FormatFloat(r.GPUTempAvg, 'f', 4, 64),
			strconv.FormatFloat(r.KeyPressIntervalAvgMs, 'f', 4, 64),
			strconv.Itoa(r.KeyW),
			strconv.Itoa(r.KeyA),
			strconv.Itoa(r.KeyS),
			strconv.Itoa(r.KeyD),
			strconv.FormatFloat(r.CursorAccelAvg, 'f', 6, 64),
			strconv.FormatFloat(r.CursorAccelMax, 'f', 6, 64),
			r.ForegroundWindowTitle,
			TitleMatchScoreCSV(r.GameName, r.ForegroundWindowTitle),
			strconv.FormatFloat(r.GPUMemAvgMB, 'f', 4, 64),
			lbl,
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func overlapSeconds(ws, we, a, b time.Time) float64 {
	if !ws.Before(we) {
		return 0
	}
	s := ws
	if a.After(s) {
		s = a
	}
	e := we
	if b.Before(e) {
		e = b
	}
	if !s.Before(e) {
		return 0
	}
	return e.Sub(s).Seconds()
}

func majorityLabel(ws, we time.Time, intervals []*models.ActivityInterval, sessionID int64) (label string, ok bool) {
	type pair struct {
		state string
		sec   float64
	}
	var pairs []pair
	for _, st := range []string{"active_gameplay", "afk", "loading", "menu"} {
		var sec float64
		for _, iv := range intervals {
			if iv.SessionID != sessionID || iv.State != st {
				continue
			}
			sec += overlapSeconds(ws, we, iv.StartAt, iv.EndAt)
		}
		if sec > 0 {
			pairs = append(pairs, pair{state: st, sec: sec})
		}
	}
	if len(pairs) == 0 {
		return "", false
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].sec != pairs[j].sec {
			return pairs[i].sec > pairs[j].sec
		}
		return pairs[i].state < pairs[j].state
	})
	return pairs[0].state, true
}
