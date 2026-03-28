package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/lib/pq"

	"game-activity-monitor/server/internal/models"
)

// TimescaleStorage implements Storage against a TimescaleDB (PostgreSQL) instance.
type TimescaleStorage struct {
	db *sql.DB
}

// NewTimescaleStorage wraps an already-opened *sql.DB.
func NewTimescaleStorage(db *sql.DB) *TimescaleStorage {
	return &TimescaleStorage{db: db}
}

func (ts *TimescaleStorage) Ping(ctx context.Context) error {
	return ts.db.PingContext(ctx)
}

func (ts *TimescaleStorage) Close() error {
	return ts.db.Close()
}

// ─── Users ────────────────────────────────────────────────────────────────────

func (ts *TimescaleStorage) CreateUser(ctx context.Context, email, passwordHash string) (*models.User, error) {
	var u models.User
	err := ts.db.QueryRowContext(ctx, `
		INSERT INTO users (email, password_hash)
		VALUES ($1, $2)
		RETURNING id, email, password_hash, created_at, updated_at`,
		email, passwordHash,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("storage.CreateUser: %w", err)
	}
	return &u, nil
}

func (ts *TimescaleStorage) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	err := ts.db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, created_at, updated_at
		FROM   users
		WHERE  email = $1`,
		email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("storage.GetUserByEmail: %w", err)
	}
	return &u, nil
}

// ─── Raw events ───────────────────────────────────────────────────────────────

func (ts *TimescaleStorage) SaveEventsBatch(ctx context.Context, events []*models.RawEvent) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := ts.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("storage.SaveEventsBatch begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	rawStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO raw_input_events (user_id, session_id, timestamp, event_type, data)
		VALUES ($1, $2, $3, $4, $5)`)
	if err != nil {
		return fmt.Errorf("storage.SaveEventsBatch prepare raw: %w", err)
	}
	defer rawStmt.Close()

	// Prepared statement for the long-term click store used by the heatmap.
	clickStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO click_events (user_id, session_id, timestamp, x, y, button)
		VALUES ($1, $2, $3, $4, $5, $6)`)
	if err != nil {
		return fmt.Errorf("storage.SaveEventsBatch prepare click: %w", err)
	}
	defer clickStmt.Close()

	// Prepared statement for aggregated window metrics used for ML training.
	winStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO session_windows
		    (time, user_id, session_id, window_start, window_end, duration_s,
		     mouse_moves, mouse_clicks, speed_avg, speed_max,
		     keystrokes, key_hold_avg_ms, active_process,
		     cpu_avg, cpu_max, mem_avg, gpu_util_avg, gpu_temp_avg)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`)
	if err != nil {
		return fmt.Errorf("storage.SaveEventsBatch prepare window: %w", err)
	}
	defer winStmt.Close()

	for _, e := range events {
		dataJSON, err := json.Marshal(e.Data)
		if err != nil {
			return fmt.Errorf("storage.SaveEventsBatch marshal: %w", err)
		}
		var sessionID sql.NullInt64
		if e.SessionID != nil {
			sessionID = sql.NullInt64{Int64: *e.SessionID, Valid: true}
		}
		if _, err := rawStmt.ExecContext(ctx, e.UserID, sessionID, e.Timestamp, string(e.EventType), dataJSON); err != nil {
			return fmt.Errorf("storage.SaveEventsBatch exec raw: %w", err)
		}

		// Mirror mouse_click events into click_events for long-term heatmap storage.
		if e.EventType == models.EventMouseClick {
			var click models.MouseClickData
			if err := json.Unmarshal(e.Data, &click); err == nil {
				if _, err := clickStmt.ExecContext(ctx,
					e.UserID, sessionID, e.Timestamp,
					click.X, click.Y, click.Button,
				); err != nil {
					return fmt.Errorf("storage.SaveEventsBatch exec click: %w", err)
				}
			}
		}

		// Mirror window_metrics events into session_windows for ML training.
		if e.EventType == models.EventWindowMetrics {
			var w models.WindowMetricsData
			if err := json.Unmarshal(e.Data, &w); err == nil {
				if _, err := winStmt.ExecContext(ctx,
					e.Timestamp, e.UserID, sessionID,
					w.WindowStart, w.WindowEnd, w.DurationS,
					w.MouseMoves, w.MouseClicks, w.SpeedAvg, w.SpeedMax,
					w.Keystrokes, w.KeyHoldAvgMs, w.ActiveProcess,
					w.CPUAvg, w.CPUMax, w.MemAvg, w.GPUUtilAvg, w.GPUTempAvg,
				); err != nil {
					return fmt.Errorf("storage.SaveEventsBatch exec window: %w", err)
				}
			}
		}
	}

	return tx.Commit()
}

