ALTER TABLE session_windows
    DROP COLUMN IF EXISTS cursor_accel_avg,
    DROP COLUMN IF EXISTS cursor_accel_max,
    DROP COLUMN IF EXISTS foreground_window_title;
