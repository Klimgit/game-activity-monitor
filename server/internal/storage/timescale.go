package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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

func (ts *TimescaleStorage) ListUserIDs(ctx context.Context) ([]int64, error) {
	rows, err := ts.db.QueryContext(ctx, `SELECT id FROM users ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("storage.ListUserIDs: %w", err)
	}
	defer closeRows(rows)
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("storage.ListUserIDs: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
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
	defer rollbackTx(tx)

	rawStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO raw_input_events (user_id, session_id, timestamp, event_type, data)
		VALUES ($1, $2, $3, $4, $5)`)
	if err != nil {
		return fmt.Errorf("storage.SaveEventsBatch prepare raw: %w", err)
	}
	defer closeStmt(rawStmt)

	// Prepared statement for the long-term click store used by the heatmap.
	clickStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO click_events (user_id, session_id, timestamp, x, y, button)
		VALUES ($1, $2, $3, $4, $5, $6)`)
	if err != nil {
		return fmt.Errorf("storage.SaveEventsBatch prepare click: %w", err)
	}
	defer closeStmt(clickStmt)

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
	defer closeStmt(winStmt)

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
	defer closeRows(rows)
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
	defer closeRows(rows)
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

// ─── Activity intervals (ML ground truth) ─────────────────────────────────────

func (ts *TimescaleStorage) CreateActivityInterval(ctx context.Context, iv *models.ActivityInterval) (*models.ActivityInterval, error) {
	if iv.EndAt.Before(iv.StartAt) || iv.EndAt.Equal(iv.StartAt) {
		return nil, fmt.Errorf("invalid interval: end_at must be after start_at")
	}

	// Ensure session belongs to user and check FSM non-overlap.
	var owner int64
	err := ts.db.QueryRowContext(ctx, `
		SELECT user_id FROM activity_sessions WHERE id = $1`, iv.SessionID,
	).Scan(&owner)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("session not found")
		}
		return nil, fmt.Errorf("storage.CreateActivityInterval session: %w", err)
	}
	if owner != iv.UserID {
		return nil, fmt.Errorf("session does not belong to user")
	}

	var overlap int
	err = ts.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM activity_intervals
		WHERE session_id = $1
		  AND start_at < $3 AND end_at > $2`,
		iv.SessionID, iv.StartAt, iv.EndAt,
	).Scan(&overlap)
	if err != nil {
		return nil, fmt.Errorf("storage.CreateActivityInterval overlap: %w", err)
	}
	if overlap > 0 {
		return nil, fmt.Errorf("interval overlaps an existing interval in this session")
	}

	var out models.ActivityInterval
	err = ts.db.QueryRowContext(ctx, `
		INSERT INTO activity_intervals (user_id, session_id, state, start_at, end_at, source)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, user_id, session_id, state, start_at, end_at, source, created_at`,
		iv.UserID, iv.SessionID, iv.State, iv.StartAt, iv.EndAt, iv.Source,
	).Scan(
		&out.ID, &out.UserID, &out.SessionID, &out.State,
		&out.StartAt, &out.EndAt, &out.Source, &out.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("storage.CreateActivityInterval: %w", err)
	}
	return &out, nil
}

func (ts *TimescaleStorage) ListActivityIntervals(ctx context.Context, userID int64, sessionID *int64, from, to time.Time) ([]*models.ActivityInterval, error) {
	q := `
		SELECT id, user_id, session_id, state, start_at, end_at, source, created_at
		FROM   activity_intervals
		WHERE  user_id = $1
		  AND end_at >= $2 AND start_at <= $3`
	args := []interface{}{userID, from, to}
	argN := 4
	if sessionID != nil {
		q += fmt.Sprintf(" AND session_id = $%d", argN)
		args = append(args, *sessionID)
		argN++
	}
	q += " ORDER BY start_at ASC"

	rows, err := ts.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("storage.ListActivityIntervals: %w", err)
	}
	defer closeRows(rows)

	var list []*models.ActivityInterval
	for rows.Next() {
		var iv models.ActivityInterval
		if err := rows.Scan(
			&iv.ID, &iv.UserID, &iv.SessionID, &iv.State,
			&iv.StartAt, &iv.EndAt, &iv.Source, &iv.CreatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, &iv)
	}
	return list, rows.Err()
}

