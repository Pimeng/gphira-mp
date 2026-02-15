package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	DefaultHTTPPort     = 12347
	OTPExpireTime       = 5 * time.Minute
	TempTokenExpireTime = 4 * time.Hour
	SessionTokenExpire  = 30 * time.Minute
)

// HTTPServer HTTP服务
type HTTPServer struct {
	server     *Server
	config     HTTPConfig
	adminData  *AdminData
	otpManager *OTPManager
	httpServer *http.Server
	mu         sync.RWMutex

	// 运行时配置
	replayEnabled       bool
	roomCreationEnabled bool

	// 真实IP头配置
	realIPHeader string

	// 认证限流器
	authLimiter *AuthLimiter
}

// HTTPConfig HTTP配置
type HTTPConfig struct {
	Enabled       bool   `yaml:"http_service"`
	Port          int    `yaml:"http_port"`
	AdminToken    string `yaml:"admin_token"`
	AdminDataPath string `yaml:"admin_data_path"`
}

// DefaultHTTPConfig 默认HTTP配置
func DefaultHTTPConfig() HTTPConfig {
	return HTTPConfig{
		Enabled:       false,
		Port:          DefaultHTTPPort,
		AdminToken:    "",
		AdminDataPath: "admin_data.json",
	}
}

// NewHTTPServer 创建HTTP服务器
func NewHTTPServer(server *Server, config HTTPConfig) *HTTPServer {
	httpServer := &HTTPServer{
		server:              server,
		config:              config,
		adminData:           NewAdminData(),
		otpManager:          NewOTPManager(),
		replayEnabled:       false,
		roomCreationEnabled: true,
		realIPHeader:        server.config.RealIPHeader,
		authLimiter:         NewAuthLimiter(),
	}

	// 加载管理员数据
	httpServer.loadAdminData()

	return httpServer
}

// Start 启动HTTP服务
func (h *HTTPServer) Start() error {
	if !h.config.Enabled {
		return nil
	}

	mux := http.NewServeMux()

	// WebSocket接口
	mux.HandleFunc("/ws", h.HandleWebSocket)

	// 公共接口
	mux.HandleFunc("/room", h.handleRoomList)

	// 回放接口
	mux.HandleFunc("/replay/auth", h.handleReplayAuth)
	mux.HandleFunc("/replay/download", h.handleReplayDownload)
	mux.HandleFunc("/replay/delete", h.handleReplayDelete)

	// OTP接口（仅在未配置永久token时可用）
	mux.HandleFunc("/admin/otp/request", h.handleOTPRequest)
	mux.HandleFunc("/admin/otp/verify", h.handleOTPVerify)

	// 管理员接口
	mux.HandleFunc("/admin/rooms", h.withAdminAuth(h.handleAdminRooms))
	mux.HandleFunc("/admin/rooms/", h.withAdminAuth(h.handleAdminRoomDetail))
	mux.HandleFunc("/admin/users/", h.withAdminAuth(h.handleAdminUserOperations))
	mux.HandleFunc("/admin/ban/user", h.withAdminAuth(h.handleAdminBanUser))
	mux.HandleFunc("/admin/ban/room", h.withAdminAuth(h.handleAdminBanRoom))
	mux.HandleFunc("/admin/broadcast", h.withAdminAuth(h.handleAdminBroadcast))
	mux.HandleFunc("/admin/replay/config", h.withAdminAuth(h.handleAdminReplayConfig))
	mux.HandleFunc("/admin/room-creation/config", h.withAdminAuth(h.handleAdminRoomCreationConfig))

	// 比赛房间接口
	mux.HandleFunc("/admin/contest/rooms/", h.withAdminAuth(h.handleAdminContest))

	h.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", h.config.Port),
		Handler: withCORS(mux),
	}

	log.Printf("HTTP服务正在偷听 %d", h.config.Port)
	go func() {
		if err := h.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP服务错误: %v", err)
		}
	}()

	return nil
}

// Stop 停止HTTP服务
func (h *HTTPServer) Stop() error {
	// 停止认证限流器
	if h.authLimiter != nil {
		h.authLimiter.Stop()
	}

	if h.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return h.httpServer.Shutdown(ctx)
	}
	return nil
}

// IsReplayEnabled 是否启用回放录制
func (h *HTTPServer) IsReplayEnabled() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.replayEnabled
}

// SetReplayEnabled 设置回放录制开关
func (h *HTTPServer) SetReplayEnabled(enabled bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.replayEnabled = enabled
}

// IsRoomCreationEnabled 是否允许创建房间
func (h *HTTPServer) IsRoomCreationEnabled() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.roomCreationEnabled
}

// SetRoomCreationEnabled 设置房间创建开关
func (h *HTTPServer) SetRoomCreationEnabled(enabled bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.roomCreationEnabled = enabled
}

// 获取admin_data.json路径
func (h *HTTPServer) getAdminDataPath() string {
	if h.config.AdminDataPath != "" {
		return h.config.AdminDataPath
	}

	// 优先使用PHIRA_MP_HOME环境变量
	if home := os.Getenv("PHIRA_MP_HOME"); home != "" {
		return filepath.Join(home, "admin_data.json")
	}

	// 使用工作目录
	return "admin_data.json"
}

// 加载管理员数据
func (h *HTTPServer) loadAdminData() {
	path := h.getAdminDataPath()
	if err := h.adminData.Load(path); err != nil {
		log.Printf("加载管理员数据失败: %v", err)
	}
}

