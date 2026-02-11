package test

import (
	"sync"
	"testing"
	"time"

	"phira-mp/common"
	"phira-mp/server"
)

// TestServerCreation 测试服务器创建
func TestServerCreation(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	if srv == nil {
		t.Fatal("服务器创建失败")
	}

	// 验证初始统计
	stats := srv.GetStats()
	if stats["sessions"] != 0 {
		t.Errorf("初始会话数应该是0，实际: %d", stats["sessions"])
	}
	if stats["users"] != 0 {
		t.Errorf("初始用户数应该是0，实际: %d", stats["users"])
	}
	if stats["rooms"] != 0 {
		t.Errorf("初始房间数应该是0，实际: %d", stats["rooms"])
	}
}

// TestServerUserManagement 测试服务器用户管理
func TestServerUserManagement(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	// 添加用户
	user1 := server.NewUser(1, "User1", "zh-CN", srv)
	srv.AddUser(user1)

	// 验证用户已添加
	retrievedUser := srv.GetUser(1)
	if retrievedUser == nil {
		t.Fatal("获取用户失败")
	}
	if retrievedUser.Name != "User1" {
		t.Errorf("用户名不匹配，期望: User1, 实际: %s", retrievedUser.Name)
	}

	// 添加多个用户
	for i := int32(2); i <= 5; i++ {
		user := server.NewUser(i, "User", "zh-CN", srv)
		srv.AddUser(user)
	}

	// 验证统计
	stats := srv.GetStats()
	if stats["users"] != 5 {
		t.Errorf("用户数应该是5，实际: %d", stats["users"])
	}

	// 移除用户
	srv.RemoveUser(1)
	if srv.GetUser(1) != nil {
		t.Error("用户应该已被移除")
	}

	stats = srv.GetStats()
	if stats["users"] != 4 {
		t.Errorf("移除后用户数应该是4，实际: %d", stats["users"])
	}
}

// TestServerRoomManagement 测试服务器房间管理
func TestServerRoomManagement(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	// 创建用户
	host := server.NewUser(1, "Host", "zh-CN", srv)
	srv.AddUser(host)

	// 创建房间
	roomID, _ := common.NewRoomId("test-room-1")
	room := server.NewRoom(roomID, host, srv)
	srv.AddRoom(room)

	// 验证房间已添加
	retrievedRoom := srv.GetRoom(roomID)
	if retrievedRoom == nil {
		t.Fatal("获取房间失败")
	}
	if retrievedRoom.ID.Value != "test-room-1" {
		t.Errorf("房间ID不匹配，期望: test-room-1, 实际: %s", retrievedRoom.ID.Value)
	}

	// 创建多个房间
	for i := 2; i <= 5; i++ {
		user := server.NewUser(int32(i), "Host", "zh-CN", srv)
		rid, _ := common.NewRoomId("test-room-" + string(rune('0'+i)))
		r := server.NewRoom(rid, user, srv)
		srv.AddRoom(r)
	}

	// 获取所有房间
	rooms := srv.GetAllRooms()
	if len(rooms) != 5 {
		t.Errorf("房间数应该是5，实际: %d", len(rooms))
	}

	// 验证统计
	stats := srv.GetStats()
	if stats["rooms"] != 5 {
		t.Errorf("统计中的房间数应该是5，实际: %d", stats["rooms"])
	}

	// 移除房间
	srv.RemoveRoom(roomID, "测试移除")
	if srv.GetRoom(roomID) != nil {
		t.Error("房间应该已被移除")
	}
}

// TestServerSessionManagement 测试服务器会话管理
func TestServerSessionManagement(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	// 由于会话需要网络连接，这里主要测试统计功能
	stats := srv.GetStats()
	if stats["sessions"] != 0 {
		t.Errorf("初始会话数应该是0，实际: %d", stats["sessions"])
	}
}

// TestServerConcurrentAccess 测试服务器并发访问
func TestServerConcurrentAccess(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	var wg sync.WaitGroup

	// 并发添加用户
	for i := int32(1); i <= 100; i++ {
		wg.Add(1)
		go func(id int32) {
			defer wg.Done()
			user := server.NewUser(id, "User", "zh-CN", srv)
			srv.AddUser(user)
		}(i)
	}

	// 并发读取用户
	for i := int32(1); i <= 50; i++ {
		wg.Add(1)
		go func(id int32) {
			defer wg.Done()
			_ = srv.GetUser(id)
		}(i)
	}

	// 并发获取统计
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = srv.GetStats()
		}()
	}

	wg.Wait()

	// 验证最终状态
	stats := srv.GetStats()
	if stats["users"] != 100 {
		t.Errorf("最终用户数应该是100，实际: %d", stats["users"])
	}
}

// TestServerRoomConcurrentOperations 测试服务器房间并发操作
func TestServerRoomConcurrentOperations(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	var wg sync.WaitGroup

	// 并发创建房间
	for i := int32(1); i <= 50; i++ {
		wg.Add(1)
		go func(id int32) {
			defer wg.Done()
			user := server.NewUser(id, "Host", "zh-CN", srv)
			srv.AddUser(user)
			roomID, _ := common.NewRoomId("room-" + string(rune('0'+byte(id%10))))
			room := server.NewRoom(roomID, user, srv)
			srv.AddRoom(room)
		}(i)
	}

	// 并发获取所有房间
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = srv.GetAllRooms()
		}()
	}

	wg.Wait()

	// 验证房间数量（由于可能有重复ID，数量可能少于50）
	rooms := srv.GetAllRooms()
	if len(rooms) == 0 {
		t.Error("应该有房间被创建")
	}
}

