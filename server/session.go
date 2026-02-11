package server

import (
	"fmt"
	"log"
	"sync"
	"time"

	"phira-mp/common"

	"github.com/google/uuid"
)

// Session 会话
type Session struct {
	ID     uuid.UUID
	Stream *common.ServerStream
	User   *User

	server        *Server
	stopChan      chan struct{}
	stopped       bool
	disconnecting bool // 是否正在断开连接，避免重复处理
	lastPing      time.Time
	authenticated bool
}

// NewSession 创建新会话
func NewSession(id uuid.UUID, stream *common.ServerStream, server *Server) *Session {
	return &Session{
		ID:       id,
		Stream:   stream,
		server:   server,
		stopChan: make(chan struct{}),
		lastPing: time.Now(),
	}
}

// Start 启动会话处理
func (s *Session) Start() {
	go s.recvLoop()
	go s.heartbeatCheck()
}

// Stop 停止会话
func (s *Session) Stop() {
	if !s.stopped {
		s.stopped = true
		close(s.stopChan)
		s.Stream.Close()
	}
}

// Send 发送命令
func (s *Session) Send(cmd common.ServerCommand) error {
	return s.Stream.Send(cmd)
}

// recvLoop 接收循环
func (s *Session) recvLoop() {
	for {
		select {
		case <-s.stopChan:
			return
		default:
		}

		cmd, err := s.Stream.Recv()
		if err != nil {
			RateLimitedLog("会话 %s 接收错误: %v", s.ID, err)
			s.handleDisconnect()
			return
		}

		s.lastPing = time.Now()

		// 选择谱面命令输出详细日志（常规输出）
		if cmd.Type == common.ClientCmdSelectChart && s.User != nil {
			room := s.User.GetRoom()
			if room != nil {
				chart, _ := FetchChart(cmd.ChartID)
				if chart != nil {
					log.Printf("玩家 `%s(%d)` 在房间 `%s` 选择了谱面 `%s(%d)`", s.User.Name, s.User.ID, room.ID, chart.Name, chart.ID)
				} else {
					log.Printf("玩家 `%s(%d)` 在房间 `%s` 选择了谱面 `ID(%d)`", s.User.Name, s.User.ID, room.ID, cmd.ChartID)
				}
			} else {
				log.Printf("玩家 `%s(%d)` 选择了谱面 `ID(%d)`", s.User.Name, s.User.ID, cmd.ChartID)
			}
		}

		// 记录接收到的命令类型（只在 DEBUG 模式下输出）
		if s.server.IsDebugEnabled() {
			log.Printf("[DEBUG] 会话 %s 收到命令: 类型=%d", s.ID, cmd.Type)
		}

		if err := s.handleCommand(cmd); err != nil {
			RateLimitedLog("会话 %s 处理命令错误: %v", s.ID, err)
		}
	}
}

// heartbeatCheck 心跳检测
func (s *Session) heartbeatCheck() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			if time.Since(s.lastPing) > common.HeartbeatDisconnectTimeout {
				log.Printf("会话 %s 心跳超时", s.ID)
				s.handleDisconnect()
				return
			}
		}
	}
}

// handleDisconnect 处理断开连接
func (s *Session) handleDisconnect() {
	// 检查是否已经在处理断开，避免重复执行
	if s.disconnecting {
		return
	}
	s.disconnecting = true

	s.server.RemoveSession(s.ID)
	if s.User != nil {
		s.User.Dangle()
	}
	s.Stop()
}

