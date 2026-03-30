package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"game-activity-monitor/server/internal/models"
)

func validatedSessionIDs(c *gin.Context, deps *Dependencies, uid int64, events []*models.RawEvent) map[int64]bool {
	seen := make(map[int64]struct{})
	for _, e := range events {
		if e != nil && e.SessionID != nil {
			seen[*e.SessionID] = struct{}{}
		}
	}
	valid := make(map[int64]bool, len(seen))
	for sid := range seen {
		s, err := deps.Storage.GetSessionByID(c.Request.Context(), sid, uid)
		if err == nil && s != nil {
			valid[sid] = true
		}
	}
	return valid
}

// maxBatchSize caps the number of events accepted in a single request.
// This prevents a misbehaving or malicious client from allocating unbounded
// memory on the server. The client sync worker sends at most 1 000 events per
// flush, so 5 000 gives comfortable headroom without risking DoS.
const maxBatchSize = 5_000

// ReceiveMetricsBatch accepts a JSON array of raw events from the desktop client.
// The user_id on each event is overwritten with the authenticated user's ID
// to prevent clients from spoofing other users' data.
func ReceiveMetricsBatch(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetInt64("user_id")

		var events []*models.RawEvent
		if err := c.ShouldBindJSON(&events); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if len(events) > maxBatchSize {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("batch too large: %d events (max %d)", len(events), maxBatchSize),
			})
			return
		}

		validSIDs := validatedSessionIDs(c, deps, uid, events)

		for _, e := range events {
			if e == nil {
				continue
			}
			e.UserID = uid
			if e.SessionID != nil && !validSIDs[*e.SessionID] {
				e.SessionID = nil
			}
		}

		clean := events[:0]
		for _, e := range events {
			if e != nil {
				clean = append(clean, e)
			}
		}
		events = clean

		if err := deps.Storage.SaveEventsBatch(c.Request.Context(), events); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save events"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"saved": len(events)})
	}
}

func GetRecentMetrics(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetInt64("user_id")

		seconds := 30
		if s := c.Query("seconds"); s != "" {
			if v, err := strconv.Atoi(s); err == nil && v > 0 && v <= 300 {
				seconds = v
			}
		}

		since := time.Now().Add(-time.Duration(seconds) * time.Second)
		events, err := deps.Storage.GetRecentEvents(c.Request.Context(), uid, since)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch events"})
			return
		}
		if events == nil {
			events = []*models.RawEvent{}
		}

		c.JSON(http.StatusOK, events)
	}
}
