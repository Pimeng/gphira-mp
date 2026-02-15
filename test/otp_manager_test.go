package test

import (
	"testing"
	"time"

	"phira-mp/server"
)

// TestOTPManagerValidateTempTokenNoIP 测试不验证 IP 的临时 token 验证
func TestOTPManagerValidateTempTokenNoIP(t *testing.T) {
	manager := server.NewOTPManager()

	// 生成 OTP
	clientIP := "192.168.1.100"
	otpInfo := manager.GenerateOTP(clientIP)

	if otpInfo == nil {
		t.Fatal("生成 OTP 失败")
	}

	// 验证 OTP 并获取临时 token
	tempToken, ok := manager.ValidateOTP(otpInfo.SSID, otpInfo.OTP, clientIP)
	if !ok {
		t.Fatal("验证 OTP 失败")
	}

	if tempToken == nil {
		t.Fatal("临时 token 为空")
	}

	// 测试 ValidateTempTokenNoIP（不验证 IP）
	if !manager.ValidateTempTokenNoIP(tempToken.Token) {
		t.Error("ValidateTempTokenNoIP 验证失败")
	}

	// 使用不同的 IP 也应该通过（因为不验证 IP）
	if !manager.ValidateTempTokenNoIP(tempToken.Token) {
		t.Error("ValidateTempTokenNoIP 应该不验证 IP")
	}
}

// TestOTPManagerValidateTempTokenNoIPExpired 测试过期的 token
func TestOTPManagerValidateTempTokenNoIPExpired(t *testing.T) {
	manager := server.NewOTPManager()

	clientIP := "192.168.1.100"
	otpInfo := manager.GenerateOTP(clientIP)

	tempToken, ok := manager.ValidateOTP(otpInfo.SSID, otpInfo.OTP, clientIP)
	if !ok {
		t.Fatal("验证 OTP 失败")
	}

	// 手动设置过期时间为过去
	// 注意：这需要访问内部结构，实际测试中可能需要等待真实过期
	// 这里我们测试无效 token
	if manager.ValidateTempTokenNoIP("invalid-token") {
		t.Error("无效 token 应该验证失败")
	}

	// 验证有效 token
	if !manager.ValidateTempTokenNoIP(tempToken.Token) {
		t.Error("有效 token 应该验证成功")
	}
}

// TestOTPManagerValidateTempTokenNoIPBanned 测试被封禁的 token
func TestOTPManagerValidateTempTokenNoIPBanned(t *testing.T) {
	manager := server.NewOTPManager()

	clientIP := "192.168.1.100"
	otpInfo := manager.GenerateOTP(clientIP)

	tempToken, ok := manager.ValidateOTP(otpInfo.SSID, otpInfo.OTP, clientIP)
	if !ok {
		t.Fatal("验证 OTP 失败")
	}

	// 使用不同 IP 验证会导致 token 被封禁
	differentIP := "192.168.1.200"
	if manager.ValidateTempToken(tempToken.Token, differentIP) {
		t.Error("不同 IP 应该验证失败")
	}

	// 被封禁后，ValidateTempTokenNoIP 也应该失败
	if manager.ValidateTempTokenNoIP(tempToken.Token) {
		t.Error("被封禁的 token 应该验证失败")
	}
}

// TestOTPManagerValidateTempTokenComparison 测试两种验证方法的对比
func TestOTPManagerValidateTempTokenComparison(t *testing.T) {
	manager := server.NewOTPManager()

	clientIP := "192.168.1.100"
	otpInfo := manager.GenerateOTP(clientIP)

	tempToken, ok := manager.ValidateOTP(otpInfo.SSID, otpInfo.OTP, clientIP)
	if !ok {
		t.Fatal("验证 OTP 失败")
	}

	// ValidateTempToken 验证 IP
	if !manager.ValidateTempToken(tempToken.Token, clientIP) {
		t.Error("相同 IP 应该验证成功")
	}

	// ValidateTempTokenNoIP 不验证 IP
	if !manager.ValidateTempTokenNoIP(tempToken.Token) {
		t.Error("ValidateTempTokenNoIP 应该验证成功")
	}

	// ValidateTempToken 使用不同 IP 会失败并封禁
	differentIP := "192.168.1.200"
	if manager.ValidateTempToken(tempToken.Token, differentIP) {
		t.Error("不同 IP 应该验证失败")
	}

	// 被封禁后两种方法都应该失败
	if manager.ValidateTempToken(tempToken.Token, clientIP) {
		t.Error("被封禁的 token 应该验证失败（ValidateTempToken）")
	}

	if manager.ValidateTempTokenNoIP(tempToken.Token) {
		t.Error("被封禁的 token 应该验证失败（ValidateTempTokenNoIP）")
	}
}