// handleCommand 处理命令
func (s *Session) handleCommand(cmd common.ClientCommand) error {
	// Ping
	if cmd.Type == common.ClientCmdPing {
		return s.Send(common.ServerCommand{Type: common.ServerCmdPong})
	}

	// 未认证时只允许Authenticate
	if !s.authenticated {
		if cmd.Type != common.ClientCmdAuthenticate {
			return fmt.Errorf("未认证")
		}
		return s.handleAuthenticate(cmd.Token)
	}

	// 已认证，处理其他命令
	switch cmd.Type {
	case common.ClientCmdChat:
		return s.handleChat(cmd.Message)
	case common.ClientCmdTouches:
		return s.handleTouches(cmd.Frames)
	case common.ClientCmdJudges:
		return s.handleJudges(cmd.Judges)
	case common.ClientCmdCreateRoom:
		return s.handleCreateRoom(cmd.RoomId)
	case common.ClientCmdJoinRoom:
		return s.handleJoinRoom(cmd.RoomId, cmd.Monitor)
	case common.ClientCmdLeaveRoom:
		return s.handleLeaveRoom()
	case common.ClientCmdLockRoom:
		return s.handleLockRoom(cmd.Lock)
	case common.ClientCmdCycleRoom:
		return s.handleCycleRoom(cmd.Cycle)
	case common.ClientCmdSelectChart:
		return s.handleSelectChart(cmd.ChartID)
	case common.ClientCmdRequestStart:
		return s.handleRequestStart()
	case common.ClientCmdReady:
		return s.handleReady()
	case common.ClientCmdCancelReady:
		return s.handleCancelReady()
	case common.ClientCmdPlayed:
		return s.handlePlayed(cmd.RecordID)
	case common.ClientCmdAbort:
		return s.handleAbort()
	default:
		log.Printf("会话 %s 未知命令类型: %d (最大有效值: %d), 断开连接", s.ID, cmd.Type, common.ClientCmdAbort)
		// 发送错误响应
		s.Send(common.ServerCommand{
			Type: common.ServerCmdMessage,
			Message: &common.Message{
				Type:    common.MsgChat,
				User:    -1,
				Content: "未知命令类型",
			},
		})
		return fmt.Errorf("未知命令类型: %d", cmd.Type)
	}
}

// handleAuthenticate 处理认证
func (s *Session) handleAuthenticate(token string) error {
	user, _, err := UserInfoFromAPI(token)
	if err != nil {
		s.Send(common.ServerCommand{
			Type: common.ServerCmdAuthenticate,
			AuthenticateResult: &common.Result[common.AuthResult]{
				Err: strPtr("认证失败"),
			},
		})
		return err
	}

	// 检查用户是否被封禁
	if s.server.IsUserBanned(user.ID) {
		s.Send(common.ServerCommand{
			Type: common.ServerCmdAuthenticate,
			AuthenticateResult: &common.Result[common.AuthResult]{
				Err: strPtr("用户已被封禁"),
			},
		})
		return fmt.Errorf("user %d is banned", user.ID)
	}

	// 检查是否已有相同用户在线
	if existingUser := s.server.GetUser(user.ID); existingUser != nil {
		// 重连逻辑
		existingUser.SetSession(s)
		s.User = existingUser
	} else {
		user.server = s.server
		user.SetSession(s)
		s.server.AddUser(user)
		s.User = user
	}

	s.authenticated = true

	// 获取房间状态
	var clientRoomState *common.ClientRoomState
	if room := s.User.GetRoom(); room != nil {
		state := room.GetClientRoomState(s.User)
		clientRoomState = &state
	}

	return s.Send(common.ServerCommand{
		Type: common.ServerCmdAuthenticate,
		AuthenticateResult: &common.Result[common.AuthResult]{
			Ok: &common.AuthResult{
				User: s.User.ToInfo(),
				Room: clientRoomState,
			},
		},
	})
}

// handleChat 处理聊天（已禁用，强制替换为规范提示）
func (s *Session) handleChat(message string) error {
	room := s.User.GetRoom()
	if room == nil {
		return s.Send(common.ServerCommand{
			Type:       common.ServerCmdChat,
			ChatResult: &common.Result[struct{}]{Err: strPtr("不在房间中")},
		})
	}

	// 聊天功能已禁用，强制替换为规范提示消息
	room.SendMessage(common.Message{
		Type:    common.MsgChat,
		User:    s.User.ID,
		Content: "为符合规范，该服务器已禁用聊天功能",
	})

	return s.Send(common.ServerCommand{
		Type:       common.ServerCmdChat,
		ChatResult: &common.Result[struct{}]{Ok: &struct{}{}},
	})
}

