package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"game-activity-monitor/client/internal/models"
)

const schema = `
CREATE TABLE IF NOT EXISTS pending_events (
    id         INTEGER  PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER  NOT NULL,
    session_id INTEGER,
    timestamp  INTEGER  NOT NULL, -- unix nano
    event_type TEXT     NOT NULL,
    data       TEXT     NOT NULL, -- JSON
    created_at INTEGER  NOT NULL  -- unix nano
);

CREATE INDEX IF NOT EXISTS idx_pending_ts ON pending_events (timestamp ASC);
`

// LocalStorage is a SQLite-backed offline buffer for raw events.
// It persists events when the server is unreachable and allows
// the sync worker to flush them once connectivity is restored.
type LocalStorage struct {
	db *sql.DB
}

// New opens (or creates) the SQLite database at path and applies the schema.
func New(path string) (*LocalStorage, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("storage.New: open %s: %w", path, err)
	}

	// Single writer to avoid SQLITE_BUSY on concurrent goroutines.
	db.SetMaxOpenConns(1)

	// WAL mode: faster writes, no exclusive lock during reads.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage.New: enable WAL: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage.New: apply schema: %w", err)
	}

	return &LocalStorage{db: db}, nil
}

// Close releases the database connection.
func (s *LocalStorage) Close() error {
	return s.db.Close()
}

// Save persists a single event to the local buffer.
func (s *LocalStorage) Save(ctx context.Context, e *models.RawEvent) error {
	dataJSON, err := json.Marshal(e.Data)
	if err != nil {
		return fmt.Errorf("storage.Save: marshal data: %w", err)
	}

	var sessionID sql.NullInt64
	if e.SessionID != nil {
		sessionID = sql.NullInt64{Int64: *e.SessionID, Valid: true}
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO pending_events (user_id, session_id, timestamp, event_type, data, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		e.UserID,
		sessionID,
		e.Timestamp.UnixNano(),
		string(e.EventType),
		string(dataJSON),
		time.Now().UnixNano(),
	)
	return err
}

// SaveBatch persists multiple events in a single transaction.
func (s *LocalStorage) SaveBatch(ctx context.Context, events []*models.RawEvent) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO pending_events (user_id, session_id, timestamp, event_type, data, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().UnixNano()
	for _, e := range events {
		dataJSON, err := json.Marshal(e.Data)
		if err != nil {
			return fmt.Errorf("storage.SaveBatch: marshal data: %w", err)
		}

		var sessionID sql.NullInt64
		if e.SessionID != nil {
			sessionID = sql.NullInt64{Int64: *e.SessionID, Valid: true}
		}

		if _, err := stmt.ExecContext(ctx,
			e.UserID, sessionID,
			e.Timestamp.UnixNano(),
			string(e.EventType),
			string(dataJSON),
			now,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// pendingRow is an internal struct that also carries the row id for deletion.
type pendingRow struct {
	id    int64
	event *models.RawEvent
}

// FetchBatch reads up to limit pending events ordered by timestamp ascending.
func (s *LocalStorage) FetchBatch(ctx context.Context, limit int) ([]*models.RawEvent, []int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, session_id, timestamp, event_type, data
		FROM   pending_events
		ORDER  BY timestamp ASC
		LIMIT  ?`, limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var events []*models.RawEvent
	var ids []int64

	for rows.Next() {
		var (
			id        int64
			userID    int64
			sessionID sql.NullInt64
			tsNano    int64
			evType    string
			dataStr   string
		)
		if err := rows.Scan(&id, &userID, &sessionID, &tsNano, &evType, &dataStr); err != nil {
			return nil, nil, err
		}

		e := &models.RawEvent{
			UserID:    userID,
			Timestamp: time.Unix(0, tsNano),
			EventType: models.EventType(evType),
			Data:      json.RawMessage(dataStr),
		}
		if sessionID.Valid {
			sid := sessionID.Int64
			e.SessionID = &sid
		}

		events = append(events, e)
		ids = append(ids, id)
	}

	return events, ids, rows.Err()
}

// DeleteByIDs removes successfully-synced events in a single DELETE … IN (…)
// statement, which is dramatically faster than N individual deletes.
func (s *LocalStorage) DeleteByIDs(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	// Build "DELETE FROM pending_events WHERE id IN (?,?,…)"
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1] // trim trailing comma

	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	_, err := s.db.ExecContext(ctx,
		"DELETE FROM pending_events WHERE id IN ("+placeholders+")",
		args...,
	)
	return err
}

// PendingCount returns the number of events waiting to be synced.
func (s *LocalStorage) PendingCount(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pending_events`).Scan(&n)
	return n, err
}
