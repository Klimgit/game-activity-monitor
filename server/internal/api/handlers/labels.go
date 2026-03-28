package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"game-activity-monitor/server/internal/models"
)

type createLabelRequest struct {
	SessionID *int64    `json:"session_id"`
	Timestamp time.Time `json:"timestamp"`
	// State: "active_gameplay" | "afk" | "menu" | "loading"
	State string `json:"state" binding:"required"`
	// Source: "manual_hotkey" | "auto_detected"
	Source string `json:"source" binding:"required"`
}

// CreateLabel stores a manual or auto-detected activity label.
func CreateLabel(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetInt64("user_id")

		var req createLabelRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ts := req.Timestamp
		if ts.IsZero() {
			ts = time.Now().UTC()
		}

		label := &models.ActivityLabel{
			UserID:    uid,
			SessionID: req.SessionID,
			Timestamp: ts,
			State:     req.State,
			Source:    req.Source,
		}

		created, err := deps.Storage.CreateLabel(c.Request.Context(), label)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create label"})
			return
		}

		c.JSON(http.StatusCreated, created)
	}
}

// GetLabels returns labels for the authenticated user, optionally filtered by session.
func GetLabels(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetInt64("user_id")

		var sessionID *int64
		if s := c.Query("session_id"); s != "" {
			id, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
				return
			}
			sessionID = &id
		}

		labels, err := deps.Storage.GetLabels(c.Request.Context(), uid, sessionID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch labels"})
			return
		}
		if labels == nil {
			labels = []*models.ActivityLabel{}
		}

		c.JSON(http.StatusOK, labels)
	}
}