// handleTouches 处理触摸数据
func (s *Session) handleTouches(frames []common.TouchFrame) error {
	room := s.User.GetRoom()
	if room == nil || !room.IsLive() {
		return nil
	}

	// 更新游戏时间
	if len(frames) > 0 {
		s.User.gameTime.Store(uint32(frames[len(frames)-1].Time))
	}

	// 广播给观察者
	room.BroadcastMonitors(common.ServerCommand{
		Type:          common.ServerCmdTouches,
		TouchesPlayer: s.User.ID,
		TouchesFrames: frames,
	})

	// 录制回放
	if recorder := s.server.GetReplayRecorder(); recorder != nil {
		recorder.RecordTouch(room.ID.Value, s.User.ID, frames)
	}

	return nil
}

// handleJudges 处理判定数据
func (s *Session) handleJudges(judges []common.JudgeEvent) error {
	room := s.User.GetRoom()
	if room == nil || !room.IsLive() {
		return nil
	}

	// 广播给观察者
	room.BroadcastMonitors(common.ServerCommand{
		Type:         common.ServerCmdJudges,
		JudgesPlayer: s.User.ID,
		JudgesEvents: judges,
	})

	// 录制回放
	if recorder := s.server.GetReplayRecorder(); recorder != nil {
		recorder.RecordJudge(room.ID.Value, s.User.ID, judges)
	}

	return nil
}

// handleCreateRoom 处理创建房间
func (s *Session) handleCreateRoom(roomId common.RoomId) error {
	if s.User.GetRoom() != nil {
		return s.Send(common.ServerCommand{
			Type:             common.ServerCmdCreateRoom,
			CreateRoomResult: &common.Result[struct{}]{Err: strPtr("已在房间中")},
		})
	}

	// 检查是否允许创建房间
	if !s.server.IsRoomCreationEnabled() {
		return s.Send(common.ServerCommand{
			Type:             common.ServerCmdCreateRoom,
			CreateRoomResult: &common.Result[struct{}]{Err: strPtr("房间创建已被禁用")},
		})
	}

	if s.server.GetRoom(roomId) != nil {
		return s.Send(common.ServerCommand{
			Type:             common.ServerCmdCreateRoom,
			CreateRoomResult: &common.Result[struct{}]{Err: strPtr("房间ID已被占用")},
		})
	}

	room := NewRoom(roomId, s.User, s.server)
	s.server.AddRoom(room)
	s.User.SetRoom(room)

	room.SendMessage(common.Message{
		Type: common.MsgCreateRoom,
		User: s.User.ID,
	})

	// 如果启用了回放录制，插入虚拟monitor并设置live模式
	if s.server.GetReplayRecorder() != nil && s.server.GetHTTPServer() != nil && s.server.GetHTTPServer().IsReplayEnabled() {
		s.setupVirtualMonitorForReplay(room)
	}

	return s.Send(common.ServerCommand{
		Type:             common.ServerCmdCreateRoom,
		CreateRoomResult: &common.Result[struct{}]{Ok: &struct{}{}},
	})
}

// setupVirtualMonitorForReplay 为回放录制设置虚拟monitor
func (s *Session) setupVirtualMonitorForReplay(room *Room) {
	// 设置房间为live模式
	room.SetLive(true)

	// 创建虚拟monitor用户信息
	virtualUser := &common.UserInfo{
		ID:      2_000_000_000, // 虚拟monitor的固定ID
		Name:    "回放录制器",
		Monitor: true,
	}

	// 广播虚拟monitor加入房间
	room.Broadcast(common.ServerCommand{
		Type:           common.ServerCmdOnJoinRoom,
		OnJoinRoomUser: virtualUser,
	})

	// 发送系统消息
	room.SendMessage(common.Message{
		Type: common.MsgJoinRoom,
		User: virtualUser.ID,
		Name: virtualUser.Name,
	})

	log.Printf("房间 %s 已启用回放录制模式（虚拟monitor加入）", room.ID.Value)

	// 延迟2秒后播报虚拟monitor退出
	go func() {
		time.Sleep(2 * time.Second)

		// 检查房间是否还存在
		if s.server.GetRoom(room.ID) == nil {
			return
		}

		// 广播虚拟monitor离开
		room.Broadcast(common.ServerCommand{
			Type: common.ServerCmdMessage,
			Message: &common.Message{
				Type: common.MsgLeaveRoom,
				User: virtualUser.ID,
				Name: virtualUser.Name,
			},
		})

		log.Printf("房间 %s 虚拟monitor已退出，房间保持live模式", room.ID.Value)
	}()
}