// TestServerDebugMode 测试服务器调试模式
func TestServerDebugMode(t *testing.T) {
	// 测试调试模式开启
	config := server.DefaultConfig()
	config.LogLevel = "debug"
	srv := server.NewServer(config)

	if !srv.IsDebugEnabled() {
		t.Error("调试模式应该已启用")
	}

	// 测试调试模式关闭
	config2 := server.DefaultConfig()
	config2.LogLevel = "info"
	srv2 := server.NewServer(config2)

	if srv2.IsDebugEnabled() {
		t.Error("调试模式应该未启用")
	}
}

// TestServerRoomCreationEnabled 测试服务器房间创建开关
func TestServerRoomCreationEnabled(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	// 默认应该允许创建房间
	if !srv.IsRoomCreationEnabled() {
		t.Error("默认应该允许创建房间")
	}
}

// TestServerBanUser 测试服务器封禁用户功能
func TestServerBanUser(t *testing.T) {
	config := server.DefaultConfig()
	config.HTTPService = true
	config.AdminToken = "test-token"
	srv := server.NewServer(config)

	// 用户初始状态：未封禁
	if srv.IsUserBanned(1) {
		t.Error("新用户不应该被封禁")
	}

	// 注意：实际封禁功能需要通过HTTP API操作
	// 这里只是测试接口存在
}

// TestServerBanUserFromRoom 测试服务器房间级封禁
func TestServerBanUserFromRoom(t *testing.T) {
	config := server.DefaultConfig()
	config.HTTPService = true
	config.AdminToken = "test-token"
	srv := server.NewServer(config)

	// 用户初始状态：未被封禁
	if srv.IsUserBannedFromRoom(1, "test-room") {
		t.Error("新用户不应该被禁止进入房间")
	}
}

// TestServerGetHTTPServer 测试获取HTTP服务器
func TestServerGetHTTPServer(t *testing.T) {
	config := server.DefaultConfig()
	config.HTTPService = true
	srv := server.NewServer(config)

	httpServer := srv.GetHTTPServer()
	if httpServer == nil {
		t.Error("应该能获取HTTP服务器")
	}
}

// TestServerGetReplayRecorder 测试获取回放录制器
func TestServerGetReplayRecorder(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	recorder := srv.GetReplayRecorder()
	// 即使没有启用HTTP服务，也应该有录制器实例
	if recorder == nil {
		t.Error("应该能获取回放录制器")
	}
}

// TestServerPrintStats 测试打印统计信息
func TestServerPrintStats(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	// 添加一些数据
	for i := int32(1); i <= 5; i++ {
		user := server.NewUser(i, "User", "zh-CN", srv)
		srv.AddUser(user)
	}

	// 打印统计（不应该panic）
	srv.PrintStats()
}

// TestServerStop 测试服务器停止
func TestServerStop(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	// 添加一些用户和房间
	for i := int32(1); i <= 3; i++ {
		user := server.NewUser(i, "User", "zh-CN", srv)
		srv.AddUser(user)
		roomID, _ := common.NewRoomId("room-" + string(rune('0'+byte(i))))
		room := server.NewRoom(roomID, user, srv)
		srv.AddRoom(room)
	}

	// 停止服务器（不应该panic）
	srv.Stop()
}

// TestServerStressTest 测试服务器压力测试
func TestServerStressTest(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	var wg sync.WaitGroup
	numUsers := 1000
	numRooms := 100

	// 创建大量用户
	for i := int32(1); i <= int32(numUsers); i++ {
		wg.Add(1)
		go func(id int32) {
			defer wg.Done()
			user := server.NewUser(id, "User", "zh-CN", srv)
			srv.AddUser(user)
		}(i)
	}

	// 创建大量房间
	for i := int32(1); i <= int32(numRooms); i++ {
		wg.Add(1)
		go func(id int32) {
			defer wg.Done()
			user := server.NewUser(1000+id, "Host", "zh-CN", srv)
			srv.AddUser(user)
			roomID, _ := common.NewRoomId("stress-room-" + string(rune('0'+byte(id%10))))
			room := server.NewRoom(roomID, user, srv)
			srv.AddRoom(room)
		}(i)
	}

	// 并发获取统计
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = srv.GetStats()
			_ = srv.GetAllRooms()
		}()
	}

	wg.Wait()

	// 验证结果
	stats := srv.GetStats()
	t.Logf("压力测试完成 - 用户数: %d, 房间数: %d", stats["users"], stats["rooms"])

	if stats["users"] != numUsers+numRooms {
		t.Errorf("用户数不匹配，期望: %d, 实际: %d", numUsers+numRooms, stats["users"])
	}
}

// TestServerRapidOperations 测试服务器快速操作
func TestServerRapidOperations(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	// 快速添加和移除
	for i := int32(1); i <= 100; i++ {
		user := server.NewUser(i, "User", "zh-CN", srv)
		srv.AddUser(user)
		srv.RemoveUser(i)
	}

	stats := srv.GetStats()
	if stats["users"] != 0 {
		t.Errorf("快速操作后用户数应该是0，实际: %d", stats["users"])
	}
}

// TestServerLongRunning 测试服务器长时间运行模拟
func TestServerLongRunning(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	// 模拟长时间运行的各种操作
	done := make(chan bool)
	go func() {
		for i := 0; i < 100; i++ {
			user := server.NewUser(int32(i), "User", "zh-CN", srv)
			srv.AddUser(user)
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 50; i++ {
			_ = srv.GetStats()
			_ = srv.GetAllRooms()
			time.Sleep(time.Millisecond * 2)
		}
		done <- true
	}()

	// 等待两个goroutine完成
	<-done
	<-done

	// 验证结果
	stats := srv.GetStats()
	if stats["users"] != 100 {
		t.Errorf("用户数应该是100，实际: %d", stats["users"])
	}
}
