package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/lib/pq"

	"game-activity-monitor/server/internal/inference"
	"game-activity-monitor/server/internal/models"
)

type TimescaleStorage struct {
	db        *sql.DB
	predictor inference.Predictor
}

func NewTimescaleStorage(db *sql.DB, predictor inference.Predictor) *TimescaleStorage {
	return &TimescaleStorage{db: db, predictor: predictor}
}

func (ts *TimescaleStorage) Ping(ctx context.Context) error {
	return ts.db.PingContext(ctx)
}

func (ts *TimescaleStorage) Close() error {
	return ts.db.Close()
}

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

func (ts *TimescaleStorage) SaveEventsBatch(ctx context.Context, events []*models.RawEvent) error {
	if len(events) == 0 {
		return nil
	}

	type winItem struct {
		sessionID *int64
		w         models.WindowMetricsData
	}
	var winItems []winItem
	for _, e := range events {
		if e == nil || e.EventType != models.EventWindowMetrics {
			continue
		}
		var w models.WindowMetricsData
		if err := json.Unmarshal(e.Data, &w); err != nil {
			continue
		}
		winItems = append(winItems, winItem{sessionID: e.SessionID, w: w})
	}

	var preds []string
	if len(winItems) > 0 && ts.predictor != nil {
		seen := make(map[int64]struct{})
		var sids []int64
		for _, it := range winItems {
			if it.sessionID == nil {
				continue
			}
			if _, ok := seen[*it.sessionID]; ok {
				continue
			}
			seen[*it.sessionID] = struct{}{}
			sids = append(sids, *it.sessionID)
		}
		games, err := ts.gameNamesBySessionID(ctx, sids)
		if err != nil {
			return fmt.Errorf("storage.SaveEventsBatch game names: %w", err)
		}
		features := make([]map[string]interface{}, 0, len(winItems))
		for _, it := range winItems {
			gn := ""
			if it.sessionID != nil {
				gn = games[*it.sessionID]
			}
			features = append(features, inference.WindowFeatureRow(&it.w, gn))
		}
		preds, err = ts.predictor.PredictBatch(ctx, features)
		if err != nil {
			log.Printf("storage.SaveEventsBatch: ml predict: %v", err)
			preds = make([]string, len(winItems))
		}
		if len(preds) != len(winItems) {
			preds = make([]string, len(winItems))
		}
	} else if len(winItems) > 0 {
		preds = make([]string, len(winItems))
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

	clickStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO click_events (user_id, session_id, timestamp, x, y, button)
		VALUES ($1, $2, $3, $4, $5, $6)`)
	if err != nil {
		return fmt.Errorf("storage.SaveEventsBatch prepare click: %w", err)
	}
	defer closeStmt(clickStmt)

	winStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO session_windows
		    (time, user_id, session_id, window_start, window_end, duration_s,
		     mouse_moves, mouse_clicks, speed_avg, speed_max,
		     keystrokes, key_hold_avg_ms, key_press_interval_avg_ms, key_w, key_a, key_s, key_d, active_process,
		     cpu_avg, cpu_max, mem_avg, gpu_util_avg, gpu_temp_avg, gpu_mem_avg_mb,
		     cursor_accel_avg, cursor_accel_max, foreground_window_title, ml_predicted_state)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28)`)
	if err != nil {
		return fmt.Errorf("storage.SaveEventsBatch prepare window: %w", err)
	}
	defer closeStmt(winStmt)

	winCursor := 0
	for _, e := range events {
		if e == nil {
			continue
		}
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
			if err := json.Unmarshal(e.Data, &w); err != nil {
				continue
			}
			ml := sql.NullString{}
			if winCursor < len(preds) {
				if p := preds[winCursor]; p != "" {
					ml = sql.NullString{String: p, Valid: true}
				}
			}
			winCursor++
			if _, err := winStmt.ExecContext(ctx,
				NormalizeWindowTime(e.Timestamp), e.UserID, sessionID,
				w.WindowStart, w.WindowEnd, w.DurationS,
				w.MouseMoves, w.MouseClicks, w.SpeedAvg, w.SpeedMax,
				w.Keystrokes, w.KeyHoldAvgMs,
				w.KeyPressIntervalAvgMs, w.KeyW, w.KeyA, w.KeyS, w.KeyD, w.ActiveProcess,
				w.CPUAvg, w.CPUMax, w.MemAvg, w.GPUUtilAvg, w.GPUTempAvg, w.GPUMemAvgMB,
				w.CursorAccelAvg, w.CursorAccelMax, w.ForegroundWindowTitle,
				ml,
			); err != nil {
				return fmt.Errorf("storage.SaveEventsBatch exec window: %w", err)
			}
		}
	}

	return tx.Commit()
}

