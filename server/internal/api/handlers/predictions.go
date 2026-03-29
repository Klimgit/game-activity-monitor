package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"game-activity-monitor/server/internal/models"
)

// ListPredictions returns stored per-window model predictions for the timeline UI.
func ListPredictions(deps *Dependencies) gin.HandlerFunc {
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

		list, err := deps.Storage.ListPredictedWindows(c.Request.Context(), uid, sessionID, from, to)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list predictions"})
			return
		}
		if list == nil {
			list = []*models.PredictedWindow{}
		}
		c.JSON(http.StatusOK, list)
	}
}

type batchPredictionsRequest struct {
	Windows []struct {
		SessionID      int64    `json:"session_id"       binding:"required"`
		WindowStart    string   `json:"window_start"     binding:"required"`
		WindowEnd      string   `json:"window_end"       binding:"required"`
		PredictedState string   `json:"predicted_state"  binding:"required"`
		Confidence     *float64 `json:"confidence"`
		ModelVersion   string   `json:"model_version"`
	} `json:"windows" binding:"required"`
}

// UpsertPredictionsBatch ingests per-window predictions (inference job or service).
func UpsertPredictionsBatch(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req batchPredictionsRequest
		if err := c.ShouldBindJSON(&req); err != nil || len(req.Windows) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body: need windows[]"})
			return
		}
		uid := c.GetInt64("user_id")
		rows := make([]*models.PredictedWindow, 0, len(req.Windows))
		for _, w := range req.Windows {
			if !validIntervalStates[w.PredictedState] {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid predicted_state: " + w.PredictedState})
				return
			}
			ws, err := time.Parse(time.RFC3339Nano, w.WindowStart)
			if err != nil {
				ws, err = time.Parse(time.RFC3339, w.WindowStart)
			}
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "bad window_start"})
				return
			}
			we, err := time.Parse(time.RFC3339Nano, w.WindowEnd)
			if err != nil {
				we, err = time.Parse(time.RFC3339, w.WindowEnd)
			}
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "bad window_end"})
				return
			}
			rows = append(rows, &models.PredictedWindow{
				SessionID:      w.SessionID,
				WindowStart:    ws.UTC(),
				WindowEnd:      we.UTC(),
				PredictedState: w.PredictedState,
				Confidence:     w.Confidence,
				ModelVersion:   w.ModelVersion,
			})
		}
		if err := deps.Storage.UpsertPredictedWindowsBatch(c.Request.Context(), uid, rows); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "count": len(rows)})
	}
}
