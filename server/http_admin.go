package server

import (
	"net/http"
	"strings"

	"phira-mp/common"
)

// AdminRoomInfo 管理员房间信息
type AdminRoomInfo struct {
	RoomID    string          `json:"roomid"`
	MaxUsers  int             `json:"max_users"`
	Live      bool            `json:"live"`
	Locked    bool            `json:"locked"`
	Cycle     bool            `json:"cycle"`
	Host      UserBrief       `json:"host"`
	State     interface{}     `json:"state"`
	Chart     *ChartInfo      `json:"chart,omitempty"`
	Users     []AdminUserInfo `json:"users"`
	Monitors  []AdminUserInfo `json:"monitors"`
}

// AdminRoomStateInfo 管理员房间状态信息
type AdminRoomStateInfo struct {
	Type          string  `json:"type"`
	ResultsCount  int     `json:"results_count,omitempty"`
	AbortedCount  int     `json:"aborted_count,omitempty"`
	FinishedUsers []int32 `json:"finished_users,omitempty"`
	AbortedUsers  []int32 `json:"aborted_users,omitempty"`
}

// AdminUserInfo 管理员用户信息
type AdminUserInfo struct {
	ID        int32   `json:"id"`
	Name      string  `json:"name"`
	Connected bool    `json:"connected"`
	IsHost    bool    `json:"is_host"`
	GameTime  float32 `json:"game_time"`
	Language  string  `json:"language"`
	Monitor   bool    `json:"monitor,omitempty"`
	Finished  bool    `json:"finished,omitempty"`
	Aborted   bool    `json:"aborted,omitempty"`
	RecordID  *int32  `json:"record_id,omitempty"`
}

// handleAdminRooms 处理获取所有房间详情
func (h *HTTPServer) handleAdminRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}

	rooms := h.server.GetAllRooms()
	roomInfos := make([]AdminRoomInfo, 0, len(rooms))

	for _, room := range rooms {
		roomInfo := buildAdminRoomInfo(room)
		roomInfos = append(roomInfos, roomInfo)
	}

	writeOK(w, map[string]interface{}{
		"rooms": roomInfos,
	})
}

// handleAdminRoomDetail 处理房间详情相关操作
func (h *HTTPServer) handleAdminRoomDetail(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// 解析房间ID
	roomID, ok := parseRoomIDFromPath(path, "/admin/rooms/")
	if !ok || !isValidRoomID(roomID) {
		writeError(w, http.StatusBadRequest, "bad-room-id")
		return
	}

	roomId, err := common.NewRoomId(roomID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad-room-id")
		return
	}

	room := h.server.GetRoom(roomId)
	if room == nil {
		writeError(w, http.StatusNotFound, "room-not-found")
		return
	}

	// 根据路径后缀判断操作
	switch {
	case path == "/admin/rooms/"+roomID:
		// 获取房间详情
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
			return
		}
		writeOK(w, buildAdminRoomInfo(room))

	case strings.HasSuffix(path, "/max_users"):
		// 修改最大人数
		h.handleAdminRoomMaxUsers(w, r, room)

	case strings.HasSuffix(path, "/chat"):
		// 向房间发送消息
		h.handleAdminRoomChat(w, r, room)

	case strings.HasSuffix(path, "/disband"):
		// 解散房间
		h.handleAdminRoomDisband(w, r, room)

	default:
		writeError(w, http.StatusNotFound, "not-found")
	}
}

// UpdateMaxUsersRequest 更新最大人数请求
type UpdateMaxUsersRequest struct {
	MaxUsers int `json:"maxUsers"`
}

// handleAdminRoomMaxUsers 处理修改房间最大人数
func (h *HTTPServer) handleAdminRoomMaxUsers(w http.ResponseWriter, r *http.Request, room *Room) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}

	var req UpdateMaxUsersRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad-request")
		return
	}

	// 验证范围 1-64
	if req.MaxUsers < 1 || req.MaxUsers > 64 {
		writeError(w, http.StatusBadRequest, "bad-max-users")
		return
	}

	// TODO: 实际修改房间最大人数（需要在Room结构中添加maxUsers字段）
	// 这里先返回成功
	writeOK(w, map[string]interface{}{
		"roomid":    room.ID.Value,
		"max_users": req.MaxUsers,
	})
}

// AdminRoomChatRequest 向房间发送消息请求
type AdminRoomChatRequest struct {
	Message string `json:"message"`
}

