-- Add hardware metric columns to session_windows so ML training has
-- CPU/memory/GPU features alongside mouse and keyboard activity.
ALTER TABLE session_windows
    ADD COLUMN IF NOT EXISTS cpu_avg      FLOAT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cpu_max      FLOAT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS mem_avg      FLOAT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS gpu_util_avg FLOAT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS gpu_temp_avg FLOAT NOT NULL DEFAULT 0;
