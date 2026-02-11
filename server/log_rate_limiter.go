package server

import (
	"fmt"
	"log"
	"sync"
	"time"
)

const (
	// LogRateThreshold 日志速率阈值（条/秒）
	LogRateThreshold = 10
	// LogRateWindow 日志速率计算窗口（秒）
	LogRateWindow = 1
	// LogCooldownDuration 日志限流冷却时间（3分钟）
	LogCooldownDuration = 3 * time.Minute
)

// LogRateLimiter 日志速率限制器
type LogRateLimiter struct {
	mu sync.RWMutex

	// 日志计数
	logCount    int
	windowStart time.Time

	// 限流状态
	throttled     bool
	throttleUntil time.Time
	throttleCount int // 限流期间被抑制的日志数量

	// 保护的关键日志前缀（这些日志不会被限流）
	protectedPrefixes []string
}

// NewLogRateLimiter 创建新的日志速率限制器
func NewLogRateLimiter() *LogRateLimiter {
	return &LogRateLimiter{
		windowStart:       time.Now(),
		protectedPrefixes: []string{"服务器", "房间", "玩家", "回放", "HTTP"},
	}
}

// ShouldLog 检查是否应该输出日志
func (l *LogRateLimiter) ShouldLog(message string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	// 检查是否在冷却期
	if l.throttled {
		if now.Before(l.throttleUntil) {
			// 仍在冷却期，检查是否是受保护的日志
			if l.isProtected(message) {
				return true
			}
			l.throttleCount++
			return false
		}
		// 冷却期结束，恢复日志
		l.throttled = false
		if l.throttleCount > 0 {
			log.Printf("[日志限流] 冷却期结束，期间共抑制 %d 条日志", l.throttleCount)
		}
		l.throttleCount = 0
		l.resetWindow(now)
	}

	// 检查是否需要重置窗口
	if now.Sub(l.windowStart) >= LogRateWindow*time.Second {
		l.resetWindow(now)
	}

	// 检查是否是受保护的日志
	if l.isProtected(message) {
		return true
	}

	// 增加计数
	l.logCount++

	// 检查是否超过阈值
	if l.logCount > LogRateThreshold {
		l.throttled = true
		l.throttleUntil = now.Add(LogCooldownDuration)
		l.throttleCount = 0
		log.Printf("[日志限流] 检测到日志刷屏（>%d条/秒），已启动限流，将在 %s 后恢复",
			LogRateThreshold, LogCooldownDuration)
		return false
	}

	return true
}

// resetWindow 重置计数窗口
func (l *LogRateLimiter) resetWindow(now time.Time) {
	l.logCount = 0
	l.windowStart = now
}

// isProtected 检查日志是否受保护（不被限流）
func (l *LogRateLimiter) CheckProtected(message string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.isProtected(message)
}

// isProtected 检查日志是否受保护（不加锁版本，内部使用）
func (l *LogRateLimiter) isProtected(message string) bool {
	// 检查是否是限流相关的日志本身
	if len(message) > 0 && message[0] == '[' && len(message) > 4 && message[:4] == "[日志" {
		return true
	}
	for _, prefix := range l.protectedPrefixes {
		if len(message) >= len(prefix) && message[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// GetStatus 获取当前限流状态
func (l *LogRateLimiter) GetStatus() map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return map[string]interface{}{
		"throttled":      l.throttled,
		"throttle_until": l.throttleUntil,
		"throttle_count": l.throttleCount,
		"current_count":  l.logCount,
		"window_start":   l.windowStart,
	}
}

// globalLogLimiter 全局日志限制器实例
var globalLogLimiter = NewLogRateLimiter()

// RateLimitedLog 速率受限的日志输出
func RateLimitedLog(format string, v ...interface{}) {
	message := format
	if len(v) > 0 {
		message = sprintf(format, v...)
	}

	if globalLogLimiter.ShouldLog(message) {
		log.Printf(message)
	}
}

// RateLimitedPrint 速率受限的直接输出
func RateLimitedPrint(message string) {
	if globalLogLimiter.ShouldLog(message) {
		log.Print(message)
	}
}

// sprintf 格式化字符串辅助函数
func sprintf(format string, v ...interface{}) string {
	return fmt.Sprintf(format, v...)
}

// GetLogLimiterStatus 获取日志限制器状态（供外部调用）
func GetLogLimiterStatus() map[string]interface{} {
	return globalLogLimiter.GetStatus()
}
