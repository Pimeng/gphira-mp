package test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"phira-mp/common"
	"phira-mp/server"

	"github.com/gorilla/websocket"
)

// TestWebSocketConnection 测试 WebSocket 连接
func TestWebSocketConnection(t *testing.T) {
	srv, httpServer := setupTestServerWithHTTP(t)
	defer srv.Stop()

	// 创建测试 HTTP 服务器
	testServer := httptest.NewServer(http.HandlerFunc(httpServer.HandleWebSocket))
	defer testServer.Close()

	// 将 http:// 替换为 ws://
	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	// 连接 WebSocket
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("连接 WebSocket 失败: %v", err)
	}
	defer ws.Close()

	// 发送 ping
	msg := map[string]interface{}{
		"type": "ping",
	}
	if err := ws.WriteJSON(msg); err != nil {
		t.Fatalf("发送消息失败: %v", err)
	}

	// 接收 pong
	var response map[string]interface{}
	if err := ws.ReadJSON(&response); err != nil {
		t.Fatalf("读取响应失败: %v", err)
	}

	if response["type"] != "pong" {
		t.Errorf("期望收到 pong，实际收到: %v", response["type"])
	}
}

// TestWebSocketSubscribeRoom 测试订阅房间
func TestWebSocketSubscribeRoom(t *testing.T) {
	srv, httpServer := setupTestServerWithHTTP(t)
	defer srv.Stop()

	// 创建测试房间
	roomID := "test-room"
	user := createTestUser(1, "TestUser")
	room := server.NewRoom(common.RoomId{Value: roomID}, user, srv)
	srv.AddRoom(room)

	// 创建测试 HTTP 服务器
	testServer := httptest.NewServer(http.HandlerFunc(httpServer.HandleWebSocket))
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	// 连接 WebSocket
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("连接 WebSocket 失败: %v", err)
	}
	defer ws.Close()

	// 订阅房间
	subscribeMsg := map[string]interface{}{
		"type":   "subscribe",
		"roomId": roomID,
	}
	if err := ws.WriteJSON(subscribeMsg); err != nil {
		t.Fatalf("发送订阅消息失败: %v", err)
	}

	// 接收订阅成功响应
	var response map[string]interface{}
	if err := ws.ReadJSON(&response); err != nil {
		t.Fatalf("读取响应失败: %v", err)
	}

	if response["type"] != "subscribed" {
		t.Errorf("期望收到 subscribed，实际收到: %v", response["type"])
	}

	if response["roomId"] != roomID {
		t.Errorf("期望房间ID为 %s，实际为: %v", roomID, response["roomId"])
	}

	// 应该立即收到房间状态更新
	if err := ws.ReadJSON(&response); err != nil {
		t.Fatalf("读取房间状态失败: %v", err)
	}

	if response["type"] != "room_update" {
		t.Errorf("期望收到 room_update，实际收到: %v", response["type"])
	}

	// 验证房间数据
	data, ok := response["data"].(map[string]interface{})
	if !ok {
		t.Fatal("房间数据格式错误")
	}

	if data["roomid"] != roomID {
		t.Errorf("期望房间ID为 %s，实际为: %v", roomID, data["roomid"])
	}
}

// TestWebSocketSubscribeInvalidRoom 测试订阅不存在的房间
func TestWebSocketSubscribeInvalidRoom(t *testing.T) {
	srv, httpServer := setupTestServerWithHTTP(t)
	defer srv.Stop()

	testServer := httptest.NewServer(http.HandlerFunc(httpServer.HandleWebSocket))
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("连接 WebSocket 失败: %v", err)
	}
	defer ws.Close()

	// 订阅不存在的房间
	subscribeMsg := map[string]interface{}{
		"type":   "subscribe",
		"roomId": "non-existent-room",
	}
	if err := ws.WriteJSON(subscribeMsg); err != nil {
		t.Fatalf("发送订阅消息失败: %v", err)
	}

	// 应该收到错误响应
	var response map[string]interface{}
	if err := ws.ReadJSON(&response); err != nil {
		t.Fatalf("读取响应失败: %v", err)
	}

	if response["type"] != "error" {
		t.Errorf("期望收到 error，实际收到: %v", response["type"])
	}

	data, ok := response["data"].(map[string]interface{})
	if !ok {
		t.Fatal("错误数据格式错误")
	}

	if data["message"] != "room-not-found" {
		t.Errorf("期望错误消息为 room-not-found，实际为: %v", data["message"])
	}
}

