ALTER TABLE session_windows
    DROP COLUMN IF EXISTS cpu_avg,
    DROP COLUMN IF EXISTS cpu_max,
    DROP COLUMN IF EXISTS mem_avg,
    DROP COLUMN IF EXISTS gpu_util_avg,
    DROP COLUMN IF EXISTS gpu_temp_avg;