func (ts *TimescaleStorage) GetRecentEvents(ctx context.Context, userID int64, since time.Time) ([]*models.RawEvent, error) {
	rows, err := ts.db.QueryContext(ctx, `
		SELECT user_id, session_id, timestamp, event_type, data
		FROM   raw_input_events
		WHERE  user_id = $1 AND timestamp >= $2
		ORDER  BY timestamp DESC
		LIMIT  500`,
		userID, since,
	)
	if err != nil {
		return nil, fmt.Errorf("storage.GetRecentEvents: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

func scanEvents(rows *sql.Rows) ([]*models.RawEvent, error) {
	var events []*models.RawEvent
	for rows.Next() {
		var (
			e         models.RawEvent
			sessionID sql.NullInt64
			dataBytes []byte
		)
		if err := rows.Scan(&e.UserID, &sessionID, &e.Timestamp, &e.EventType, &dataBytes); err != nil {
			return nil, err
		}
		if sessionID.Valid {
			sid := sessionID.Int64
			e.SessionID = &sid
		}
		e.Data = json.RawMessage(dataBytes)
		events = append(events, &e)
	}
	return events, rows.Err()
}

// ─── Sessions ─────────────────────────────────────────────────────────────────

func (ts *TimescaleStorage) CreateSession(ctx context.Context, s *models.Session) (*models.Session, error) {
	var created models.Session
	var sessionEnd sql.NullTime

	err := ts.db.QueryRowContext(ctx, `
		INSERT INTO activity_sessions (user_id, session_start, game_name, state)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, session_start, session_end, game_name,
		          total_duration, active_duration, afk_duration,
		          activity_score, state, created_at, updated_at`,
		s.UserID, s.SessionStart, s.GameName, "active",
	).Scan(
		&created.ID, &created.UserID, &created.SessionStart, &sessionEnd,
		&created.GameName, &created.TotalDuration, &created.ActiveDuration,
		&created.AFKDuration, &created.ActivityScore, &created.State,
		&created.CreatedAt, &created.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("storage.CreateSession: %w", err)
	}
	if sessionEnd.Valid {
		created.SessionEnd = &sessionEnd.Time
	}
	return &created, nil
}

func (ts *TimescaleStorage) UpdateSession(ctx context.Context, s *models.Session) error {
	res, err := ts.db.ExecContext(ctx, `
		UPDATE activity_sessions
		SET    session_end     = $1,
		       total_duration  = $2,
		       active_duration = $3,
		       afk_duration    = $4,
		       activity_score  = $5,
		       state           = $6,
		       updated_at      = NOW()
		WHERE  id = $7 AND user_id = $8`,
		s.SessionEnd, s.TotalDuration, s.ActiveDuration,
		s.AFKDuration, s.ActivityScore, s.State, s.ID, s.UserID,
	)
	if err != nil {
		return fmt.Errorf("storage.UpdateSession: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("storage.UpdateSession rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("storage.UpdateSession: session %d not found or not owned by user %d", s.ID, s.UserID)
	}
	return nil
}

func (ts *TimescaleStorage) GetSessions(ctx context.Context, userID int64, from, to time.Time, game string) ([]*models.Session, error) {
	query := `
		SELECT id, user_id, session_start, session_end, game_name,
		       total_duration, active_duration, afk_duration,
		       activity_score, state, created_at, updated_at
		FROM   activity_sessions
		WHERE  user_id = $1`

	args := []interface{}{userID}
	n := 2

	if !from.IsZero() {
		query += fmt.Sprintf(" AND session_start >= $%d", n)
		args = append(args, from)
		n++
	}
	if !to.IsZero() {
		query += fmt.Sprintf(" AND session_start <= $%d", n)
		args = append(args, to)
		n++
	}
	if game != "" {
		query += fmt.Sprintf(" AND game_name = $%d", n)
		args = append(args, game)
	}
	query += " ORDER BY session_start DESC LIMIT 200"

	rows, err := ts.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("storage.GetSessions: %w", err)
	}
	defer rows.Close()
	return scanSessions(rows)
}

func (ts *TimescaleStorage) GetSessionByID(ctx context.Context, id, userID int64) (*models.Session, error) {
	var s models.Session
	var sessionEnd sql.NullTime

	err := ts.db.QueryRowContext(ctx, `
		SELECT id, user_id, session_start, session_end, game_name,
		       total_duration, active_duration, afk_duration,
		       activity_score, state, created_at, updated_at
		FROM   activity_sessions
		WHERE  id = $1 AND user_id = $2`,
		id, userID,
	).Scan(
		&s.ID, &s.UserID, &s.SessionStart, &sessionEnd, &s.GameName,
		&s.TotalDuration, &s.ActiveDuration, &s.AFKDuration,
		&s.ActivityScore, &s.State, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("storage.GetSessionByID: %w", err)
	}
	if sessionEnd.Valid {
		s.SessionEnd = &sessionEnd.Time
	}
	return &s, nil
}

func scanSessions(rows *sql.Rows) ([]*models.Session, error) {
	var sessions []*models.Session
	for rows.Next() {
		var s models.Session
		var sessionEnd sql.NullTime
		err := rows.Scan(
			&s.ID, &s.UserID, &s.SessionStart, &sessionEnd, &s.GameName,
			&s.TotalDuration, &s.ActiveDuration, &s.AFKDuration,
			&s.ActivityScore, &s.State, &s.CreatedAt, &s.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if sessionEnd.Valid {
			s.SessionEnd = &sessionEnd.Time
		}
		sessions = append(sessions, &s)
	}
	return sessions, rows.Err()
}

// ─── Activity labels ──────────────────────────────────────────────────────────

func (ts *TimescaleStorage) CreateLabel(ctx context.Context, l *models.ActivityLabel) (*models.ActivityLabel, error) {
	var created models.ActivityLabel
	var sessionID sql.NullInt64
	if l.SessionID != nil {
		sessionID = sql.NullInt64{Int64: *l.SessionID, Valid: true}
	}

	err := ts.db.QueryRowContext(ctx, `
		INSERT INTO activity_labels (user_id, session_id, timestamp, state, source)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, user_id, session_id, timestamp, state, source, created_at`,
		l.UserID, sessionID, l.Timestamp, l.State, l.Source,
	).Scan(
		&created.ID, &created.UserID, &sessionID,
		&created.Timestamp, &created.State, &created.Source, &created.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("storage.CreateLabel: %w", err)
	}
	if sessionID.Valid {
		sid := sessionID.Int64
		created.SessionID = &sid
	}
	return &created, nil
}

func (ts *TimescaleStorage) GetLabels(ctx context.Context, userID int64, sessionID *int64) ([]*models.ActivityLabel, error) {
	query := `
		SELECT id, user_id, session_id, timestamp, state, source, created_at
		FROM   activity_labels
		WHERE  user_id = $1`
	args := []interface{}{userID}

	if sessionID != nil {
		query += " AND session_id = $2"
		args = append(args, *sessionID)
	}
	query += " ORDER BY timestamp DESC LIMIT 1000"

	rows, err := ts.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("storage.GetLabels: %w", err)
	}
	defer rows.Close()

	var labels []*models.ActivityLabel
	for rows.Next() {
		var l models.ActivityLabel
		var sid sql.NullInt64
		err := rows.Scan(&l.ID, &l.UserID, &sid, &l.Timestamp, &l.State, &l.Source, &l.CreatedAt)
		if err != nil {
			return nil, err
		}
		if sid.Valid {
			id := sid.Int64
			l.SessionID = &id
		}
		labels = append(labels, &l)
	}
	return labels, rows.Err()
}

// ─── Heatmap ──────────────────────────────────────────────────────────────────

func (ts *TimescaleStorage) GetHeatmapData(ctx context.Context, sessionID, userID int64) ([]models.ClickPoint, error) {
	// Query the dedicated click_events table (90-day retention) instead of the
	// short-lived raw_input_events hypertable (1-hour retention). This means
	// heatmaps remain available long after the raw event buffer has been pruned.
	rows, err := ts.db.QueryContext(ctx, `
		SELECT x, y
		FROM   click_events
		WHERE  session_id = $1
		  AND  user_id    = $2
		ORDER  BY timestamp`,
		sessionID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("storage.GetHeatmapData: %w", err)
	}
	defer rows.Close()

	var points []models.ClickPoint
	for rows.Next() {
		var p models.ClickPoint
		if err := rows.Scan(&p.X, &p.Y); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}
