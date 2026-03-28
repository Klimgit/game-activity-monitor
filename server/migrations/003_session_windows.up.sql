-- session_windows stores pre-aggregated per-window feature vectors produced by
-- the client aggregator (window_metrics events).  Unlike raw_input_events
-- (1-hour retention) this table is kept for 1 year to serve as the primary
-- dataset for ML model training.
--
-- One row = one 30-second aggregation window from one user's session.

CREATE TABLE IF NOT EXISTS session_windows (
    time             TIMESTAMPTZ NOT NULL,
    user_id          BIGINT      NOT NULL REFERENCES users(id),
    session_id       BIGINT               REFERENCES activity_sessions(id),
    window_start     TIMESTAMPTZ NOT NULL,
    window_end       TIMESTAMPTZ NOT NULL,
    duration_s       FLOAT       NOT NULL DEFAULT 0,
    mouse_moves      INT         NOT NULL DEFAULT 0,
    mouse_clicks     INT         NOT NULL DEFAULT 0,
    speed_avg        FLOAT       NOT NULL DEFAULT 0,
    speed_max        FLOAT       NOT NULL DEFAULT 0,
    keystrokes       INT         NOT NULL DEFAULT 0,
    key_hold_avg_ms  FLOAT       NOT NULL DEFAULT 0,
    active_process   TEXT        NOT NULL DEFAULT ''
);

SELECT create_hypertable('session_windows', 'time');

CREATE INDEX IF NOT EXISTS session_windows_user_session
    ON session_windows (user_id, session_id, time DESC);

SELECT add_retention_policy('session_windows', INTERVAL '1 year');
