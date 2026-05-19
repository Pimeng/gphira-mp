package replay

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ReplayEntry describes one replay file in user-facing listings.
type ReplayEntry struct {
	ChartID   int32
	Timestamp int64
	RecordID  int32
	Path      string
}

// ListReplaysForUser scans local replay files for a user and groups them by chart.
func ListReplaysForUser(baseDir string, userID int32) (map[int32][]ReplayEntry, error) {
	out := make(map[int32][]ReplayEntry)
	userDir := filepath.Join(baseDir, strconv.FormatInt(int64(userID), 10))
	charts, err := os.ReadDir(userDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return out, err
	}
	for _, chartEntry := range charts {
		if !chartEntry.IsDir() {
			continue
		}
		chartNum, err := strconv.ParseInt(chartEntry.Name(), 10, 32)
		if err != nil || chartNum < 0 {
			continue
		}
		chartID := int32(chartNum)
		chartDir := filepath.Join(userDir, chartEntry.Name())
		files, err := os.ReadDir(chartDir)
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.IsDir() || !strings.EqualFold(filepath.Ext(file.Name()), ".phirarec") {
				continue
			}
			tsText := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))
			ts, err := strconv.ParseInt(tsText, 10, 64)
			if err != nil || ts <= 0 {
				continue
			}
			path := filepath.Join(chartDir, file.Name())
			header, err := ReadReplayHeader(path)
			if err != nil || header == nil || header.UserID != userID || header.ChartID != chartID {
				continue
			}
			out[chartID] = append(out[chartID], ReplayEntry{
				ChartID:   chartID,
				Timestamp: ts,
				RecordID:  header.RecordID,
				Path:      path,
			})
		}
		sort.Slice(out[chartID], func(i, j int) bool {
			return out[chartID][i].Timestamp > out[chartID][j].Timestamp
		})
		if len(out[chartID]) == 0 {
			delete(out, chartID)
		}
	}
	return out, nil
}

// DeleteReplayForUser deletes a local replay file and prunes empty parent dirs.
func DeleteReplayForUser(baseDir string, userID, chartID int32, timestamp int64) (bool, error) {
	path := ReplayFilePath(baseDir, userID, chartID, timestamp)
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	removeIfEmpty(filepath.Dir(path))
	removeIfEmpty(filepath.Dir(filepath.Dir(path)))
	return true, nil
}

func removeIfEmpty(dir string) {
	entries, err := os.ReadDir(dir)
	if err == nil && len(entries) == 0 {
		_ = os.Remove(dir)
	}
}