// handleJoinRoom 处理加入房间
func (s *Session) handleJoinRoom(roomId common.RoomId, monitor bool) error {
	if s.User.GetRoom() != nil {
		return s.Send(common.ServerCommand{
			Type:           common.ServerCmdJoinRoom,
			JoinRoomResult: &common.Result[common.JoinRoomResponse]{Err: strPtr("已在房间中")},
		})
	}

	room := s.server.GetRoom(roomId)
	if room == nil {
		return s.Send(common.ServerCommand{
			Type:           common.ServerCmdJoinRoom,
			JoinRoomResult: &common.Result[common.JoinRoomResponse]{Err: strPtr("房间不存在")},
		})
	}

	// 检查用户是否被禁止进入该房间
	if s.server.IsUserBannedFromRoom(s.User.ID, roomId.Value) {
		return s.Send(common.ServerCommand{
			Type:           common.ServerCmdJoinRoom,
			JoinRoomResult: &common.Result[common.JoinRoomResponse]{Err: strPtr("已被禁止进入该房间")},
		})
	}

	if room.IsLocked() {
		return s.Send(common.ServerCommand{
			Type:           common.ServerCmdJoinRoom,
			JoinRoomResult: &common.Result[common.JoinRoomResponse]{Err: strPtr("房间已锁定")},
		})
	}

	if room.GetState() != InternalStateSelectChart {
		return s.Send(common.ServerCommand{
			Type:           common.ServerCmdJoinRoom,
			JoinRoomResult: &common.Result[common.JoinRoomResponse]{Err: strPtr("游戏进行中")},
		})
	}

	if monitor && !s.User.CanMonitor() {
		return s.Send(common.ServerCommand{
			Type:           common.ServerCmdJoinRoom,
			JoinRoomResult: &common.Result[common.JoinRoomResponse]{Err: strPtr("无法观察")},
		})
	}

	if !room.AddUser(s.User, monitor) {
		return s.Send(common.ServerCommand{
			Type:           common.ServerCmdJoinRoom,
			JoinRoomResult: &common.Result[common.JoinRoomResponse]{Err: strPtr("房间已满")},
		})
	}

	s.User.SetMonitor(monitor)
	s.User.SetRoom(room)

	if monitor && !room.IsLive() {
		room.SetLive(true)
	}

	room.Broadcast(common.ServerCommand{
		Type: common.ServerCmdOnJoinRoom,
		OnJoinRoomUser: &common.UserInfo{
			ID:      s.User.ID,
			Name:    s.User.Name,
			Monitor: monitor,
		},
	})

	room.SendMessage(common.Message{
		Type: common.MsgJoinRoom,
		User: s.User.ID,
		Name: s.User.Name,
	})

	// 获取所有用户信息
	users := room.GetAllUsers()
	userInfos := make([]common.UserInfo, 0, len(users))
	for _, u := range users {
		userInfos = append(userInfos, u.ToInfo())
	}

	chart := room.GetChart()
	var chartID *int32
	if chart != nil {
		chartID = &chart.ID
	}

	return s.Send(common.ServerCommand{
		Type: common.ServerCmdJoinRoom,
		JoinRoomResult: &common.Result[common.JoinRoomResponse]{
			Ok: &common.JoinRoomResponse{
				State: room.GetState().ToClientState(chartID),
				Users: userInfos,
				Live:  room.IsLive(),
			},
		},
	})
}

