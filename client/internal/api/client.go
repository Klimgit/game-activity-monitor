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
type Client struct {
	baseURL       string
	httpClient    *http.Client
	flushInterval time.Duration
	store         *storage.LocalStorage

	// Credentials stored for automatic re-authentication on reconnect.
	email    string
	password string

	// Set after successful login.
	token     string
	userID    int64
	sessionID *int64

	// Session duration tracking.
	tracker *session.Tracker

	// Most-recently observed active process name from system_metrics events.
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
func (c *Client) UserID() int64 { return c.userID }

// CurrentSessionID returns the active session ID, or nil when no session is open.
func (c *Client) CurrentSessionID() *int64 { return c.sessionID }

// LastKnownProcess returns the most recently detected active process name.
// It is updated automatically by Enqueue() whenever a system_metrics event
// is received from the system collector, so it reflects the process that was
// using the most CPU at the last poll interval.
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
	c.token = resp.Token
	c.userID = resp.User.ID
	return nil
}

// ─── Event buffering ──────────────────────────────────────────────────────────

// Enqueue stamps the event with the current user/session IDs, updates internal
// state (process tracking, session activity), and persists it to the local
// SQLite buffer. The sync worker will forward it to the server.
func (c *Client) Enqueue(ctx context.Context, e *models.RawEvent) error {
	e.UserID = c.userID
	e.SessionID = c.sessionID

	// Extract and cache the active process name so StartSession can use it.
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

// StartSession opens a new session on the server, records its ID locally,
// and starts the duration tracker.
func (c *Client) StartSession(ctx context.Context, gameName string) error {
	var resp sessionResponse
	if err := c.post(ctx, "/api/v1/sessions/start", startSessionRequest{gameName}, &resp); err != nil {
		return fmt.Errorf("start session: %w", err)
	}
	sid := resp.ID
	c.sessionID = &sid
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
	if c.sessionID == nil {
		return nil // no active session
	}
	total, active, afk, score := c.tracker.Stop()
	req := endSessionRequest{
		TotalDuration:  total,
		ActiveDuration: active,
		AFKDuration:    afk,
		ActivityScore:  score,
	}
	path := fmt.Sprintf("/api/v1/sessions/%d/end", *c.sessionID)
	if err := c.post(ctx, path, req, nil); err != nil {
		return fmt.Errorf("end session: %w", err)
	}
	c.sessionID = nil
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
// Labels must be sent immediately so the ground-truth timestamps are accurate.
func (c *Client) SendLabel(ctx context.Context, state string) error {
	req := labelRequest{
		SessionID: c.sessionID,
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
			// Best-effort final flush on shutdown.
			shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			c.flush(shutCtx)
			cancel()
			return
		}
	}
}

func (c *Client) flush(ctx context.Context) {
	// Auto-reconnect: if we have credentials but no token, try to log in first.
	if c.token == "" {
		if c.email == "" {
			return // no credentials configured
		}
		if err := c.Login(ctx, c.email, c.password); err != nil {
			return // server still unreachable; retry next tick
		}
	}

	for {
		events, ids, err := c.store.FetchBatch(ctx, syncBatchSize)
		if err != nil || len(events) == 0 {
			return
		}

		if err := c.sendBatch(ctx, events); err != nil {
			// Server unreachable — keep events in SQLite, retry next tick.
			// Clear the token so the next flush attempt will re-authenticate.
			c.token = ""
			return
		}

		// Successfully sent; remove from local buffer.
		if err := c.store.DeleteByIDs(ctx, ids); err != nil {
			// Rare: events were sent but deletion failed.
			// They will be re-sent on the next flush (server must be idempotent).
			return
		}

		if len(events) < syncBatchSize {
			return // no more pending events
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
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
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
