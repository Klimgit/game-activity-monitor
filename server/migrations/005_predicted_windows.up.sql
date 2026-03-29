-- Per-window model predictions (aligned with session_windows time buckets).
-- Ingested by an offline/online inference job; dashboard reads for timeline UI.

CREATE TABLE IF NOT EXISTS predicted_windows (
    id               BIGSERIAL PRIMARY KEY,
    user_id          BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id       BIGINT       NOT NULL REFERENCES activity_sessions(id) ON DELETE CASCADE,
    window_start     TIMESTAMPTZ  NOT NULL,
    window_end       TIMESTAMPTZ  NOT NULL,
    predicted_state  VARCHAR(50)  NOT NULL CHECK (predicted_state IN (
        'active_gameplay', 'afk', 'menu', 'loading'
    )),
    confidence       DOUBLE PRECISION,
    model_version    TEXT         NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT predicted_windows_time_order CHECK (window_end > window_start),
    CONSTRAINT predicted_windows_session_window UNIQUE (session_id, window_start)
);

CREATE INDEX IF NOT EXISTS idx_predicted_windows_user_time
    ON predicted_windows (user_id, window_start DESC);

CREATE INDEX IF NOT EXISTS idx_predicted_windows_session
    ON predicted_windows (session_id, window_start);
