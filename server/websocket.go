package server

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"phira-mp/common"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源
	},
}

// WebSocketMessage WebSocket消息
type WebSocketMessage struct {
	Type   string      `json:"type"`
	RoomID string      `json:"roomId,omitempty"`
	UserID *int32      `json:"userId,omitempty"`
	Token  string      `json:"token,omitempty"`
	Data   interface{} `json:"data,omitempty"`
}

// WebSocketClient WebSocket客户端
type WebSocketClient struct {
	conn           *websocket.Conn
	send           chan []byte
	server         *HTTPServer
	subscribedRoom string
	isAdmin        bool
	mu             sync.RWMutex
}

// WebSocketHub WebSocket中心
type WebSocketHub struct {
	clients    map[*WebSocketClient]bool
	register   chan *WebSocketClient
	unregister chan *WebSocketClient
	broadcast  chan *BroadcastMessage
	mu         sync.RWMutex
}

// BroadcastMessage 广播消息
type BroadcastMessage struct {
	roomID  string
	message []byte
	isAdmin bool
}

var hub *WebSocketHub

func init() {
	hub = &WebSocketHub{
		clients:    make(map[*WebSocketClient]bool),
		register:   make(chan *WebSocketClient),
		unregister: make(chan *WebSocketClient),
		broadcast:  make(chan *BroadcastMessage, 256),
	}
	go hub.run()
}

func (h *WebSocketHub) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				if message.isAdmin {
					// 管理员消息只发给管理员客户端
					if client.isAdmin {
						select {
						case client.send <- message.message:
						default:
							close(client.send)
							delete(h.clients, client)
						}
					}
				} else if message.roomID != "" {
					// 房间消息只发给订阅该房间的客户端
					client.mu.RLock()
					subscribed := client.subscribedRoom == message.roomID
					client.mu.RUnlock()
					if subscribed {
						select {
						case client.send <- message.message:
						default:
							close(client.send)
							delete(h.clients, client)
						}
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// HandleWebSocket 处理WebSocket连接
func (h *HTTPServer) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket升级失败: %v", err)
		return
	}

	client := &WebSocketClient{
		conn:   conn,
		send:   make(chan []byte, 256),
		server: h,
	}

	hub.register <- client

	go client.writePump()
	go client.readPump()
}

func (c *WebSocketClient) readPump() {
	defer func() {
		hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket错误: %v", err)
			}
			break
		}

		var msg WebSocketMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			c.sendError("invalid-message")
			continue
		}

		c.handleMessage(&msg)
	}
}

func (c *WebSocketClient) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *WebSocketClient) handleMessage(msg *WebSocketMessage) {
	switch msg.Type {
	case "ping":
		c.sendMessage(WebSocketMessage{Type: "pong"})

	case "subscribe":
		c.handleSubscribe(msg)

	case "unsubscribe":
		c.handleUnsubscribe()

	case "admin_subscribe":
		c.handleAdminSubscribe(msg)

	case "admin_unsubscribe":
		c.handleAdminUnsubscribe()

	default:
		c.sendError("invalid-message")
	}
}

func (c *WebSocketClient) handleSubscribe(msg *WebSocketMessage) {
	if msg.RoomID == "" {
		c.sendError("invalid-room-id")
		return
	}

	if !isValidRoomID(msg.RoomID) {
		c.sendError("invalid-room-id")
		return
	}

	room := c.server.server.GetRoom(common.RoomId{Value: msg.RoomID})
	if room == nil {
		c.sendError("room-not-found")
		return
	}

	c.mu.Lock()
	c.subscribedRoom = msg.RoomID
	c.isAdmin = false
	c.mu.Unlock()

	c.sendMessage(WebSocketMessage{
		Type:   "subscribed",
		RoomID: msg.RoomID,
	})

	// 立即发送当前房间状态
	c.sendRoomUpdate(room)
}

func (c *WebSocketClient) handleUnsubscribe() {
	c.mu.Lock()
	c.subscribedRoom = ""
	c.isAdmin = false
	c.mu.Unlock()

	c.sendMessage(WebSocketMessage{Type: "unsubscribed"})
}

func (c *WebSocketClient) handleAdminSubscribe(msg *WebSocketMessage) {
	// 验证管理员token
	if !c.validateAdminToken(msg.Token) {
		c.sendError("unauthorized")
		return
	}

	c.mu.Lock()
	c.isAdmin = true
	c.subscribedRoom = ""
	c.mu.Unlock()

	c.sendMessage(WebSocketMessage{Type: "admin_subscribed"})

	// 立即发送当前所有房间状态
	c.sendAdminUpdate()
}

func (c *WebSocketClient) handleAdminUnsubscribe() {
	c.mu.Lock()
	c.isAdmin = false
	c.mu.Unlock()

	c.sendMessage(WebSocketMessage{Type: "admin_unsubscribed"})
}

func (c *WebSocketClient) sendMessage(msg WebSocketMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("序列化WebSocket消息失败: %v", err)
		return
	}

	select {
	case c.send <- data:
	default:
		log.Printf("WebSocket发送缓冲区已满")
	}
}

func (c *WebSocketClient) sendError(errorMsg string) {
	c.sendMessage(WebSocketMessage{
		Type: "error",
		Data: map[string]string{"message": errorMsg},
	})
}

func (c *WebSocketClient) sendRoomUpdate(room *Room) {
	data := c.buildRoomData(room)
	c.sendMessage(WebSocketMessage{
		Type: "room_update",
		Data: data,
	})
}

