ALTER TABLE session_windows
    DROP COLUMN IF EXISTS key_press_interval_avg_ms,
    DROP COLUMN IF EXISTS key_w,
    DROP COLUMN IF EXISTS key_a,
    DROP COLUMN IF EXISTS key_s,
    DROP COLUMN IF EXISTS key_d;
