-- Keyboard features for ML: avg gap between key presses + WASD counts per window.
ALTER TABLE session_windows
    ADD COLUMN IF NOT EXISTS key_press_interval_avg_ms FLOAT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS key_w INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS key_a INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS key_s INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS key_d INT NOT NULL DEFAULT 0;
