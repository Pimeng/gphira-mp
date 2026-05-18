package network

import (
	"sync"
	"time"
)

// RateLimiter limits the rate of events per key using a sliding window.
type RateLimiter struct {
	window   time.Duration
	maxCount int

	mu     sync.Mutex
	events map[string][]time.Time
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(window time.Duration, maxCount int) *RateLimiter {
	return &RateLimiter{
		window:   window,
		maxCount: maxCount,
		events:   make(map[string][]time.Time),
	}
}

// Allow checks if the key is within the rate limit. If allowed, the event is recorded.
func (r *RateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.window)

	// Remove old events
	list := r.events[key]
	var kept []time.Time
	for _, t := range list {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}

	if len(kept) >= r.maxCount {
		r.events[key] = kept
		return false
	}

	kept = append(kept, now)
	r.events[key] = kept
	return true
}

// Reset clears all recorded events for the given key.
func (r *RateLimiter) Reset(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.events, key)
}