func (ts *TimescaleStorage) gameNamesBySessionID(ctx context.Context, ids []int64) (map[int64]string, error) {
	out := make(map[int64]string)
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := ts.db.QueryContext(ctx, `
		SELECT id, COALESCE(game_name, '')
		FROM activity_sessions
		WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("storage.gameNamesBySessionID: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
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
		e.Data = dataBytes
		events = append(events, &e)
	}
	return events, rows.Err()
}

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

func (ts *TimescaleStorage) UpdateSessionGameName(ctx context.Context, sessionID, userID int64, gameName string) (*models.Session, error) {
	res, err := ts.db.ExecContext(ctx, `
		UPDATE activity_sessions
		SET    game_name = $1,
		       updated_at = NOW()
		WHERE  id = $2 AND user_id = $3`,
		gameName, sessionID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("storage.UpdateSessionGameName: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("storage.UpdateSessionGameName rows affected: %w", err)
	}
	if n == 0 {
		return nil, nil
	}
	return ts.GetSessionByID(ctx, sessionID, userID)
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

func (ts *TimescaleStorage) CreateActivityInterval(ctx context.Context, iv *models.ActivityInterval) (*models.ActivityInterval, error) {
	if iv.EndAt.Before(iv.StartAt) || iv.EndAt.Equal(iv.StartAt) {
		return nil, fmt.Errorf("invalid interval: end_at must be after start_at")
	}

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
		INSERT INTO activity_intervals (user_id, session_id, state, start_at, end_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, user_id, session_id, state, start_at, end_at, created_at`,
		iv.UserID, iv.SessionID, iv.State, iv.StartAt, iv.EndAt,
	).Scan(
		&out.ID, &out.UserID, &out.SessionID, &out.State,
		&out.StartAt, &out.EndAt, &out.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("storage.CreateActivityInterval: %w", err)
	}
	return &out, nil
}

