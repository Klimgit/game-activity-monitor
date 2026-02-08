package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"game-activity-monitor/server/internal/models"
)

func GetHeatmap(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetInt64("user_id")

		sessionID, err := strconv.ParseInt(c.Param("session_id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
			return
		}

		points, err := deps.Storage.GetHeatmapData(c.Request.Context(), sessionID, uid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch heatmap data"})
			return
		}
		if points == nil {
			points = []models.ClickPoint{}
		}

		c.JSON(http.StatusOK, points)
	}
}
