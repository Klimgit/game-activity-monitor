package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"game-activity-monitor/server/internal/models"
)

type startSessionRequest struct {
	GameName string `json:"game_name"`
}

// StartSession creates a new gaming session for the authenticated user.
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

// EndSession closes an active session, recording its final durations.
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

// GetSessions returns a filtered list of sessions for the authenticated user.
// Query params: from (YYYY-MM-DD), to (YYYY-MM-DD), game (string).
func GetSessions(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetInt64("user_id")

		var from, to time.Time
		if s := c.Query("from"); s != "" {
			from, _ = time.Parse("2006-01-02", s)
		}
		if s := c.Query("to"); s != "" {
			to, _ = time.Parse("2006-01-02", s)
			if !to.IsZero() {
				to = to.Add(24*time.Hour - time.Nanosecond) // include whole day
			}
		}

		sessions, err := deps.Storage.GetSessions(c.Request.Context(), uid, from, to, c.Query("game"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch sessions"})
			return
		}
		if sessions == nil {
			sessions = []*models.Session{}
		}

		c.JSON(http.StatusOK, sessions)
	}
}

// GetSession returns a single session by ID.
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

		c.JSON(http.StatusOK, s)
	}
}
