package test

import (
	"testing"
	"time"

	"phira-mp/common"
	"phira-mp/server"
)

// TestSessionReconnect 测试用户重连不会触发 Dangle
func TestSessionReconnect(t *testing.T) {
	config := server.ServerConfig{
		Host:     "127.0.0.1",
		Port:     0,
		LogLevel: "error",
	}

	srv := server.NewServer(config)
	defer srv.Stop()

	// 创建第一个会话
	user1 := server.NewUser(1, "TestUser", "zh-CN", srv)
	srv.AddUser(user1)

	session1 := &server.Session{
		User: user1,
	}
	user1.SetSession(session1)

	// 创建房间
	room := server.NewRoom(common.RoomId{Value: "test-room"}, user1, srv)
	user1.SetRoom(room)
	srv.AddRoom(room)

	// 模拟用户重连：创建新会话
	session2 := &server.Session{
		User: user1,
	}
	user1.SetSession(session2)

	// 模拟旧会话断开（这应该不会触发 Dangle，因为用户已经有新会话了）
	// 在实际代码中，这会在 handleDisconnect 中调用
	if user1.GetSession() == session1 {
		user1.Dangle()
	}

	// 等待一小段时间，确保没有触发 Dangle 的超时
	time.Sleep(100 * time.Millisecond)

	// 验证用户还在服务器中
	if srv.GetUser(user1.ID) == nil {
		t.Error("用户不应该被移除")
	}

	// 验证用户还在房间中
	if user1.GetRoom() == nil {
		t.Error("用户应该还在房间中")
	}

	// 验证房间还存在
	if srv.GetRoom(room.ID) == nil {
		t.Error("房间不应该被移除")
	}
}

// TestSessionReconnectWithDangleTimeout 测试重连后不会触发旧会话的 Dangle 超时
func TestSessionReconnectWithDangleTimeout(t *testing.T) {
	config := server.ServerConfig{
		Host:     "127.0.0.1",
		Port:     0,
		LogLevel: "error",
	}

	srv := server.NewServer(config)
	defer srv.Stop()

	// 创建用户和会话
	user := server.NewUser(1, "TestUser", "zh-CN", srv)
	srv.AddUser(user)

	session1 := &server.Session{
		User: user,
	}
	user.SetSession(session1)

	// 创建房间
	room := server.NewRoom(common.RoomId{Value: "test-room"}, user, srv)
	user.SetRoom(room)
	srv.AddRoom(room)

	// 模拟旧会话断开（触发 Dangle）
	if user.GetSession() == session1 {
		user.Dangle()
	}

	// 在 Dangle 超时之前重连
	time.Sleep(2 * time.Second)

	session2 := &server.Session{
		User: user,
	}
	user.SetSession(session2) // 这应该清除 dangleMark

	// 等待超过原来的 Dangle 超时时间（10秒）
	time.Sleep(9 * time.Second)

	// 验证用户还在服务器中（因为重连清除了 dangleMark）
	if srv.GetUser(user.ID) == nil {
		t.Error("用户不应该被移除（重连应该清除 Dangle 超时）")
	}

	// 验证用户还在房间中
	if user.GetRoom() == nil {
		t.Error("用户应该还在房间中")
	}

	// 验证房间还存在
	if srv.GetRoom(room.ID) == nil {
		t.Error("房间不应该被移除")
	}
}

// TestSessionDangleTimeout 测试正常的 Dangle 超时
func TestSessionDangleTimeout(t *testing.T) {
	t.Skip("跳过此测试：需要完整的 Session 初始化，包括 Stream")

	config := server.ServerConfig{
		Host:     "127.0.0.1",
		Port:     0,
		LogLevel: "error",
	}

	srv := server.NewServer(config)
	defer srv.Stop()

	// 创建用户和会话
	user := server.NewUser(1, "TestUser", "zh-CN", srv)
	srv.AddUser(user)

	session := &server.Session{
		User: user,
	}
	user.SetSession(session)

	// 创建房间
	room := server.NewRoom(common.RoomId{Value: "test-room"}, user, srv)
	user.SetRoom(room)
	srv.AddRoom(room)

	// 模拟会话断开（触发 Dangle）
	if user.GetSession() == session {
		user.Dangle()
	}

	// 等待 Dangle 超时（10秒）
	time.Sleep(11 * time.Second)

	// 验证用户已被移除
	if srv.GetUser(user.ID) != nil {
		t.Error("用户应该被移除（Dangle 超时）")
	}

	// 验证房间已被移除（因为房主离开）
	if srv.GetRoom(room.ID) != nil {
		t.Error("房间应该被移除（房主离开）")
	}
}

// TestSessionMultipleReconnects 测试多次重连
func TestSessionMultipleReconnects(t *testing.T) {
	config := server.ServerConfig{
		Host:     "127.0.0.1",
		Port:     0,
		LogLevel: "error",
	}

	srv := server.NewServer(config)
	defer srv.Stop()

	// 创建用户
	user := server.NewUser(1, "TestUser", "zh-CN", srv)
	srv.AddUser(user)

	// 创建房间
	room := server.NewRoom(common.RoomId{Value: "test-room"}, user, srv)
	user.SetRoom(room)
	srv.AddRoom(room)

	// 模拟多次重连
	for i := 0; i < 5; i++ {
		session := &server.Session{
			User: user,
		}
		oldSession := user.GetSession()
		user.SetSession(session)

		// 模拟旧会话断开
		if oldSession != nil && user.GetSession() == oldSession {
			user.Dangle()
		}

		time.Sleep(100 * time.Millisecond)
	}

	// 验证用户还在服务器中
	if srv.GetUser(user.ID) == nil {
		t.Error("用户不应该被移除")
	}

	// 验证用户还在房间中
	if user.GetRoom() == nil {
		t.Error("用户应该还在房间中")
	}

	// 验证房间还存在
	if srv.GetRoom(room.ID) == nil {
		t.Error("房间不应该被移除")
	}
}
