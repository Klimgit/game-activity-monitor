package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func HealthCheck(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := deps.Storage.Ping(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"error":  err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status":                  "ok",
			"ml_inference_configured": deps.MLInferenceConfigured,
		})
	}
}
