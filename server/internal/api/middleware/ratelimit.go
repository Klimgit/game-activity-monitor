package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type slidingWindow struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

func newSlidingWindow(limit int, window time.Duration) *slidingWindow {
	sw := &slidingWindow{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
	go sw.cleanup()
	return sw
}

// allow returns true if the key has not exceeded the rate limit.
func (sw *slidingWindow) allow(key string) bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-sw.window)

	prev := sw.requests[key]
	valid := prev[:0]
	for _, t := range prev {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= sw.limit {
		sw.requests[key] = valid
		return false
	}

	sw.requests[key] = append(valid, now)
	return true
}

func (sw *slidingWindow) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		sw.mu.Lock()
		cutoff := time.Now().Add(-sw.window)
		for key, times := range sw.requests {
			valid := times[:0]
			for _, t := range times {
				if t.After(cutoff) {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(sw.requests, key)
			} else {
				sw.requests[key] = valid
			}
		}
		sw.mu.Unlock()
	}
}

func RateLimit(limit int, window time.Duration) gin.HandlerFunc {
	sw := newSlidingWindow(limit, window)
	return func(c *gin.Context) {
		if !sw.allow(c.ClientIP()) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}
