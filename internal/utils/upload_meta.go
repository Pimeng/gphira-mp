package utils

import "sync"

// UploadedReplayMeta tracks uploaded replay metadata per user and chart.
type UploadedReplayMeta struct {
	mu sync.RWMutex
	// userID -> chartID -> []*UploadedReplayMetaEntry
	data map[int32]map[int32][]*UploadedReplayMetaEntry
}

// UploadedReplayMetaEntry holds metadata for a single uploaded replay.
type UploadedReplayMetaEntry struct {
	ScoreID   int32
	ChartID   int32
	Timestamp int64
}

// NewUploadedReplayMeta creates a new metadata tracker.
func NewUploadedReplayMeta() *UploadedReplayMeta {
	return &UploadedReplayMeta{
		data: make(map[int32]map[int32][]*UploadedReplayMetaEntry),
	}
}

// Add stores metadata for an uploaded replay.
func (m *UploadedReplayMeta) Add(userID, chartID, scoreID int32, timestamp int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	chartMap, ok := m.data[userID]
	if !ok {
		chartMap = make(map[int32][]*UploadedReplayMetaEntry)
		m.data[userID] = chartMap
	}
	metaList := chartMap[chartID]
	metaList = append(metaList, &UploadedReplayMetaEntry{
		ScoreID:   scoreID,
		ChartID:   chartID,
		Timestamp: timestamp,
	})
	// Limit to 50 entries per chart to prevent memory leak
	if len(metaList) > 50 {
		metaList = metaList[len(metaList)-50:]
	}
	chartMap[chartID] = metaList
}

// Get retrieves metadata list for a user and chart.
func (m *UploadedReplayMeta) Get(userID, chartID int32) []*UploadedReplayMetaEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	chartMap, ok := m.data[userID]
	if !ok {
		return nil
	}
	list := chartMap[chartID]
	if list == nil {
		return nil
	}
	out := make([]*UploadedReplayMetaEntry, len(list))
	copy(out, list)
	return out
}

// GetUser returns a copy of all uploaded replay metadata for a user.
func (m *UploadedReplayMeta) GetUser(userID int32) map[int32][]*UploadedReplayMetaEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	chartMap, ok := m.data[userID]
	if !ok {
		return nil
	}
	out := make(map[int32][]*UploadedReplayMetaEntry, len(chartMap))
	for chartID, list := range chartMap {
		copied := make([]*UploadedReplayMetaEntry, len(list))
		copy(copied, list)
		out[chartID] = copied
	}
	return out
}

// Delete removes one uploaded replay metadata entry.
func (m *UploadedReplayMeta) Delete(userID, chartID int32, timestamp int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	chartMap, ok := m.data[userID]
	if !ok {
		return false
	}
	list := chartMap[chartID]
	for i, entry := range list {
		if entry.Timestamp != timestamp {
			continue
		}
		list = append(list[:i], list[i+1:]...)
		if len(list) == 0 {
			delete(chartMap, chartID)
		} else {
			chartMap[chartID] = list
		}
		if len(chartMap) == 0 {
			delete(m.data, userID)
		}
		return true
	}
	return false
}

// AutoUploadConfigs tracks per-user auto-upload visibility preferences.
type AutoUploadConfigs struct {
	mu sync.RWMutex
	// userID -> show
	configs map[int32]bool
}

// NewAutoUploadConfigs creates a new config tracker.
func NewAutoUploadConfigs() *AutoUploadConfigs {
	return &AutoUploadConfigs{
		configs: make(map[int32]bool),
	}
}

// SetShow sets the visibility preference for a user.
func (c *AutoUploadConfigs) SetShow(userID int32, show bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.configs[userID] = show
}

// GetShow returns the visibility preference for a user (default false).
func (c *AutoUploadConfigs) GetShow(userID int32) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.configs[userID]
}
