package network

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WSClient represents a connected WebSocket client.
type WSClient struct {
	conn     *websocket.Conn
	roomID   roomid.RoomID
	userID   int32
	isAdmin  bool
	isAlive  bool
	send     chan []byte
	server   *WSServer
}

// WSServer manages WebSocket connections and broadcasts.
type WSServer struct {
	state *HTTPServer

	mu       sync.RWMutex
	clients  map[*WSClient]struct{}
	rooms    map[roomid.RoomID]map[*WSClient]struct{}
	admins   map[*WSClient]struct{}
	register chan *WSClient
	leave    chan *WSClient
	broadcast chan wsBroadcastMsg
	stop     chan struct{}
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
	if r.URL.Path != "/ws" {
		http.NotFound(w, r)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		if s.state.state.Logger != nil {
			s.state.state.Logger.DebugL(s.state.state.ServerLang, "log-ws-upgrade-failed", map[string]string{"err": fmt.Sprintf("%v", err), "remote": r.RemoteAddr})
		}
		return
	}
	client := &WSClient{
		conn:    conn,
		isAlive: true,
		send:    make(chan []byte, 64),
		server:  s,
	}
	if s.state.state.Logger != nil {
		s.state.state.Logger.DebugL(s.state.state.ServerLang, "log-ws-connected", map[string]string{"remote": conn.RemoteAddr().String()})
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
}

// BroadcastRoomLog sends a room log message to all subscribers.
func (s *WSServer) BroadcastRoomLog(roomID roomid.RoomID, message string, timestamp int64) {
	select {
	case s.broadcast <- wsBroadcastMsg{roomID: roomID, msgType: "room_log", data: map[string]any{"message": message, "timestamp": timestamp}}:
	default:
	}
}

func (s *WSServer) run() {
	for {
		select {
		case client := <-s.register:
			s.mu.Lock()
			s.clients[client] = struct{}{}
			s.mu.Unlock()
			if s.state.state.Logger != nil {
				s.state.state.Logger.DebugL(s.state.state.ServerLang, "log-ws-client-registered", map[string]string{"clients": fmt.Sprintf("%d", len(s.clients))})
			}

		case client := <-s.leave:
			if s.state.state.Logger != nil {
				s.state.state.Logger.DebugL(s.state.state.ServerLang, "log-ws-client-leaving", map[string]string{"room": string(client.roomID)})
			}
			s.removeClient(client)

		case msg := <-s.broadcast:
			s.mu.RLock()
			subs := s.rooms[msg.roomID]
			if len(subs) == 0 && len(s.admins) == 0 {
				s.mu.RUnlock()
				continue
			}
			payload, _ := json.Marshal(map[string]any{"type": msg.msgType, "data": msg.data})
			count := 0
			for client := range subs {
				select {
				case client.send <- payload:
					count++
				default:
				}
			}
			for client := range s.admins {
				select {
				case client.send <- payload:
					count++
				default:
				}
			}
			if s.state.state.Logger != nil {
				s.state.state.Logger.DebugL(s.state.state.ServerLang, "log-ws-broadcast", map[string]string{"room": string(msg.roomID), "type": msg.msgType, "subs": fmt.Sprintf("%d", len(subs)), "sent": fmt.Sprintf("%d", count)})
			}
			s.mu.RUnlock()

		case <-s.stop:
			s.mu.Lock()
			for client := range s.clients {
				close(client.send)
				client.conn.Close()
			}
			s.clients = make(map[*WSClient]struct{})
			s.rooms = make(map[roomid.RoomID]map[*WSClient]struct{})
			s.admins = make(map[*WSClient]struct{})
			s.mu.Unlock()
			return
		}
	}
}

func (s *WSServer) removeClient(client *WSClient) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.clients[client]; !ok {
		return
	}
	delete(s.clients, client)
	if client.isAdmin {
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
	close(client.send)
	client.conn.Close()
}

func (s *WSServer) heartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.mu.Lock()
			for client := range s.clients {
				if !client.isAlive {
					client.conn.Close()
					continue
				}
				client.isAlive = false
				_ = client.conn.WriteMessage(websocket.PingMessage, nil)
			}
			s.mu.Unlock()
		case <-s.stop:
			return
		}
	}
}

func (c *WSClient) readPump() {
	defer func() {
		c.server.leave <- c
	}()

	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.isAlive = true
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				if c.server.state.state.Logger != nil {
					c.server.state.state.Logger.DebugL(c.server.state.state.ServerLang, "log-ws-unexpected-close", map[string]string{"err": fmt.Sprintf("%v", err)})
				}
			}
			break
		}

		var msg struct {
			Type      string `json:"type"`
			RoomID    string `json:"roomId"`
			UserID    int32  `json:"userId"`
			Token     string `json:"token"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			c.sendJSON(map[string]any{"type": "error", "message": "invalid-message"})
			continue
		}

		switch msg.Type {
		case "ping":
			c.isAlive = true
			c.sendJSON(map[string]any{"type": "pong"})

		case "subscribe":
			if c.server.state.state.Logger != nil {
				c.server.state.state.Logger.DebugL(c.server.state.state.ServerLang, "log-ws-subscribe", map[string]string{"room": msg.RoomID, "user": fmt.Sprintf("%d", msg.UserID)})
			}
			rid, err := roomid.Parse(msg.RoomID)
			if err != nil {
				c.sendJSON(map[string]any{"type": "error", "message": "invalid-room-id"})
				continue
			}
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
			})

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
				c.server.state.state.Logger.DebugL(c.server.state.state.ServerLang, "log-ws-admin-subscribe", nil)
			}
			token := msg.Token
			expected := c.server.state.state.Config.AdminToken
			if expected == "" || token != expected {
				c.sendJSON(map[string]any{"type": "error", "message": "unauthorized"})
				continue
			}
			c.isAdmin = true
			c.server.mu.Lock()
			c.server.admins[c] = struct{}{}
			c.server.mu.Unlock()
			c.sendJSON(map[string]any{"type": "admin_subscribed"})
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
			_ = c.conn.WriteMessage(websocket.TextMessage, msg)

		case <-ticker.C:
			_ = c.conn.WriteMessage(websocket.PingMessage, nil)
		}
	}
}

func (c *WSClient) sendJSON(v any) {
	data, _ := json.Marshal(v)
	select {
	case c.send <- data:
	default:
	}
}
