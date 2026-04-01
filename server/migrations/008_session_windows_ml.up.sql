ALTER TABLE session_windows
    ADD COLUMN IF NOT EXISTS ml_predicted_state TEXT;

COMMENT ON COLUMN session_windows.ml_predicted_state IS
    'Classifier label per window (active_gameplay, afk, menu, loading); NULL if inference unavailable.';