// handleLeaveRoom 处理离开房间
func (s *Session) handleLeaveRoom() error {
	room := s.User.GetRoom()
	if room == nil {
		return s.Send(common.ServerCommand{
			Type:            common.ServerCmdLeaveRoom,
			LeaveRoomResult: &common.Result[struct{}]{Err: strPtr("不在房间中")},
		})
	}

	// 如果在游戏中离开，标记为放弃
	if room.GetState() == InternalStatePlaying {
		log.Printf("用户 %d(%s) 在游戏中离开房间，标记为放弃", s.User.ID, s.User.Name)
		room.aborted.Store(s.User.ID, true)
		room.SendMessage(common.Message{
			Type: common.MsgAbort,
			User: s.User.ID,
		})
		room.CheckAllReady()
	}

	if room.OnUserLeave(s.User) {
		s.server.RemoveRoom(room.ID, "房间为空")
	}

	return s.Send(common.ServerCommand{
		Type:            common.ServerCmdLeaveRoom,
		LeaveRoomResult: &common.Result[struct{}]{Ok: &struct{}{}},
	})
}

// handleLockRoom 处理锁定房间
func (s *Session) handleLockRoom(lock bool) error {
	room := s.User.GetRoom()
	if room == nil {
		return s.Send(common.ServerCommand{
			Type:           common.ServerCmdLockRoom,
			LockRoomResult: &common.Result[struct{}]{Err: strPtr("不在房间中")},
		})
	}

	if err := room.CheckHost(s.User); err != nil {
		return s.Send(common.ServerCommand{
			Type:           common.ServerCmdLockRoom,
			LockRoomResult: &common.Result[struct{}]{Err: strPtr("只有房主可以锁定")},
		})
	}

	room.SetLocked(lock)
	room.SendMessage(common.Message{
		Type: common.MsgLockRoom,
		Lock: lock,
	})

	return s.Send(common.ServerCommand{
		Type:           common.ServerCmdLockRoom,
		LockRoomResult: &common.Result[struct{}]{Ok: &struct{}{}},
	})
}

// handleCycleRoom 处理循环房间
func (s *Session) handleCycleRoom(cycle bool) error {
	room := s.User.GetRoom()
	if room == nil {
		return s.Send(common.ServerCommand{
			Type:            common.ServerCmdCycleRoom,
			CycleRoomResult: &common.Result[struct{}]{Err: strPtr("不在房间中")},
		})
	}

	if err := room.CheckHost(s.User); err != nil {
		return s.Send(common.ServerCommand{
			Type:            common.ServerCmdCycleRoom,
			CycleRoomResult: &common.Result[struct{}]{Err: strPtr("只有房主可以设置循环")},
		})
	}

	room.SetCycle(cycle)
	room.SendMessage(common.Message{
		Type:  common.MsgCycleRoom,
		Cycle: cycle,
	})

	return s.Send(common.ServerCommand{
		Type:            common.ServerCmdCycleRoom,
		CycleRoomResult: &common.Result[struct{}]{Ok: &struct{}{}},
	})
}

// handleSelectChart 处理选择谱面
func (s *Session) handleSelectChart(chartID int32) error {
	room := s.User.GetRoom()
	if room == nil {
		return s.Send(common.ServerCommand{
			Type:              common.ServerCmdSelectChart,
			SelectChartResult: &common.Result[struct{}]{Err: strPtr("不在房间中")},
		})
	}

	if room.GetState() != InternalStateSelectChart {
		return s.Send(common.ServerCommand{
			Type:              common.ServerCmdSelectChart,
			SelectChartResult: &common.Result[struct{}]{Err: strPtr("无效状态")},
		})
	}

	if err := room.CheckHost(s.User); err != nil {
		return s.Send(common.ServerCommand{
			Type:              common.ServerCmdSelectChart,
			SelectChartResult: &common.Result[struct{}]{Err: strPtr("只有房主可以选择谱面")},
		})
	}

	chart, err := FetchChart(chartID)
	if err != nil {
		return s.Send(common.ServerCommand{
			Type:              common.ServerCmdSelectChart,
			SelectChartResult: &common.Result[struct{}]{Err: strPtr("谱面不存在")},
		})
	}

	room.SetChart(chart)
	room.SendMessage(common.Message{
		Type:    common.MsgSelectChart,
		User:    s.User.ID,
		Name:    chart.Name,
		ChartID: chart.ID,
	})
	room.OnStateChange()

	return s.Send(common.ServerCommand{
		Type:              common.ServerCmdSelectChart,
		SelectChartResult: &common.Result[struct{}]{Ok: &struct{}{}},
	})
}

