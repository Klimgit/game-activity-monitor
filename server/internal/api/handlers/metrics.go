package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"game-activity-monitor/server/internal/models"
	"game-activity-monitor/server/internal/storage"
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

const maxBatchSize = 5_000

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

		var keys []storage.WindowKey
		for _, e := range events {
			if e == nil || e.EventType != models.EventWindowMetrics || e.SessionID == nil {
				continue
			}
			keys = append(keys, storage.WindowKey{
				Time:      storage.NormalizeWindowTime(e.Timestamp),
				SessionID: *e.SessionID,
			})
		}
		if len(keys) > 0 {
			states, err := deps.Storage.WindowMLStates(c.Request.Context(), uid, keys)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load ml states"})
				return
			}
			for _, e := range events {
				if e == nil || e.EventType != models.EventWindowMetrics || e.SessionID == nil {
					continue
				}
				k := storage.WindowKey{Time: storage.NormalizeWindowTime(e.Timestamp), SessionID: *e.SessionID}
				st, ok := states[k]
				if !ok {
					continue
				}
				var m map[string]interface{}
				if err := json.Unmarshal(e.Data, &m); err != nil || m == nil {
					m = map[string]interface{}{}
				}
				m["ml_predicted_state"] = st
				raw, err := json.Marshal(m)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enrich events"})
					return
				}
				e.Data = raw
			}
		}

		c.JSON(http.StatusOK, events)
	}
}

func GetWindowMetricsSummary(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetInt64("user_id")

		fromStr := c.Query("from")
		toStr := c.Query("to")
		if fromStr == "" || toStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "from and to are required (YYYY-MM-DD)"})
			return
		}
		from, err := time.ParseInLocation("2006-01-02", fromStr, time.UTC)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from date (use YYYY-MM-DD)"})
			return
		}
		toDay, err := time.ParseInLocation("2006-01-02", toStr, time.UTC)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid to date (use YYYY-MM-DD)"})
			return
		}
		to := toDay.Add(24*time.Hour - time.Nanosecond)

		summary, err := deps.Storage.WindowMetricsSummary(c.Request.Context(), uid, from, to)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load window summary"})
			return
		}
		c.JSON(http.StatusOK, summary)
	}
}
