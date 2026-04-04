package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"game-activity-monitor/client/internal/models"
	"game-activity-monitor/client/internal/session"
	"game-activity-monitor/client/internal/storage"
)

type Client struct {
	baseURL       string
	httpClient    *http.Client
	flushInterval time.Duration
	store         *storage.LocalStorage
	mu            sync.Mutex
	token         string
	userID        int64
	sessionID     *int64
	email         string
	password      string
	tracker       *session.Tracker
}

func NewClient(baseURL string, flushInterval time.Duration, store *storage.LocalStorage) *Client {
	return &Client{
		baseURL:       baseURL,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		flushInterval: flushInterval,
		store:         store,
		tracker:       session.NewTracker(),
	}
}

func (c *Client) UserID() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.userID
}

func (c *Client) CurrentSessionID() *int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionID
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
	User  struct {
		ID    int64  `json:"id"`
		Email string `json:"email"`
	} `json:"user"`
}

func (c *Client) SetCredentials(email, password string) {
	c.mu.Lock()
	c.email = email
	c.password = password
	c.mu.Unlock()
}

func (c *Client) Login(ctx context.Context, email, password string) error {
	var resp loginResponse
	if err := c.post(ctx, "/api/v1/auth/login", loginRequest{email, password}, &resp); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	c.mu.Lock()
	c.token = resp.Token
	c.userID = resp.User.ID
	c.mu.Unlock()
	return nil
}

func (c *Client) Enqueue(ctx context.Context, e *models.RawEvent) error {
	c.mu.Lock()
	e.UserID = c.userID
	e.SessionID = c.sessionID
	c.mu.Unlock()

	switch e.EventType {
	case models.EventMouseMove, models.EventMouseClick,
		models.EventKeyPress, models.EventKeyRelease:
		c.tracker.RecordInput()
	}

	return c.store.Save(ctx, e)
}

type startSessionRequest struct {
	GameName string `json:"game_name"`
}

type sessionResponse struct {
	ID int64 `json:"id"`
}

func (c *Client) StartSession(ctx context.Context, gameName string) error {
	c.mu.Lock()
	if c.sessionID != nil {
		c.mu.Unlock()
		return fmt.Errorf("session %d is already active; end it first", *c.sessionID)
	}
	c.mu.Unlock()

	var resp sessionResponse
	if err := c.post(ctx, "/api/v1/sessions/start", startSessionRequest{gameName}, &resp); err != nil {
		return fmt.Errorf("start session: %w", err)
	}
	sid := resp.ID

	c.mu.Lock()
	c.sessionID = &sid
	c.mu.Unlock()

	c.tracker.Start()
	return nil
}

type endSessionRequest struct {
	TotalDuration  int     `json:"total_duration"`
	ActiveDuration int     `json:"active_duration"`
	AFKDuration    int     `json:"afk_duration"`
	ActivityScore  float64 `json:"activity_score"`
}

func (c *Client) EndSession(ctx context.Context) error {
	c.mu.Lock()
	if c.sessionID == nil {
		c.mu.Unlock()
		return nil
	}
	sid := *c.sessionID
	c.mu.Unlock()

	if err := c.flushUntilEmpty(ctx); err != nil {
		return fmt.Errorf("end session: %w", err)
	}

	total, active, afk, score := c.tracker.Stop()
	req := endSessionRequest{
		TotalDuration:  total,
		ActiveDuration: active,
		AFKDuration:    afk,
		ActivityScore:  score,
	}
	path := fmt.Sprintf("/api/v1/sessions/%d/end", sid)
	if err := c.post(ctx, path, req, nil); err != nil {
		return fmt.Errorf("end session: %w", err)
	}

	c.mu.Lock()
	c.sessionID = nil
	c.mu.Unlock()

	if n, err := c.store.DeleteBySessionID(ctx, sid); err != nil {
		log.Printf("storage: delete session %d pending rows: %v", sid, err)
	} else if n > 0 {
		log.Printf("storage: cleared %d local row(s) for ended session %d", n, sid)
	}
	return nil
}

type createIntervalRequest struct {
	SessionID int64     `json:"session_id"`
	State     string    `json:"state"`
	StartAt   time.Time `json:"start_at"`
	EndAt     time.Time `json:"end_at"`
}

