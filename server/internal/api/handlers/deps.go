package handlers

import (
	"game-activity-monitor/server/internal/auth"
	"game-activity-monitor/server/internal/storage"
)

// Dependencies groups everything the handlers need.
// Constructed once in main and passed to router.SetupRouter.
type Dependencies struct {
	Storage    storage.Storage
	JWTManager *auth.JWTManager
}

// userID extracts the authenticated user's ID from the Gin context.
// Panics if AuthRequired middleware was not applied — intentional: it is a
// programmer error, not a runtime error.
func userID(c interface{ GetInt64(string) int64 }) int64 {
	return c.GetInt64("user_id")
}
