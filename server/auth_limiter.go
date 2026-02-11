package server

import (
	"net"
	"sync"
	"time"
)

const (
	// MaxAuthAttempts 最大认证尝试次数
	MaxAuthAttempts = 5
	// AuthBlockDuration 认证失败后的封禁时间（15分钟）
	AuthBlockDuration = 15 * time.Minute
	// AuthAttemptWindow 认证尝试计数窗口（5分钟）
	AuthAttemptWindow = 5 * time.Minute
	// AuthCleanupInterval 清理过期记录的间隔
	AuthCleanupInterval = 10 * time.Minute
)

// AuthAttempt 认证尝试记录
type AuthAttempt struct {
	Count      int
	FirstAttempt time.Time
	LastAttempt  time.Time
	BlockedUntil *time.Time
}

// AuthLimiter 认证限流器
type AuthLimiter struct {
	mu sync.RWMutex
	
	// IP -> 尝试记录
	attempts map[string]*AuthAttempt
	
	// 清理定时器
	cleanupTicker *time.Ticker
	stopChan      chan struct{}
}

// NewAuthLimiter 创建新的认证限流器
func NewAuthLimiter() *AuthLimiter {
	al := &AuthLimiter{
		attempts: make(map[string]*AuthAttempt),
		stopChan: make(chan struct{}),
	}
	
	// 启动清理协程
	al.cleanupTicker = time.NewTicker(AuthCleanupInterval)
	go al.cleanupLoop()
	
	return al
}

// Stop 停止清理协程
func (al *AuthLimiter) Stop() {
	close(al.stopChan)
	if al.cleanupTicker != nil {
		al.cleanupTicker.Stop()
	}
}

// AllowAttempt 检查是否允许认证尝试
func (al *AuthLimiter) AllowAttempt(ip string) bool {
	al.mu.Lock()
	defer al.mu.Unlock()
	
	now := time.Now()
	attempt, exists := al.attempts[ip]
	
	if !exists {
		// 首次尝试
		al.attempts[ip] = &AuthAttempt{
			Count:        1,
			FirstAttempt: now,
			LastAttempt:  now,
		}
		return true
	}
	
	// 检查是否被封禁
	if attempt.BlockedUntil != nil && now.Before(*attempt.BlockedUntil) {
		return false
	}
	
	// 检查是否需要重置计数窗口
	if now.Sub(attempt.FirstAttempt) > AuthAttemptWindow {
		// 重置计数
		attempt.Count = 1
		attempt.FirstAttempt = now
		attempt.LastAttempt = now
		attempt.BlockedUntil = nil
		return true
	}
	
	// 增加尝试计数
	attempt.Count++
	attempt.LastAttempt = now
	
	// 检查是否超过最大尝试次数
	if attempt.Count > MaxAuthAttempts {
		blockUntil := now.Add(AuthBlockDuration)
		attempt.BlockedUntil = &blockUntil
		return false
	}
	
	return true
}

// RecordSuccess 记录认证成功（清除失败记录）
func (al *AuthLimiter) RecordSuccess(ip string) {
	al.mu.Lock()
	defer al.mu.Unlock()
	
	delete(al.attempts, ip)
}

// GetRemainingAttempts 获取剩余尝试次数
func (al *AuthLimiter) GetRemainingAttempts(ip string) int {
	al.mu.RLock()
	defer al.mu.RUnlock()
	
	attempt, exists := al.attempts[ip]
	if !exists {
		return MaxAuthAttempts
	}
	
	now := time.Now()
	
	// 检查是否被封禁
	if attempt.BlockedUntil != nil && now.Before(*attempt.BlockedUntil) {
		return 0
	}
	
	// 检查是否需要重置
	if now.Sub(attempt.FirstAttempt) > AuthAttemptWindow {
		return MaxAuthAttempts
	}
	
	remaining := MaxAuthAttempts - attempt.Count
	if remaining < 0 {
		return 0
	}
	return remaining
}

// IsBlocked 检查IP是否被封禁
func (al *AuthLimiter) IsBlocked(ip string) bool {
	al.mu.RLock()
	defer al.mu.RUnlock()
	
	attempt, exists := al.attempts[ip]
	if !exists {
		return false
	}
	
	if attempt.BlockedUntil == nil {
		return false
	}
	
	return time.Now().Before(*attempt.BlockedUntil)
}

// GetBlockTimeRemaining 获取剩余封禁时间
func (al *AuthLimiter) GetBlockTimeRemaining(ip string) time.Duration {
	al.mu.RLock()
	defer al.mu.RUnlock()
	
	attempt, exists := al.attempts[ip]
	if !exists || attempt.BlockedUntil == nil {
		return 0
	}
	
	remaining := attempt.BlockedUntil.Sub(time.Now())
	if remaining < 0 {
		return 0
	}
	return remaining
}

// cleanupLoop 清理过期记录的循环
func (al *AuthLimiter) cleanupLoop() {
	for {
		select {
		case <-al.cleanupTicker.C:
			al.cleanup()
		case <-al.stopChan:
			return
		}
	}
}

// cleanup 清理过期的尝试记录
func (al *AuthLimiter) cleanup() {
	al.mu.Lock()
	defer al.mu.Unlock()
	
	now := time.Now()
	for ip, attempt := range al.attempts {
		// 删除超过窗口期且未被封禁的记录
		if attempt.BlockedUntil == nil && now.Sub(attempt.LastAttempt) > AuthAttemptWindow {
			delete(al.attempts, ip)
			continue
		}
		
		// 删除封禁已过期的记录
		if attempt.BlockedUntil != nil && now.After(*attempt.BlockedUntil) {
			// 封禁已过期，但保留记录以防立即重试
			if now.Sub(*attempt.BlockedUntil) > AuthAttemptWindow {
				delete(al.attempts, ip)
			}
		}
	}
}

// GetClientIP 从请求中获取客户端IP（辅助函数）
func GetClientIP(r interface{}) string {
	// 尝试从http.Request获取
	if req, ok := r.(*interface{}); ok {
		_ = req
		// 这里需要具体的实现，取决于调用方式
	}
	return ""
}

// extractIPFromAddr 从地址字符串提取IP
func extractIPFromAddr(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}
