// Package ratelimit provides a small, dependency-free fixed-window limiter for
// bounding abusive request rates on a single API instance. It is intentionally
// in-memory: clustered rate limiting is deferred (see the V4 backlog) and would
// use a shared store. Keys are opaque strings chosen by the caller (for example
// a public token combined with a client address).
package ratelimit

import (
	"sync"
	"time"
)

type window struct {
	count   int
	resetAt time.Time
}

// Limiter allows up to `limit` requests per `window` per key. It is safe for
// concurrent use and prunes expired windows lazily to bound memory.
type Limiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu        sync.Mutex
	windows   map[string]window
	lastSwept time.Time
}

// New returns a limiter permitting `limit` requests per `window`. A non-positive
// limit or window disables limiting (Allow always returns true), which keeps the
// caller simple when a deployment opts out via configuration.
func New(limit int, per time.Duration) *Limiter {
	return newWithClock(limit, per, time.Now)
}

func newWithClock(limit int, per time.Duration, now func() time.Time) *Limiter {
	return &Limiter{limit: limit, window: per, now: now, windows: make(map[string]window)}
}

// Allow reports whether a request for the given key may proceed, counting it
// against the current window when permitted.
func (l *Limiter) Allow(key string) bool {
	if l.limit <= 0 || l.window <= 0 {
		return true
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweep(now)
	current, ok := l.windows[key]
	if !ok || !now.Before(current.resetAt) {
		l.windows[key] = window{count: 1, resetAt: now.Add(l.window)}
		return true
	}
	if current.count >= l.limit {
		return false
	}
	current.count++
	l.windows[key] = current
	return true
}

// sweep drops expired windows at most once per window to keep the map bounded
// without a background goroutine. The caller must hold the mutex.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSwept) < l.window {
		return
	}
	l.lastSwept = now
	for key, value := range l.windows {
		if !now.Before(value.resetAt) {
			delete(l.windows, key)
		}
	}
}
