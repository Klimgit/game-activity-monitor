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

// parseExportFilters extracts the shared from/to/game query parameters used by
// both export endpoints.
func parseExportFilters(c *gin.Context) (from, to time.Time, game string) {
	if s := c.Query("from"); s != "" {
		from, _ = time.Parse("2006-01-02", s)
	}
	if s := c.Query("to"); s != "" {
		to, _ = time.Parse("2006-01-02", s)
		if !to.IsZero() {
			to = to.Add(24*time.Hour - time.Nanosecond)
		}
	}
	game = c.Query("game")
	return
}

// ExportCSV streams session data as a UTF-8 CSV file.
// Query params: from (YYYY-MM-DD), to (YYYY-MM-DD), game.
func ExportCSV(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetInt64("user_id")
		from, to, game := parseExportFilters(c)

		sessions, err := deps.Storage.GetSessions(c.Request.Context(), uid, from, to, game)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch sessions"})
			return
		}

		var buf bytes.Buffer
		w := csv.NewWriter(&buf)

		_ = w.Write([]string{
			"session_id", "session_start", "session_end",
			"game_name",
			"total_duration_s", "active_duration_s", "afk_duration_s",
			"activity_score", "state",
		})

		for _, s := range sessions {
			sessionEnd := ""
			if s.SessionEnd != nil {
				sessionEnd = s.SessionEnd.UTC().Format(time.RFC3339)
			}
			_ = w.Write([]string{
				strconv.FormatInt(s.ID, 10),
				s.SessionStart.UTC().Format(time.RFC3339),
				sessionEnd,
				s.GameName,
				strconv.Itoa(s.TotalDuration),
				strconv.Itoa(s.ActiveDuration),
				strconv.Itoa(s.AFKDuration),
				strconv.FormatFloat(s.ActivityScore, 'f', 4, 64),
				s.State,
			})
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

// ExportJSON streams session data as a JSON file download.
// Query params: from (YYYY-MM-DD), to (YYYY-MM-DD), game.
func ExportJSON(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetInt64("user_id")
		from, to, game := parseExportFilters(c)

		sessions, err := deps.Storage.GetSessions(c.Request.Context(), uid, from, to, game)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch sessions"})
			return
		}
		if sessions == nil {
			sessions = nil // keep JSON null rather than []
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
