package storage

import (
	"database/sql"
	"time"
)

// SessionWindowRow is one row from session_windows for ML export.
type SessionWindowRow struct {
	Time          time.Time
	UserID        int64
	SessionID     sql.NullInt64
	WindowStart   time.Time
	WindowEnd     time.Time
	DurationS     float64
	MouseMoves    int
	MouseClicks   int
	SpeedAvg      float64
	SpeedMax      float64
	Keystrokes    int
	KeyHoldAvgMs  float64
	ActiveProcess string
	CPUAvg        float64
	CPUMax        float64
	MemAvg        float64
	GPUUtilAvg    float64
	GPUTempAvg    float64
}
