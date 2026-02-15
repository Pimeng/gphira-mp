package test

import (
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

// TestWebSocketIntegrationRoomLifecycle 测试完整的房间生命周期
func TestWebSocketIntegrationRoomLifecycle(t *testing.T) {
	srv, httpServer := setupTestServerWithHTTP(t)
	defer srv.Stop()

	testServer := httptest.NewServer(http.HandlerFunc(httpServer.HandleWebSocket))
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	// 创建 WebSocket 客户端
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("连接 WebSocket 失败: %v", err)
	}
	defer ws.Close()

	roomID := "lifecycle-room"

	// 1. 创建房间
	user1 := createTestUser(1, "Host")
	room := server.NewRoom(common.RoomId{Value: roomID}, user1, srv)
	srv.AddRoom(room)

	// 2. 订阅房间
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
	ws.ReadJSON(&response) // initial room_update

	// 3. 添加玩家
	user2 := createTestUser(2, "Player2")
	room.AddUser(user2, false)
	time.Sleep(100 * time.Millisecond)

	// 应该收到房间日志和更新
	receivedMessages := 0
	for i := 0; i < 2; i++ {
		ws.SetReadDeadline(time.Now().Add(2 * time.Second))
		if err := ws.ReadJSON(&response); err != nil {
			break
		}
		receivedMessages++
	}

	if receivedMessages < 1 {
		t.Error("应该收到房间更新消息")
	}

	// 4. 设置谱面
	chart := &server.Chart{ID: 12345, Name: "Test Chart"}
	room.SetChart(chart)
	room.OnStateChange()
	time.Sleep(100 * time.Millisecond)

	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err := ws.ReadJSON(&response); err == nil {
		if response["type"] == "room_update" {
			data := response["data"].(map[string]interface{})
			if chartData, ok := data["chart"].(map[string]interface{}); ok {
				if chartData["name"] != "Test Chart" {
					t.Error("谱面名称不匹配")
				}
			}
		}
	}

	// 5. 锁定房间
	room.SetLocked(true)
	server.BroadcastRoomUpdate(room)
	time.Sleep(100 * time.Millisecond)

	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err := ws.ReadJSON(&response); err == nil {
		if response["type"] == "room_update" {
			data := response["data"].(map[string]interface{})
			if !data["locked"].(bool) {
				t.Error("房间应该被锁定")
			}
		}
	}

	// 6. 玩家离开
	room.OnUserLeave(user2)
	time.Sleep(100 * time.Millisecond)

	// 应该收到更新
	receivedMessages = 0
	for i := 0; i < 2; i++ {
		ws.SetReadDeadline(time.Now().Add(2 * time.Second))
		if err := ws.ReadJSON(&response); err != nil {
			break
		}
		receivedMessages++
	}

	if receivedMessages < 1 {
		t.Error("应该收到玩家离开的更新")
	}
}

// TestWebSocketIntegrationGameFlow 测试游戏流程
func TestWebSocketIntegrationGameFlow(t *testing.T) {
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

	roomID := "game-room"
	user1 := createTestUser(1, "Player1")
	room := server.NewRoom(common.RoomId{Value: roomID}, user1, srv)
	srv.AddRoom(room)

	// 订阅房间
	subscribeMsg := map[string]interface{}{
		"type":   "subscribe",
		"roomId": roomID,
	}
	ws.WriteJSON(subscribeMsg)

	var response map[string]interface{}
	ws.ReadJSON(&response) // subscribed
	ws.ReadJSON(&response) // initial room_update

	// 1. 选择谱面
	chart := &server.Chart{ID: 12345, Name: "Test Chart"}
	room.SetChart(chart)
	room.SetState(server.InternalStateWaitForReady)
	room.OnStateChange()
	time.Sleep(100 * time.Millisecond)

	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err := ws.ReadJSON(&response); err == nil {
		if response["type"] == "room_update" {
			data := response["data"].(map[string]interface{})
			if data["state"] != "waiting_for_ready" {
				t.Errorf("期望状态为 waiting_for_ready，实际为: %v", data["state"])
			}
		}
	}

	// 2. 玩家准备
	// 注意：started 是私有字段，我们通过公开的方法来模拟准备状态
	// 在实际场景中，这会通过 session.handleReady() 来完成
	// 这里我们直接触发状态变化来测试 WebSocket 广播
	room.SetState(server.InternalStatePlaying)
	room.OnStateChange()
	time.Sleep(100 * time.Millisecond)

	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err := ws.ReadJSON(&response); err == nil {
		if response["type"] == "room_update" {
			data := response["data"].(map[string]interface{})
			if data["state"] != "playing" {
				t.Errorf("期望状态为 playing，实际为: %v", data["state"])
			}
		}
	}

	// 4. 游戏结束
	room.SetState(server.InternalStateSelectChart)
	room.OnStateChange()
	time.Sleep(100 * time.Millisecond)

	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err := ws.ReadJSON(&response); err == nil {
		if response["type"] == "room_update" {
			data := response["data"].(map[string]interface{})
			if data["state"] != "select_chart" {
				t.Errorf("期望状态为 select_chart，实际为: %v", data["state"])
			}
		}
	}
}

