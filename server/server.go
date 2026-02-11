package server

import (
	"log"
	"net"
	"sync"

	"phira-mp/common"

	"github.com/google/uuid"
)

// Server 服务器
type Server struct {
	config ServerConfig

	sessions sync.Map // map[uuid.UUID]*Session
	users    sync.Map // map[int32]*User
	rooms    sync.Map // map[common.RoomId]*Room

	listener net.Listener
}

// NewServer 创建新服务器
func NewServer(config ServerConfig) *Server {
	return &Server{
		config: config,
	}
}

// Start 启动服务器
func (s *Server) Start(address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	s.listener = listener

	log.Printf("Server started on %s", address)

	for {
		conn, err := listener.Accept()
		if err != nil {
			// 检查是否是关闭导致的错误
			if opErr, ok := err.(*net.OpError); ok && !opErr.Temporary() {
				log.Printf("Server listener closed")
				return nil
			}
			log.Printf("Accept error: %v", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

// Stop 停止服务器
func (s *Server) Stop() {
	if s.listener != nil {
		s.listener.Close()
	}

	// 关闭所有会话
	s.sessions.Range(func(key, value interface{}) bool {
		if session, ok := value.(*Session); ok {
			session.Stop()
		}
		return true
	})
}

// handleConnection 处理新连接
func (s *Server) handleConnection(conn net.Conn) {
	// 创建Stream
	stream, err := common.NewServerStream(conn)
	if err != nil {
		log.Printf("Failed to create stream: %v", err)
		conn.Close()
		return
	}

	// 生成UUID
	id := uuid.New()

	// 创建Session
	session := NewSession(id, stream, s)
	s.sessions.Store(id, session)

	log.Printf("New connection from %s (ID: %s, Version: %d)", conn.RemoteAddr(), id, stream.Version())

	// 启动会话
	session.Start()
}

// RemoveSession 移除会话
func (s *Server) RemoveSession(id uuid.UUID) {
	s.sessions.Delete(id)
	log.Printf("Session removed: %s", id)
}

// GetSession 获取会话
func (s *Server) GetSession(id uuid.UUID) *Session {
	if val, ok := s.sessions.Load(id); ok {
		return val.(*Session)
	}
	return nil
}

// AddUser 添加用户
func (s *Server) AddUser(user *User) {
	s.users.Store(user.ID, user)
	log.Printf("User added: %d (%s)", user.ID, user.Name)
}

// RemoveUser 移除用户
func (s *Server) RemoveUser(id int32) {
	s.users.Delete(id)
	log.Printf("User removed: %d", id)
}

// GetUser 获取用户
func (s *Server) GetUser(id int32) *User {
	if val, ok := s.users.Load(id); ok {
		return val.(*User)
	}
	return nil
}

// AddRoom 添加房间
func (s *Server) AddRoom(room *Room) {
	s.rooms.Store(room.ID, room)
	log.Printf("Room created: %s", room.ID.Value)
}

// RemoveRoom 移除房间
func (s *Server) RemoveRoom(id common.RoomId) {
	s.rooms.Delete(id)
	log.Printf("Room removed: %s", id.Value)
}

// GetRoom 获取房间
func (s *Server) GetRoom(id common.RoomId) *Room {
	if val, ok := s.rooms.Load(id); ok {
		return val.(*Room)
	}
	return nil
}

// GetAllRooms 获取所有房间
func (s *Server) GetAllRooms() []*Room {
	var rooms []*Room
	s.rooms.Range(func(key, value interface{}) bool {
		if room, ok := value.(*Room); ok {
			rooms = append(rooms, room)
		}
		return true
	})
	return rooms
}

// GetStats 获取服务器统计
func (s *Server) GetStats() map[string]interface{} {
	sessionCount := 0
	s.sessions.Range(func(_, _ interface{}) bool {
		sessionCount++
		return true
	})

	userCount := 0
	s.users.Range(func(_, _ interface{}) bool {
		userCount++
		return true
	})

	roomCount := 0
	s.rooms.Range(func(_, _ interface{}) bool {
		roomCount++
		return true
	})

	return map[string]interface{}{
		"sessions": sessionCount,
		"users":    userCount,
		"rooms":    roomCount,
	}
}

// PrintStats 打印统计信息
func (s *Server) PrintStats() {
	stats := s.GetStats()
	log.Printf("Server stats - Sessions: %d, Users: %d, Rooms: %d",
		stats["sessions"], stats["users"], stats["rooms"])
}
