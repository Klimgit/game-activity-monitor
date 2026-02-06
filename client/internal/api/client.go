package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"game-activity-monitor/client/internal/models"
	"game-activity-monitor/client/internal/session"
	"game-activity-monitor/client/internal/storage"
)

// Client handles authentication and communication with the game-monitor server.
// Unsent events are buffered in the local SQLite storage; the sync worker
// flushes them on a regular interval.
//
// Thread-safety: all exported methods are safe for concurrent use. The fields
// token, userID, and sessionID are guarded by mu; lastProcess by procMu.
type Client struct {
	baseURL       string
	httpClient    *http.Client
	flushInterval time.Duration
	store         *storage.LocalStorage

	// mu guards token, userID, and sessionID which are accessed from the
	// forwardEvents goroutine, the hotkey goroutine, and the sync worker.
	mu        sync.Mutex
	token     string
	userID    int64
	sessionID *int64

	// Credentials stored for automatic re-authentication on reconnect.
	email    string
	password string

	// Session duration tracking.
	tracker *session.Tracker

	// procMu guards lastProcess updated by system_metrics events.
	procMu      sync.Mutex
	lastProcess string
}

// NewClient constructs a Client. store must be non-nil.
func NewClient(baseURL string, flushInterval time.Duration, store *storage.LocalStorage) *Client {
	return &Client{
		baseURL:       baseURL,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		flushInterval: flushInterval,
		store:         store,
		tracker:       session.NewTracker(),
	}
}

// UserID returns the ID of the currently authenticated user (0 if not logged in).
func (c *Client) UserID() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.userID
}

// CurrentSessionID returns the active session ID, or nil when no session is open.
func (c *Client) CurrentSessionID() *int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionID
}

// LastKnownProcess returns the most recently detected active process name.
// Updated automatically by Enqueue() from system_metrics events.
func (c *Client) LastKnownProcess() string {
	c.procMu.Lock()
	defer c.procMu.Unlock()
	return c.lastProcess
}

// ─── Authentication ───────────────────────────────────────────────────────────

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

// SetCredentials stores the email/password so the sync worker can
// automatically re-authenticate after a connection loss.
func (c *Client) SetCredentials(email, password string) {
	c.email = email
	c.password = password
}

// Login authenticates with the server and stores the JWT + user ID.
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

// ─── Event buffering ──────────────────────────────────────────────────────────

// Enqueue stamps the event with the current user/session IDs, updates process
// tracking and session activity state, and persists to the local SQLite buffer.
func (c *Client) Enqueue(ctx context.Context, e *models.RawEvent) error {
	c.mu.Lock()
	e.UserID = c.userID
	e.SessionID = c.sessionID
	c.mu.Unlock()

	// Extract and cache the active process name for StartSession auto-detection.
	if e.EventType == models.EventSystemMetrics {
		var data models.SystemMetricsData
		if err := json.Unmarshal(e.Data, &data); err == nil && data.ActiveProcess != "" {
			c.procMu.Lock()
			c.lastProcess = data.ActiveProcess
			c.procMu.Unlock()
		}
	}

	// Notify the session tracker of user input activity.
	switch e.EventType {
	case models.EventMouseMove, models.EventMouseClick,
		models.EventKeyPress, models.EventKeyRelease:
		c.tracker.RecordInput()
	}

	return c.store.Save(ctx, e)
}

// ─── Sessions ─────────────────────────────────────────────────────────────────

type startSessionRequest struct {
	GameName string `json:"game_name"`
}

type sessionResponse struct {
	ID int64 `json:"id"`
}

// StartSession opens a new session on the server and starts the duration tracker.
// Returns an error if a session is already active — call EndSession first.
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

// EndSession stops the tracker, computes real durations, closes the active
// session on the server, and clears the local session ID.
func (c *Client) EndSession(ctx context.Context) error {
	c.mu.Lock()
	if c.sessionID == nil {
		c.mu.Unlock()
		return nil // no active session
	}
	sid := *c.sessionID
	c.mu.Unlock()

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
	return nil
}

// ─── Labels ───────────────────────────────────────────────────────────────────

type labelRequest struct {
	SessionID *int64    `json:"session_id,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	State     string    `json:"state"`
	Source    string    `json:"source"`
}

// SendLabel posts an activity label directly to the server (bypasses the queue).
func (c *Client) SendLabel(ctx context.Context, state string) error {
	c.mu.Lock()
	sid := c.sessionID
	c.mu.Unlock()

	req := labelRequest{
		SessionID: sid,
		Timestamp: time.Now().UTC(),
		State:     state,
		Source:    "manual_hotkey",
	}
	if err := c.post(ctx, "/api/v1/labels", req, nil); err != nil {
		return fmt.Errorf("send label: %w", err)
	}
	return nil
}

// ─── Sync worker ──────────────────────────────────────────────────────────────

const syncBatchSize = 1000

// StartSyncWorker runs a loop that flushes pending SQLite events to the server.
// It blocks until ctx is cancelled, then performs one final flush.
func (c *Client) StartSyncWorker(ctx context.Context) {
	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.flush(context.Background())
		case <-ctx.Done():
			shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			c.flush(shutCtx)
			cancel()
			return
		}
	}
}

func (c *Client) flush(ctx context.Context) {
	c.mu.Lock()
	tok := c.token
	email := c.email
	password := c.password
	c.mu.Unlock()

	// Auto-reconnect: re-authenticate if token is missing.
	if tok == "" {
		if email == "" {
			return // no credentials configured
		}
		if err := c.Login(ctx, email, password); err != nil {
			return // server still unreachable; retry next tick
		}
	}

	for {
		events, ids, err := c.store.FetchBatch(ctx, syncBatchSize)
		if err != nil || len(events) == 0 {
			return
		}

		if err := c.sendBatch(ctx, events); err != nil {
			// Server unreachable — clear token so next flush re-authenticates.
			c.mu.Lock()
			c.token = ""
			c.mu.Unlock()
			return
		}

		if err := c.store.DeleteByIDs(ctx, ids); err != nil {
			return
		}

		if len(events) < syncBatchSize {
			return
		}
	}
}

func (c *Client) sendBatch(ctx context.Context, events []*models.RawEvent) error {
	return c.post(ctx, "/api/v1/metrics/batch", events, nil)
}

// ─── HTTP helper ──────────────────────────────────────────────────────────────

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
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, errBody.Error)
	}

	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