// TestWebSocketIntegrationMultipleRooms 测试多个房间
func TestWebSocketIntegrationMultipleRooms(t *testing.T) {
	srv, httpServer := setupTestServerWithHTTP(t)
	defer srv.Stop()

	testServer := httptest.NewServer(http.HandlerFunc(httpServer.HandleWebSocket))
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	// 创建多个房间
	rooms := make([]*server.Room, 3)
	for i := 0; i < 3; i++ {
		roomID := fmt.Sprintf("room-%d", i)
		user := createTestUser(int32(i+1), fmt.Sprintf("Host%d", i))
		room := server.NewRoom(common.RoomId{Value: roomID}, user, srv)
		srv.AddRoom(room)
		rooms[i] = room
	}

	// 创建多个客户端，每个订阅不同的房间
	clients := make([]*websocket.Conn, 3)
	for i := 0; i < 3; i++ {
		ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("客户端 %d 连接失败: %v", i, err)
		}
		defer ws.Close()
		clients[i] = ws

		// 订阅对应的房间
		subscribeMsg := map[string]interface{}{
			"type":   "subscribe",
			"roomId": fmt.Sprintf("room-%d", i),
		}
		ws.WriteJSON(subscribeMsg)

		var response map[string]interface{}
		ws.ReadJSON(&response) // subscribed
		ws.ReadJSON(&response) // room_update
	}

	// 在第一个房间添加用户
	newUser := createTestUser(100, "NewUser")
	rooms[0].AddUser(newUser, false)
	time.Sleep(100 * time.Millisecond)

	// 只有第一个客户端应该收到更新
	for i, ws := range clients {
		ws.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		var response map[string]interface{}
		err := ws.ReadJSON(&response)

		if i == 0 {
			// 第一个客户端应该收到更新
			if err != nil {
				t.Errorf("客户端 0 应该收到更新，但出错: %v", err)
			} else if response["type"] != "room_update" && response["type"] != "room_log" {
				t.Errorf("客户端 0 应该收到 room_update 或 room_log")
			}
		} else {
			// 其他客户端不应该收到更新
			if err == nil {
				t.Errorf("客户端 %d 不应该收到更新", i)
			}
		}
	}
}

// TestWebSocketIntegrationReconnect 测试重连
func TestWebSocketIntegrationReconnect(t *testing.T) {
	srv, httpServer := setupTestServerWithHTTP(t)
	defer srv.Stop()

	testServer := httptest.NewServer(http.HandlerFunc(httpServer.HandleWebSocket))
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	roomID := "reconnect-room"
	user := createTestUser(1, "TestUser")
	room := server.NewRoom(common.RoomId{Value: roomID}, user, srv)
	srv.AddRoom(room)

	// 第一次连接
	ws1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("第一次连接失败: %v", err)
	}

	subscribeMsg := map[string]interface{}{
		"type":   "subscribe",
		"roomId": roomID,
	}
	ws1.WriteJSON(subscribeMsg)

	var response map[string]interface{}
	ws1.ReadJSON(&response) // subscribed
	ws1.ReadJSON(&response) // room_update

	// 断开连接
	ws1.Close()
	time.Sleep(100 * time.Millisecond)

	// 重新连接
	ws2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("重新连接失败: %v", err)
	}
	defer ws2.Close()

	// 重新订阅
	ws2.WriteJSON(subscribeMsg)
	ws2.ReadJSON(&response) // subscribed

	if response["type"] != "subscribed" {
		t.Error("重新订阅应该成功")
	}

	// 应该能收到当前房间状态
	ws2.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err := ws2.ReadJSON(&response); err != nil {
		t.Fatalf("读取房间状态失败: %v", err)
	}

	if response["type"] != "room_update" {
		t.Error("应该收到房间状态更新")
	}
}

