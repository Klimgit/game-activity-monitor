package storage

import (
	"context"
	"time"

	"game-activity-monitor/server/internal/models"
)

// Storage is the persistence interface used by all API handlers.
// The single implementation is TimescaleStorage, but the interface
// makes unit-testing handlers straightforward.
type Storage interface {
	// --- Users ---
	CreateUser(ctx context.Context, email, passwordHash string) (*models.User, error)
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)

	// --- Raw events (hypertable) ---
	SaveEventsBatch(ctx context.Context, events []*models.RawEvent) error
	// GetRecentEvents returns events for the given user created after `since`.
	GetRecentEvents(ctx context.Context, userID int64, since time.Time) ([]*models.RawEvent, error)

	// --- Sessions ---
	CreateSession(ctx context.Context, s *models.Session) (*models.Session, error)
	UpdateSession(ctx context.Context, s *models.Session) error
	GetSessions(ctx context.Context, userID int64, from, to time.Time, game string) ([]*models.Session, error)
	GetSessionByID(ctx context.Context, id, userID int64) (*models.Session, error)

	// --- Activity labels ---
	CreateLabel(ctx context.Context, l *models.ActivityLabel) (*models.ActivityLabel, error)
	GetLabels(ctx context.Context, userID int64, sessionID *int64) ([]*models.ActivityLabel, error)

	// --- Heatmap ---
	GetHeatmapData(ctx context.Context, sessionID, userID int64) ([]models.ClickPoint, error)

	// --- Infra ---
	Ping(ctx context.Context) error
	Close() error
}