// handleRequestStart 处理请求开始
func (s *Session) handleRequestStart() error {
	room := s.User.GetRoom()
	if room == nil {
		return s.Send(common.ServerCommand{
			Type:               common.ServerCmdRequestStart,
			RequestStartResult: &common.Result[struct{}]{Err: strPtr("不在房间中")},
		})
	}

	if room.GetState() != InternalStateSelectChart {
		return s.Send(common.ServerCommand{
			Type:               common.ServerCmdRequestStart,
			RequestStartResult: &common.Result[struct{}]{Err: strPtr("无效状态")},
		})
	}

	if err := room.CheckHost(s.User); err != nil {
		return s.Send(common.ServerCommand{
			Type:               common.ServerCmdRequestStart,
			RequestStartResult: &common.Result[struct{}]{Err: strPtr("只有房主可以开始")},
		})
	}

	if room.GetChart() == nil {
		return s.Send(common.ServerCommand{
			Type:               common.ServerCmdRequestStart,
			RequestStartResult: &common.Result[struct{}]{Err: strPtr("未选择谱面")},
		})
	}

	room.ResetGameTime()
	room.SendMessage(common.Message{
		Type: common.MsgGameStart,
		User: s.User.ID,
	})
	room.SetState(InternalStateWaitForReady)
	room.started.Store(s.User.ID, true)
	room.OnStateChange()
	room.CheckAllReady()

	return s.Send(common.ServerCommand{
		Type:               common.ServerCmdRequestStart,
		RequestStartResult: &common.Result[struct{}]{Ok: &struct{}{}},
	})
}

// handleReady 处理准备
func (s *Session) handleReady() error {
	room := s.User.GetRoom()
	if room == nil {
		return s.Send(common.ServerCommand{
			Type:        common.ServerCmdReady,
			ReadyResult: &common.Result[struct{}]{Err: strPtr("不在房间中")},
		})
	}

	if room.GetState() != InternalStateWaitForReady {
		return s.Send(common.ServerCommand{
			Type:        common.ServerCmdReady,
			ReadyResult: &common.Result[struct{}]{Err: strPtr("无效状态")},
		})
	}

	if _, loaded := room.started.LoadOrStore(s.User.ID, true); loaded {
		return s.Send(common.ServerCommand{
			Type:        common.ServerCmdReady,
			ReadyResult: &common.Result[struct{}]{Err: strPtr("已准备")},
		})
	}

	room.SendMessage(common.Message{
		Type: common.MsgReady,
		User: s.User.ID,
	})
	room.CheckAllReady()

	return s.Send(common.ServerCommand{
		Type:        common.ServerCmdReady,
		ReadyResult: &common.Result[struct{}]{Ok: &struct{}{}},
	})
}

// handleCancelReady 处理取消准备
func (s *Session) handleCancelReady() error {
	room := s.User.GetRoom()
	if room == nil {
		return s.Send(common.ServerCommand{
			Type:              common.ServerCmdCancelReady,
			CancelReadyResult: &common.Result[struct{}]{Err: strPtr("不在房间中")},
		})
	}

	if room.GetState() != InternalStateWaitForReady {
		return s.Send(common.ServerCommand{
			Type:              common.ServerCmdCancelReady,
			CancelReadyResult: &common.Result[struct{}]{Err: strPtr("无效状态")},
		})
	}

	if _, ok := room.started.Load(s.User.ID); !ok {
		return s.Send(common.ServerCommand{
			Type:              common.ServerCmdCancelReady,
			CancelReadyResult: &common.Result[struct{}]{Err: strPtr("未准备")},
		})
	}

	room.started.Delete(s.User.ID)

	// 房主取消则取消游戏
	if room.GetHost().ID == s.User.ID {
		// 清空游戏状态
		room.started = sync.Map{}
		room.results = sync.Map{}
		room.aborted = sync.Map{}
		
		room.SendMessage(common.Message{
			Type: common.MsgCancelGame,
			User: s.User.ID,
		})
		room.SetState(InternalStateSelectChart)
		room.OnStateChange()
	} else {
		room.SendMessage(common.Message{
			Type: common.MsgCancelReady,
			User: s.User.ID,
		})
	}

	return s.Send(common.ServerCommand{
		Type:              common.ServerCmdCancelReady,
		CancelReadyResult: &common.Result[struct{}]{Ok: &struct{}{}},
	})
}