// 保存管理员数据
func (h *HTTPServer) saveAdminData() {
	path := h.getAdminDataPath()
	if err := h.adminData.Save(path); err != nil {
		log.Printf("保存管理员数据失败: %v", err)
	}
}

// JSON响应辅助函数
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// 成功响应
func writeOK(w http.ResponseWriter, data interface{}) {
	response := map[string]interface{}{
		"ok": true,
	}
	if data != nil {
		switch v := data.(type) {
		case map[string]interface{}:
			for key, val := range v {
				response[key] = val
			}
		default:
			response["data"] = data
		}
	}
	writeJSON(w, http.StatusOK, response)
}

// 错误响应
func writeError(w http.ResponseWriter, status int, errorCode string) {
	writeJSON(w, status, map[string]interface{}{
		"ok":    false,
		"error": errorCode,
	})
}

// 从请求中获取token
func extractToken(r *http.Request) string {
	// 1. 检查Header X-Admin-Token
	if token := r.Header.Get("X-Admin-Token"); token != "" {
		return token
	}

	// 2. 检查Header Authorization: Bearer xxx
	if auth := r.Header.Get("Authorization"); auth != "" {
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return parts[1]
		}
	}

	// 3. 检查Query参数
	if token := r.URL.Query().Get("token"); token != "" {
		return token
	}

	return ""
}

// 管理员认证中间件
func (h *HTTPServer) withAdminAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientIP := h.getClientIP(r)

		// 检查IP是否被封禁
		if h.authLimiter.IsBlocked(clientIP) {
			remaining := h.authLimiter.GetBlockTimeRemaining(clientIP)
			writeError(w, http.StatusTooManyRequests, "too-many-requests")
			log.Printf("[安全] IP %s 因多次认证失败被封禁，剩余时间: %v", clientIP, remaining)
			return
		}

		// 检查是否允许尝试
		if !h.authLimiter.AllowAttempt(clientIP) {
			remaining := h.authLimiter.GetBlockTimeRemaining(clientIP)
			writeError(w, http.StatusTooManyRequests, "too-many-requests")
			log.Printf("[安全] IP %s 触发认证限流，封禁 %v", clientIP, remaining)
			return
		}

		// 检查是否配置了永久token
		if h.config.AdminToken != "" {
			token := extractToken(r)
			if token != h.config.AdminToken {
				remaining := h.authLimiter.GetRemainingAttempts(clientIP)
				writeError(w, http.StatusUnauthorized, "unauthorized")
				log.Printf("[安全] IP %s 管理员认证失败，剩余尝试次数: %d", clientIP, remaining)
				return
			}
			// 认证成功，清除失败记录
			h.authLimiter.RecordSuccess(clientIP)
			handler(w, r)
			return
		}

		// 未配置永久token，检查临时token
		token := extractToken(r)
		if token == "" {
			remaining := h.authLimiter.GetRemainingAttempts(clientIP)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			log.Printf("[安全] IP %s 未提供token，剩余尝试次数: %d", clientIP, remaining)
			return
		}

		// 验证临时token
		if !h.otpManager.ValidateTempToken(token, clientIP) {
			remaining := h.authLimiter.GetRemainingAttempts(clientIP)
			writeError(w, http.StatusUnauthorized, "token-expired")
			log.Printf("[安全] IP %s 临时token验证失败，剩余尝试次数: %d", clientIP, remaining)
			return
		}

		// 认证成功，清除失败记录
		h.authLimiter.RecordSuccess(clientIP)
		handler(w, r)
	}
}

// getClientIP 获取客户端IP（HTTP服务使用配置的头）
func (h *HTTPServer) getClientIP(r *http.Request) string {
	// 如果配置了特定的真实IP头，优先使用
	if h.realIPHeader != "" {
		if ip := r.Header.Get(h.realIPHeader); ip != "" {
			// 处理可能包含多个IP的情况（如X-Forwarded-For）
			ips := strings.Split(ip, ",")
			if len(ips) > 0 {
				return strings.TrimSpace(ips[0])
			}
		}
	}

	// 默认使用标准方法
	return getClientIPFromRequest(r)
}

// getClientIPFromRequest 从请求中获取客户端IP（标准方法）
func getClientIPFromRequest(r *http.Request) string {
	// 检查X-Forwarded-For
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// 检查X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// 使用RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// 解析请求体
func parseBody(r *http.Request, v interface{}) error {
	if r.Body == nil {
		return fmt.Errorf("empty body")
	}
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// 解析房间ID（从URL路径）
func parseRoomIDFromPath(path string, prefix string) (string, bool) {
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}

	remaining := strings.TrimPrefix(path, prefix)
	parts := strings.SplitN(remaining, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "", false
	}

	return parts[0], true
}

// 解析用户ID（从URL路径）
func parseUserIDFromPath(path string, prefix string) (int32, bool) {
	if !strings.HasPrefix(path, prefix) {
		return 0, false
	}

	remaining := strings.TrimPrefix(path, prefix)
	parts := strings.SplitN(remaining, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		return 0, false
	}

	id, err := strconv.ParseInt(parts[0], 10, 32)
	if err != nil {
		return 0, false
	}

	return int32(id), true
}

// 验证房间ID
func isValidRoomID(roomID string) bool {
	if roomID == "" {
		return false
	}
	for _, c := range roomID {
		if c != '-' && c != '_' && !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// 生成UUID
func generateUUID() string {
	return uuid.New().String()
}

// withCORS CORS中间件 - 允许所有来源
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 设置CORS响应头
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Admin-Token")
		w.Header().Set("Access-Control-Max-Age", "86400")

		// 处理预检请求
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
