package handlers

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func parseExportFilters(c *gin.Context) (from, to time.Time, game string, err error) {
	if s := c.Query("from"); s != "" {
		from, err = time.ParseInLocation("2006-01-02", s, time.UTC)
		if err != nil {
			return time.Time{}, time.Time{}, "", err
		}
	}
	if s := c.Query("to"); s != "" {
		var t time.Time
		t, err = time.ParseInLocation("2006-01-02", s, time.UTC)
		if err != nil {
			return from, time.Time{}, "", err
		}
		to = t.Add(24*time.Hour - time.Nanosecond)
	}
	game = c.Query("game")
	return from, to, game, nil
}

func writeCSVRow(w *csv.Writer, row []string) error {
	return w.Write(row)
}

func ExportCSV(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetInt64("user_id")
		from, to, game, err := parseExportFilters(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from or to date (use YYYY-MM-DD)"})
			return
		}

		sessions, err := deps.Storage.GetSessions(c.Request.Context(), uid, from, to, game)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch sessions"})
			return
		}

		var buf bytes.Buffer
		w := csv.NewWriter(&buf)

		if err := writeCSVRow(w, []string{
			"session_id", "session_start", "session_end",
			"game_name",
			"total_duration_s", "active_duration_s", "afk_duration_s",
			"activity_score", "state",
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "csv encoding error"})
			return
		}

		for _, s := range sessions {
			sessionEnd := ""
			if s.SessionEnd != nil {
				sessionEnd = s.SessionEnd.UTC().Format(time.RFC3339)
			}
			if err := writeCSVRow(w, []string{
				strconv.FormatInt(s.ID, 10),
				s.SessionStart.UTC().Format(time.RFC3339),
				sessionEnd,
				s.GameName,
				strconv.Itoa(s.TotalDuration),
				strconv.Itoa(s.ActiveDuration),
				strconv.Itoa(s.AFKDuration),
				strconv.FormatFloat(s.ActivityScore, 'f', 4, 64),
				s.State,
			}); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "csv encoding error"})
				return
			}
		}
		w.Flush()

		if err := w.Error(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "csv encoding error"})
			return
		}

		c.Header("Content-Disposition", `attachment; filename="game-activity.csv"`)
		c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
	}
}

func ExportJSON(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetInt64("user_id")
		from, to, game, err := parseExportFilters(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from or to date (use YYYY-MM-DD)"})
			return
		}

		sessions, err := deps.Storage.GetSessions(c.Request.Context(), uid, from, to, game)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch sessions"})
			return
		}
		if sessions == nil {
			sessions = nil
		}

		payload := struct {
			ExportedAt time.Time   `json:"exported_at"`
			Count      int         `json:"count"`
			Sessions   interface{} `json:"sessions"`
		}{
			ExportedAt: time.Now().UTC(),
			Count:      len(sessions),
			Sessions:   sessions,
		}

		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "json encoding error"})
			return
		}

		c.Header("Content-Disposition", `attachment; filename="game-activity.json"`)
		c.Data(http.StatusOK, "application/json; charset=utf-8", data)
	}
}
