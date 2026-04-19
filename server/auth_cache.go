package server

import (
	"sync"
	"time"
)

const (
	// authCacheTTL 认证缓存有效期（同一Token在此时间内重连直接复用缓存）
	authCacheTTL = 3 * time.Minute
)

// authCacheEntry 认证缓存条目
type authCacheEntry struct {
	id   int32
	name string
	lang string
	at   time.Time
}

// authCache Token认证结果缓存
type authCache struct {
	mu      sync.Mutex
	entries map[string]*authCacheEntry
}

var globalAuthCache = &authCache{
	entries: make(map[string]*authCacheEntry),
}

// get 查询缓存，若命中且未过期则返回用户基本信息
func (c *authCache) get(token string) (id int32, name, lang string, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, exists := c.entries[token]
	if !exists {
		return 0, "", "", false
	}
	if time.Since(entry.at) > authCacheTTL {
		delete(c.entries, token)
		return 0, "", "", false
	}
	return entry.id, entry.name, entry.lang, true
}

// set 写入缓存，同时清理过期条目
func (c *authCache) set(token string, id int32, name, lang string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// 清理过期条目，避免内存泄漏
	now := time.Now()
	for k, v := range c.entries {
		if now.Sub(v.at) > authCacheTTL {
			delete(c.entries, k)
		}
	}
	c.entries[token] = &authCacheEntry{
		id:   id,
		name: name,
		lang: lang,
		at:   now,
	}
}
