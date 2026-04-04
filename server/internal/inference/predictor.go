package inference

import "context"

type Predictor interface {
	PredictBatch(ctx context.Context, rows []map[string]interface{}) ([]string, error)
}
