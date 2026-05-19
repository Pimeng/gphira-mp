package network

import (
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/config"
	"github.com/Pimeng/gphira-mp-next/internal/replay"
	"github.com/Pimeng/gphira-mp-next/internal/state"
	"github.com/Pimeng/gphira-mp-next/internal/utils"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
	"github.com/Pimeng/gphira-mp-next/pkg/stream"
)

// Server manages the TCP listener and all active sessions.
type Server struct {
	listener      net.Listener
	state         *state.ServerState
	logger        *utils.Logger
	cfg           *config.ServerConfig
	rateLimiter   *RateLimiter
	httpServer    *HTTPServer
	replayCleanup *replay.CleanupHandle

	mu       sync.RWMutex
	sessions map[string]*Session
	closed   bool
	watcher  *config.Watcher
}

// StartServer creates and starts a new TCP server.
func StartServer(cfg *config.ServerConfig, logger *utils.Logger, configPath string) (*Server, error) {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	s := &Server{
		listener:    listener,
		state:       state.NewServerState(cfg, logger, cfg.ServerName, cfg.AdminDataPath),
		logger:      logger,
		cfg:         cfg,
		rateLimiter: NewRateLimiter(time.Minute, 60),
		sessions:    make(map[string]*Session),
	}
	s.state.ConfigPath = configPath

	if config.DerefBool(cfg.ReplayEnabled, false) {
		s.state.ReplayRecorder = replay.NewRecorder(cfg.ReplayBaseDir, logger)
		s.replayCleanup = replay.StartReplayCleanup(cfg.ReplayBaseDir, 4)
		s.state.AutoUploadCallback = replay.CreateAutoUploadHandler(cfg, logger, s.state.UploadedReplayMeta, s.state.AutoUploadConfigs)
	}

	if err := s.state.LoadAdminData(); err != nil {
		logger.WarnL(s.state.SnapshotRuntime().ServerLang, "log-admin-data-load-failed", map[string]string{"err": err.Error()})
	}

	go s.acceptLoop()
	go s.heartbeatLoop()

	if config.DerefBool(cfg.HTTPService, false) {
		httpAddr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.HTTPPort))
		httpServer, err := StartHTTPServer(httpAddr, s.state, logger)
		if err != nil {
			logger.WarnL(s.state.SnapshotRuntime().ServerLang, "log-http-start-failed", map[string]string{"err": err.Error()})
		} else {
			s.httpServer = httpServer
		}
	}

	runtime := s.state.SnapshotRuntime()
	logger.Mark(runtime.ServerLang.Format("log-server-listening", map[string]string{"addr": addr}))
	logger.Mark(runtime.ServerLang.Format("log-server-info", map[string]string{
		"name":  runtime.ServerName,
		"level": strings.ToUpper(logger.GetLevel()),
	}))

	// Start config watcher if config file exists
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			s.watcher = config.NewWatcher(configPath, 5*time.Second, func() {
				loaded, err := config.LoadConfig(configPath)
				if err != nil {
					logger.Warn("config reload failed", "err", err)
					return
				}
				envCfg := config.LoadEnvConfig()
				newCfg := config.MergeConfig(config.MergeConfig(config.DefaultConfig(), loaded), envCfg)
				s.state.ApplyConfig(newCfg)
				logger.MarkL(s.state.SnapshotRuntime().ServerLang, "log-config-reloaded", nil)
			})
			s.watcher.Start()
		}
	}

	return s, nil
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.isClosed() {
				return
			}
			s.logger.WarnL(s.state.SnapshotRuntime().ServerLang, "log-accept-failed", map[string]string{"err": err.Error()})
			continue
		}
		if !s.rateLimiter.Allow(conn.RemoteAddr().String()) {
			s.logger.WarnL(s.state.SnapshotRuntime().ServerLang, "log-rate-limit-exceeded", map[string]string{"remote": conn.RemoteAddr().String()}, &utils.LogContext{IP: conn.RemoteAddr().String(), IsConnectionLog: true})
			conn.Close()
			continue
		}
		s.logger.DebugL(s.state.SnapshotRuntime().ServerLang, "log-connection-accepted", map[string]string{"remote": conn.RemoteAddr().String()})
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	id := newUUID()
	remoteIP := conn.RemoteAddr().String()
	runtime := s.state.SnapshotRuntime()

	// HAProxy PROXY Protocol
	if config.DerefBool(runtime.Config.HAProxyProtocol, false) {
		info, wrappedConn, err := ParseProxyProtocol(conn, 5*time.Second)
		if err != nil {
			s.logger.WarnL(runtime.ServerLang, "log-proxy-protocol-failed", map[string]string{"err": err.Error()}, &utils.LogContext{IP: remoteIP, IsConnectionLog: true})
			conn.Close()
			return
		}
		if info != nil {
			remoteIP = fmt.Sprintf("%s:%d", info.SourceAddress, info.SourcePort)
			s.logger.DebugL(runtime.ServerLang, "log-proxy-protocol-ok", map[string]string{"source": remoteIP}, &utils.LogContext{IP: remoteIP, IsConnectionLog: true})
		}
		conn = wrappedConn
	}

	s.logger.DebugL(runtime.ServerLang, "log-new-connection", map[string]string{"id": id, "remote": remoteIP}, &utils.LogContext{IP: remoteIP, IsConnectionLog: true})

	sess := NewSession(id, conn, s.state, remoteIP)

	codec := stream.Codec[protocol.ServerCommand, protocol.ClientCommand]{
		EncodeSend: func(cmd protocol.ServerCommand) []byte {
			w := protocol.NewBinaryWriter()
			protocol.EncodeServerCommand(w, cmd)
			return w.Bytes()
		},
		DecodeRecv: func(data []byte) (protocol.ClientCommand, error) {
			r := protocol.NewBinaryReader(data)
			return protocol.DecodeClientCommand(r), nil
		},
		IsHighPriority: func(cmd protocol.ServerCommand) bool {
			switch cmd.Type {
			case protocol.ServerCmdPong, protocol.ServerCmdAuthenticate,
				protocol.ServerCmdChangeState, protocol.ServerCmdChangeHost,
				protocol.ServerCmdOnJoinRoom:
				return true
			}
			return false
		},
	}

	strm, err := stream.New(conn, codec, sess.OnCommand, func(cmd protocol.ClientCommand) bool {
		return cmd.Type == protocol.ClientCmdPing
	}, func(phase string, err error) {
		s.logger.WarnL(s.state.SnapshotRuntime().ServerLang, "log-stream-error", map[string]string{"id": id, "phase": phase, "err": fmt.Sprintf("%v", err)}, &utils.LogContext{IP: remoteIP, IsConnectionLog: true})
	})
	if err != nil {
		s.logger.WarnL(s.state.SnapshotRuntime().ServerLang, "log-handshake-failed", map[string]string{"id": id, "err": err.Error()})
		conn.Close()
		return
	}

	sess.BindStream(strm)

	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	s.state.WithLock(func() {
		s.state.Sessions[id] = sess
	})

	s.logger.DebugL(s.state.SnapshotRuntime().ServerLang, "log-handshake-ok", map[string]string{"id": id})
}