// handlePlayed 处理游戏完成
func (s *Session) handlePlayed(recordID int32) error {
	room := s.User.GetRoom()
	if room == nil {
		return s.Send(common.ServerCommand{
			Type:         common.ServerCmdPlayed,
			PlayedResult: &common.Result[struct{}]{Err: strPtr("不在房间中")},
		})
	}

	if room.GetState() != InternalStatePlaying {
		return s.Send(common.ServerCommand{
			Type:         common.ServerCmdPlayed,
			PlayedResult: &common.Result[struct{}]{Err: strPtr("未在游戏中")},
		})
	}

	record, err := FetchRecord(recordID)
	if err != nil {
		return s.Send(common.ServerCommand{
			Type:         common.ServerCmdPlayed,
			PlayedResult: &common.Result[struct{}]{Err: strPtr("记录不存在")},
		})
	}

	if record.Player != s.User.ID {
		return s.Send(common.ServerCommand{
			Type:         common.ServerCmdPlayed,
			PlayedResult: &common.Result[struct{}]{Err: strPtr("无效记录")},
		})
	}

	// 检查是否已放弃
	if _, aborted := room.aborted.Load(s.User.ID); aborted {
		return s.Send(common.ServerCommand{
			Type:         common.ServerCmdPlayed,
			PlayedResult: &common.Result[struct{}]{Err: strPtr("已放弃")},
		})
	}

	// 检查是否已上传
	if _, hasResult := room.results.Load(s.User.ID); hasResult {
		return s.Send(common.ServerCommand{
			Type:         common.ServerCmdPlayed,
			PlayedResult: &common.Result[struct{}]{Err: strPtr("已上传")},
		})
	}

	room.results.Store(s.User.ID, record)
	room.SendMessage(common.Message{
		Type:      common.MsgPlayed,
		User:      s.User.ID,
		Score:     record.Score,
		Accuracy:  record.Accuracy,
		FullCombo: record.FullCombo,
	})

	// 更新回放文件的成绩ID
	if recorder := s.server.GetReplayRecorder(); recorder != nil {
		recorder.UpdateRecordID(room.ID.Value, s.User.ID, recordID)
	}

	room.CheckAllReady()

	return s.Send(common.ServerCommand{
		Type:         common.ServerCmdPlayed,
		PlayedResult: &common.Result[struct{}]{Ok: &struct{}{}},
	})
}

// handleAbort 处理放弃
func (s *Session) handleAbort() error {
	room := s.User.GetRoom()
	if room == nil {
		return s.Send(common.ServerCommand{
			Type:        common.ServerCmdAbort,
			AbortResult: &common.Result[struct{}]{Err: strPtr("不在房间中")},
		})
	}

	if room.GetState() != InternalStatePlaying {
		return s.Send(common.ServerCommand{
			Type:        common.ServerCmdAbort,
			AbortResult: &common.Result[struct{}]{Err: strPtr("未在游戏中")},
		})
	}

	// 检查是否已上传结果
	if _, hasResult := room.results.Load(s.User.ID); hasResult {
		return s.Send(common.ServerCommand{
			Type:        common.ServerCmdAbort,
			AbortResult: &common.Result[struct{}]{Err: strPtr("已上传")},
		})
	}

	// 检查是否已放弃
	if _, aborted := room.aborted.Load(s.User.ID); aborted {
		return s.Send(common.ServerCommand{
			Type:        common.ServerCmdAbort,
			AbortResult: &common.Result[struct{}]{Err: strPtr("已放弃")},
		})
	}

	room.aborted.Store(s.User.ID, true)
	room.SendMessage(common.Message{
		Type: common.MsgAbort,
		User: s.User.ID,
	})
	room.CheckAllReady()

	return s.Send(common.ServerCommand{
		Type:        common.ServerCmdAbort,
		AbortResult: &common.Result[struct{}]{Ok: &struct{}{}},
	})
}

func strPtr(s string) *string {
	return &s
}