func (c *Client) CreateActivityInterval(ctx context.Context, state string, start, end time.Time) error {
	c.mu.Lock()
	sid := c.sessionID
	c.mu.Unlock()
	if sid == nil {
		return fmt.Errorf("no active session — start a session before marking intervals")
	}

	req := createIntervalRequest{
		SessionID: *sid,
		State:     state,
		StartAt:   start.UTC(),
		EndAt:     end.UTC(),
	}
	if err := c.post(ctx, "/api/v1/intervals", req, nil); err != nil {
		return fmt.Errorf("create interval: %w", err)
	}
	return nil
}

const syncBatchSize = 1000

type offlineBackoff struct {
	failures  int
	nextRetry time.Time
	offline   bool
}

const offlineThreshold = 3
const maxBackoff = 5 * time.Minute

func (b *offlineBackoff) ready() bool {
	return b.nextRetry.IsZero() || time.Now().After(b.nextRetry)
}

func (b *offlineBackoff) recordFailure(base time.Duration) {
	b.failures++
	shift := b.failures - 1
	if shift > 6 {
		shift = 6
	}
	delay := base * (1 << uint(shift))
	if delay > maxBackoff {
		delay = maxBackoff
	}
	b.nextRetry = time.Now().Add(delay)

	if !b.offline && b.failures >= offlineThreshold {
		b.offline = true
		log.Printf("sync: server unreachable after %d attempts — entering offline mode (retry in %s)",
			b.failures, delay.Round(time.Second))
	}
}

func (b *offlineBackoff) recordSuccess() {
	if b.offline {
		log.Printf("sync: server reachable again — leaving offline mode")
	}
	b.failures = 0
	b.offline = false
	b.nextRetry = time.Time{}
}

func (c *Client) StartSyncWorker(ctx context.Context) {
	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

	var bo offlineBackoff

	for {
		select {
		case <-ticker.C:
			if !bo.ready() {
				continue
			}
			if c.flush(context.Background()) {
				bo.recordSuccess()
			} else {
				bo.recordFailure(c.flushInterval)
			}

		case <-ctx.Done():
			shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			c.flush(shutCtx)
			cancel()
			return
		}
	}
}

func (c *Client) flushUntilEmpty(ctx context.Context) error {
	const (
		maxWait = 45 * time.Second
		poll    = 25 * time.Millisecond
	)
	deadline := time.Now().Add(maxWait)
	for {
		if !c.flush(ctx) {
			return fmt.Errorf("cannot reach server to sync metrics")
		}
		n, err := c.store.PendingCount(ctx)
		if err != nil {
			return fmt.Errorf("pending count: %w", err)
		}
		if n == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout: %d metric event(s) still queued", n)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(poll):
		}
	}
}

func (c *Client) flush(ctx context.Context) bool {
	c.mu.Lock()
	tok := c.token
	email := c.email
	password := c.password
	c.mu.Unlock()

	if tok == "" {
		if email == "" {
			return true // nothing to do — client not configured yet
		}
		if err := c.Login(ctx, email, password); err != nil {
			return false // server unreachable
		}
	}

	for {
		events, ids, err := c.store.FetchBatch(ctx, syncBatchSize)
		if err != nil || len(events) == 0 {
			return true // queue empty or unreadable; either way not a network failure
		}

		if err := c.sendBatch(ctx, events); err != nil {
			c.mu.Lock()
			c.token = ""
			c.mu.Unlock()
			return false
		}

		if err := c.store.DeleteByIDs(ctx, ids); err != nil {
			return true // delivered but cleanup failed; non-fatal
		}

		if len(events) < syncBatchSize {
			return true
		}
	}
}

func (c *Client) sendBatch(ctx context.Context, events []*models.RawEvent) error {
	return c.post(ctx, "/api/v1/metrics/batch", events, nil)
}

func (c *Client) post(ctx context.Context, path string, body, out interface{}) error {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	c.mu.Lock()
	tok := c.token
	c.mu.Unlock()
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			log.Printf("api: close response body: %v", cerr)
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errBody struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(respBody, &errBody); err != nil || errBody.Error == "" {
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, errBody.Error)
	}

	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
