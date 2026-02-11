package test

import (
	"sync"
	"testing"
	"time"

	"phira-mp/common"
	"phira-mp/server"
)

// TestRoomCreation 测试房间创建
func TestRoomCreation(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	// 创建测试用户
	host := server.NewUser(1, "TestHost", "zh-CN", srv)
	srv.AddUser(host)

	// 创建房间
	roomID, _ := common.NewRoomId("test-room-1")
	room := server.NewRoom(roomID, host, srv)

	if room.ID.Value != "test-room-1" {
		t.Errorf("房间ID不匹配，期望: test-room-1, 实际: %s", room.ID.Value)
	}

	if room.GetHost().ID != 1 {
		t.Errorf("房主ID不匹配，期望: 1, 实际: %d", room.GetHost().ID)
	}

	if room.GetState() != server.InternalStateSelectChart {
		t.Errorf("初始状态不匹配，期望: SelectChart, 实际: %v", room.GetState())
	}

	// 检查房间属性默认值
	if room.IsLive() {
		t.Error("新房间不应该是直播状态")
	}

	if room.IsLocked() {
		t.Error("新房间不应该被锁定")
	}

	if room.IsCycle() {
		t.Error("新房间不应该开启循环模式")
	}
}

// TestRoomUserManagement 测试房间用户管理
func TestRoomUserManagement(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	// 创建房主
	host := server.NewUser(1, "Host", "zh-CN", srv)
	srv.AddUser(host)

	roomID, _ := common.NewRoomId("test-room-users")
	room := server.NewRoom(roomID, host, srv)

	// 测试添加用户
	user2 := server.NewUser(2, "Player2", "zh-CN", srv)
	user3 := server.NewUser(3, "Player3", "zh-CN", srv)

	if !room.AddUser(user2, false) {
		t.Error("添加用户2失败")
	}

	if !room.AddUser(user3, false) {
		t.Error("添加用户3失败")
	}

	users := room.GetUsers()
	if len(users) != 3 { // host + user2 + user3
		t.Errorf("用户数量不匹配，期望: 3, 实际: %d", len(users))
	}

	// 测试添加观察者
	monitor := server.NewUser(4, "Monitor", "zh-CN", srv)
	if !room.AddUser(monitor, true) {
		t.Error("添加观察者失败")
	}

	monitors := room.GetMonitors()
	if len(monitors) != 1 {
		t.Errorf("观察者数量不匹配，期望: 1, 实际: %d", len(monitors))
	}

	allUsers := room.GetAllUsers()
	if len(allUsers) != 4 {
		t.Errorf("总用户数量不匹配，期望: 4, 实际: %d", len(allUsers))
	}

	// 测试移除用户
	room.RemoveUser(user2.ID)
	users = room.GetUsers()
	if len(users) != 2 {
		t.Errorf("移除后用户数量不匹配，期望: 2, 实际: %d", len(users))
	}
}

// TestRoomMaxUsers 测试房间最大用户数限制
func TestRoomMaxUsers(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	roomID, _ := common.NewRoomId("test-room-max")
	room := server.NewRoom(roomID, host, srv)

	// 尝试添加超过最大限制的用户
	for i := int32(2); i <= int32(server.RoomMaxUsers+2); i++ {
		user := server.NewUser(i, "Player", "zh-CN", srv)
		result := room.AddUser(user, false)

		if i <= int32(server.RoomMaxUsers) {
			if !result {
				t.Errorf("用户 %d 应该能加入房间", i)
			}
		} else {
			if result {
				t.Errorf("用户 %d 不应该能加入已满的房间", i)
			}
		}
	}
}

// TestRoomHostTransfer 测试房主转移
func TestRoomHostTransfer(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	roomID, _ := common.NewRoomId("test-room-host")
	room := server.NewRoom(roomID, host, srv)

	// 添加其他用户
	user2 := server.NewUser(2, "Player2", "zh-CN", srv)
	room.AddUser(user2, false)

	// 切换房主
	room.SetHost(user2)

	if room.GetHost().ID != 2 {
		t.Errorf("房主切换失败，期望: 2, 实际: %d", room.GetHost().ID)
	}

	// 检查权限
	if room.CheckHost(host) == nil {
		t.Error("原房主不应该有权限")
	}

	if room.CheckHost(user2) != nil {
		t.Error("新房主应该有权限")
	}
}

// TestRoomStateTransitions 测试房间状态转换
func TestRoomStateTransitions(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	roomID, _ := common.NewRoomId("test-room-state")
	room := server.NewRoom(roomID, host, srv)

	// 初始状态：选谱
	if room.GetState() != server.InternalStateSelectChart {
		t.Error("初始状态应该是 SelectChart")
	}

	// 切换到等待准备状态
	room.SetState(server.InternalStateWaitForReady)
	if room.GetState() != server.InternalStateWaitForReady {
		t.Error("状态切换失败")
	}

	// 切换到游戏中状态
	room.SetState(server.InternalStatePlaying)
	if room.GetState() != server.InternalStatePlaying {
		t.Error("状态切换失败")
	}

	// 测试状态转换到客户端状态
	chartID := int32(123)
	clientState := room.GetState().ToClientState(&chartID)

	if clientState.Type != common.RoomStatePlaying {
		t.Errorf("客户端状态不匹配，期望: Playing, 实际: %v", clientState.Type)
	}
}

