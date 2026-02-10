package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"game-activity-monitor/server/internal/models"
)

type startSessionRequest struct {
	GameName string `json:"game_name"`
}

func StartSession(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetInt64("user_id")

		var req startSessionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		s := &models.Session{
			UserID:       uid,
			SessionStart: time.Now().UTC(),
			GameName:     req.GameName,
			State:        "active",
		}

		created, err := deps.Storage.CreateSession(c.Request.Context(), s)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
			return
		}

		c.JSON(http.StatusCreated, created)
	}
}

type endSessionRequest struct {
	TotalDuration  int     `json:"total_duration"`
	ActiveDuration int     `json:"active_duration"`
	AFKDuration    int     `json:"afk_duration"`
	ActivityScore  float64 `json:"activity_score"`
}

func EndSession(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetInt64("user_id")

		sessionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
			return
		}

		s, err := deps.Storage.GetSessionByID(c.Request.Context(), sessionID, uid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
		if s == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		if s.State == "ended" {
			c.JSON(http.StatusConflict, gin.H{"error": "session is already ended"})
			return
		}

		var req endSessionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		now := time.Now().UTC()
		s.SessionEnd = &now
		s.TotalDuration = req.TotalDuration
		s.ActiveDuration = req.ActiveDuration
		s.AFKDuration = req.AFKDuration
		s.ActivityScore = req.ActivityScore
		s.State = "ended"

		if err := deps.Storage.UpdateSession(c.Request.Context(), s); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update session"})
			return
		}

		c.JSON(http.StatusOK, s)
	}
}

func GetSessions(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetInt64("user_id")

		var from, to time.Time
		if s := c.Query("from"); s != "" {
			t, err := time.ParseInLocation("2006-01-02", s, time.UTC)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from date (use YYYY-MM-DD)"})
				return
			}
			from = t
		}
		if s := c.Query("to"); s != "" {
			t, err := time.ParseInLocation("2006-01-02", s, time.UTC)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid to date (use YYYY-MM-DD)"})
				return
			}
			to = t.Add(24*time.Hour - time.Nanosecond) // include whole day
		}

		sessions, err := deps.Storage.GetSessions(c.Request.Context(), uid, from, to, c.Query("game"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch sessions"})
			return
		}
		if sessions == nil {
			sessions = []*models.Session{}
		}

		if len(sessions) > 0 {
			ids := make([]int64, len(sessions))
			for i, s := range sessions {
				ids[i] = s.ID
			}
			ml, err := deps.Storage.MLPlaytimeBySessionIDs(c.Request.Context(), uid, ids)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load ml playtime"})
				return
			}
			for _, s := range sessions {
				if m := ml[s.ID]; len(m) > 0 {
					s.MLPlaytimeSeconds = m
				}
			}
		}

		c.JSON(http.StatusOK, sessions)
	}
}

func GetSession(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetInt64("user_id")

		sessionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
			return
		}

		s, err := deps.Storage.GetSessionByID(c.Request.Context(), sessionID, uid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
		if s == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}

		ml, err := deps.Storage.MLPlaytimeBySessionIDs(c.Request.Context(), uid, []int64{sessionID})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load ml playtime"})
			return
		}
		if m := ml[sessionID]; len(m) > 0 {
			s.MLPlaytimeSeconds = m
		}

		c.JSON(http.StatusOK, s)
	}
}

type patchSessionRequest struct {
	GameName string `json:"game_name"`
}

// PatchSession updates editable session fields (currently only game_name).
func PatchSession(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetInt64("user_id")

		sessionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
			return
		}

		var req patchSessionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		name := strings.TrimSpace(req.GameName)

		updated, err := deps.Storage.UpdateSessionGameName(c.Request.Context(), sessionID, uid, name)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update session"})
			return
		}
		if updated == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}

		c.JSON(http.StatusOK, updated)
	}
}
