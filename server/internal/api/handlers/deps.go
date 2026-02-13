package handlers

import (
	"game-activity-monitor/server/internal/auth"
	"game-activity-monitor/server/internal/storage"
)

type Dependencies struct {
	Storage               storage.Storage
	JWTManager            *auth.JWTManager
	MLInferenceConfigured bool
}