// TestRoomLockAndCycle 测试房间锁定和循环模式
func TestRoomLockAndCycle(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	roomID, _ := common.NewRoomId("test-room-lock")
	room := server.NewRoom(roomID, host, srv)

	// 测试锁定
	room.SetLocked(true)
	if !room.IsLocked() {
		t.Error("房间锁定失败")
	}

	room.SetLocked(false)
	if room.IsLocked() {
		t.Error("房间解锁失败")
	}

	// 测试循环模式
	room.SetCycle(true)
	if !room.IsCycle() {
		t.Error("循环模式设置失败")
	}

	room.SetCycle(false)
	if room.IsCycle() {
		t.Error("循环模式关闭失败")
	}
}

// TestRoomChartManagement 测试房间谱面管理
func TestRoomChartManagement(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	roomID, _ := common.NewRoomId("test-room-chart")
	room := server.NewRoom(roomID, host, srv)

	// 初始应该没有谱面
	if room.GetChart() != nil {
		t.Error("新房间不应该有谱面")
	}

	// 设置谱面
	chart := &server.Chart{
		ID:   123,
		Name: "Test Chart",
	}
	room.SetChart(chart)

	retrievedChart := room.GetChart()
	if retrievedChart == nil {
		t.Fatal("获取谱面失败")
	}

	if retrievedChart.ID != 123 {
		t.Errorf("谱面ID不匹配，期望: 123, 实际: %d", retrievedChart.ID)
	}

	if retrievedChart.Name != "Test Chart" {
		t.Errorf("谱面名称不匹配，期望: Test Chart, 实际: %s", retrievedChart.Name)
	}
}

// TestRoomCycleHost 测试房主循环
func TestRoomCycleHost(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	user2 := server.NewUser(2, "Player2", "zh-CN", srv)
	user3 := server.NewUser(3, "Player3", "zh-CN", srv)

	roomID, _ := common.NewRoomId("test-room-cycle-host")
	room := server.NewRoom(roomID, host, srv)
	room.AddUser(user2, false)
	room.AddUser(user3, false)

	// 启用循环模式
	room.SetCycle(true)

	// 记录初始房主
	initialHost := room.GetHost()

	// 执行房主循环
	room.CycleHost()

	// 验证房主已切换
	newHost := room.GetHost()
	if newHost.ID == initialHost.ID {
		t.Error("房主应该已切换")
	}

	// 再次循环
	room.CycleHost()
	thirdHost := room.GetHost()

	if thirdHost.ID == newHost.ID {
		t.Error("房主应该再次切换")
	}

	// 循环三次应该回到初始房主
	room.CycleHost()
	if room.GetHost().ID != initialHost.ID {
		t.Error("循环三次后应该回到初始房主")
	}
}

// TestRoomConcurrentAccess 测试房间并发访问
func TestRoomConcurrentAccess(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	roomID, _ := common.NewRoomId("test-room-concurrent")
	room := server.NewRoom(roomID, host, srv)

	var wg sync.WaitGroup

	// 并发添加用户
	for i := int32(2); i <= 10; i++ {
		wg.Add(1)
		go func(id int32) {
			defer wg.Done()
			user := server.NewUser(id, "Player", "zh-CN", srv)
			room.AddUser(user, false)
		}(i)
	}

	// 并发读取
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = room.GetUsers()
			_ = room.GetAllUsers()
		}()
	}

	// 并发修改状态
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(state server.InternalRoomState) {
			defer wg.Done()
			room.SetState(state)
		}(server.InternalRoomState(i % 3))
	}

	wg.Wait()

	// 验证最终状态一致性
	users := room.GetUsers()
	if len(users) < 2 {
		t.Error("并发添加用户失败")
	}
}

// TestRoomBroadcast 测试房间广播功能
func TestRoomBroadcast(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	roomID, _ := common.NewRoomId("test-room-broadcast")
	room := server.NewRoom(roomID, host, srv)

	// 添加多个用户
	for i := int32(2); i <= 4; i++ {
		user := server.NewUser(i, "Player", "zh-CN", srv)
		room.AddUser(user, false)
	}

	// 添加观察者
	monitor := server.NewUser(5, "Monitor", "zh-CN", srv)
	room.AddUser(monitor, true)

	// 测试广播（这里只是验证不会panic）
	cmd := common.ServerCommand{
		Type: common.ServerCmdMessage,
		Message: &common.Message{
			Type:    common.MsgChat,
			User:    1,
			Content: "Test broadcast",
		},
	}

	// 广播给所有用户
	room.Broadcast(cmd)

	// 广播给观察者
	room.BroadcastMonitors(cmd)

	// 发送房间消息
	room.SendMessage(common.Message{
		Type:    common.MsgChat,
		User:    1,
		Content: "Room message",
	})
}

