package network

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WSClient represents a connected WebSocket client.
type WSClient struct {
	conn              *websocket.Conn
	roomID            roomid.RoomID
	userID            int32
	isAdmin           bool
	clientIP          string
	isAlive           bool
	closed            bool
	lastAdminSnapshot string
	send              chan []byte
	server            *WSServer
	writeMu           sync.Mutex
	stateMu           sync.RWMutex
}

// WSServer manages WebSocket connections and broadcasts.
type WSServer struct {
	state *HTTPServer

	mu        sync.RWMutex
	clients   map[*WSClient]struct{}
	rooms     map[roomid.RoomID]map[*WSClient]struct{}
	admins    map[*WSClient]struct{}
	register  chan *WSClient
	leave     chan *WSClient
	broadcast chan wsBroadcastMsg
	stop      chan struct{}
}

type wsBroadcastMsg struct {
	roomID  roomid.RoomID
	msgType string
	data    any
}

// NewWSServer creates a new WebSocket server.
func NewWSServer(state *HTTPServer) *WSServer {
	return &WSServer{
		state:     state,
		clients:   make(map[*WSClient]struct{}),
		rooms:     make(map[roomid.RoomID]map[*WSClient]struct{}),
		admins:    make(map[*WSClient]struct{}),
		register:  make(chan *WSClient),
		leave:     make(chan *WSClient),
		broadcast: make(chan wsBroadcastMsg, 64),
		stop:      make(chan struct{}),
	}
}

// Start begins the WebSocket server goroutines.
func (s *WSServer) Start() {
	go s.run()
	go s.heartbeat()
}

// Stop shuts down the WebSocket server.
func (s *WSServer) Stop() {
	close(s.stop)
}

// ServeHTTP handles WebSocket upgrade requests.
func (s *WSServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	runtime := s.state.state.SnapshotRuntime()
	if r.URL.Path != "/ws" {
		http.NotFound(w, r)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		if s.state.state.Logger != nil {
			s.state.state.Logger.DebugL(runtime.ServerLang, "log-ws-upgrade-failed", map[string]string{"err": fmt.Sprintf("%v", err), "remote": r.RemoteAddr})
		}
		return
	}
	client := &WSClient{
		conn:     conn,
		clientIP: s.state.clientIPFromRequest(r),
		isAlive:  true,
		send:     make(chan []byte, 64),
		server:   s,
	}
	if s.state.state.Logger != nil {
		s.state.state.Logger.DebugL(runtime.ServerLang, "log-ws-connected", map[string]string{"remote": conn.RemoteAddr().String()})
	}
	s.register <- client

	go client.writePump()
	go client.readPump()
}

// BroadcastRoomUpdate sends a room update to all subscribers.
func (s *WSServer) BroadcastRoomUpdate(roomID roomid.RoomID, data any) {
	select {
	case s.broadcast <- wsBroadcastMsg{roomID: roomID, msgType: "room_update", data: data}:
	default:
	}
	s.BroadcastAdminUpdate()
}

// BroadcastAdminUpdate sends a deduplicated admin room snapshot to admin subscribers.
func (s *WSServer) BroadcastAdminUpdate() {
	select {
	case s.broadcast <- wsBroadcastMsg{msgType: "admin_update"}:
	default:
	}
}

// BroadcastRoomLog sends a room log message to all subscribers.
func (s *WSServer) BroadcastRoomLog(roomID roomid.RoomID, message string, timestamp int64) {
	select {
	case s.broadcast <- wsBroadcastMsg{roomID: roomID, msgType: "room_log", data: map[string]any{"message": message, "timestamp": timestamp}}:
	default:
	}
}