// handleAdminRoomChat 处理向房间发送消息
func (h *HTTPServer) handleAdminRoomChat(w http.ResponseWriter, r *http.Request, room *Room) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}

	var req AdminRoomChatRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad-request")
		return
	}

	// 验证消息长度 1-200
	if len(req.Message) == 0 {
		writeError(w, http.StatusBadRequest, "bad-message")
		return
	}
	if len(req.Message) > 200 {
		writeError(w, http.StatusBadRequest, "message-too-long")
		return
	}

	// 发送系统消息（user=0表示系统）
	room.SendMessage(common.Message{
		Type:    common.MsgChat,
		User:    0,
		Content: req.Message,
	})

	writeOK(w, nil)
}

// handleAdminRoomDisband 处理解散房间
func (h *HTTPServer) handleAdminRoomDisband(w http.ResponseWriter, r *http.Request, room *Room) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}

	// 发送解散通知
	room.SendMessage(common.Message{
		Type:    common.MsgChat,
		User:    0,
		Content: "房间已被管理员解散",
	})

	// 停止回放录制（如果有）
	if recorder := h.server.GetReplayRecorder(); recorder != nil {
		recorder.StopRecording(room.ID.Value)
	}

	// 断开所有用户连接
	for _, user := range room.GetAllUsers() {
		if session := user.GetSession(); session != nil {
			session.Stop()
		}
	}

	// 移除房间
	h.server.RemoveRoom(room.ID, "管理员解散")

	writeOK(w, map[string]interface{}{
		"roomid": room.ID.Value,
	})
}

// handleAdminUserDetail 处理查询用户详情
func (h *HTTPServer) handleAdminUserDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}

	// 解析用户ID
	userID, ok := parseUserIDFromPath(r.URL.Path, "/admin/users/")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad-request")
		return
	}

	user := h.server.GetUser(userID)
	if user == nil {
		writeError(w, http.StatusNotFound, "user-not-found")
		return
	}

	// 获取用户所在房间
	room := user.GetRoom()
	roomID := ""
	if room != nil {
		roomID = room.ID.Value
	}

	writeOK(w, map[string]interface{}{
		"user": map[string]interface{}{
			"id":        user.ID,
			"name":      user.Name,
			"monitor":   user.IsMonitor(),
			"connected": !user.IsDisconnected(),
			"room":      roomID,
			"banned":    h.adminData.IsUserBanned(userID),
		},
	})
}

// AdminBanUserRequest 封禁用户请求
type AdminBanUserRequest struct {
	UserID     int32 `json:"userId"`
	Banned     bool  `json:"banned"`
	Disconnect bool  `json:"disconnect"`
}

// handleAdminBanUser 处理封禁/解封用户
func (h *HTTPServer) handleAdminBanUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}

	var req AdminBanUserRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad-request")
		return
	}

	// 封禁/解封用户
	h.adminData.BanUser(req.UserID, req.Banned)
	h.saveAdminData()

	// 如果需要断开连接
	if req.Disconnect && req.Banned {
		user := h.server.GetUser(req.UserID)
		if user != nil {
			room := user.GetRoom()
			
			// 如果在游戏中，标记为放弃
			if room != nil && room.GetState() == InternalStatePlaying {
				room.aborted.Store(user.ID, true)
				room.SendMessage(common.Message{
					Type: common.MsgAbort,
					User: user.ID,
				})
			}
			
			// 断开连接
			if session := user.GetSession(); session != nil {
				session.Stop()
			}
			
			// 从房间移除
			if room != nil {
				if room.OnUserLeave(user) {
					h.server.RemoveRoom(room.ID, "房间为空")
				} else {
					room.CheckAllReady()
				}
			}
		}
	}

	writeOK(w, nil)
}

// AdminBanRoomRequest 房间级封禁请求
type AdminBanRoomRequest struct {
	UserID int32  `json:"userId"`
	RoomID string `json:"roomId"`
	Banned bool   `json:"banned"`
}

// handleAdminBanRoom 处理房间级封禁
func (h *HTTPServer) handleAdminBanRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}

	var req AdminBanRoomRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad-request")
		return
	}

	// 验证房间ID
	if !isValidRoomID(req.RoomID) {
		writeError(w, http.StatusBadRequest, "bad-room-id")
		return
	}

	// 封禁/解封用户进入房间
	h.adminData.BanUserFromRoom(req.UserID, req.RoomID, req.Banned)
	h.saveAdminData()

	writeOK(w, nil)
}

