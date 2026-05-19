package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestChartCacheSaveCoalescedNoStuckFlags(t *testing.T) {
	tmp := t.TempDir()
	cache := NewChartCache(100, time.Minute)
	cache.persistPath = filepath.Join(tmp, "chart_cache.json")

	cache.mu.Lock()
	for i := 0; i < 50; i++ {
		cache.scheduleSaveLocked()
	}
	cache.mu.Unlock()

	deadline := time.Now().Add(2 * time.Second)
	for {
		cache.mu.RLock()
		inFlight := cache.saveInFlight
		pending := cache.savePending
		cache.mu.RUnlock()
		if !inFlight && !pending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("save flags stuck: inFlight=%v pending=%v", inFlight, pending)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestChartCacheConcurrentSetProducesValidJSON(t *testing.T) {
	tmp := t.TempDir()
	cache := NewChartCache(200, time.Minute)
	cache.persistPath = filepath.Join(tmp, "chart_cache.json")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.Set(int32(i), "name")
		}()
	}
	wg.Wait()

	deadline := time.Now().Add(2 * time.Second)
	for {
		cache.mu.RLock()
		inFlight := cache.saveInFlight
		pending := cache.savePending
		cache.mu.RUnlock()
		if !inFlight && !pending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cache save did not settle")
		}
		time.Sleep(10 * time.Millisecond)
	}

	data, err := os.ReadFile(cache.persistPath)
	if err != nil {
		t.Fatalf("read cache file failed: %v", err)
	}

	var parsed map[string]map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("cache file is invalid json: %v", err)
	}
	if len(parsed) == 0 {
		t.Fatal("cache file should contain entries")
	}
}
