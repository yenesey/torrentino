package web

import (
	"sync"
	"time"
)

// loginLimiter implements a small fixed-window failure limiter for /login.
// It is in-memory on purpose: sessions are in-memory too, and a restart
// clearing the counters is acceptable for a home NAS.
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]*loginAttempt
	max      int
	window   time.Duration
	now      func() time.Time
}

type loginAttempt struct {
	count        int
	first        time.Time
	last         time.Time
	blockedUntil time.Time
}

func newLoginLimiter(max int, window time.Duration) *loginLimiter {
	return &loginLimiter{
		attempts: make(map[string]*loginAttempt),
		max:      max,
		window:   window,
		now:      time.Now,
	}
}

// Allow reports whether a key may attempt a login. When it is blocked, retry
// is the remaining wait.
func (l *loginLimiter) Allow(key string) (retry time.Duration, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.pruneLocked(now)
	attempt, found := l.attempts[key]
	if !found {
		return 0, true
	}
	if now.Before(attempt.blockedUntil) {
		return attempt.blockedUntil.Sub(now), false
	}
	if now.Sub(attempt.first) > l.window {
		delete(l.attempts, key)
		return 0, true
	}
	return 0, true
}

// Failure records one failed attempt and starts a block after max failures.
func (l *loginLimiter) Failure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.pruneLocked(now)
	attempt, found := l.attempts[key]
	if !found || now.Sub(attempt.first) > l.window {
		l.attempts[key] = &loginAttempt{count: 1, first: now, last: now}
		return
	}
	attempt.count++
	attempt.last = now
	if attempt.count >= l.max {
		attempt.blockedUntil = now.Add(l.window)
	}
}

// Success clears the counters for a key.
func (l *loginLimiter) Success(key string) {
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}

func (l *loginLimiter) pruneLocked(now time.Time) {
	if len(l.attempts) < 1024 {
		return
	}
	for key, attempt := range l.attempts {
		if now.Sub(attempt.last) > l.window && now.After(attempt.blockedUntil) {
			delete(l.attempts, key)
		}
	}
}

// windowLimiter is a fixed-window request counter used for search. Unlike
// loginLimiter it counts every request, successful or not.
type windowLimiter struct {
	mu      sync.Mutex
	entries map[string]*windowEntry
	limit   int
	window  time.Duration
	now     func() time.Time
}

type windowEntry struct {
	count int
	reset time.Time
}

func newWindowLimiter(limit int, window time.Duration) *windowLimiter {
	return &windowLimiter{
		entries: make(map[string]*windowEntry),
		limit:   limit,
		window:  window,
		now:     time.Now,
	}
}

func (l *windowLimiter) Allow(key string) (retry time.Duration, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.entries) >= 1024 {
		for existing, entry := range l.entries {
			if !now.Before(entry.reset) {
				delete(l.entries, existing)
			}
		}
	}
	entry, found := l.entries[key]
	if !found || !now.Before(entry.reset) {
		l.entries[key] = &windowEntry{count: 1, reset: now.Add(l.window)}
		return 0, true
	}
	if entry.count >= l.limit {
		return entry.reset.Sub(now), false
	}
	entry.count++
	return 0, true
}