func (s *WSServer) run() {
	runtime := s.state.state.SnapshotRuntime()
	for {
		select {
		case client := <-s.register:
			s.mu.Lock()
			s.clients[client] = struct{}{}
			s.mu.Unlock()
			if s.state.state.Logger != nil {
				s.state.state.Logger.DebugL(runtime.ServerLang, "log-ws-client-registered", map[string]string{"clients": fmt.Sprintf("%d", len(s.clients))})
			}

		case client := <-s.leave:
			s.mu.RLock()
			leavingRoom := client.roomID
			s.mu.RUnlock()
			if s.state.state.Logger != nil {
				s.state.state.Logger.DebugL(runtime.ServerLang, "log-ws-client-leaving", map[string]string{"room": string(leavingRoom)})
			}
			s.removeClient(client)

		case msg := <-s.broadcast:
			switch msg.msgType {
			case "room_update":
				data := msg.data
				if data == nil {
					data = buildRoomUpdateData(s.state.state, msg.roomID)
				}
				if data == nil {
					continue
				}
				payload, _ := json.Marshal(map[string]any{"type": msg.msgType, "data": data})
				sent, subs := s.broadcastToRoom(msg.roomID, payload)
				if s.state.state.Logger != nil {
					s.state.state.Logger.DebugL(runtime.ServerLang, "log-ws-broadcast", map[string]string{"room": string(msg.roomID), "type": msg.msgType, "subs": fmt.Sprintf("%d", subs), "sent": fmt.Sprintf("%d", sent)})
				}
			case "room_log":
				payload, _ := json.Marshal(map[string]any{"type": msg.msgType, "data": msg.data})
				sent, subs := s.broadcastToRoom(msg.roomID, payload)
				if s.state.state.Logger != nil {
					s.state.state.Logger.DebugL(runtime.ServerLang, "log-ws-broadcast", map[string]string{"room": string(msg.roomID), "type": msg.msgType, "subs": fmt.Sprintf("%d", subs), "sent": fmt.Sprintf("%d", sent)})
				}
			case "admin_update":
				s.broadcastAdminSnapshot(false)
			}

		case <-s.stop:
			s.mu.Lock()
			for client := range s.clients {
				client.close()
			}
			s.clients = make(map[*WSClient]struct{})
			s.rooms = make(map[roomid.RoomID]map[*WSClient]struct{})
			s.admins = make(map[*WSClient]struct{})
			s.mu.Unlock()
			return
		}
	}
}

func (s *WSServer) broadcastToRoom(roomID roomid.RoomID, payload []byte) (sent int, subs int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	subscribers := s.rooms[roomID]
	subs = len(subscribers)
	for client := range subscribers {
		if client.enqueue(payload) {
			sent++
		}
	}
	return sent, subs
}

func (s *WSServer) broadcastAdminSnapshot(force bool) {
	rooms := buildAdminRoomsData(s.state.state)
	snapshotBytes, _ := json.Marshal(rooms)
	snapshot := string(snapshotBytes)

	s.mu.RLock()
	clients := make([]*WSClient, 0, len(s.admins))
	for client := range s.admins {
		clients = append(clients, client)
	}
	s.mu.RUnlock()

	for _, client := range clients {
		s.sendAdminUpdateToClient(client, rooms, snapshot, force)
	}
}

func (s *WSServer) sendAdminUpdateToClient(client *WSClient, rooms []adminRoomData, snapshot string, force bool) {
	client.stateMu.Lock()
	if client.closed || !client.isAdmin || (!force && client.lastAdminSnapshot == snapshot) {
		client.stateMu.Unlock()
		return
	}
	client.lastAdminSnapshot = snapshot
	client.stateMu.Unlock()

	payload, _ := json.Marshal(map[string]any{
		"type": "admin_update",
		"data": map[string]any{
			"timestamp": time.Now().UnixMilli(),
			"changes": map[string]any{
				"rooms":       rooms,
				"total_rooms": len(rooms),
			},
		},
	})
	_ = client.enqueue(payload)
}

func (s *WSServer) removeClient(client *WSClient) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.clients[client]; !ok {
		return
	}
	delete(s.clients, client)
	if client.isAdminUser() {
		delete(s.admins, client)
	}
	if client.roomID != "" {
		if subs := s.rooms[client.roomID]; subs != nil {
			delete(subs, client)
			if len(subs) == 0 {
				delete(s.rooms, client.roomID)
			}
		}
	}
	client.close()
}

func (c *WSClient) writeMessage(messageType int, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.conn == nil {
		return nil
	}
	return c.conn.WriteMessage(messageType, data)
}

func (c *WSClient) setAlive(v bool) {
	c.stateMu.Lock()
	c.isAlive = v
	c.stateMu.Unlock()
}

// consumeAliveForHeartbeat resets alive flag and reports whether client should be kept.
func (c *WSClient) consumeAliveForHeartbeat() bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.closed {
		return false
	}
	if !c.isAlive {
		return false
	}
	c.isAlive = false
	return true
}

func (c *WSClient) setAdmin(v bool) {
	c.stateMu.Lock()
	c.isAdmin = v
	if !v {
		c.lastAdminSnapshot = ""
	}
	c.stateMu.Unlock()
}

func (c *WSClient) isAdminUser() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.isAdmin
}

func (c *WSClient) enqueue(data []byte) (ok bool) {
	c.stateMu.RLock()
	closed := c.closed
	c.stateMu.RUnlock()
	if closed {
		return false
	}
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	select {
	case c.send <- data:
		return true
	default:
		return false
	}
}

