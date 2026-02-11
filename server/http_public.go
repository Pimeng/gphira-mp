package server

import (
	"encoding/binary"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// RoomListResponse 房间列表响应
type RoomListResponse struct {
	Rooms []RoomInfo `json:"rooms"`
	Total int        `json:"total"`
}

// RoomInfo 房间信息
type RoomInfo struct {
	RoomID  string     `json:"roomid"`
	Cycle   bool       `json:"cycle"`
	Lock    bool       `json:"lock"`
	Host    UserBrief  `json:"host"`
	State   string     `json:"state"`
	Chart   *ChartInfo `json:"chart,omitempty"`
	Players []UserBrief `json:"players"`
}

// UserBrief 用户简要信息
type UserBrief struct {
	ID   int32  `json:"id"`
	Name string `json:"name"`
}

// ChartInfo 谱面信息
type ChartInfo struct {
	ID   int32  `json:"id"`
	Name string `json:"name"`
}

// handleRoomList 处理获取房间列表请求
func (h *HTTPServer) handleRoomList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}

	rooms := h.server.GetAllRooms()
	roomInfos := make([]RoomInfo, 0, len(rooms))

	for _, room := range rooms {
		host := room.GetHost()
		state := "select_chart"
		switch room.GetState() {
		case InternalStateWaitForReady:
			state = "waiting_for_ready"
		case InternalStatePlaying:
			state = "playing"
		}

		// 获取玩家列表
		users := room.GetUsers()
		players := make([]UserBrief, 0, len(users))
		for _, u := range users {
			players = append(players, UserBrief{
				ID:   u.ID,
				Name: u.Name,
			})
		}

		info := RoomInfo{
			RoomID:  room.ID.Value,
			Cycle:   room.IsCycle(),
			Lock:    room.IsLocked(),
			Host:    UserBrief{ID: host.ID, Name: host.Name},
			State:   state,
			Players: players,
		}

		// 添加谱面信息
		if chart := room.GetChart(); chart != nil {
			info.Chart = &ChartInfo{
				ID:   chart.ID,
				Name: chart.Name,
			}
		}

		roomInfos = append(roomInfos, info)
	}

	writeOK(w, RoomListResponse{
		Rooms: roomInfos,
		Total: len(roomInfos),
	})
}

// ==================== 回放相关接口 ====================

// ReplayAuthRequest 回放认证请求
type ReplayAuthRequest struct {
	Token string `json:"token"`
}

// ReplayAuthResponse 回放认证响应
type ReplayAuthResponse struct {
	OK           bool           `json:"ok"`
	UserID       int32          `json:"userId"`
	Charts       []ChartReplay  `json:"charts"`
	SessionToken string         `json:"sessionToken"`
	ExpiresAt    int64          `json:"expiresAt"`
}

// ChartReplay 谱面回放信息
type ChartReplay struct {
	ChartID int32          `json:"chartId"`
	Replays []ReplayInfo   `json:"replays"`
}

// ReplayInfo 回放信息
type ReplayInfo struct {
	Timestamp int64 `json:"timestamp"`
	RecordID  int32 `json:"recordId"`
}

// SessionTokenInfo session token信息
type SessionTokenInfo struct {
	UserID    int32
	Token     string
	ExpiresAt time.Time
}

// ReplaySessionManager 回放会话管理器
type ReplaySessionManager struct {
	tokens map[string]*SessionTokenInfo
}

var replaySessions = &ReplaySessionManager{
	tokens: make(map[string]*SessionTokenInfo),
}

// handleReplayAuth 处理回放认证
func (h *HTTPServer) handleReplayAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}

	var req ReplayAuthRequest
	if err := parseBody(r, &req); err != nil || req.Token == "" {
		writeError(w, http.StatusBadRequest, "bad-request")
		return
	}

	// 使用token获取用户信息
	user, _, err := UserInfoFromAPI(req.Token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// 生成session token
	sessionToken := generateUUID()
	expiresAt := time.Now().Add(SessionTokenExpire)

	replaySessions.tokens[sessionToken] = &SessionTokenInfo{
		UserID:    user.ID,
		Token:     sessionToken,
		ExpiresAt: expiresAt,
	}

	// 获取用户的回放列表
	charts := getUserReplays(user.ID)

	writeOK(w, map[string]interface{}{
		"userId":       user.ID,
		"charts":       charts,
		"sessionToken": sessionToken,
		"expiresAt":    expiresAt.UnixMilli(),
	})
}

// getUserReplays 获取用户回放列表
func getUserReplays(userID int32) []ChartReplay {
	recordDir := filepath.Join("record", fmt.Sprintf("%d", userID))
	
	entries, err := os.ReadDir(recordDir)
	if err != nil {
		return []ChartReplay{}
	}

	chartMap := make(map[int32][]ReplayInfo)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// 解析文件名: {timestamp}.phirarec
		name := entry.Name()
		if !strings.HasSuffix(name, ".phirarec") {
			continue
		}

		// 读取文件头获取chartID
		filepath := filepath.Join(recordDir, name)
		chartID, timestamp, recordID := parseReplayFileHeader(filepath)
		if chartID == 0 {
			continue
		}

		chartMap[chartID] = append(chartMap[chartID], ReplayInfo{
			Timestamp: timestamp,
			RecordID:  recordID,
		})
	}

	// 转换为响应格式
	result := make([]ChartReplay, 0, len(chartMap))
	for chartID, replays := range chartMap {
		result = append(result, ChartReplay{
			ChartID: chartID,
			Replays: replays,
		})
	}

	return result
}

