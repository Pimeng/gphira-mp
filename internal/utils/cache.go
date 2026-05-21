package utils

import (
	"container/list"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
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
//
// Internally it uses a map + doubly-linked list (`container/list`) so that
// Get/Set/Delete and the eviction of the least-recently-used entry are all
// O(1). The list head is the most-recently-used entry; the tail is the
// eviction candidate. Cache contents are persisted to disk so they survive
// server restart, and (optionally) mirrored to Redis for cross-instance sharing.
type ChartCache struct {
	mu sync.RWMutex
	// entries maps chart id -> the linked-list element holding *chartCacheEntry.
	entries map[int32]*list.Element
	// order is the LRU list. Front = MRU, Back = LRU (next eviction target).
	order        *list.List
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
		entries:     make(map[int32]*list.Element),
		order:       list.New(),
		maxSize:     maxSize,
		ttl:         ttl,
		persistPath: filepath.Join(".", "data", "cache", "chart_cache.json"),
	}
}

// Get retrieves a chart from cache. Returns nil if not found or expired.
//
// On a hit it bumps the entry to the LRU head; on a TTL expiry it removes the
// entry as a side effect (so callers that miss-then-refetch don't re-encounter
// the stale row).
func (c *ChartCache) Get(id int32) *struct {
	ID   int32
	Name string
} {
	// Try Redis first. A Redis hit is treated as evidence that another
	// instance saw this chart recently, so we replay it into the local LRU.
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
					c.putLocked(&entry)
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

	elem, ok := c.entries[id]
	if !ok {
		c.stats.misses++
		return nil
	}

	entry := elem.Value.(*chartCacheEntry)
	if c.ttl > 0 && time.Since(time.UnixMilli(entry.CachedAt)) > c.ttl {
		c.removeElementLocked(elem)
		c.stats.misses++
		return nil
	}

	entry.LastAccessedAt = time.Now().UnixMilli()
	c.order.MoveToFront(elem)
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
	c.putLocked(entry)
	c.scheduleSaveLocked()

	// Also write to Redis (best-effort, async).
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
	if elem, ok := c.entries[id]; ok {
		c.removeElementLocked(elem)
	}
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
	c.entries = make(map[int32]*list.Element)
	c.order.Init()
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

// putLocked inserts or refreshes an entry and evicts the LRU tail if needed.
// Caller must hold c.mu in write mode.
func (c *ChartCache) putLocked(entry *chartCacheEntry) {
	if existing, ok := c.entries[entry.ID]; ok {
		existing.Value = entry
		c.order.MoveToFront(existing)
		return
	}
	elem := c.order.PushFront(entry)
	c.entries[entry.ID] = elem
	if c.maxSize > 0 && c.order.Len() > c.maxSize {
		c.removeElementLocked(c.order.Back())
	}
}

// removeElementLocked drops a single LRU element and its map entry.
// Caller must hold c.mu in write mode.
func (c *ChartCache) removeElementLocked(elem *list.Element) {
	if elem == nil {
		return
	}
	entry := elem.Value.(*chartCacheEntry)
	c.order.Remove(elem)
	delete(c.entries, entry.ID)
}

// ensureInitializedLocked lazily loads entries from disk on first use. Entries
// past their TTL are skipped; the rest are inserted in LastAccessedAt order so
// the on-disk LRU ranking survives a restart.
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
	loaded := make([]*chartCacheEntry, 0, len(raw))
	for _, entry := range raw {
		if entry == nil {
			continue
		}
		if c.ttl > 0 && now-entry.CachedAt > int64(c.ttl.Milliseconds()) {
			continue
		}
		loaded = append(loaded, entry)
	}
	// Insert by ascending LastAccessedAt using PushFront — the last entry
	// added (newest) ends up at Front, oldest at Back, matching LRU semantics.
	sort.Slice(loaded, func(i, j int) bool {
		return loaded[i].LastAccessedAt < loaded[j].LastAccessedAt
	})
	for _, entry := range loaded {
		elem := c.order.PushFront(entry)
		c.entries[entry.ID] = elem
	}
	// Trim if the on-disk file somehow exceeds maxSize.
	for c.maxSize > 0 && c.order.Len() > c.maxSize {
		c.removeElementLocked(c.order.Back())
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
		obj[intToStr(int(k))] = v.Value.(*chartCacheEntry)
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

// intToStr is a small, allocation-free int → string helper used both for the
// in-memory map's JSON keys and for Redis cache keys. Equivalent to
// strconv.Itoa but kept inline so this file has zero non-std dependencies.
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
