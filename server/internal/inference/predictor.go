package inference

import "context"

// Predictor runs batch classification on window feature rows (same schema as training CSV features).
type Predictor interface {
	PredictBatch(ctx context.Context, rows []map[string]interface{}) ([]string, error)
}