// TestRoomOnUserLeave 测试用户离开处理
func TestRoomOnUserLeave(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	user2 := server.NewUser(2, "Player2", "zh-CN", srv)

	roomID, _ := common.NewRoomId("test-room-leave")
	room := server.NewRoom(roomID, host, srv)
	room.AddUser(user2, false)

	// 普通用户离开
	shouldDelete := room.OnUserLeave(user2)
	if shouldDelete {
		t.Error("还有房主在，不应该删除房间")
	}

	users := room.GetUsers()
	if len(users) != 1 {
		t.Errorf("用户数量应该是1，实际: %d", len(users))
	}

	// 房主离开（只剩房主）
	shouldDelete = room.OnUserLeave(host)
	if !shouldDelete {
		t.Error("房间空了应该被删除")
	}
}

// TestRoomOnHostLeave 测试房主离开后的转移
func TestRoomOnHostLeave(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	user2 := server.NewUser(2, "Player2", "zh-CN", srv)
	user3 := server.NewUser(3, "Player3", "zh-CN", srv)

	roomID, _ := common.NewRoomId("test-room-host-leave")
	room := server.NewRoom(roomID, host, srv)
	room.AddUser(user2, false)
	room.AddUser(user3, false)

	// 房主离开
	shouldDelete := room.OnUserLeave(host)
	if shouldDelete {
		t.Error("还有其他用户，不应该删除房间")
	}

	// 验证新房主
	newHost := room.GetHost()
	if newHost.ID != 2 && newHost.ID != 3 {
		t.Errorf("新房主ID应该是2或3，实际: %d", newHost.ID)
	}

	// 验证用户列表
	users := room.GetUsers()
	if len(users) != 2 {
		t.Errorf("用户数量应该是2，实际: %d", len(users))
	}
}

// TestRoomClientRoomState 测试获取客户端房间状态
func TestRoomClientRoomState(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	user2 := server.NewUser(2, "Player2", "zh-CN", srv)

	roomID, _ := common.NewRoomId("test-room-client-state")
	room := server.NewRoom(roomID, host, srv)
	room.AddUser(user2, false)

	// 设置一些状态
	room.SetLive(true)
	room.SetLocked(true)
	room.SetCycle(true)
	room.SetChart(&server.Chart{ID: 456, Name: "Test Chart"})

	// 获取房主的客户端状态
	hostState := room.GetClientRoomState(host)

	if hostState.ID.Value != "test-room-client-state" {
		t.Error("房间ID不匹配")
	}

	if !hostState.IsHost {
		t.Error("房主应该识别为host")
	}

	if !hostState.Live {
		t.Error("Live状态不匹配")
	}

	if !hostState.Locked {
		t.Error("Locked状态不匹配")
	}

	if !hostState.Cycle {
		t.Error("Cycle状态不匹配")
	}

	if len(hostState.Users) != 2 {
		t.Errorf("用户数量应该是2，实际: %d", len(hostState.Users))
	}

	// 获取普通用户的客户端状态
	user2State := room.GetClientRoomState(user2)
	if user2State.IsHost {
		t.Error("普通用户不应该识别为host")
	}
}

// TestRoomCheckAllReady 测试检查所有玩家准备
func TestRoomCheckAllReady(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	user2 := server.NewUser(2, "Player2", "zh-CN", srv)

	roomID, _ := common.NewRoomId("test-room-ready")
	room := server.NewRoom(roomID, host, srv)
	room.AddUser(user2, false)

	// 设置等待准备状态
	room.SetState(server.InternalStateWaitForReady)

	// 调用CheckAllReady（不应该panic）
	room.CheckAllReady()

	// 等待异步操作完成
	time.Sleep(100 * time.Millisecond)
}

// TestRoomResetGameTime 测试重置游戏时间
func TestRoomResetGameTime(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	user2 := server.NewUser(2, "Player2", "zh-CN", srv)

	roomID, _ := common.NewRoomId("test-room-reset-time")
	room := server.NewRoom(roomID, host, srv)
	room.AddUser(user2, false)

	// 重置游戏时间不应该panic
	room.ResetGameTime()
}

// TestRoomLiveMode 测试房间直播模式
func TestRoomLiveMode(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	roomID, _ := common.NewRoomId("test-room-live")
	room := server.NewRoom(roomID, host, srv)

	// 初始状态
	if room.IsLive() {
		t.Error("新房间不应该处于直播模式")
	}

	// 开启直播模式
	room.SetLive(true)
	if !room.IsLive() {
		t.Error("房间应该处于直播模式")
	}

	// 关闭直播模式
	room.SetLive(false)
	if room.IsLive() {
		t.Error("房间不应该处于直播模式")
	}
}