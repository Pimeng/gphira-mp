package utils

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// chartCacheEntry holds a cached chart with metadata.
type chartCacheEntry struct {
	ID             int32  `json:"id"`
	Name           string `json:"name"`
	CachedAt       int64  `json:"cached_at"`
	LastAccessedAt int64  `json:"last_accessed_at"`
}

// ChartCache is an LRU cache for Phira chart metadata.
// It persists to disk so cache survives server restarts.
type ChartCache struct {
	mu           sync.RWMutex
	entries      map[int32]*chartCacheEntry
	maxSize      int
	ttl          time.Duration
	persistPath  string
	initialized  bool
	saveInFlight bool
	savePending  bool
	stats        struct {
		hits   int
		misses int
	}
}

// NewChartCache creates a new chart cache.
// maxSize: maximum number of entries (LRU eviction).
// ttl: 0 means no expiration.
func NewChartCache(maxSize int, ttl time.Duration) *ChartCache {
	return &ChartCache{
		entries:     make(map[int32]*chartCacheEntry),
		maxSize:     maxSize,
		ttl:         ttl,
		persistPath: filepath.Join(".", "data", "cache", "chart_cache.json"),
	}
}

// Get retrieves a chart from cache. Returns nil if not found or expired.
func (c *ChartCache) Get(id int32) *struct {
	ID   int32
	Name string
} {
	// Try Redis first
	if rdb := GetRedisClient(); rdb != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		key := "chart_cache:" + intToStr(int(id))
		data, err := rdb.Get(ctx, key).Result()
		if err == nil {
			var entry chartCacheEntry
			if json.Unmarshal([]byte(data), &entry) == nil {
				if c.ttl == 0 || time.Since(time.UnixMilli(entry.CachedAt)) <= c.ttl {
					c.mu.Lock()
					c.entries[id] = &entry
					c.stats.hits++
					c.mu.Unlock()
					return &struct {
						ID   int32
						Name string
					}{ID: entry.ID, Name: entry.Name}
				}
			}
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.ensureInitializedLocked()

	entry, ok := c.entries[id]
	if !ok {
		c.stats.misses++
		return nil
	}

	if c.ttl > 0 && time.Since(time.UnixMilli(entry.CachedAt)) > c.ttl {
		delete(c.entries, id)
		c.stats.misses++
		return nil
	}

	entry.LastAccessedAt = time.Now().UnixMilli()
	c.stats.hits++
	return &struct {
		ID   int32
		Name string
	}{ID: entry.ID, Name: entry.Name}
}

// Set stores a chart in the cache.
func (c *ChartCache) Set(id int32, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ensureInitializedLocked()

	now := time.Now().UnixMilli()
	entry := &chartCacheEntry{
		ID:             id,
		Name:           name,
		CachedAt:       now,
		LastAccessedAt: now,
	}
	c.entries[id] = entry

	// LRU eviction
	if len(c.entries) > c.maxSize {
		var oldestID int32
		var oldestTime int64 = 1<<63 - 1
		for k, v := range c.entries {
			if v.LastAccessedAt < oldestTime {
				oldestTime = v.LastAccessedAt
				oldestID = k
			}
		}
		delete(c.entries, oldestID)
	}

	c.scheduleSaveLocked()

	// Also write to Redis
	if rdb := GetRedisClient(); rdb != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			data, _ := json.Marshal(entry)
			key := "chart_cache:" + intToStr(int(id))
			_ = rdb.Set(ctx, key, data, c.ttl).Err()
		}()
	}
}

// Delete removes a chart from the cache.
func (c *ChartCache) Delete(id int32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, id)
	c.scheduleSaveLocked()

	if rdb := GetRedisClient(); rdb != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = rdb.Del(ctx, "chart_cache:"+intToStr(int(id))).Err()
		}()
	}
}

// Clear removes all entries.
func (c *ChartCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[int32]*chartCacheEntry)
	c.stats.hits = 0
	c.stats.misses = 0
	c.scheduleSaveLocked()

	if rdb := GetRedisClient(); rdb != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var cursor uint64
			for {
				keys, next, err := rdb.Scan(ctx, cursor, "chart_cache:*", 100).Result()
				if err != nil {
					return
				}
				if len(keys) > 0 {
					_ = rdb.Del(ctx, keys...).Err()
				}
				if next == 0 {
					break
				}
				cursor = next
			}
		}()
	}
}

// Stats returns cache statistics.
func (c *ChartCache) Stats() (hits, misses int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats.hits, c.stats.misses
}

func (c *ChartCache) ensureInitializedLocked() {
	if c.initialized {
		return
	}
	c.initialized = true

	data, err := os.ReadFile(c.persistPath)
	if err != nil {
		return
	}

	var raw map[string]*chartCacheEntry
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}

	now := time.Now().UnixMilli()
	for _, entry := range raw {
		if entry == nil {
			continue
		}
		if c.ttl > 0 && now-entry.CachedAt > int64(c.ttl.Milliseconds()) {
			continue
		}
		c.entries[entry.ID] = entry
	}
}

func (c *ChartCache) scheduleSaveLocked() {
	if c.saveInFlight {
		c.savePending = true
		return
	}
	c.saveInFlight = true
	go c.saveToDiskWorker()
}

func (c *ChartCache) saveToDiskWorker() {
	for {
		if !c.saveToDisk() {
			c.mu.Lock()
			if c.savePending {
				c.savePending = false
				c.mu.Unlock()
				continue
			}
			c.saveInFlight = false
			c.mu.Unlock()
			return
		}

		c.mu.Lock()
		if c.savePending {
			c.savePending = false
			c.mu.Unlock()
			continue
		}
		c.saveInFlight = false
		c.mu.Unlock()
		return
	}
}

func (c *ChartCache) saveToDisk() bool {
	c.mu.RLock()
	obj := make(map[string]*chartCacheEntry, len(c.entries))
	for k, v := range c.entries {
		obj[intToStr(int(k))] = v
	}
	path := c.persistPath
	c.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false
	}

	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return false
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return false
	}
	return true
}

func intToStr(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf) - 1
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		buf[i] = byte('0' + v%10)
		v /= 10
		i--
	}
	if neg {
		buf[i] = '-'
		i--
	}
	return string(buf[i+1:])
}