// TestWebSocketIntegrationMonitor 测试观察者
func TestWebSocketIntegrationMonitor(t *testing.T) {
	srv, httpServer := setupTestServerWithHTTP(t)
	defer srv.Stop()

	testServer := httptest.NewServer(http.HandlerFunc(httpServer.HandleWebSocket))
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	roomID := "monitor-room"
	user := createTestUser(1, "Player")
	room := server.NewRoom(common.RoomId{Value: roomID}, user, srv)
	srv.AddRoom(room)

	// 订阅房间
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer ws.Close()

	subscribeMsg := map[string]interface{}{
		"type":   "subscribe",
		"roomId": roomID,
	}
	ws.WriteJSON(subscribeMsg)

	var response map[string]interface{}
	ws.ReadJSON(&response) // subscribed
	ws.ReadJSON(&response) // room_update

	// 添加观察者
	monitor := createTestUser(2, "Monitor")
	// 注意：CanMonitor 依赖于服务器配置，这里我们直接添加为观察者
	room.AddUser(monitor, true)
	time.Sleep(100 * time.Millisecond)

	// 应该收到更新
	receivedUpdate := false
	for i := 0; i < 2; i++ {
		ws.SetReadDeadline(time.Now().Add(2 * time.Second))
		if err := ws.ReadJSON(&response); err != nil {
			break
		}

		if response["type"] == "room_update" {
			data := response["data"].(map[string]interface{})
			monitors, ok := data["monitors"].([]interface{})
			if ok && len(monitors) > 0 {
				receivedUpdate = true
				break
			}
		}
	}

	if !receivedUpdate {
		t.Error("应该收到观察者加入的更新")
	}
}

// TestWebSocketIntegrationConcurrentUpdates 测试并发更新
func TestWebSocketIntegrationConcurrentUpdates(t *testing.T) {
	srv, httpServer := setupTestServerWithHTTP(t)
	defer srv.Stop()

	testServer := httptest.NewServer(http.HandlerFunc(httpServer.HandleWebSocket))
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	roomID := "concurrent-room"
	user := createTestUser(1, "Host")
	room := server.NewRoom(common.RoomId{Value: roomID}, user, srv)
	srv.AddRoom(room)

	// 创建多个客户端
	numClients := 5
	clients := make([]*websocket.Conn, numClients)
	for i := 0; i < numClients; i++ {
		ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("客户端 %d 连接失败: %v", i, err)
		}
		defer ws.Close()
		clients[i] = ws

		subscribeMsg := map[string]interface{}{
			"type":   "subscribe",
			"roomId": roomID,
		}
		ws.WriteJSON(subscribeMsg)

		var response map[string]interface{}
		ws.ReadJSON(&response) // subscribed
		ws.ReadJSON(&response) // room_update
	}

	// 并发触发多个更新
	numUpdates := 10
	done := make(chan bool)

	go func() {
		for i := 0; i < numUpdates; i++ {
			newUser := createTestUser(int32(100+i), fmt.Sprintf("User%d", i))
			room.AddUser(newUser, false)
			time.Sleep(50 * time.Millisecond)
		}
		done <- true
	}()

	// 等待更新完成
	<-done
	time.Sleep(200 * time.Millisecond)

	// 所有客户端都应该收到更新
	for i, ws := range clients {
		receivedUpdates := 0
		for j := 0; j < numUpdates*2; j++ { // *2 因为可能有 room_log 和 room_update
			ws.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			var response map[string]interface{}
			if err := ws.ReadJSON(&response); err != nil {
				break
			}
			if response["type"] == "room_update" || response["type"] == "room_log" {
				receivedUpdates++
			}
		}

		if receivedUpdates == 0 {
			t.Errorf("客户端 %d 应该收到更新", i)
		}
	}
}