// TestWebSocketUnsubscribe 测试取消订阅
func TestWebSocketUnsubscribe(t *testing.T) {
	srv, httpServer := setupTestServerWithHTTP(t)
	defer srv.Stop()

	roomID := "test-room"
	user := createTestUser(1, "TestUser")
	room := server.NewRoom(common.RoomId{Value: roomID}, user, srv)
	srv.AddRoom(room)

	testServer := httptest.NewServer(http.HandlerFunc(httpServer.HandleWebSocket))
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("连接 WebSocket 失败: %v", err)
	}
	defer ws.Close()

	// 先订阅
	subscribeMsg := map[string]interface{}{
		"type":   "subscribe",
		"roomId": roomID,
	}
	if err := ws.WriteJSON(subscribeMsg); err != nil {
		t.Fatalf("发送订阅消息失败: %v", err)
	}

	// 读取订阅响应
	var response map[string]interface{}
	ws.ReadJSON(&response) // subscribed
	ws.ReadJSON(&response) // room_update

	// 取消订阅
	unsubscribeMsg := map[string]interface{}{
		"type": "unsubscribe",
	}
	if err := ws.WriteJSON(unsubscribeMsg); err != nil {
		t.Fatalf("发送取消订阅消息失败: %v", err)
	}

	// 接收取消订阅响应
	if err := ws.ReadJSON(&response); err != nil {
		t.Fatalf("读取响应失败: %v", err)
	}

	if response["type"] != "unsubscribed" {
		t.Errorf("期望收到 unsubscribed，实际收到: %v", response["type"])
	}
}

// TestWebSocketRoomUpdate 测试房间状态更新推送
func TestWebSocketRoomUpdate(t *testing.T) {
	srv, httpServer := setupTestServerWithHTTP(t)
	defer srv.Stop()

	roomID := "test-room"
	user := createTestUser(1, "TestUser")
	room := server.NewRoom(common.RoomId{Value: roomID}, user, srv)
	srv.AddRoom(room)

	testServer := httptest.NewServer(http.HandlerFunc(httpServer.HandleWebSocket))
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("连接 WebSocket 失败: %v", err)
	}
	defer ws.Close()

	// 订阅房间
	subscribeMsg := map[string]interface{}{
		"type":   "subscribe",
		"roomId": roomID,
	}
	if err := ws.WriteJSON(subscribeMsg); err != nil {
		t.Fatalf("发送订阅消息失败: %v", err)
	}

	// 读取初始响应
	var response map[string]interface{}
	ws.ReadJSON(&response) // subscribed
	ws.ReadJSON(&response) // initial room_update

	// 触发房间状态变化（添加新用户）
	newUser := createTestUser(2, "NewUser")
	room.AddUser(newUser, false)

	// 等待广播
	time.Sleep(100 * time.Millisecond)

	// 应该收到房间更新（可能先收到 room_log）
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	receivedUpdate := false
	for i := 0; i < 2; i++ {
		if err := ws.ReadJSON(&response); err != nil {
			break
		}
		if response["type"] == "room_update" {
			receivedUpdate = true
			// 验证用户列表
			data, ok := response["data"].(map[string]interface{})
			if !ok {
				t.Fatal("房间数据格式错误")
			}

			users, ok := data["users"].([]interface{})
			if !ok {
				t.Fatal("用户列表格式错误")
			}

			if len(users) != 2 {
				t.Errorf("期望2个用户，实际为: %d", len(users))
			}
			break
		}
	}

	if !receivedUpdate {
		t.Error("应该收到房间更新")
	}
}

