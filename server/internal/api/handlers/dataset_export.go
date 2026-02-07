package handlers

import (
	"bytes"
	"encoding/csv"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"game-activity-monitor/server/internal/models"
)

// overlapSeconds returns the length of intersection of [ws,we) and [a,b) in seconds.
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

// majorityLabel returns the state with maximum overlap seconds with [ws,we);
// tie-break: lexicographically smallest state name.
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

// ExportDatasetWindowsCSV streams 10s (or any) feature rows with a label column for ML.
// Query: from, to (YYYY-MM-DD), session_id (optional), training_only (default true) — omit rows with no label overlap.
func ExportDatasetWindowsCSV(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetInt64("user_id")
		from, to, err := parseTimeRangeQuery(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		var sessionID *int64
		if s := c.Query("session_id"); s != "" {
			id, e := strconv.ParseInt(s, 10, 64)
			if e != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "bad session_id"})
				return
			}
			sessionID = &id
		}
		trainingOnly := c.DefaultQuery("training_only", "true") != "false"

		intervals, err := deps.Storage.ListActivityIntervals(c.Request.Context(), uid, sessionID, from, to)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load intervals"})
			return
		}
		bySession := make(map[int64][]*models.ActivityInterval)
		for _, iv := range intervals {
			bySession[iv.SessionID] = append(bySession[iv.SessionID], iv)
		}

		rows, err := deps.Storage.SessionWindowsForUser(c.Request.Context(), uid, from, to, sessionID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load windows"})
			return
		}

		// window_index per session
		type key struct {
			sid int64
			t   time.Time
		}
		order := make([]key, 0, len(rows))
		for _, r := range rows {
			if !r.SessionID.Valid {
				continue
			}
			order = append(order, key{sid: r.SessionID.Int64, t: r.WindowStart})
		}
		sort.Slice(order, func(i, j int) bool {
			if order[i].sid != order[j].sid {
				return order[i].sid < order[j].sid
			}
			return order[i].t.Before(order[j].t)
		})
		idxMap := make(map[key]int)
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

		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		header := []string{
			"user_id", "session_id", "window_index", "window_start", "window_end", "duration_s",
			"mouse_moves", "mouse_clicks", "speed_avg", "speed_max", "keystrokes", "key_hold_avg_ms",
			"active_process", "cpu_avg", "cpu_max", "mem_avg", "gpu_util_avg", "gpu_temp_avg", "label",
		}
		if err := writeCSVRow(w, header); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "csv encoding error"})
			return
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
			widx := strconv.Itoa(idxMap[key{sid: sid, t: r.WindowStart}])
			ssid := strconv.FormatInt(sid, 10)
			if err := writeCSVRow(w, []string{
				strconv.FormatInt(r.UserID, 10),
				ssid,
				widx,
				r.WindowStart.UTC().Format(time.RFC3339Nano),
				r.WindowEnd.UTC().Format(time.RFC3339Nano),
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
				lbl,
			}); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "csv encoding error"})
				return
			}
		}
		w.Flush()
		if err := w.Error(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "csv error"})
			return
		}
		c.Header("Content-Disposition", `attachment; filename="dataset-windows.csv"`)
		c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
	}
}

// ExportPlaytimeJSON returns summed seconds per state from activity_intervals.
func ExportPlaytimeJSON(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetInt64("user_id")
		from, to, err := parseTimeRangeQuery(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		var sessionID *int64
		if s := c.Query("session_id"); s != "" {
			id, e := strconv.ParseInt(s, 10, 64)
			if e != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "bad session_id"})
				return
			}
			sessionID = &id
		}

		m, err := deps.Storage.PlaytimeByState(c.Request.Context(), uid, from, to, sessionID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to aggregate playtime"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"from":              from,
			"to":                to,
			"seconds_by_state":  m,
			"active_gameplay_s": m["active_gameplay"],
		})
	}
}