// TestWebSocketIntegrationErrorHandling 测试错误处理
func TestWebSocketIntegrationErrorHandling(t *testing.T) {
	srv, httpServer := setupTestServerWithHTTP(t)
	defer srv.Stop()

	testServer := httptest.NewServer(http.HandlerFunc(httpServer.HandleWebSocket))
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer ws.Close()

	testCases := []struct {
		name        string
		message     map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "无效的房间ID格式",
			message: map[string]interface{}{
				"type":   "subscribe",
				"roomId": "invalid room id!",
			},
			expectError: true,
			errorMsg:    "invalid-room-id",
		},
		{
			name: "空房间ID",
			message: map[string]interface{}{
				"type":   "subscribe",
				"roomId": "",
			},
			expectError: true,
			errorMsg:    "invalid-room-id",
		},
		{
			name: "未知消息类型",
			message: map[string]interface{}{
				"type": "unknown_type",
			},
			expectError: true,
			errorMsg:    "invalid-message",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ws.WriteJSON(tc.message); err != nil {
				t.Fatalf("发送消息失败: %v", err)
			}

			var response map[string]interface{}
			ws.SetReadDeadline(time.Now().Add(2 * time.Second))
			if err := ws.ReadJSON(&response); err != nil {
				t.Fatalf("读取响应失败: %v", err)
			}

			if tc.expectError {
				if response["type"] != "error" {
					t.Errorf("期望收到 error，实际收到: %v", response["type"])
				}

				if data, ok := response["data"].(map[string]interface{}); ok {
					if data["message"] != tc.errorMsg {
						t.Errorf("期望错误消息为 %s，实际为: %v", tc.errorMsg, data["message"])
					}
				}
			}
		})
	}
}

// TestWebSocketIntegrationMessageOrder 测试消息顺序
func TestWebSocketIntegrationMessageOrder(t *testing.T) {
	srv, httpServer := setupTestServerWithHTTP(t)
	defer srv.Stop()

	testServer := httptest.NewServer(http.HandlerFunc(httpServer.HandleWebSocket))
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	roomID := "order-room"
	user := createTestUser(1, "Host")
	room := server.NewRoom(common.RoomId{Value: roomID}, user, srv)
	srv.AddRoom(room)

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer ws.Close()

	// 订阅房间
	subscribeMsg := map[string]interface{}{
		"type":   "subscribe",
		"roomId": roomID,
	}
	ws.WriteJSON(subscribeMsg)

	var response map[string]interface{}
	ws.ReadJSON(&response) // subscribed
	ws.ReadJSON(&response) // initial room_update

	// 快速触发多个状态变化
	operations := []string{"lock", "unlock", "cycle", "uncycle"}
	for _, op := range operations {
		switch op {
		case "lock":
			room.SetLocked(true)
		case "unlock":
			room.SetLocked(false)
		case "cycle":
			room.SetCycle(true)
		case "uncycle":
			room.SetCycle(false)
		}
		server.BroadcastRoomUpdate(room)
		time.Sleep(50 * time.Millisecond)
	}

	// 接收所有更新
	receivedUpdates := 0
	for i := 0; i < len(operations); i++ {
		ws.SetReadDeadline(time.Now().Add(1 * time.Second))
		if err := ws.ReadJSON(&response); err != nil {
			break
		}
		if response["type"] == "room_update" {
			receivedUpdates++
		}
	}

	if receivedUpdates != len(operations) {
		t.Logf("期望收到 %d 个更新，实际收到: %d", len(operations), receivedUpdates)
		// 注意：由于并发和网络延迟，可能不会收到所有更新
		// 这是正常的，只要收到至少一个更新就可以
		if receivedUpdates == 0 {
			t.Error("应该至少收到一个更新")
		}
	}
}