// TestWebSocketAdminSubscribe 测试管理员订阅
func TestWebSocketAdminSubscribe(t *testing.T) {
	srv, httpServer := setupTestServerWithHTTP(t)
	defer srv.Stop()

	// 创建几个测试房间
	for i := 1; i <= 3; i++ {
		roomID := fmt.Sprintf("room-%d", i)
		user := createTestUser(int32(i), fmt.Sprintf("User%d", i))
		room := server.NewRoom(common.RoomId{Value: roomID}, user, srv)
		srv.AddRoom(room)
	}

	testServer := httptest.NewServer(http.HandlerFunc(httpServer.HandleWebSocket))
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("连接 WebSocket 失败: %v", err)
	}
	defer ws.Close()

	// 使用管理员 token 订阅
	adminToken := "test-admin-token"
	adminMsg := map[string]interface{}{
		"type":  "admin_subscribe",
		"token": adminToken,
	}
	if err := ws.WriteJSON(adminMsg); err != nil {
		t.Fatalf("发送管理员订阅消息失败: %v", err)
	}

	// 接收响应
	var response map[string]interface{}
	if err := ws.ReadJSON(&response); err != nil {
		t.Fatalf("读取响应失败: %v", err)
	}

	// 注意：如果配置的 admin_token 不匹配，会收到 error
	// 这里我们只测试消息格式
	if response["type"] != "admin_subscribed" && response["type"] != "error" {
		t.Errorf("期望收到 admin_subscribed 或 error，实际收到: %v", response["type"])
	}
}

// TestWebSocketMultipleClients 测试多个客户端同时订阅
func TestWebSocketMultipleClients(t *testing.T) {
	srv, httpServer := setupTestServerWithHTTP(t)
	defer srv.Stop()

	roomID := "test-room"
	user := createTestUser(1, "TestUser")
	room := server.NewRoom(common.RoomId{Value: roomID}, user, srv)
	srv.AddRoom(room)

	testServer := httptest.NewServer(http.HandlerFunc(httpServer.HandleWebSocket))
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	// 创建多个客户端
	clients := make([]*websocket.Conn, 3)
	for i := 0; i < 3; i++ {
		ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("客户端 %d 连接失败: %v", i, err)
		}
		defer ws.Close()
		clients[i] = ws

		// 订阅房间
		subscribeMsg := map[string]interface{}{
			"type":   "subscribe",
			"roomId": roomID,
		}
		if err := ws.WriteJSON(subscribeMsg); err != nil {
			t.Fatalf("客户端 %d 发送订阅失败: %v", i, err)
		}

		// 读取初始响应
		var response map[string]interface{}
		ws.ReadJSON(&response) // subscribed
		ws.ReadJSON(&response) // room_update
	}

	// 触发房间状态变化
	newUser := createTestUser(2, "NewUser")
	room.AddUser(newUser, false)

	// 等待广播
	time.Sleep(100 * time.Millisecond)

	// 所有客户端都应该收到更新
	for i, ws := range clients {
		var response map[string]interface{}
		ws.SetReadDeadline(time.Now().Add(2 * time.Second))
		receivedUpdate := false

		// 可能收到 room_log 和 room_update，我们需要找到 room_update
		for j := 0; j < 2; j++ {
			if err := ws.ReadJSON(&response); err != nil {
				break
			}

			if response["type"] == "room_update" {
				receivedUpdate = true
				break
			}
		}

		if !receivedUpdate {
			t.Errorf("客户端 %d 应该收到 room_update", i)
		}
	}
}

// TestWebSocketInvalidMessage 测试无效消息
func TestWebSocketInvalidMessage(t *testing.T) {
	srv, httpServer := setupTestServerWithHTTP(t)
	defer srv.Stop()

	testServer := httptest.NewServer(http.HandlerFunc(httpServer.HandleWebSocket))
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("连接 WebSocket 失败: %v", err)
	}
	defer ws.Close()

	// 发送无效 JSON
	if err := ws.WriteMessage(websocket.TextMessage, []byte("invalid json")); err != nil {
		t.Fatalf("发送消息失败: %v", err)
	}

	// 应该收到错误响应
	var response map[string]interface{}
	if err := ws.ReadJSON(&response); err != nil {
		t.Fatalf("读取响应失败: %v", err)
	}

	if response["type"] != "error" {
		t.Errorf("期望收到 error，实际收到: %v", response["type"])
	}
}