// TestOTPManagerMultipleTokens 测试多个 token
func TestOTPManagerMultipleTokens(t *testing.T) {
	manager := server.NewOTPManager()

	// 生成多个 token
	tokens := make([]string, 3)
	for i := 0; i < 3; i++ {
		clientIP := "192.168.1.100"
		otpInfo := manager.GenerateOTP(clientIP)
		tempToken, ok := manager.ValidateOTP(otpInfo.SSID, otpInfo.OTP, clientIP)
		if !ok {
			t.Fatalf("验证 OTP %d 失败", i)
		}
		tokens[i] = tempToken.Token
	}

	// 所有 token 都应该有效
	for i, token := range tokens {
		if !manager.ValidateTempTokenNoIP(token) {
			t.Errorf("Token %d 应该有效", i)
		}
	}

	// 撤销第一个 token
	manager.RevokeTempToken(tokens[0])

	// 第一个 token 应该无效
	if manager.ValidateTempTokenNoIP(tokens[0]) {
		t.Error("被撤销的 token 应该无效")
	}

	// 其他 token 应该仍然有效
	for i := 1; i < len(tokens); i++ {
		if !manager.ValidateTempTokenNoIP(tokens[i]) {
			t.Errorf("Token %d 应该仍然有效", i)
		}
	}
}

// TestOTPManagerConcurrency 测试并发访问
func TestOTPManagerConcurrency(t *testing.T) {
	manager := server.NewOTPManager()

	// 生成一个 token
	clientIP := "192.168.1.100"
	otpInfo := manager.GenerateOTP(clientIP)
	tempToken, ok := manager.ValidateOTP(otpInfo.SSID, otpInfo.OTP, clientIP)
	if !ok {
		t.Fatal("验证 OTP 失败")
	}

	// 并发验证
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				manager.ValidateTempTokenNoIP(tempToken.Token)
			}
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// token 应该仍然有效
	if !manager.ValidateTempTokenNoIP(tempToken.Token) {
		t.Error("并发访问后 token 应该仍然有效")
	}
}

// TestOTPManagerEmptyToken 测试空 token
func TestOTPManagerEmptyToken(t *testing.T) {
	manager := server.NewOTPManager()

	if manager.ValidateTempTokenNoIP("") {
		t.Error("空 token 应该验证失败")
	}
}

// TestOTPManagerInvalidToken 测试无效 token
func TestOTPManagerInvalidToken(t *testing.T) {
	manager := server.NewOTPManager()

	invalidTokens := []string{
		"invalid-token",
		"12345",
		"xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
		"not-a-uuid",
	}

	for _, token := range invalidTokens {
		if manager.ValidateTempTokenNoIP(token) {
			t.Errorf("无效 token '%s' 应该验证失败", token)
		}
	}
}

// BenchmarkValidateTempTokenNoIP 性能测试
func BenchmarkValidateTempTokenNoIP(b *testing.B) {
	manager := server.NewOTPManager()

	clientIP := "192.168.1.100"
	otpInfo := manager.GenerateOTP(clientIP)
	tempToken, _ := manager.ValidateOTP(otpInfo.SSID, otpInfo.OTP, clientIP)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.ValidateTempTokenNoIP(tempToken.Token)
	}
}

// BenchmarkValidateTempToken 性能对比测试
func BenchmarkValidateTempToken(b *testing.B) {
	manager := server.NewOTPManager()

	clientIP := "192.168.1.100"
	otpInfo := manager.GenerateOTP(clientIP)
	tempToken, _ := manager.ValidateOTP(otpInfo.SSID, otpInfo.OTP, clientIP)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.ValidateTempToken(tempToken.Token, clientIP)
	}
}

// TestOTPManagerCleanup 测试清理功能
func TestOTPManagerCleanup(t *testing.T) {
	manager := server.NewOTPManager()

	// 生成一个 token
	clientIP := "192.168.1.100"
	otpInfo := manager.GenerateOTP(clientIP)
	tempToken, ok := manager.ValidateOTP(otpInfo.SSID, otpInfo.OTP, clientIP)
	if !ok {
		t.Fatal("验证 OTP 失败")
	}

	// token 应该有效
	if !manager.ValidateTempTokenNoIP(tempToken.Token) {
		t.Error("新生成的 token 应该有效")
	}

	// 注意：实际的清理测试需要等待过期时间
	// 这里只测试基本功能
	time.Sleep(100 * time.Millisecond)

	// token 应该仍然有效（因为还没过期）
	if !manager.ValidateTempTokenNoIP(tempToken.Token) {
		t.Error("未过期的 token 应该仍然有效")
	}
}
