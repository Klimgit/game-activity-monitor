-- Enable TimescaleDB extension
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- ============================================================
-- users
-- ============================================================
CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL    PRIMARY KEY,
    email         VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- ============================================================
-- raw_input_events  (TimescaleDB hypertable)
-- Stores every mouse/keyboard/system event from the client.
-- Short retention (1 hour) — used only for real-time display.
-- ============================================================
CREATE TABLE IF NOT EXISTS raw_input_events (
    user_id    BIGINT      NOT NULL,
    timestamp  TIMESTAMPTZ NOT NULL,
    session_id BIGINT,
    event_type TEXT        NOT NULL,
    data       JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

SELECT create_hypertable('raw_input_events', 'timestamp', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_raw_events_user    ON raw_input_events (user_id,    timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_raw_events_session ON raw_input_events (session_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_raw_events_type    ON raw_input_events (event_type, timestamp DESC);

-- Keep raw events for 1 hour only
SELECT add_retention_policy('raw_input_events', INTERVAL '1 hour', if_not_exists => TRUE);

-- ============================================================
-- activity_sessions
-- One row per gaming session; durations updated by the client.
-- ============================================================
CREATE TABLE IF NOT EXISTS activity_sessions (
    id              BIGSERIAL    PRIMARY KEY,
    user_id         BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_start   TIMESTAMPTZ  NOT NULL,
    session_end     TIMESTAMPTZ,
    game_name       VARCHAR(255) NOT NULL DEFAULT '',
    total_duration  INTEGER      NOT NULL DEFAULT 0,   -- seconds
    active_duration INTEGER      NOT NULL DEFAULT 0,
    afk_duration    INTEGER      NOT NULL DEFAULT 0,
    activity_score  REAL         NOT NULL DEFAULT 0,   -- 0..1
    state           VARCHAR(50)  NOT NULL DEFAULT 'active',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sessions_user ON activity_sessions (user_id, session_start DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_game ON activity_sessions (game_name, session_start DESC);

-- ============================================================
-- activity_intervals
-- Ground-truth time ranges for ML (FSM: non-overlapping per session).
-- ============================================================
CREATE TABLE IF NOT EXISTS activity_intervals (
    id          BIGSERIAL   PRIMARY KEY,
    user_id     BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id  BIGINT      NOT NULL REFERENCES activity_sessions(id) ON DELETE CASCADE,
    state       VARCHAR(50) NOT NULL CHECK (state IN (
        'active_gameplay', 'afk', 'menu', 'loading'
    )),
    start_at    TIMESTAMPTZ NOT NULL,
    end_at      TIMESTAMPTZ NOT NULL,
    source      VARCHAR(50) NOT NULL DEFAULT 'client',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT activity_intervals_time_order CHECK (end_at > start_at)
);

CREATE INDEX IF NOT EXISTS idx_activity_intervals_session
    ON activity_intervals (session_id, start_at);

CREATE INDEX IF NOT EXISTS idx_activity_intervals_user_time
    ON activity_intervals (user_id, start_at DESC);