func (ts *TimescaleStorage) ListActivityIntervals(ctx context.Context, userID int64, sessionID *int64, from, to time.Time) ([]*models.ActivityInterval, error) {
	q := `
		SELECT id, user_id, session_id, state, start_at, end_at, created_at
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
			&iv.StartAt, &iv.EndAt, &iv.CreatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, &iv)
	}
	return list, rows.Err()
}

func (ts *TimescaleStorage) SessionWindowsForUser(ctx context.Context, userID int64, from, to time.Time, sessionID *int64) ([]SessionWindowRow, error) {
	q := `
		SELECT sw.time, sw.user_id, sw.session_id, sw.window_start, sw.window_end, sw.duration_s,
		       sw.mouse_moves, sw.mouse_clicks, sw.speed_avg, sw.speed_max,
		       sw.keystrokes, sw.key_hold_avg_ms, sw.key_press_interval_avg_ms, sw.key_w, sw.key_a, sw.key_s, sw.key_d, sw.active_process,
		       sw.cpu_avg, sw.cpu_max, sw.mem_avg, sw.gpu_util_avg, sw.gpu_temp_avg, sw.gpu_mem_avg_mb,
		       sw.cursor_accel_avg, sw.cursor_accel_max, sw.foreground_window_title,
		       COALESCE(s.game_name, '') AS game_name
		FROM   session_windows sw
		LEFT JOIN activity_sessions s ON s.id = sw.session_id
		WHERE  sw.user_id = $1
		  AND sw.window_end >= $2 AND sw.window_start <= $3`
	args := []interface{}{userID, from, to}
	if sessionID != nil {
		q += " AND sw.session_id = $4"
		args = append(args, *sessionID)
	}
	q += " ORDER BY sw.window_start ASC"

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
			&r.Keystrokes, &r.KeyHoldAvgMs, &r.KeyPressIntervalAvgMs, &r.KeyW, &r.KeyA, &r.KeyS, &r.KeyD, &r.ActiveProcess,
			&r.CPUAvg, &r.CPUMax, &r.MemAvg, &r.GPUUtilAvg, &r.GPUTempAvg, &r.GPUMemAvgMB,
			&r.CursorAccelAvg, &r.CursorAccelMax, &r.ForegroundWindowTitle,
			&r.GameName,
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

func (ts *TimescaleStorage) MLPlaytimeBySessionIDs(ctx context.Context, userID int64, sessionIDs []int64) (map[int64]map[string]int64, error) {
	out := make(map[int64]map[string]int64)
	if len(sessionIDs) == 0 {
		return out, nil
	}
	rows, err := ts.db.QueryContext(ctx, `
		SELECT session_id, ml_predicted_state,
		       COALESCE(SUM(duration_s), 0)::bigint
		FROM session_windows
		WHERE user_id = $1
		  AND session_id = ANY($2)
		  AND ml_predicted_state IS NOT NULL
		  AND ml_predicted_state <> ''
		GROUP BY session_id, ml_predicted_state`,
		userID, pq.Array(sessionIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("storage.MLPlaytimeBySessionIDs: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var sid int64
		var st string
		var sec int64
		if err := rows.Scan(&sid, &st, &sec); err != nil {
			return nil, err
		}
		if out[sid] == nil {
			out[sid] = make(map[string]int64)
		}
		out[sid][st] = sec
	}
	return out, rows.Err()
}

func (ts *TimescaleStorage) WindowMetricsSummary(ctx context.Context, userID int64, from, to time.Time) (*WindowMetricsSummary, error) {
	var summary WindowMetricsSummary
	summary.MLPlaytimeSeconds = map[string]int64{
		"active_gameplay": 0,
		"afk":             0,
		"menu":            0,
		"loading":         0,
	}

	err := ts.db.QueryRowContext(ctx, `
		SELECT COALESCE(COUNT(*), 0)::bigint,
		       COALESCE(SUM(duration_s), 0)::double precision,
		       COALESCE(SUM(mouse_moves), 0)::bigint,
		       COALESCE(SUM(keystrokes), 0)::bigint
		FROM session_windows
		WHERE user_id = $1
		  AND window_end >= $2 AND window_start <= $3`,
		userID, from, to,
	).Scan(&summary.WindowCount, &summary.TotalDurationS, &summary.TotalMouseMoves, &summary.TotalKeystrokes)
	if err != nil {
		return nil, fmt.Errorf("storage.WindowMetricsSummary totals: %w", err)
	}

	rows, err := ts.db.QueryContext(ctx, `
		SELECT ml_predicted_state, COALESCE(SUM(duration_s), 0)::bigint
		FROM session_windows
		WHERE user_id = $1
		  AND window_end >= $2 AND window_start <= $3
		  AND ml_predicted_state IS NOT NULL
		  AND ml_predicted_state <> ''
		GROUP BY ml_predicted_state`,
		userID, from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("storage.WindowMetricsSummary by state: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var st string
		var sec int64
		if err := rows.Scan(&st, &sec); err != nil {
			return nil, err
		}
		summary.MLPlaytimeSeconds[st] = sec
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &summary, nil
}

func (ts *TimescaleStorage) WindowMLStates(ctx context.Context, userID int64, keys []WindowKey) (map[WindowKey]string, error) {
	out := make(map[WindowKey]string)
	if len(keys) == 0 {
		return out, nil
	}
	times := make([]time.Time, len(keys))
	sids := make([]int64, len(keys))
	for i, k := range keys {
		times[i] = k.Time
		sids[i] = k.SessionID
	}
	rows, err := ts.db.QueryContext(ctx, `
		SELECT sw.time, sw.session_id, sw.ml_predicted_state
		FROM session_windows sw
		INNER JOIN (
			SELECT unnest($1::timestamptz[]) AS time,
			       unnest($2::bigint[]) AS session_id
		) AS q ON sw.time = q.time AND sw.session_id = q.session_id
		WHERE sw.user_id = $3`,
		pq.Array(times), pq.Array(sids), userID,
	)
	if err != nil {
		return nil, fmt.Errorf("storage.WindowMLStates: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var t time.Time
		var sid int64
		var ml sql.NullString
		if err := rows.Scan(&t, &sid, &ml); err != nil {
			return nil, err
		}
		k := WindowKey{Time: NormalizeWindowTime(t), SessionID: sid}
		if ml.Valid && ml.String != "" {
			out[k] = ml.String
		}
	}
	return out, rows.Err()
}

// ─── Heatmap ──────────────────────────────────────────────────────────────────

func (ts *TimescaleStorage) GetHeatmapData(ctx context.Context, sessionID, userID int64) ([]models.ClickPoint, error) {
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