// AdminBroadcastRequest 广播请求
type AdminBroadcastRequest struct {
	Message string `json:"message"`
}

// handleAdminBroadcast 处理全服广播
func (h *HTTPServer) handleAdminBroadcast(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}

	var req AdminBroadcastRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad-request")
		return
	}

	// 验证消息长度 1-200
	if len(req.Message) == 0 {
		writeError(w, http.StatusBadRequest, "bad-message")
		return
	}
	if len(req.Message) > 200 {
		writeError(w, http.StatusBadRequest, "message-too-long")
		return
	}

	// 向所有房间发送消息
	rooms := h.server.GetAllRooms()
	for _, room := range rooms {
		room.SendMessage(common.Message{
			Type:    common.MsgChat,
			User:    0,
			Content: req.Message,
		})
	}

	writeOK(w, map[string]interface{}{
		"rooms": len(rooms),
	})
}

// ReplayConfigResponse 回放配置响应
type ReplayConfigResponse struct {
	OK      bool `json:"ok"`
	Enabled bool `json:"enabled"`
}

// handleAdminReplayConfig 处理回放配置
func (h *HTTPServer) handleAdminReplayConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// 获取当前配置
		writeOK(w, map[string]interface{}{
			"enabled": h.IsReplayEnabled(),
		})

	case http.MethodPost:
		// 修改配置
		var req struct {
			Enabled *bool `json:"enabled"`
		}
		if err := parseBody(r, &req); err != nil || req.Enabled == nil {
			writeError(w, http.StatusBadRequest, "bad-enabled")
			return
		}

		h.SetReplayEnabled(*req.Enabled)

		// 如果关闭回放，停止所有房间的录制
		if !*req.Enabled {
			// TODO: 停止所有房间的录制
		}

		writeOK(w, map[string]interface{}{
			"enabled": *req.Enabled,
		})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
	}
}

// handleAdminRoomCreationConfig 处理房间创建配置
func (h *HTTPServer) handleAdminRoomCreationConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// 获取当前配置
		writeOK(w, map[string]interface{}{
			"enabled": h.IsRoomCreationEnabled(),
		})

	case http.MethodPost:
		// 修改配置
		var req struct {
			Enabled *bool `json:"enabled"`
		}
		if err := parseBody(r, &req); err != nil || req.Enabled == nil {
			writeError(w, http.StatusBadRequest, "bad-enabled")
			return
		}

		h.SetRoomCreationEnabled(*req.Enabled)

		writeOK(w, map[string]interface{}{
			"enabled": *req.Enabled,
		})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
	}
}

// buildAdminRoomInfo 构建管理员房间信息
func buildAdminRoomInfo(room *Room) AdminRoomInfo {
	host := room.GetHost()

	// 构建状态信息
	var stateInfo interface{}
	roomState := room.GetState()
	
	switch roomState {
	case InternalStateSelectChart:
		stateInfo = AdminRoomStateInfo{Type: "select_chart"}
	case InternalStateWaitForReady:
		stateInfo = AdminRoomStateInfo{Type: "waiting_for_ready"}
	case InternalStatePlaying:
		// 收集已完成和放弃的玩家
		var finishedUsers []int32
		var abortedUsers []int32
		resultsCount := 0
		abortedCount := 0
		
		room.results.Range(func(key, value interface{}) bool {
			userID := key.(int32)
			finishedUsers = append(finishedUsers, userID)
			resultsCount++
			return true
		})
		
		room.aborted.Range(func(key, value interface{}) bool {
			userID := key.(int32)
			abortedUsers = append(abortedUsers, userID)
			abortedCount++
			return true
		})
		
		stateInfo = AdminRoomStateInfo{
			Type:          "playing",
			ResultsCount:  resultsCount,
			AbortedCount:  abortedCount,
			FinishedUsers: finishedUsers,
			AbortedUsers:  abortedUsers,
		}
	default:
		stateInfo = AdminRoomStateInfo{Type: "select_chart"}
	}

	// 获取用户列表
	users := room.GetUsers()
	userInfos := make([]AdminUserInfo, 0, len(users))
	for _, u := range users {
		userInfo := AdminUserInfo{
			ID:        u.ID,
			Name:      u.Name,
			Connected: !u.IsDisconnected(),
			IsHost:    u.ID == host.ID,
			GameTime:  float32(u.gameTime.Load()),
			Language:  u.Lang,
		}
		
		// 如果房间在游戏中，添加游戏状态信息
		if roomState == InternalStatePlaying {
			_, finished := room.results.Load(u.ID)
			_, aborted := room.aborted.Load(u.ID)
			userInfo.Finished = finished
			userInfo.Aborted = aborted
			
			// 如果有成绩，添加record_id
			if finished {
				if record, ok := room.results.Load(u.ID); ok {
					if r, ok := record.(*Record); ok {
						userInfo.RecordID = &r.ID
					}
				}
			}
		}
		
		userInfos = append(userInfos, userInfo)
	}

	// 获取观察者列表
	monitors := room.GetMonitors()
	monitorInfos := make([]AdminUserInfo, 0, len(monitors))
	for _, u := range monitors {
		monitorInfos = append(monitorInfos, AdminUserInfo{
			ID:        u.ID,
			Name:      u.Name,
			Connected: !u.IsDisconnected(),
			IsHost:    false,
			GameTime:  float32(u.gameTime.Load()),
			Language:  u.Lang,
			Monitor:   true,
		})
	}

	info := AdminRoomInfo{
		RoomID:   room.ID.Value,
		MaxUsers: RoomMaxUsers,
		Live:     room.IsLive(),
		Locked:   room.IsLocked(),
		Cycle:    room.IsCycle(),
		Host:     UserBrief{ID: host.ID, Name: host.Name},
		State:    stateInfo,
		Users:    userInfos,
		Monitors: monitorInfos,
	}

	// 添加谱面信息
	if chart := room.GetChart(); chart != nil {
		info.Chart = &ChartInfo{
			ID:   chart.ID,
			Name: chart.Name,
		}
	}

	return info
}

