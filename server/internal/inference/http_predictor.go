package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type HTTPPredictor struct {
	baseURL string
	client  *http.Client
}

func NewHTTPPredictor(baseURL string) *HTTPPredictor {
	return &HTTPPredictor{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		client:  &http.Client{Timeout: 20 * time.Second},
	}
}

type predictRequest struct {
	Rows []map[string]interface{} `json:"rows"`
}

type predictResponse struct {
	Predictions []string `json:"predictions"`
	Error       string   `json:"error,omitempty"`
}

func (p *HTTPPredictor) PredictBatch(ctx context.Context, rows []map[string]interface{}) ([]string, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	if p.baseURL == "" {
		return make([]string, len(rows)), nil
	}
	body, err := json.Marshal(predictRequest{Rows: rows})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/predict", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var out predictResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode inference response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if out.Error != "" {
			return nil, fmt.Errorf("inference %s: %s", resp.Status, out.Error)
		}
		return nil, fmt.Errorf("inference: %s", resp.Status)
	}
	if len(out.Predictions) != len(rows) {
		return nil, fmt.Errorf("inference: got %d predictions for %d rows", len(out.Predictions), len(rows))
	}
	return out.Predictions, nil
}