// SessionWindowsForUser returns session_windows rows in the time range.
func (ts *TimescaleStorage) SessionWindowsForUser(ctx context.Context, userID int64, from, to time.Time, sessionID *int64) ([]SessionWindowRow, error) {
	q := `
		SELECT time, user_id, session_id, window_start, window_end, duration_s,
		       mouse_moves, mouse_clicks, speed_avg, speed_max,
		       keystrokes, key_hold_avg_ms, active_process,
		       cpu_avg, cpu_max, mem_avg, gpu_util_avg, gpu_temp_avg
		FROM   session_windows
		WHERE  user_id = $1
		  AND window_end >= $2 AND window_start <= $3`
	args := []interface{}{userID, from, to}
	if sessionID != nil {
		q += " AND session_id = $4"
		args = append(args, *sessionID)
	}
	q += " ORDER BY window_start ASC"

	rows, err := ts.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("storage.SessionWindowsForUser: %w", err)
	}
	defer closeRows(rows)

	var out []SessionWindowRow
	for rows.Next() {
		var r SessionWindowRow
		if err := rows.Scan(
			&r.Time, &r.UserID, &r.SessionID, &r.WindowStart, &r.WindowEnd, &r.DurationS,
			&r.MouseMoves, &r.MouseClicks, &r.SpeedAvg, &r.SpeedMax,
			&r.Keystrokes, &r.KeyHoldAvgMs, &r.ActiveProcess,
			&r.CPUAvg, &r.CPUMax, &r.MemAvg, &r.GPUUtilAvg, &r.GPUTempAvg,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PlaytimeByState returns total seconds per state from activity_intervals in range.
func (ts *TimescaleStorage) PlaytimeByState(ctx context.Context, userID int64, from, to time.Time, sessionID *int64) (map[string]int64, error) {
	q := `
		SELECT state,
		       COALESCE(SUM(EXTRACT(EPOCH FROM (
		         LEAST(end_at, $3::timestamptz) - GREATEST(start_at, $2::timestamptz)
		       )))::bigint, 0)
		FROM   activity_intervals
		WHERE  user_id = $1
		  AND end_at >= $2 AND start_at <= $3`
	args := []interface{}{userID, from, to}
	if sessionID != nil {
		q += " AND session_id = $4"
		args = append(args, *sessionID)
	}
	q += " GROUP BY state"

	rows, err := ts.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("storage.PlaytimeByState: %w", err)
	}
	defer closeRows(rows)

	out := map[string]int64{
		"active_gameplay": 0,
		"afk":             0,
		"menu":            0,
		"loading":         0,
	}
	for rows.Next() {
		var state string
		var sec int64
		if err := rows.Scan(&state, &sec); err != nil {
			return nil, err
		}
		out[state] = sec
	}
	return out, rows.Err()
}

func (ts *TimescaleStorage) ListPredictedWindows(ctx context.Context, userID int64, sessionID *int64, from, to time.Time) ([]*models.PredictedWindow, error) {
	q := `
		SELECT id, user_id, session_id, window_start, window_end, predicted_state, confidence, model_version, created_at
		FROM   predicted_windows
		WHERE  user_id = $1
		  AND window_end >= $2 AND window_start <= $3`
	args := []interface{}{userID, from, to}
	argN := 4
	if sessionID != nil {
		q += fmt.Sprintf(" AND session_id = $%d", argN)
		args = append(args, *sessionID)
	}
	q += " ORDER BY window_start ASC"

	rows, err := ts.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("storage.ListPredictedWindows: %w", err)
	}
	defer closeRows(rows)

	var list []*models.PredictedWindow
	for rows.Next() {
		var p models.PredictedWindow
		var conf sql.NullFloat64
		if err := rows.Scan(
			&p.ID, &p.UserID, &p.SessionID, &p.WindowStart, &p.WindowEnd,
			&p.PredictedState, &conf, &p.ModelVersion, &p.CreatedAt,
		); err != nil {
			return nil, err
		}
		if conf.Valid {
			v := conf.Float64
			p.Confidence = &v
		}
		list = append(list, &p)
	}
	return list, rows.Err()
}

func (ts *TimescaleStorage) UpsertPredictedWindowsBatch(ctx context.Context, userID int64, rows []*models.PredictedWindow) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := ts.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("storage.UpsertPredictedWindowsBatch begin: %w", err)
	}
	defer rollbackTx(tx)

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO predicted_windows (user_id, session_id, window_start, window_end, predicted_state, confidence, model_version)
		SELECT $1, $2, $3, $4, $5, $6, $7
		FROM activity_sessions s
		WHERE s.id = $2 AND s.user_id = $1
		ON CONFLICT (session_id, window_start) DO UPDATE SET
			window_end = EXCLUDED.window_end,
			predicted_state = EXCLUDED.predicted_state,
			confidence = EXCLUDED.confidence,
			model_version = EXCLUDED.model_version`)
	if err != nil {
		return fmt.Errorf("storage.UpsertPredictedWindowsBatch prepare: %w", err)
	}
	defer closeStmt(stmt)

	for _, r := range rows {
		r.UserID = userID
		var conf interface{}
		if r.Confidence != nil {
			conf = *r.Confidence
		}
		mv := r.ModelVersion
		if mv == "" {
			mv = "unknown"
		}
		res, err := stmt.ExecContext(ctx,
			userID, r.SessionID, r.WindowStart, r.WindowEnd, r.PredictedState, conf, mv,
		)
		if err != nil {
			return fmt.Errorf("storage.UpsertPredictedWindowsBatch exec: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("storage.UpsertPredictedWindowsBatch rows: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("session %d not found or not owned by user", r.SessionID)
		}
	}
	return tx.Commit()
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
	defer closeRows(rows)

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

func closeRows(rows *sql.Rows) {
	if rows == nil {
		return
	}
	if err := rows.Close(); err != nil {
		log.Printf("storage: close rows: %v", err)
	}
}

func closeStmt(stmt *sql.Stmt) {
	if stmt == nil {
		return
	}
	if err := stmt.Close(); err != nil {
		log.Printf("storage: close stmt: %v", err)
	}
}

func rollbackTx(tx *sql.Tx) {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		log.Printf("storage: tx rollback: %v", err)
	}
}