// handleAdminContest 处理比赛房间相关操作
func (h *HTTPServer) handleAdminContest(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// 解析房间ID
	roomID, ok := parseRoomIDFromPath(path, "/admin/contest/rooms/")
	if !ok || !isValidRoomID(roomID) {
		writeError(w, http.StatusBadRequest, "bad-room-id")
		return
	}

	roomId, err := common.NewRoomId(roomID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad-room-id")
		return
	}

	room := h.server.GetRoom(roomId)
	if room == nil {
		writeError(w, http.StatusNotFound, "room-not-found")
		return
	}

	// 根据路径后缀判断操作
	switch {
	case strings.HasSuffix(path, "/config"):
		h.handleAdminContestConfig(w, r, room)
	case strings.HasSuffix(path, "/whitelist"):
		h.handleAdminContestWhitelist(w, r, room)
	case strings.HasSuffix(path, "/start"):
		h.handleAdminContestStart(w, r, room)
	default:
		writeError(w, http.StatusNotFound, "not-found")
	}
}

// ContestConfigRequest 比赛配置请求
type ContestConfigRequest struct {
	Enabled   bool    `json:"enabled"`
	Whitelist []int32 `json:"whitelist"`
}

// handleAdminContestConfig 处理比赛房间配置
func (h *HTTPServer) handleAdminContestConfig(w http.ResponseWriter, r *http.Request, room *Room) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}

	var req ContestConfigRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad-request")
		return
	}

	// TODO: 实现比赛房间配置（需要在Room结构中添加比赛模式相关字段）
	// enabled=true: 启用比赛模式（手动开始 + 结算后解散）
	// enabled=false: 关闭比赛模式
	// whitelist为空时，默认取当前房间内所有用户/观战者为白名单

	writeOK(w, nil)
}

// ContestWhitelistRequest 白名单请求
type ContestWhitelistRequest struct {
	UserIDs []int32 `json:"userIds"`
}

// handleAdminContestWhitelist 处理更新白名单
func (h *HTTPServer) handleAdminContestWhitelist(w http.ResponseWriter, r *http.Request, room *Room) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}

	var req ContestWhitelistRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad-request")
		return
	}

	// TODO: 实现白名单更新（需要在Room结构中添加白名单字段）
	// 自动把当前已经在房间内的用户/观战者补进白名单

	writeOK(w, nil)
}

// ContestStartRequest 比赛开始请求
type ContestStartRequest struct {
	Force bool `json:"force"`
}

