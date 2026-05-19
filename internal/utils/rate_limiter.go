package utils

import (
	"sync"
	"time"
)

// RateLimiter provides per-IP connection log rate limiting and blacklist.
type RateLimiter struct {
	mu sync.RWMutex

	// windows maps IP -> list of recent log timestamps (ms)
	windows map[string][]int64

	// blacklisted maps IP -> expiry timestamp (ms)
	blacklisted map[string]int64

	// config
	maxPerWindow int
	windowMs     int64
	blacklistMs  int64

	callsSinceCleanup int
}

const cleanupTriggerCalls = 256

// NewRateLimiter creates a rate limiter for connection logs.
// maxPerWindow: max logs allowed per IP within the time window.
// window: duration of the sliding window.
// blacklist: duration an IP stays blacklisted after repeated abuse.
func NewRateLimiter(maxPerWindow int, window, blacklist time.Duration) *RateLimiter {
	return &RateLimiter{
		windows:      make(map[string][]int64),
		blacklisted:  make(map[string]int64),
		maxPerWindow: maxPerWindow,
		windowMs:     window.Milliseconds(),
		blacklistMs:  blacklist.Milliseconds(),
	}
}

// ShouldLogConnection returns true if the connection log should be allowed.
// It updates the sliding window and checks the blacklist.
func (r *RateLimiter) ShouldLogConnection(ip string) bool {
	if r == nil || ip == "" {
		return true
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UnixMilli()
	r.callsSinceCleanup++
	if r.callsSinceCleanup >= cleanupTriggerCalls {
		r.cleanupExpiredLocked(now)
		r.callsSinceCleanup = 0
	}

	// Check blacklist
	if expiry, ok := r.blacklisted[ip]; ok {
		if now < expiry {
			return false
		}
		delete(r.blacklisted, ip)
	}

	// Clean old entries in window
	window := r.windows[ip]
	cutoff := now - r.windowMs
	kept := window[:0]
	for _, t := range window {
		if t >= cutoff {
			kept = append(kept, t)
		}
	}
	window = kept

	// Check limit
	if len(window) >= r.maxPerWindow {
		// Blacklist the IP
		r.blacklisted[ip] = now + r.blacklistMs
		delete(r.windows, ip)
		return false
	}

	window = append(window, now)
	r.windows[ip] = window
	return true
}

// CleanUp removes expired blacklist entries and stale per-IP windows.
func (r *RateLimiter) CleanUp() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupExpiredLocked(time.Now().UnixMilli())
	r.callsSinceCleanup = 0
}

func (r *RateLimiter) cleanupExpiredLocked(now int64) {
	for ip, expiry := range r.blacklisted {
		if now >= expiry {
			delete(r.blacklisted, ip)
		}
	}

	cutoff := now - r.windowMs
	for ip, window := range r.windows {
		kept := window[:0]
		for _, t := range window {
			if t >= cutoff {
				kept = append(kept, t)
			}
		}
		if len(kept) == 0 {
			delete(r.windows, ip)
			continue
		}
		r.windows[ip] = kept
	}
}

// GetBlacklistedIPs returns a snapshot of currently blacklisted IPs with remaining ms.
func (r *RateLimiter) GetBlacklistedIPs() []struct {
	IP        string
	ExpiresIn int64
} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now().UnixMilli()
	var out []struct {
		IP        string
		ExpiresIn int64
	}
	for ip, expiry := range r.blacklisted {
		if now < expiry {
			out = append(out, struct {
				IP        string
				ExpiresIn int64
			}{IP: ip, ExpiresIn: expiry - now})
		}
	}
	return out
}

// RemoveFromBlacklist manually removes an IP from the blacklist.
func (r *RateLimiter) RemoveFromBlacklist(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.blacklisted, ip)
	delete(r.windows, ip)
}

// ClearBlacklist clears all blacklisted IPs.
func (r *RateLimiter) ClearBlacklist() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.blacklisted = make(map[string]int64)
	r.windows = make(map[string][]int64)
}
