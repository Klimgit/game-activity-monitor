-- Dedicated table for mouse click coordinates.
-- Unlike raw_input_events (1-hour retention), click_events are kept for 90 days
-- so heatmaps remain available long after the raw event buffer has been pruned.

CREATE TABLE IF NOT EXISTS click_events (
    id         BIGSERIAL   NOT NULL,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id BIGINT      REFERENCES activity_sessions(id) ON DELETE SET NULL,
    timestamp  TIMESTAMPTZ NOT NULL,
    x          INTEGER     NOT NULL,
    y          INTEGER     NOT NULL,
    button     TEXT        NOT NULL DEFAULT 'left',
    PRIMARY KEY (id, timestamp)
);

SELECT create_hypertable('click_events', 'timestamp', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_click_events_session
    ON click_events (session_id, timestamp DESC);

CREATE INDEX IF NOT EXISTS idx_click_events_user
    ON click_events (user_id, timestamp DESC);

SELECT add_retention_policy('click_events', INTERVAL '90 days', if_not_exists => TRUE);
