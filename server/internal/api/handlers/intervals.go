package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"game-activity-monitor/server/internal/models"
)

var validIntervalStates = map[string]bool{
	"active_gameplay": true,
	"afk":             true,
	"menu":            true,
	"loading":         true,
}

type createIntervalRequest struct {
	SessionID int64     `json:"session_id" binding:"required"`
	State     string    `json:"state"        binding:"required"`
	StartAt   time.Time `json:"start_at"     binding:"required"`
	EndAt     time.Time `json:"end_at"       binding:"required"`
}

// CreateActivityInterval stores one closed interval for ML ground truth.
func CreateActivityInterval(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createIntervalRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
			return
		}
		if !validIntervalStates[req.State] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state"})
			return
		}
		uid := c.GetInt64("user_id")

		iv := &models.ActivityInterval{
			UserID:    uid,
			SessionID: req.SessionID,
			State:     req.State,
			StartAt:   req.StartAt.UTC(),
			EndAt:     req.EndAt.UTC(),
		}

		created, err := deps.Storage.CreateActivityInterval(c.Request.Context(), iv)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, created)
	}
}

func ListActivityIntervals(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetInt64("user_id")
		var sessionID *int64
		if s := c.Query("session_id"); s != "" {
			id, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "bad session_id"})
				return
			}
			sessionID = &id
		}
		from, to, err := parseTimeRangeQuery(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		list, err := deps.Storage.ListActivityIntervals(c.Request.Context(), uid, sessionID, from, to)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list intervals"})
			return
		}
		if list == nil {
			list = []*models.ActivityInterval{}
		}
		c.JSON(http.StatusOK, list)
	}
}

func parseTimeRangeQuery(c *gin.Context) (from, to time.Time, err error) {
	from = time.Unix(0, 0).UTC()
	to = time.Now().UTC().Add(24 * time.Hour)
	if s := c.Query("from"); s != "" {
		from, err = time.Parse("2006-01-02", s)
		if err != nil {
			return from, to, err
		}
		from = from.UTC()
	}
	if s := c.Query("to"); s != "" {
		t, e := time.Parse("2006-01-02", s)
		if e != nil {
			return from, to, e
		}
		to = t.UTC().Add(24*time.Hour - time.Nanosecond)
	}
	return from, to, nil
}
