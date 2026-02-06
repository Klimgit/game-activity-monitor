package models

import "time"

// Session represents a single gaming session.
type Session struct {
	ID             int64      `json:"id"              db:"id"`
	UserID         int64      `json:"user_id"         db:"user_id"`
	SessionStart   time.Time  `json:"session_start"   db:"session_start"`
	SessionEnd     *time.Time `json:"session_end,omitempty" db:"session_end"`
	GameName       string     `json:"game_name"       db:"game_name"`
	TotalDuration  int        `json:"total_duration"  db:"total_duration"`  // seconds
	ActiveDuration int        `json:"active_duration" db:"active_duration"` // seconds
	AFKDuration    int        `json:"afk_duration"    db:"afk_duration"`    // seconds
	ActivityScore  float64    `json:"activity_score"  db:"activity_score"`  // 0..1
	State          string     `json:"state"           db:"state"`
	CreatedAt      time.Time  `json:"created_at"      db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"      db:"updated_at"`
}

// ActivityLabel is a manual or auto-detected ground-truth label for a moment in time.
// Used for dataset annotation.
type ActivityLabel struct {
	ID        int64     `json:"id"                   db:"id"`
	UserID    int64     `json:"user_id"              db:"user_id"`
	SessionID *int64    `json:"session_id,omitempty" db:"session_id"`
	Timestamp time.Time `json:"timestamp"            db:"timestamp"`
	// State: "active_gameplay" | "afk" | "menu" | "loading"
	State string `json:"state"      db:"state"`
	// Source: "manual_hotkey" | "auto_detected"
	Source    string    `json:"source"     db:"source"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}
