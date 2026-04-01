package storage

import (
	"context"
	"time"

	"game-activity-monitor/server/internal/models"
)

type Storage interface {
	// --- Users ---
	CreateUser(ctx context.Context, email, passwordHash string) (*models.User, error)
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
	// ListUserIDs returns all user ids (for internal dataset export).
	ListUserIDs(ctx context.Context) ([]int64, error)

	// --- Raw events (hypertable) ---
	SaveEventsBatch(ctx context.Context, events []*models.RawEvent) error
	// GetRecentEvents returns events for the given user created after `since`.
	GetRecentEvents(ctx context.Context, userID int64, since time.Time) ([]*models.RawEvent, error)

	// --- Sessions ---
	CreateSession(ctx context.Context, s *models.Session) (*models.Session, error)
	UpdateSession(ctx context.Context, s *models.Session) error
	// UpdateSessionGameName sets game_name for a session owned by userID; returns the updated row.
	UpdateSessionGameName(ctx context.Context, sessionID, userID int64, gameName string) (*models.Session, error)
	GetSessions(ctx context.Context, userID int64, from, to time.Time, game string) ([]*models.Session, error)
	GetSessionByID(ctx context.Context, id, userID int64) (*models.Session, error)

	// --- Activity intervals (ML ground truth) ---
	CreateActivityInterval(ctx context.Context, iv *models.ActivityInterval) (*models.ActivityInterval, error)
	ListActivityIntervals(ctx context.Context, userID int64, sessionID *int64, from, to time.Time) ([]*models.ActivityInterval, error)
	// SessionWindowsForUser returns feature rows for export/join with intervals.
	SessionWindowsForUser(ctx context.Context, userID int64, from, to time.Time, sessionID *int64) ([]SessionWindowRow, error)
	// PlaytimeByState sums interval lengths per state for the user in [from, to].
	PlaytimeByState(ctx context.Context, userID int64, from, to time.Time, sessionID *int64) (map[string]int64, error)
	// MLPlaytimeBySessionIDs sums session_windows.duration_s by ml_predicted_state per session (inference labels).
	MLPlaytimeBySessionIDs(ctx context.Context, userID int64, sessionIDs []int64) (map[int64]map[string]int64, error)
	// WindowMetricsSummary aggregates session_windows in the time range (overlap with [from, to]).
	WindowMetricsSummary(ctx context.Context, userID int64, from, to time.Time) (*WindowMetricsSummary, error)
	// WindowMLStates returns ml_predicted_state for matching (time, session_id) rows.
	WindowMLStates(ctx context.Context, userID int64, keys []WindowKey) (map[WindowKey]string, error)

	// --- Heatmap ---
	GetHeatmapData(ctx context.Context, sessionID, userID int64) ([]models.ClickPoint, error)

	// --- Infra ---
	Ping(ctx context.Context) error
	Close() error
}
