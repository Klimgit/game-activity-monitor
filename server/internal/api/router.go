package api

import (
	"time"

	"github.com/gin-gonic/gin"

	"game-activity-monitor/server/internal/api/handlers"
	"game-activity-monitor/server/internal/api/middleware"
	"game-activity-monitor/server/internal/auth"
	"game-activity-monitor/server/internal/storage"
)

// SetupRouter builds the Gin engine with all routes and middleware configured.
func SetupRouter(store storage.Storage, jwtMgr *auth.JWTManager) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	// Trust only loopback addresses so c.ClientIP() resolves correctly when the
	// server runs behind a reverse proxy or inside Docker. Without this, Gin
	// trusts all proxies and rate-limiting by IP becomes ineffective.
	_ = r.SetTrustedProxies([]string{"127.0.0.1", "::1"})

	r.Use(middleware.CORS())
	r.Use(middleware.RateLimit(300, time.Minute)) // 300 req/min per IP globally

	deps := &handlers.Dependencies{
		Storage:    store,
		JWTManager: jwtMgr,
	}

	v1 := r.Group("/api/v1")

	// ── Public ──────────────────────────────────────────────────────────────
	v1.GET("/health", handlers.HealthCheck(deps))
	v1.POST("/auth/register", handlers.Register(deps))
	v1.POST("/auth/login", handlers.Login(deps))

	// ── Protected ───────────────────────────────────────────────────────────
	auth := v1.Group("/")
	auth.Use(middleware.AuthRequired(jwtMgr))

	// Metrics (high-throughput; generous rate limit for batch uploads)
	metrics := auth.Group("/metrics")
	metrics.Use(middleware.RateLimit(600, time.Minute))
	{
		metrics.POST("/batch", handlers.ReceiveMetricsBatch(deps))
		metrics.GET("/recent", handlers.GetRecentMetrics(deps))
	}

	// Sessions
	sessions := auth.Group("/sessions")
	{
		sessions.POST("/start", handlers.StartSession(deps))
		sessions.POST("/:id/end", handlers.EndSession(deps))
		sessions.GET("", handlers.GetSessions(deps))
		sessions.GET("/:id", handlers.GetSession(deps))
	}

	// Activity labels (dataset annotation)
	labels := auth.Group("/labels")
	{
		labels.POST("", handlers.CreateLabel(deps))
		labels.GET("", handlers.GetLabels(deps))
	}

	// Heatmap
	auth.GET("/heatmap/:session_id", handlers.GetHeatmap(deps))

	// Export (stubs)
	export := auth.Group("/export")
	{
		export.GET("/csv", handlers.ExportCSV(deps))
		export.GET("/json", handlers.ExportJSON(deps))
	}

	return r
}
