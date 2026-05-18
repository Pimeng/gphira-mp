package replay

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CleanupHandle controls the replay cleanup background task.
type CleanupHandle struct {
	stopCh chan struct{}
}

// Stop terminates the cleanup scheduler.
func (h *CleanupHandle) Stop() {
	close(h.stopCh)
}

// StartReplayCleanup starts a background task that removes replay files older than ttlDays.
// It runs daily at midnight.
func StartReplayCleanup(baseDir string, ttlDays int) *CleanupHandle {
	h := &CleanupHandle{stopCh: make(chan struct{})}
	go h.loop(baseDir, ttlDays)
	return h
}

func (h *CleanupHandle) loop(baseDir string, ttlDays int) {
	for {
		delay := msUntilNextMidnight()
		select {
		case <-time.After(delay):
			_ = cleanupExpiredReplays(baseDir, time.Now(), ttlDays)
		case <-h.stopCh:
			return
		}
	}
}

func msUntilNextMidnight() time.Duration {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	return next.Sub(now)
}

// cleanupExpiredReplays walks baseDir and deletes .phirarec files older than ttlDays.
func cleanupExpiredReplays(baseDir string, now time.Time, ttlDays int) error {
	if baseDir == "" {
		return nil
	}
	cutoff := now.AddDate(0, 0, -ttlDays)
	return filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // ignore permission errors
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".phirarec") {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(path)
		}
		return nil
	})
}
