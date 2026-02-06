package session

import (
	"sync"
	"time"
)

// Tracker measures total, active, and AFK duration for a gaming session.
//
// A second is counted as "active" when at least one input event was received
// within the last [InactivityThreshold] seconds; otherwise it is "AFK".
// A background goroutine ticks every second while the session is running,
// so Stop() returns accurate values regardless of when it is called.
type Tracker struct {
	mu        sync.Mutex
	startedAt time.Time
	lastInput time.Time
	threshold time.Duration
	active    time.Duration
	afk       time.Duration
	running   bool
	done      chan struct{}
}

// InactivityThreshold is the period of silence after which a player is
// considered AFK. Adjust if games have long passive sections.
const InactivityThreshold = 30 * time.Second

// NewTracker returns a Tracker ready to Start().
func NewTracker() *Tracker {
	return &Tracker{threshold: InactivityThreshold}
}

// Start begins the session clock. Calling Start on a running tracker is a no-op.
func (t *Tracker) Start() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.running {
		return
	}
	now := time.Now()
	t.startedAt = now
	t.lastInput = now // treat session start as initial activity
	t.active = 0
	t.afk = 0
	t.running = true
	t.done = make(chan struct{})
	go t.tick()
}

// RecordInput notes that input occurred now; resets the AFK countdown.
func (t *Tracker) RecordInput() {
	t.mu.Lock()
	t.lastInput = time.Now()
	t.mu.Unlock()
}

// Stop halts the session clock and returns the measured durations plus an
// activity score (active / total, clamped to [0, 1]).
// Returns zeros if the tracker was not running.
func (t *Tracker) Stop() (totalSec, activeSec, afkSec int, score float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.running {
		return 0, 0, 0, 0
	}
	t.running = false
	close(t.done) // signal tick goroutine to exit

	total := time.Since(t.startedAt)
	totalSec = int(total.Seconds())
	activeSec = int(t.active.Seconds())
	afkSec = int(t.afk.Seconds())
	if totalSec > 0 {
		score = float64(activeSec) / float64(totalSec)
		if score > 1 {
			score = 1
		}
	}
	return
}

// IsRunning reports whether a session is currently being tracked.
func (t *Tracker) IsRunning() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running
}

func (t *Tracker) tick() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			t.mu.Lock()
			if time.Since(t.lastInput) < t.threshold {
				t.active += time.Second
			} else {
				t.afk += time.Second
			}
			t.mu.Unlock()
		case <-t.done:
			return
		}
	}
}
