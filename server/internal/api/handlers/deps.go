package handlers

import (
	"game-activity-monitor/server/internal/auth"
	"game-activity-monitor/server/internal/storage"
)

type Dependencies struct {
	Storage    storage.Storage
	JWTManager *auth.JWTManager
	// MLInferenceConfigured is true when ML_INFERENCE_URL was set at server start (ingest calls the classifier).
	MLInferenceConfigured bool
}