func (c *WebSocketClient) sendAdminUpdate() {
	rooms := c.server.server.GetAllRooms()
	roomsData := make([]interface{}, 0, len(rooms))

	for _, room := range rooms {
		roomsData = append(roomsData, c.buildAdminRoomData(room))
	}

	c.sendMessage(WebSocketMessage{
		Type: "admin_update",
		Data: map[string]interface{}{
			"timestamp": time.Now().UnixMilli(),
			"changes": map[string]interface{}{
				"rooms":       roomsData,
				"total_rooms": len(rooms),
			},
		},
	})
}

func (c *WebSocketClient) buildRoomData(room *Room) map[string]interface{} {
	host := room.GetHost()
	chart := room.GetChart()

	data := map[string]interface{}{
		"roomid": room.ID.Value,
		"state":  c.getRoomStateString(room),
		"locked": room.IsLocked(),
		"cycle":  room.IsCycle(),
		"live":   room.IsLive(),
		"host": map[string]interface{}{
			"id":   host.ID,
			"name": host.Name,
		},
	}

	if chart != nil {
		data["chart"] = map[string]interface{}{
			"name": chart.Name,
			"id":   chart.ID,
		}
	}

	users := room.GetUsers()
	usersData := make([]map[string]interface{}, 0, len(users))
	for _, u := range users {
		_, isReady := room.started.Load(u.ID)
		usersData = append(usersData, map[string]interface{}{
			"id":       u.ID,
			"name":     u.Name,
			"is_ready": isReady,
		})
	}
	data["users"] = usersData

	monitors := room.GetMonitors()
	monitorsData := make([]map[string]interface{}, 0, len(monitors))
	for _, m := range monitors {
		monitorsData = append(monitorsData, map[string]interface{}{
			"id":   m.ID,
			"name": m.Name,
		})
	}
	data["monitors"] = monitorsData

	return data
}

func (c *WebSocketClient) buildAdminRoomData(room *Room) map[string]interface{} {
	// 复用基础房间数据
	data := c.buildRoomData(room)

	// 添加管理员专属信息
	users := room.GetUsers()
	data["max_users"] = RoomMaxUsers
	data["current_users"] = len(users)
	data["current_monitors"] = len(room.GetMonitors())

	// 添加详细的用户信息
	usersData := make([]map[string]interface{}, 0, len(users))
	for _, u := range users {
		_, isReady := room.started.Load(u.ID)
		_, finished := room.results.Load(u.ID)
		_, aborted := room.aborted.Load(u.ID)

		usersData = append(usersData, map[string]interface{}{
			"id":        u.ID,
			"name":      u.Name,
			"connected": !u.IsDisconnected(),
			"is_host":   room.GetHost().ID == u.ID,
			"is_ready":  isReady,
			"finished":  finished,
			"aborted":   aborted,
		})
	}
	data["users"] = usersData

	return data
}

func (c *WebSocketClient) getRoomStateString(room *Room) string {
	state := room.GetState()
	switch state {
	case InternalStateSelectChart:
		return "select_chart"
	case InternalStateWaitForReady:
		return "waiting_for_ready"
	case InternalStatePlaying:
		return "playing"
	default:
		return "select_chart"
	}
}

func (c *WebSocketClient) validateAdminToken(token string) bool {
	// 检查永久token
	if c.server.config.AdminToken != "" && token == c.server.config.AdminToken {
		return true
	}

	// 检查临时token（不验证IP，因为WebSocket可能来自不同IP）
	return c.server.otpManager.ValidateTempTokenNoIP(token)
}

// BroadcastRoomUpdate 广播房间更新
func BroadcastRoomUpdate(room *Room) {
	client := &WebSocketClient{server: room.server.GetHTTPServer()}
	data := client.buildRoomData(room)

	msg := WebSocketMessage{
		Type: "room_update",
		Data: data,
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		log.Printf("序列化房间更新失败: %v", err)
		return
	}

	hub.broadcast <- &BroadcastMessage{
		roomID:  room.ID.Value,
		message: msgBytes,
		isAdmin: false,
	}

	// 同时发送给管理员
	BroadcastAdminUpdate(room.server)
}

// BroadcastRoomLog 广播房间日志
func BroadcastRoomLog(roomID string, message string) {
	msg := WebSocketMessage{
		Type: "room_log",
		Data: map[string]interface{}{
			"message":   message,
			"timestamp": time.Now().UnixMilli(),
		},
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		log.Printf("序列化房间日志失败: %v", err)
		return
	}

	hub.broadcast <- &BroadcastMessage{
		roomID:  roomID,
		message: msgBytes,
		isAdmin: false,
	}
}

// BroadcastAdminUpdate 广播管理员更新
func BroadcastAdminUpdate(server *Server) {
	if server.GetHTTPServer() == nil {
		return
	}

	client := &WebSocketClient{server: server.GetHTTPServer()}
	rooms := server.GetAllRooms()
	roomsData := make([]interface{}, 0, len(rooms))

	for _, room := range rooms {
		roomsData = append(roomsData, client.buildAdminRoomData(room))
	}

	msg := WebSocketMessage{
		Type: "admin_update",
		Data: map[string]interface{}{
			"timestamp": time.Now().UnixMilli(),
			"changes": map[string]interface{}{
				"rooms":       roomsData,
				"total_rooms": len(rooms),
			},
		},
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		log.Printf("序列化管理员更新失败: %v", err)
		return
	}

	hub.broadcast <- &BroadcastMessage{
		roomID:  "",
		message: msgBytes,
		isAdmin: true,
	}
}
