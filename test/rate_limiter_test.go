package test

import (
	"testing"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/network"
)

func TestRateLimiter(t *testing.T) {
	lim := network.NewRateLimiter(100*time.Millisecond, 3)

	key := "192.168.1.1"
	if !lim.Allow(key) {
		t.Error("first request should be allowed")
	}
	if !lim.Allow(key) {
		t.Error("second request should be allowed")
	}
	if !lim.Allow(key) {
		t.Error("third request should be allowed")
	}
	if lim.Allow(key) {
		t.Error("fourth request should be blocked")
	}

	// Different key should not be affected
	if !lim.Allow("192.168.1.2") {
		t.Error("different key should be allowed")
	}

	// Wait for window to expire
	time.Sleep(150 * time.Millisecond)
	if !lim.Allow(key) {
		t.Error("request after window should be allowed")
	}
}

func TestRateLimiterReset(t *testing.T) {
	lim := network.NewRateLimiter(time.Minute, 1)
	key := "10.0.0.1"

	if !lim.Allow(key) {
		t.Error("first request should be allowed")
	}
	if lim.Allow(key) {
		t.Error("second request should be blocked")
	}

	lim.Reset(key)
	if !lim.Allow(key) {
		t.Error("request after reset should be allowed")
	}
}
