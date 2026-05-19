package utils

import (
	"testing"
	"time"
)

func TestRateLimiterCleanUpRemovesExpiredWindows(t *testing.T) {
	lim := NewRateLimiter(3, 20*time.Millisecond, 20*time.Millisecond)

	if !lim.ShouldLogConnection("1.1.1.1") {
		t.Fatal("first request should be allowed")
	}
	if !lim.ShouldLogConnection("2.2.2.2") {
		t.Fatal("first request for second IP should be allowed")
	}

	time.Sleep(30 * time.Millisecond)
	lim.CleanUp()

	if got := len(lim.windows); got != 0 {
		t.Fatalf("windows size = %d, want 0", got)
	}
}

func TestRateLimiterCleanUpRemovesExpiredBlacklist(t *testing.T) {
	lim := NewRateLimiter(1, time.Second, 20*time.Millisecond)
	ip := "3.3.3.3"

	if !lim.ShouldLogConnection(ip) {
		t.Fatal("first request should be allowed")
	}
	if lim.ShouldLogConnection(ip) {
		t.Fatal("second request should be blocked and blacklisted")
	}

	time.Sleep(30 * time.Millisecond)
	lim.CleanUp()

	if got := len(lim.blacklisted); got != 0 {
		t.Fatalf("blacklisted size = %d, want 0", got)
	}
}
