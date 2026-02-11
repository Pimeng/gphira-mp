package test

import (
	"sync"
	"testing"
	"time"

	"phira-mp/common"
	"phira-mp/server"
)

// TestUserCreation 测试用户创建
func TestUserCreation(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	user := server.NewUser(1, "TestUser", "zh-CN", srv)

	if user.ID != 1 {
		t.Errorf("用户ID不匹配，期望: 1, 实际: %d", user.ID)
	}

	if user.Name != "TestUser" {
		t.Errorf("用户名不匹配，期望: TestUser, 实际: %s", user.Name)
	}

	if user.Lang != "zh-CN" {
		t.Errorf("用户语言不匹配，期望: zh-CN, 实际: %s", user.Lang)
	}
}

// TestUserToInfo 测试用户信息转换
func TestUserToInfo(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	user := server.NewUser(1, "TestUser", "zh-CN", srv)
	info := user.ToInfo()

	if info.ID != 1 {
		t.Errorf("Info ID不匹配，期望: 1, 实际: %d", info.ID)
	}

	if info.Name != "TestUser" {
		t.Errorf("Info Name不匹配，期望: TestUser, 实际: %s", info.Name)
	}

	if info.Monitor {
		t.Error("新用户不应该被标记为观察者")
	}
}

// TestUserMonitorStatus 测试用户观察者状态
func TestUserMonitorStatus(t *testing.T) {
	config := server.DefaultConfig()
	config.LiveMode = true
	config.Monitors = []int32{1, 2}
	srv := server.NewServer(config)

	// 用户1在监控列表中
	user1 := server.NewUser(1, "MonitorUser", "zh-CN", srv)
	if !user1.CanMonitor() {
		t.Error("用户1应该能观察")
	}

	// 用户3不在监控列表中
	user3 := server.NewUser(3, "NormalUser", "zh-CN", srv)
	if user3.CanMonitor() {
		t.Error("用户3不应该能观察")
	}

	// 测试设置观察者状态
	user1.SetMonitor(true)
	if !user1.IsMonitor() {
		t.Error("用户应该被标记为观察者")
	}

	user1.SetMonitor(false)
	if user1.IsMonitor() {
		t.Error("用户不应该被标记为观察者")
	}
}

// TestUserMonitorDisabled 测试直播模式关闭时
func TestUserMonitorDisabled(t *testing.T) {
	config := server.DefaultConfig()
	config.LiveMode = false // 关闭直播模式
	config.Monitors = []int32{1}
	srv := server.NewServer(config)

	user := server.NewUser(1, "User", "zh-CN", srv)
	if user.CanMonitor() {
		t.Error("直播模式关闭时，用户不应该能观察")
	}
}

// TestUserRoomManagement 测试用户房间管理
func TestUserRoomManagement(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	user := server.NewUser(1, "TestUser", "zh-CN", srv)

	// 初始状态：不在任何房间
	if user.GetRoom() != nil {
		t.Error("新用户不应该在任何房间")
	}

	// 创建房间并设置
	roomID, _ := common.NewRoomId("test-room")
	room := server.NewRoom(roomID, user, srv)
	user.SetRoom(room)

	if user.GetRoom() == nil {
		t.Error("用户应该在房间中")
	}

	if user.GetRoom().ID.Value != "test-room" {
		t.Error("房间ID不匹配")
	}

	// 离开房间
	user.SetRoom(nil)
	if user.GetRoom() != nil {
		t.Error("用户应该已离开房间")
	}
}

// TestUserDisconnectedStatus 测试用户断开连接状态
func TestUserDisconnectedStatus(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	user := server.NewUser(1, "TestUser", "zh-CN", srv)

	// 初始状态：未断开
	if user.IsDisconnected() {
		t.Error("新用户不应该处于断开状态")
	}

	// 设置断开状态
	user.SetDisconnected(true)
	if !user.IsDisconnected() {
		t.Error("用户应该处于断开状态")
	}

	user.SetDisconnected(false)
	if user.IsDisconnected() {
		t.Error("用户不应该处于断开状态")
	}
}

// TestUserDangle 测试用户悬挂状态
func TestUserDangle(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	user := server.NewUser(1, "TestUser", "zh-CN", srv)

	// 设置悬挂状态
	user.Dangle()

	// 等待一段时间，但不要太久，避免触发超时处理
	time.Sleep(50 * time.Millisecond)

	// 这里主要验证不会panic
}

// TestUserConcurrentAccess 测试用户并发访问
func TestUserConcurrentAccess(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	user := server.NewUser(1, "TestUser", "zh-CN", srv)

	var wg sync.WaitGroup

	// 并发设置房间
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			roomID, _ := common.NewRoomId("room" + string(rune('0'+idx)))
			room := server.NewRoom(roomID, user, srv)
			user.SetRoom(room)
		}(i % 10)
	}

	// 并发读取房间
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = user.GetRoom()
		}()
	}

	// 并发设置断开状态
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(disconnected bool) {
			defer wg.Done()
			user.SetDisconnected(disconnected)
		}(i%2 == 0)
	}

	// 并发读取断开状态
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = user.IsDisconnected()
		}()
	}

	wg.Wait()

	// 验证最终状态一致性
	_ = user.GetRoom()
	_ = user.IsDisconnected()
}

// TestUserMultipleUsers 测试多用户场景
func TestUserMultipleUsers(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	users := make([]*server.User, 100)

	// 创建多个用户
	for i := int32(1); i <= 100; i++ {
		users[i-1] = server.NewUser(i, "User", "zh-CN", srv)
	}

	// 验证每个用户
	for i, user := range users {
		expectedID := int32(i + 1)
		if user.ID != expectedID {
			t.Errorf("用户 %d ID不匹配，期望: %d, 实际: %d", i, expectedID, user.ID)
		}
	}
}

// TestUserServerAssociation 测试用户与服务器关联
func TestUserServerAssociation(t *testing.T) {
	// 先设置直播模式配置
	config := server.DefaultConfig()
	config.LiveMode = true
	config.Monitors = []int32{1}
	srv := server.NewServer(config)

	// 创建用户（使用已启用直播模式的服务器）
	user := server.NewUser(1, "TestUser", "zh-CN", srv)

	// 用户应该与服务器关联
	// 注意：由于server字段是未导出的，我们通过行为来验证
	// 例如，CanMonitor()方法需要访问server.config

	if !user.CanMonitor() {
		t.Error("用户应该能观察")
	}
}
