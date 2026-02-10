package api

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"

	"game-activity-monitor/server/internal/api/handlers"
	"game-activity-monitor/server/internal/api/middleware"
	"game-activity-monitor/server/internal/auth"
	"game-activity-monitor/server/internal/storage"
)

func SetupRouter(store storage.Storage, jwtMgr *auth.JWTManager) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	if err := r.SetTrustedProxies([]string{"127.0.0.1", "::1"}); err != nil {
		log.Printf("SetTrustedProxies: %v", err)
	}

	r.Use(middleware.CORS())
	r.Use(middleware.RateLimit(300, time.Minute))

	deps := &handlers.Dependencies{
		Storage:    store,
		JWTManager: jwtMgr,
	}

	v1 := r.Group("/api/v1")

	v1.GET("/health", handlers.HealthCheck(deps))
	v1.POST("/auth/register", handlers.Register(deps))
	v1.POST("/auth/login", handlers.Login(deps))

	protected := v1.Group("/")
	protected.Use(middleware.AuthRequired(jwtMgr))

	metrics := protected.Group("/metrics")
	metrics.Use(middleware.RateLimit(600, time.Minute))
	{
		metrics.POST("/batch", handlers.ReceiveMetricsBatch(deps))
		metrics.GET("/recent", handlers.GetRecentMetrics(deps))
	}

	sessions := protected.Group("/sessions")
	{
		sessions.POST("/start", handlers.StartSession(deps))
		sessions.POST("/:id/end", handlers.EndSession(deps))
		sessions.GET("", handlers.GetSessions(deps))
		sessions.PATCH("/:id", handlers.PatchSession(deps))
		sessions.GET("/:id", handlers.GetSession(deps))
	}

	intervals := protected.Group("/intervals")
	{
		intervals.POST("", handlers.CreateActivityInterval(deps))
		intervals.GET("", handlers.ListActivityIntervals(deps))
	}

	protected.GET("/heatmap/:session_id", handlers.GetHeatmap(deps))

	export := protected.Group("/export")
	{
		export.GET("/csv", handlers.ExportCSV(deps))
		export.GET("/json", handlers.ExportJSON(deps))
	}

	return r
}
