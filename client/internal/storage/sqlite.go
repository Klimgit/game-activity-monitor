package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
    timestamp  INTEGER  NOT NULL,
    event_type TEXT     NOT NULL,
    data       TEXT     NOT NULL,
    created_at INTEGER  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_pending_ts ON pending_events (timestamp ASC);
`

type LocalStorage struct {
	db *sql.DB
}

func New(path string) (*LocalStorage, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("storage.New: open %s: %w", path, err)
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		if cerr := db.Close(); cerr != nil {
			return nil, fmt.Errorf("storage.New: enable WAL: %w (close db: %w)", err, cerr)
		}
		return nil, fmt.Errorf("storage.New: enable WAL: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		if cerr := db.Close(); cerr != nil {
			return nil, fmt.Errorf("storage.New: apply schema: %w (close db: %w)", err, cerr)
		}
		return nil, fmt.Errorf("storage.New: apply schema: %w", err)
	}

	return &LocalStorage{db: db}, nil
}

func (s *LocalStorage) Close() error {
	return s.db.Close()
}

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

func (s *LocalStorage) FetchBatch(ctx context.Context, limit int) (events []*models.RawEvent, ids []int64, err error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, session_id, timestamp, event_type, data
		FROM   pending_events
		ORDER  BY timestamp ASC
		LIMIT  ?`, limit)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			err = errors.Join(err, cerr)
		}
	}()

	for rows.Next() {
		var (
			id        int64
			userID    int64
			sessionID sql.NullInt64
			tsNano    int64
			evType    string
			dataStr   string
		)
		if err = rows.Scan(&id, &userID, &sessionID, &tsNano, &evType, &dataStr); err != nil {
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

	err = rows.Err()
	return events, ids, err
}

func (s *LocalStorage) PendingCount(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pending_events`).Scan(&n)
	return n, err
}

func (s *LocalStorage) DeleteBySessionID(ctx context.Context, sessionID int64) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM pending_events WHERE session_id = ?`, sessionID)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return n, err
}

func (s *LocalStorage) DeleteByIDs(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]

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