// parseReplayFileHeader 解析回放文件头
func parseReplayFileHeader(path string) (chartID int32, timestamp int64, recordID int32) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, 0
	}
	defer file.Close()

	// 读取文件头 (14字节)
	header := make([]byte, 14)
	if _, err := file.Read(header); err != nil {
		return 0, 0, 0
	}

	// 验证文件标识 (0x504D)
	if binary.LittleEndian.Uint16(header[0:2]) != 0x504D {
		return 0, 0, 0
	}

	chartID = int32(binary.LittleEndian.Uint32(header[2:6]))
	// userID在header[6:10]，不需要
	recordID = int32(binary.LittleEndian.Uint32(header[10:14]))

	// 从文件名获取时间戳
	filename := filepath.Base(path)
	filename = strings.TrimSuffix(filename, ".phirarec")
	ts, _ := strconv.ParseInt(filename, 10, 64)
	timestamp = ts

	return chartID, timestamp, recordID
}

// handleReplayDownload 处理回放下载
func (h *HTTPServer) handleReplayDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}

	// 获取参数
	sessionToken := r.URL.Query().Get("sessionToken")
	chartIDStr := r.URL.Query().Get("chartId")
	timestampStr := r.URL.Query().Get("timestamp")

	if sessionToken == "" || chartIDStr == "" || timestampStr == "" {
		writeError(w, http.StatusBadRequest, "bad-request")
		return
	}

	// 验证session token
	session, ok := replaySessions.tokens[sessionToken]
	if !ok || time.Now().After(session.ExpiresAt) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	chartID, err := strconv.ParseInt(chartIDStr, 10, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad-request")
		return
	}

	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad-request")
		return
	}

	// 构建文件路径
	filepath := filepath.Join("record", fmt.Sprintf("%d", session.UserID), 
		fmt.Sprintf("%d", chartID), fmt.Sprintf("%d.phirarec", timestamp))

	// 检查文件是否存在
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "not-found")
		return
	}

	// 打开文件
	file, err := os.Open(filepath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal-error")
		return
	}
	defer file.Close()

	// 获取文件信息
	stat, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal-error")
		return
	}

	// 设置响应头
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%d.phirarec\"", timestamp))

	// 限速50KB/s传输
	const rateLimit = 50 * 1024 // 50KB/s
	buffer := make([]byte, rateLimit)
	for {
		n, err := file.Read(buffer)
		if n > 0 {
			w.Write(buffer[:n])
			// 限速：每秒传输50KB
			time.Sleep(1 * time.Second)
		}
		if err != nil {
			break
		}
	}
}

// ReplayDeleteRequest 删除回放请求
type ReplayDeleteRequest struct {
	SessionToken string `json:"sessionToken"`
	ChartID      int32  `json:"chartId"`
	Timestamp    int64  `json:"timestamp"`
}

// handleReplayDelete 处理删除回放
func (h *HTTPServer) handleReplayDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}

	var req ReplayDeleteRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad-request")
		return
	}

	// 验证session token
	session, ok := replaySessions.tokens[req.SessionToken]
	if !ok || time.Now().After(session.ExpiresAt) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// 构建文件路径
	filepath := filepath.Join("record", fmt.Sprintf("%d", session.UserID),
		fmt.Sprintf("%d", req.ChartID), fmt.Sprintf("%d.phirarec", req.Timestamp))

	// 检查文件是否存在
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "not-found")
		return
	}

	// 删除文件
	if err := os.Remove(filepath); err != nil {
		writeError(w, http.StatusInternalServerError, "internal-error")
		return
	}

	writeOK(w, nil)
}

// ==================== OTP接口 ====================

// OTPRequestResponse OTP请求响应
type OTPRequestResponse struct {
	OK        bool   `json:"ok"`
	SSID      string `json:"ssid"`
	ExpiresIn int64  `json:"expiresIn"`
}

// handleOTPRequest 处理OTP请求
func (h *HTTPServer) handleOTPRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}

	// 如果配置了永久token，OTP不可用
	if h.config.AdminToken != "" {
		writeError(w, http.StatusForbidden, "otp-disabled-when-token-configured")
		return
	}

	clientIP := getClientIP(r)
	otpInfo := h.otpManager.GenerateOTP(clientIP)

	writeOK(w, map[string]interface{}{
		"ssid":      otpInfo.SSID,
		"expiresIn": OTPExpireTime.Milliseconds(),
	})
}

// OTPVerifyRequest OTP验证请求
type OTPVerifyRequest struct {
	SSID string `json:"ssid"`
	OTP  string `json:"otp"`
}

// OTPVerifyResponse OTP验证响应
type OTPVerifyResponse struct {
	OK        bool   `json:"ok"`
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expiresAt"`
	ExpiresIn int64  `json:"expiresIn"`
}

// handleOTPVerify 处理OTP验证
func (h *HTTPServer) handleOTPVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}

	// 如果配置了永久token，OTP不可用
	if h.config.AdminToken != "" {
		writeError(w, http.StatusForbidden, "otp-disabled-when-token-configured")
		return
	}

	var req OTPVerifyRequest
	if err := parseBody(r, &req); err != nil || req.SSID == "" || req.OTP == "" {
		writeError(w, http.StatusBadRequest, "bad-request")
		return
	}

	clientIP := getClientIP(r)
	tempToken, ok := h.otpManager.ValidateOTP(req.SSID, req.OTP, clientIP)
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid-or-expired-otp")
		return
	}

	writeOK(w, map[string]interface{}{
		"token":     tempToken.Token,
		"expiresAt": tempToken.ExpiresAt.UnixMilli(),
		"expiresIn": TempTokenExpireTime.Milliseconds(),
	})
}