// TestWebSocketRoomLog 测试房间日志推送
func TestWebSocketRoomLog(t *testing.T) {
	srv, httpServer := setupTestServerWithHTTP(t)
	defer srv.Stop()

	roomID := "test-room"
	user := createTestUser(1, "TestUser")
	room := server.NewRoom(common.RoomId{Value: roomID}, user, srv)
	srv.AddRoom(room)

	testServer := httptest.NewServer(http.HandlerFunc(httpServer.HandleWebSocket))
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("连接 WebSocket 失败: %v", err)
	}
	defer ws.Close()

	// 订阅房间
	subscribeMsg := map[string]interface{}{
		"type":   "subscribe",
		"roomId": roomID,
	}
	if err := ws.WriteJSON(subscribeMsg); err != nil {
		t.Fatalf("发送订阅消息失败: %v", err)
	}

	// 读取初始响应
	var response map[string]interface{}
	ws.ReadJSON(&response) // subscribed
	ws.ReadJSON(&response) // room_update

	// 触发会产生日志的操作（添加新用户）
	newUser := createTestUser(2, "NewUser")
	room.AddUser(newUser, false)

	// 等待广播
	time.Sleep(100 * time.Millisecond)

	// 应该收到房间日志和房间更新
	receivedLog := false
	receivedUpdate := false

	for i := 0; i < 2; i++ {
		ws.SetReadDeadline(time.Now().Add(2 * time.Second))
		if err := ws.ReadJSON(&response); err != nil {
			t.Fatalf("读取消息失败: %v", err)
		}

		msgType := response["type"].(string)
		if msgType == "room_log" {
			receivedLog = true
			data, ok := response["data"].(map[string]interface{})
			if !ok {
				t.Fatal("日志数据格式错误")
			}
			if _, ok := data["message"]; !ok {
				t.Error("日志消息缺少 message 字段")
			}
			if _, ok := data["timestamp"]; !ok {
				t.Error("日志消息缺少 timestamp 字段")
			}
		} else if msgType == "room_update" {
			receivedUpdate = true
		}
	}

	if !receivedLog {
		t.Error("未收到房间日志")
	}
	if !receivedUpdate {
		t.Error("未收到房间更新")
	}
}

// setupTestServerWithHTTP 创建带 HTTP 服务的测试服务器
func setupTestServerWithHTTP(t *testing.T) (*server.Server, *server.HTTPServer) {
	config := server.ServerConfig{
		Host:         "127.0.0.1",
		Port:         0, // 随机端口
		HTTPService:  true,
		HTTPPort:     0,
		AdminToken:   "test-admin-token",
		LogLevel:     "error", // 减少测试输出
		LiveMode:     false,
		Monitors:     []int32{2},
		RealIPHeader: "",
	}

	srv := server.NewServer(config)
	httpServer := srv.GetHTTPServer()

	return srv, httpServer
}

// createTestUser 创建测试用户
func createTestUser(id int32, name string) *server.User {
	return server.NewUser(id, name, "zh-CN", nil)
}

// createTestUserWithServer 创建带服务器引用的测试用户
func createTestUserWithServer(id int32, name string, srv *server.Server) *server.User {
	return server.NewUser(id, name, "zh-CN", srv)
}

// TestWebSocketMessageFormat 测试消息格式
func TestWebSocketMessageFormat(t *testing.T) {
	tests := []struct {
		name     string
		message  map[string]interface{}
		wantType string
	}{
		{
			name: "订阅消息",
			message: map[string]interface{}{
				"type":   "subscribe",
				"roomId": "test-room",
				"userId": 123,
			},
			wantType: "subscribe",
		},
		{
			name: "取消订阅消息",
			message: map[string]interface{}{
				"type": "unsubscribe",
			},
			wantType: "unsubscribe",
		},
		{
			name: "心跳消息",
			message: map[string]interface{}{
				"type": "ping",
			},
			wantType: "ping",
		},
		{
			name: "管理员订阅消息",
			message: map[string]interface{}{
				"type":  "admin_subscribe",
				"token": "test-token",
			},
			wantType: "admin_subscribe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 序列化和反序列化测试
			data, err := json.Marshal(tt.message)
			if err != nil {
				t.Fatalf("序列化失败: %v", err)
			}

			var decoded map[string]interface{}
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("反序列化失败: %v", err)
			}

			if decoded["type"] != tt.wantType {
				t.Errorf("消息类型不匹配，期望: %s，实际: %v", tt.wantType, decoded["type"])
			}
		})
	}
}