func (c *WSClient) close() {
	c.stateMu.Lock()
	if c.closed {
		c.stateMu.Unlock()
		return
	}
	c.closed = true
	c.stateMu.Unlock()

	close(c.send)
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

func (s *WSServer) heartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			var toRemove []*WSClient
			s.mu.Lock()
			for client := range s.clients {
				if !client.consumeAliveForHeartbeat() {
					toRemove = append(toRemove, client)
					continue
				}
				_ = client.writeMessage(websocket.PingMessage, nil)
			}
			s.mu.Unlock()
			for _, client := range toRemove {
				s.removeClient(client)
			}
		case <-s.stop:
			return
		}
	}
}

func (c *WSClient) readPump() {
	runtime := c.server.state.state.SnapshotRuntime()
	defer func() {
		select {
		case c.server.leave <- c:
		case <-c.server.stop:
		}
	}()

	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.setAlive(true)
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				if c.server.state.state.Logger != nil {
					c.server.state.state.Logger.DebugL(runtime.ServerLang, "log-ws-unexpected-close", map[string]string{"err": fmt.Sprintf("%v", err)})
				}
			}
			break
		}

		var msg struct {
			Type   string `json:"type"`
			RoomID string `json:"roomId"`
			UserID int32  `json:"userId"`
			Token  string `json:"token"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			c.sendJSON(map[string]any{"type": "error", "message": "invalid-message"})
			continue
		}

		switch msg.Type {
		case "ping":
			c.setAlive(true)
			c.sendJSON(map[string]any{"type": "pong"})

		case "subscribe":
			if c.server.state.state.Logger != nil {
				c.server.state.state.Logger.DebugL(runtime.ServerLang, "log-ws-subscribe", map[string]string{"room": msg.RoomID, "user": fmt.Sprintf("%d", msg.UserID)})
			}
			rid, err := roomid.Parse(msg.RoomID)
			if err != nil {
				c.sendJSON(map[string]any{"type": "error", "message": "invalid-room-id"})
				continue
			}
			subscribed := false
			c.server.state.state.WithRLock(func() {
				_, exists := c.server.state.state.Rooms[rid]
				if !exists {
					c.sendJSON(map[string]any{"type": "error", "message": "room-not-found"})
					return
				}
				c.server.mu.Lock()
				if c.roomID != "" {
					if subs := c.server.rooms[c.roomID]; subs != nil {
						delete(subs, c)
					}
				}
				c.roomID = rid
				c.userID = msg.UserID
				if c.server.rooms[rid] == nil {
					c.server.rooms[rid] = make(map[*WSClient]struct{})
				}
				c.server.rooms[rid][c] = struct{}{}
				c.server.mu.Unlock()
				c.sendJSON(map[string]any{"type": "subscribed", "roomId": msg.RoomID})
				subscribed = true
			})
			if subscribed {
				if data := buildRoomUpdateData(c.server.state.state, rid); data != nil {
					c.sendJSON(map[string]any{"type": "room_update", "data": data})
				}
			}

		case "unsubscribe":
			c.server.mu.Lock()
			if c.roomID != "" {
				if subs := c.server.rooms[c.roomID]; subs != nil {
					delete(subs, c)
					if len(subs) == 0 {
						delete(c.server.rooms, c.roomID)
					}
				}
				c.roomID = ""
				c.userID = 0
			}
			c.server.mu.Unlock()
			c.sendJSON(map[string]any{"type": "unsubscribed"})

		case "admin_subscribe":
			if c.server.state.state.Logger != nil {
				c.server.state.state.Logger.DebugL(runtime.ServerLang, "log-ws-admin-subscribe", nil)
			}
			if !c.server.state.adminTokenStringOK(msg.Token, c.clientIP) {
				c.sendJSON(map[string]any{"type": "error", "message": "unauthorized"})
				continue
			}
			c.stateMu.Lock()
			c.isAdmin = true
			c.lastAdminSnapshot = ""
			c.stateMu.Unlock()
			c.server.mu.Lock()
			c.server.admins[c] = struct{}{}
			c.server.mu.Unlock()
			c.sendJSON(map[string]any{"type": "admin_subscribed"})
			rooms := buildAdminRoomsData(c.server.state.state)
			snapshotBytes, _ := json.Marshal(rooms)
			c.server.sendAdminUpdateToClient(c, rooms, string(snapshotBytes), true)

		case "admin_unsubscribe":
			c.setAdmin(false)
			c.server.mu.Lock()
			delete(c.server.admins, c)
			c.server.mu.Unlock()
			c.sendJSON(map[string]any{"type": "admin_unsubscribed"})
		}
	}
}

func (c *WSClient) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			_ = c.writeMessage(websocket.TextMessage, msg)

		case <-ticker.C:
			_ = c.writeMessage(websocket.PingMessage, nil)
		}
	}
}

func (c *WSClient) sendJSON(v any) {
	data, _ := json.Marshal(v)
	_ = c.enqueue(data)
}
