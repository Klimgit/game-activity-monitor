package session

import (
	"sync"
	"time"
)

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

const InactivityThreshold = 30 * time.Second

func NewTracker() *Tracker {
	return &Tracker{threshold: InactivityThreshold}
}

func (t *Tracker) Start() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.running {
		return
	}
	now := time.Now()
	t.startedAt = now
	t.lastInput = now
	t.active = 0
	t.afk = 0
	t.running = true
	t.done = make(chan struct{})
	go t.tick()
}

func (t *Tracker) RecordInput() {
	t.mu.Lock()
	t.lastInput = time.Now()
	t.mu.Unlock()
}

func (t *Tracker) Stop() (totalSec, activeSec, afkSec int, score float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.running {
		return 0, 0, 0, 0
	}
	t.running = false
	close(t.done)

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
