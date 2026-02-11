package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

// OTPInfo OTP信息
type OTPInfo struct {
	SSID      string
	OTP       string
	ClientIP  string
	ExpiresAt time.Time
}

// TempTokenInfo 临时token信息
type TempTokenInfo struct {
	Token     string
	ClientIP  string
	CreatedAt time.Time
	ExpiresAt time.Time
	Banned    bool // IP不匹配时标记为封禁
}

// OTPManager OTP管理器
type OTPManager struct {
	mu sync.RWMutex

	// OTP存储 ssid -> OTPInfo
	otps map[string]*OTPInfo

	// 临时token存储 token -> TempTokenInfo
	tempTokens map[string]*TempTokenInfo
}

// NewOTPManager 创建OTP管理器
func NewOTPManager() *OTPManager {
	m := &OTPManager{
		otps:       make(map[string]*OTPInfo),
		tempTokens: make(map[string]*TempTokenInfo),
	}

	// 启动清理协程
	go m.cleanupLoop()

	return m
}

// GenerateOTP 生成OTP
func (m *OTPManager) GenerateOTP(clientIP string) *OTPInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	ssid := uuid.New().String()
	otp := generateRandomCode(8)

	info := &OTPInfo{
		SSID:      ssid,
		OTP:       otp,
		ClientIP:  clientIP,
		ExpiresAt: time.Now().Add(OTPExpireTime),
	}

	m.otps[ssid] = info

	// 输出到终端（INFO级别）
	log.Printf("[OTP Request] SSID: %s, OTP: %s, Expires in 5 minutes", ssid, otp)

	return info
}

// ValidateOTP 验证OTP
func (m *OTPManager) ValidateOTP(ssid, otp, clientIP string) (*TempTokenInfo, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, ok := m.otps[ssid]
	if !ok {
		return nil, false
	}

	// 检查是否过期
	if time.Now().After(info.ExpiresAt) {
		delete(m.otps, ssid)
		return nil, false
	}

	// 验证OTP
	if info.OTP != otp {
		return nil, false
	}

	// OTP验证成功，删除OTP记录
	delete(m.otps, ssid)

	// 生成临时token
	tempToken := &TempTokenInfo{
		Token:     uuid.New().String(),
		ClientIP:  clientIP,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(TempTokenExpireTime),
		Banned:    false,
	}

	m.tempTokens[tempToken.Token] = tempToken

	return tempToken, true
}

// ValidateTempToken 验证临时token
func (m *OTPManager) ValidateTempToken(token, clientIP string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, ok := m.tempTokens[token]
	if !ok {
		return false
	}

	// 检查是否被封禁
	if info.Banned {
		return false
	}

	// 检查是否过期
	if time.Now().After(info.ExpiresAt) {
		delete(m.tempTokens, token)
		return false
	}

	// 检查IP是否匹配
	if info.ClientIP != clientIP {
		// IP不匹配，封禁该token
		info.Banned = true
		return false
	}

	return true
}

// RevokeTempToken 撤销临时token
func (m *OTPManager) RevokeTempToken(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tempTokens, token)
}

// 清理过期数据
func (m *OTPManager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	// 清理过期OTP
	for ssid, info := range m.otps {
		if now.After(info.ExpiresAt) {
			delete(m.otps, ssid)
		}
	}

	// 清理过期临时token
	for token, info := range m.tempTokens {
		if now.After(info.ExpiresAt) {
			delete(m.tempTokens, token)
		}
	}
}

// 清理循环
func (m *OTPManager) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.cleanup()
	}
}

// 生成随机验证码
func generateRandomCode(length int) string {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		// 如果随机生成失败，使用时间戳作为备选
		return fmt.Sprintf("%08d", time.Now().UnixNano()%100000000)
	}
	return hex.EncodeToString(bytes)[:length]
}
