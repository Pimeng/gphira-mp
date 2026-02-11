package server

import (
	"fmt"
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

	httpServer     *HTTPServer
	replayRecorder *ReplayRecorder
}

// NewServer 创建新服务器
func NewServer(config ServerConfig) *Server {
	server := &Server{
		config: config,
	}

	// 创建HTTP配置
	httpConfig := HTTPConfig{
		Enabled:       config.HTTPService,
		Port:          config.HTTPPort,
		AdminToken:    config.AdminToken,
		AdminDataPath: config.AdminDataPath,
	}

	// 创建HTTP服务器
	server.httpServer = NewHTTPServer(server, httpConfig)

	// 创建回放录制器
	server.replayRecorder = NewReplayRecorder(server.httpServer)

	return server
}

// Start 启动服务器
func (s *Server) Start(address string) error {
	// 启动HTTP服务
	if err := s.httpServer.Start(); err != nil {
		return fmt.Errorf("启动HTTP服务失败: %w", err)
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	s.listener = listener

	log.Printf("服务器正在偷听 %s", address)

	for {
		conn, err := listener.Accept()
		if err != nil {
			// 检查是否是关闭导致的错误
			if opErr, ok := err.(*net.OpError); ok && !opErr.Temporary() {
				log.Printf("服务器监听已关闭")
				return nil
			}
			log.Printf("接受连接错误: %v", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

// Stop 停止服务器
func (s *Server) Stop() {
	// 停止HTTP服务
	if s.httpServer != nil {
		s.httpServer.Stop()
	}

	// 停止所有回放录制
	if s.replayRecorder != nil {
		s.replayRecorder.StopAllRecordings()
	}

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
	// 如果启用了PROXY Protocol，尝试解析真实IP
	if s.config.TCPProxyProtocol {
		info, _, err := ParseProxyProtocol(conn, nil)
		if err == nil && info != nil && info.SourceIP != nil {
			// 包装连接以使用真实IP
			conn = NewProxyConn(conn, info)
			log.Printf("[PROXY] 解析到真实IP: %s", info.SourceIP.String())
		}
	}

	// 创建Stream
	stream, err := common.NewServerStream(conn)
	if err != nil {
		log.Printf("创建流失败: %v", err)
		conn.Close()
		return
	}

	// 生成UUID
	id := uuid.New()

	// 创建Session
	session := NewSession(id, stream, s)
	s.sessions.Store(id, session)

	log.Printf("新连接来自 %s (ID: %s, 版本: %d)", conn.RemoteAddr(), id, stream.Version())

	// 启动会话
	session.Start()
}

// RemoveSession 移除会话
func (s *Server) RemoveSession(id uuid.UUID) {
	s.sessions.Delete(id)
	log.Printf("会话已移除: %s", id)
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
	log.Printf("用户已添加: %d (%s)", user.ID, user.Name)
}

// RemoveUser 移除用户
func (s *Server) RemoveUser(id int32) {
	s.users.Delete(id)
	log.Printf("用户已移除: %d", id)
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
	host := room.GetHost()
	log.Printf("玩家 %s(%d) 创建了房间 %s", host.Name, host.ID, room.ID.Value)
}

// RemoveRoom 移除房间
func (s *Server) RemoveRoom(id common.RoomId, reason string) {
	s.rooms.Delete(id)
	if reason != "" {
		log.Printf("房间已移除: %s (原因: %s)", id.Value, reason)
	} else {
		log.Printf("房间已移除: %s", id.Value)
	}
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
	log.Printf("服务器统计 - 会话数: %d, 用户数: %d, 房间数: %d",
		stats["sessions"], stats["users"], stats["rooms"])
}

// IsDebugEnabled 是否启用 DEBUG 日志
func (s *Server) IsDebugEnabled() bool {
	return s.config.IsDebugEnabled()
}

// GetHTTPServer 获取HTTP服务器
func (s *Server) GetHTTPServer() *HTTPServer {
	return s.httpServer
}

// GetReplayRecorder 获取回放录制器
func (s *Server) GetReplayRecorder() *ReplayRecorder {
	return s.replayRecorder
}

// IsRoomCreationEnabled 是否允许创建房间
func (s *Server) IsRoomCreationEnabled() bool {
	if s.httpServer != nil {
		return s.httpServer.IsRoomCreationEnabled()
	}
	return true
}

// IsUserBanned 检查用户是否被封禁
func (s *Server) IsUserBanned(userID int32) bool {
	if s.httpServer != nil && s.httpServer.adminData != nil {
		return s.httpServer.adminData.IsUserBanned(userID)
	}
	return false
}

// IsUserBannedFromRoom 检查用户是否被禁止进入房间
func (s *Server) IsUserBannedFromRoom(userID int32, roomID string) bool {
	if s.httpServer != nil && s.httpServer.adminData != nil {
		return s.httpServer.adminData.IsUserBannedFromRoom(userID, roomID)
	}
	return false
}
