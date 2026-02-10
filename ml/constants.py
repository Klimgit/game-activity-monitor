"""Column names aligned with server/internal/dataset/windows.go CSV export."""

LABEL_COL = "label"

GPU_COLS = ("gpu_util_avg", "gpu_temp_avg", "gpu_mem_avg_mb")

# Not used as model inputs (identifiers / time / target).
META_COLS = (
    "user_id",
    "session_id",
    "window_index",
    "window_start",
    "window_end",
    "label",
)

# Text features (process + window title + session game name).
TEXT_COLS = ("active_process", "foreground_window_title", "game_name")
