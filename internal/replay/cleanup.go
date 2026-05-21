package replay

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CleanupHandle controls the replay cleanup background task.
type CleanupHandle struct {
	stopCh   chan struct{}
	stopOnce sync.Once
}

// Stop terminates the cleanup scheduler.
func (h *CleanupHandle) Stop() {
	h.stopOnce.Do(func() {
		close(h.stopCh)
	})
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
// After removing files, it cleans up empty chart and user directories.
func cleanupExpiredReplays(baseDir string, now time.Time, ttlDays int) error {
	if baseDir == "" {
		return nil
	}
	cutoff := now.AddDate(0, 0, -ttlDays)

	userDirs, err := os.ReadDir(baseDir)
	if err != nil {
		return nil
	}

	for _, userEntry := range userDirs {
		if !userEntry.IsDir() {
			continue
		}
		userDir := filepath.Join(baseDir, userEntry.Name())
		chartDirs, err := os.ReadDir(userDir)
		if err != nil {
			continue
		}

		for _, chartEntry := range chartDirs {
			if !chartEntry.IsDir() {
				continue
			}
			chartDir := filepath.Join(userDir, chartEntry.Name())
			files, err := os.ReadDir(chartDir)
			if err != nil {
				continue
			}

			for _, f := range files {
				if f.IsDir() || !strings.HasSuffix(f.Name(), ".phirarec") {
					continue
				}
				info, err := f.Info()
				if err != nil {
					continue
				}
				if info.ModTime().Before(cutoff) {
					_ = os.Remove(filepath.Join(chartDir, f.Name()))
				}
			}

			removeIfEmpty(chartDir)
		}

		removeIfEmpty(userDir)
	}

	return nil
}