func (s *Server) heartbeatLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			s.mu.Lock()
			for id, sess := range s.sessions {
				if sess.IsLost() {
					delete(s.sessions, id)
					continue
				}
				sess.CheckHeartbeat(now)
			}
			s.mu.Unlock()
		}
		if s.isClosed() {
			return
		}
	}
}

func (s *Server) isClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

// State returns the server state.
func (s *Server) State() *state.ServerState {
	return s.state
}

// BroadcastAll sends a command to all connected sessions.
func (s *Server) BroadcastAll(cmd protocol.ServerCommand) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sess := range s.sessions {
		_ = sess.Send(cmd)
	}
}

// Close gracefully shuts down the server.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	for _, sess := range s.sessions {
		sess.MarkLost()
	}
	s.mu.Unlock()
	if s.httpServer != nil {
		if err := s.httpServer.Close(); err != nil {
			s.logger.WarnL(s.state.SnapshotRuntime().ServerLang, "log-http-close-error", map[string]string{"err": err.Error()})
		}
	}
	if s.watcher != nil {
		s.watcher.Stop()
	}
	if s.replayCleanup != nil {
		s.replayCleanup.Stop()
	}
	if s.state.ReplayRecorder != nil {
		s.state.ReplayRecorder.CloseAll()
	}
	return s.listener.Close()
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	// variant and version
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