// handleAdminContestStart 处理手动开始比赛
func (h *HTTPServer) handleAdminContestStart(w http.ResponseWriter, r *http.Request, room *Room) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}

	var req ContestStartRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad-request")
		return
	}

	// 检查房间状态
	if room.GetState() != InternalStateWaitForReady {
		writeError(w, http.StatusBadRequest, "invalid-state")
		return
	}

	// 检查是否全员ready
	if !req.Force {
		allReady := true
		for _, user := range room.GetAllUsers() {
			if _, ok := room.started.Load(user.ID); !ok {
				allReady = false
				break
			}
		}
		if !allReady {
			writeError(w, http.StatusBadRequest, "not-all-ready")
			return
		}
	}

	// 开始游戏
	room.SendMessage(common.Message{Type: common.MsgStartPlaying})
	room.ResetGameTime()
	room.SetState(InternalStatePlaying)
	room.Broadcast(common.ServerCommand{
		Type:        common.ServerCmdChangeState,
		ChangeState: &common.RoomState{Type: common.RoomStatePlaying},
	})

	writeOK(w, nil)
}

// handleAdminUserOperations 处理用户相关操作（查询、断开、移动）
func (h *HTTPServer) handleAdminUserOperations(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// 检查是否是断开连接请求
	if strings.HasSuffix(path, "/disconnect") {
		h.handleAdminUserDisconnect(w, r)
		return
	}

	// 检查是否是移动用户请求
	if strings.HasSuffix(path, "/move") {
		h.handleAdminUserMove(w, r)
		return
	}

	// 否则是查询用户详情
	h.handleAdminUserDetail(w, r)
}

// handleAdminUserDisconnect 处理断开用户连接
func (h *HTTPServer) handleAdminUserDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}

	// 解析用户ID (路径格式: /admin/users/{id}/disconnect)
	path := strings.TrimSuffix(r.URL.Path, "/disconnect")
	userID, ok := parseUserIDFromPath(path, "/admin/users/")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad-request")
		return
	}

	user := h.server.GetUser(userID)
	if user == nil {
		writeError(w, http.StatusNotFound, "user-not-connected")
		return
	}

	room := user.GetRoom()
	
	// 如果在游戏中，标记为放弃
	if room != nil && room.GetState() == InternalStatePlaying {
		room.aborted.Store(user.ID, true)
		room.SendMessage(common.Message{
			Type: common.MsgAbort,
			User: user.ID,
		})
	}

	// 断开连接
	session := user.GetSession()
	if session != nil {
		session.Stop()
	}

	// 从房间移除用户并触发结算检查
	if room != nil {
		if room.OnUserLeave(user) {
			h.server.RemoveRoom(room.ID, "房间为空")
		} else {
			room.CheckAllReady()
		}
	}

	writeOK(w, nil)
}

// handleAdminUserMove 处理转移用户
func (h *HTTPServer) handleAdminUserMove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}

	// 解析用户ID (路径格式: /admin/users/{id}/move)
	path := strings.TrimSuffix(r.URL.Path, "/move")
	userID, ok := parseUserIDFromPath(path, "/admin/users/")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad-request")
		return
	}

	user := h.server.GetUser(userID)
	if user == nil {
		writeError(w, http.StatusNotFound, "user-not-found")
		return
	}

	var req struct {
		RoomID  string `json:"roomId"`
		Monitor bool   `json:"monitor"`
	}
	if err := parseBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad-request")
		return
	}

	// 检查用户是否断线
	if !user.IsDisconnected() {
		writeError(w, http.StatusBadRequest, "user-still-connected")
		return
	}

	// 验证目标房间
	roomId, err := common.NewRoomId(req.RoomID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad-room-id")
		return
	}

	targetRoom := h.server.GetRoom(roomId)
	if targetRoom == nil {
		writeError(w, http.StatusNotFound, "room-not-found")
		return
	}

	// 检查房间状态
	if targetRoom.GetState() != InternalStateSelectChart {
		writeError(w, http.StatusBadRequest, "invalid-state")
		return
	}

	// 获取源房间
	sourceRoom := user.GetRoom()
	if sourceRoom != nil {
		// 检查源房间状态
		if sourceRoom.GetState() != InternalStateSelectChart {
			writeError(w, http.StatusBadRequest, "invalid-state")
			return
		}
		// 从源房间移除
		sourceRoom.RemoveUser(user.ID)
	}

	// 添加到目标房间
	targetRoom.AddUser(user, req.Monitor)
	user.SetRoom(targetRoom)
	user.SetMonitor(req.Monitor)

	writeOK(w, nil)
}
