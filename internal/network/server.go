package network

import (
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/config"
	"github.com/Pimeng/gphira-mp-next/internal/replay"
	"github.com/Pimeng/gphira-mp-next/internal/state"
	"github.com/Pimeng/gphira-mp-next/internal/utils"
	"github.com/Pimeng/gphira-mp-next/internal/version"
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
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
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

	if cfg.ReplayEnabled {
		s.state.ReplayRecorder = replay.NewRecorder(cfg.ReplayBaseDir, logger)
		s.replayCleanup = replay.StartReplayCleanup(cfg.ReplayBaseDir, 4)
		s.state.AutoUploadCallback = replay.CreateAutoUploadHandler(cfg, logger, s.state.UploadedReplayMeta, s.state.AutoUploadConfigs)
	}

	if err := s.state.LoadAdminData(); err != nil {
		logger.Warn("failed to load admin data", "err", err)
	}

	go s.acceptLoop()
	go s.heartbeatLoop()

	if cfg.HTTPService {
		httpAddr := fmt.Sprintf("%s:%d", cfg.Host, cfg.HTTPPort)
		httpServer, err := StartHTTPServer(httpAddr, s.state, logger)
		if err != nil {
			logger.Warn("failed to start http server", "err", err)
		} else {
			s.httpServer = httpServer
		}
	}

	ver := version.ReadVersion()
	logger.Mark("server version", "version", ver)
	logger.Mark("runtime env", "go", runtime.Version(), "os", runtime.GOOS, "arch", runtime.GOARCH)
	logger.Mark("server started", "addr", addr, "name", s.state.ServerName)

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
				logger.Mark("config reloaded")
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
			s.logger.Warn("accept failed", "err", err)
			continue
		}
		if !s.rateLimiter.Allow(conn.RemoteAddr().String()) {
			s.logger.Warn("rate limit exceeded", "remote", conn.RemoteAddr().String(), &utils.LogContext{IP: conn.RemoteAddr().String(), IsConnectionLog: true})
			conn.Close()
			continue
		}
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	id := newUUID()
	remoteIP := conn.RemoteAddr().String()

	// HAProxy PROXY Protocol
	if s.cfg.HAProxyProtocol {
		info, wrappedConn, err := ParseProxyProtocol(conn, 5*time.Second)
		if err != nil {
			s.logger.Warn("proxy protocol parse failed", "err", err, &utils.LogContext{IP: remoteIP, IsConnectionLog: true})
			conn.Close()
			return
		}
		if info != nil {
			remoteIP = fmt.Sprintf("%s:%d", info.SourceAddress, info.SourcePort)
			s.logger.Debug("proxy protocol ok", "source", remoteIP, &utils.LogContext{IP: remoteIP, IsConnectionLog: true})
		}
		conn = wrappedConn
	}

	s.logger.Debug("new connection", "id", id, "remote", remoteIP, &utils.LogContext{IP: remoteIP, IsConnectionLog: true})

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
		s.logger.Warn("stream error", "id", id, "phase", phase, "err", err, &utils.LogContext{IP: remoteIP, IsConnectionLog: true})
	})
	if err != nil {
		s.logger.Warn("handshake failed", "id", id, "err", err)
		conn.Close()
		return
	}

	sess.BindStream(strm)

	s.mu.Lock()
	s.sessions[id] = sess
	s.state.Sessions[id] = sess
	s.mu.Unlock()

	s.logger.Debug("handshake ok", "id", id)
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
	s.closed = true
	for _, sess := range s.sessions {
		sess.MarkLost()
	}
	s.mu.Unlock()
	if s.httpServer != nil {
		if err := s.httpServer.Close(); err != nil {
			s.logger.Warn("http server close error", "err", err)
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
