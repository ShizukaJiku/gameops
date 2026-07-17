package webauth

import (
	"sync"
	"time"
)

// RateLimiter is a simple in-memory sliding-window limiter, keyed by an
// arbitrary string (the login handler uses the client IP). No background
// goroutine — old attempts are pruned lazily on every Allow call.
type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	max      int
	window   time.Duration
}

func NewRateLimiter(max int, window time.Duration) *RateLimiter {
	return &RateLimiter{attempts: make(map[string][]time.Time), max: max, window: window}
}

// Allow reports whether key may attempt again right now, recording this
// attempt if so.
func (l *RateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)
	kept := l.attempts[key][:0]
	for _, t := range l.attempts[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.max {
		l.attempts[key] = kept
		return false
	}
	l.attempts[key] = append(kept, now)
	return true
}
