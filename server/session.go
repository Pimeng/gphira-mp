package server

import (
	"fmt"
	"log"
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
			log.Printf("Session %s recv error: %v", s.ID, err)
			s.handleDisconnect()
			return
		}

		s.lastPing = time.Now()

		// 记录接收到的命令类型
		log.Printf("Session %s received command: type=%d", s.ID, cmd.Type)

		if err := s.handleCommand(cmd); err != nil {
			log.Printf("Session %s handle command error: %v", s.ID, err)
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
				log.Printf("Session %s heartbeat timeout", s.ID)
				s.handleDisconnect()
				return
			}
		}
	}
}

// handleDisconnect 处理断开连接
func (s *Session) handleDisconnect() {
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
			return fmt.Errorf("not authenticated")
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
		log.Printf("Session %s unknown command type: %d (max valid: %d), disconnecting", s.ID, cmd.Type, common.ClientCmdAbort)
		// 发送错误响应
		s.Send(common.ServerCommand{
			Type: common.ServerCmdMessage,
			Message: &common.Message{
				Type:    common.MsgChat,
				User:    -1,
				Content: "Unknown command type",
			},
		})
		return fmt.Errorf("unknown command type: %d", cmd.Type)
	}
}

// handleAuthenticate 处理认证
func (s *Session) handleAuthenticate(token string) error {
	user, _, err := UserInfoFromAPI(token)
	if err != nil {
		s.Send(common.ServerCommand{
			Type: common.ServerCmdAuthenticate,
			AuthenticateResult: &common.Result[common.AuthResult]{
				Err: strPtr("authentication failed"),
			},
		})
		return err
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

// handleChat 处理聊天
func (s *Session) handleChat(message string) error {
	room := s.User.GetRoom()
	if room == nil {
		return s.Send(common.ServerCommand{
			Type:       common.ServerCmdChat,
			ChatResult: &common.Result[struct{}]{Err: strPtr("not in room")},
		})
	}

	room.SendMessage(common.Message{
		Type:    common.MsgChat,
		User:    s.User.ID,
		Content: message,
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
	return nil
}

// handleCreateRoom 处理创建房间
func (s *Session) handleCreateRoom(roomId common.RoomId) error {
	if s.User.GetRoom() != nil {
		return s.Send(common.ServerCommand{
			Type:             common.ServerCmdCreateRoom,
			CreateRoomResult: &common.Result[struct{}]{Err: strPtr("already in room")},
		})
	}

	if s.server.GetRoom(roomId) != nil {
		return s.Send(common.ServerCommand{
			Type:             common.ServerCmdCreateRoom,
			CreateRoomResult: &common.Result[struct{}]{Err: strPtr("room id occupied")},
		})
	}

	room := NewRoom(roomId, s.User, s.server)
	s.server.AddRoom(room)
	s.User.SetRoom(room)

	room.SendMessage(common.Message{
		Type: common.MsgCreateRoom,
		User: s.User.ID,
	})

	return s.Send(common.ServerCommand{
		Type:             common.ServerCmdCreateRoom,
		CreateRoomResult: &common.Result[struct{}]{Ok: &struct{}{}},
	})
}

// handleJoinRoom 处理加入房间
func (s *Session) handleJoinRoom(roomId common.RoomId, monitor bool) error {
	if s.User.GetRoom() != nil {
		return s.Send(common.ServerCommand{
			Type:           common.ServerCmdJoinRoom,
			JoinRoomResult: &common.Result[common.JoinRoomResponse]{Err: strPtr("already in room")},
		})
	}

	room := s.server.GetRoom(roomId)
	if room == nil {
		return s.Send(common.ServerCommand{
			Type:           common.ServerCmdJoinRoom,
			JoinRoomResult: &common.Result[common.JoinRoomResponse]{Err: strPtr("room not found")},
		})
	}

	if room.IsLocked() {
		return s.Send(common.ServerCommand{
			Type:           common.ServerCmdJoinRoom,
			JoinRoomResult: &common.Result[common.JoinRoomResponse]{Err: strPtr("room is locked")},
		})
	}

	if room.GetState() != InternalStateSelectChart {
		return s.Send(common.ServerCommand{
			Type:           common.ServerCmdJoinRoom,
			JoinRoomResult: &common.Result[common.JoinRoomResponse]{Err: strPtr("game ongoing")},
		})
	}

	if monitor && !s.User.CanMonitor() {
		return s.Send(common.ServerCommand{
			Type:           common.ServerCmdJoinRoom,
			JoinRoomResult: &common.Result[common.JoinRoomResponse]{Err: strPtr("can't monitor")},
		})
	}

	if !room.AddUser(s.User, monitor) {
		return s.Send(common.ServerCommand{
			Type:           common.ServerCmdJoinRoom,
			JoinRoomResult: &common.Result[common.JoinRoomResponse]{Err: strPtr("room is full")},
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
			LeaveRoomResult: &common.Result[struct{}]{Err: strPtr("not in room")},
		})
	}

	if room.OnUserLeave(s.User) {
		s.server.RemoveRoom(room.ID)
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
			LockRoomResult: &common.Result[struct{}]{Err: strPtr("not in room")},
		})
	}

	if err := room.CheckHost(s.User); err != nil {
		return s.Send(common.ServerCommand{
			Type:           common.ServerCmdLockRoom,
			LockRoomResult: &common.Result[struct{}]{Err: strPtr("only host can lock")},
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
			CycleRoomResult: &common.Result[struct{}]{Err: strPtr("not in room")},
		})
	}

	if err := room.CheckHost(s.User); err != nil {
		return s.Send(common.ServerCommand{
			Type:            common.ServerCmdCycleRoom,
			CycleRoomResult: &common.Result[struct{}]{Err: strPtr("only host can cycle")},
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
			SelectChartResult: &common.Result[struct{}]{Err: strPtr("not in room")},
		})
	}

	if room.GetState() != InternalStateSelectChart {
		return s.Send(common.ServerCommand{
			Type:              common.ServerCmdSelectChart,
			SelectChartResult: &common.Result[struct{}]{Err: strPtr("invalid state")},
		})
	}

	if err := room.CheckHost(s.User); err != nil {
		return s.Send(common.ServerCommand{
			Type:              common.ServerCmdSelectChart,
			SelectChartResult: &common.Result[struct{}]{Err: strPtr("only host can select")},
		})
	}

	chart, err := FetchChart(chartID)
	if err != nil {
		return s.Send(common.ServerCommand{
			Type:              common.ServerCmdSelectChart,
			SelectChartResult: &common.Result[struct{}]{Err: strPtr("chart not found")},
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
			RequestStartResult: &common.Result[struct{}]{Err: strPtr("not in room")},
		})
	}

	if room.GetState() != InternalStateSelectChart {
		return s.Send(common.ServerCommand{
			Type:               common.ServerCmdRequestStart,
			RequestStartResult: &common.Result[struct{}]{Err: strPtr("invalid state")},
		})
	}

	if err := room.CheckHost(s.User); err != nil {
		return s.Send(common.ServerCommand{
			Type:               common.ServerCmdRequestStart,
			RequestStartResult: &common.Result[struct{}]{Err: strPtr("only host can start")},
		})
	}

	if room.GetChart() == nil {
		return s.Send(common.ServerCommand{
			Type:               common.ServerCmdRequestStart,
			RequestStartResult: &common.Result[struct{}]{Err: strPtr("no chart selected")},
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
			ReadyResult: &common.Result[struct{}]{Err: strPtr("not in room")},
		})
	}

	if room.GetState() != InternalStateWaitForReady {
		return s.Send(common.ServerCommand{
			Type:        common.ServerCmdReady,
			ReadyResult: &common.Result[struct{}]{Err: strPtr("invalid state")},
		})
	}

	if _, loaded := room.started.LoadOrStore(s.User.ID, true); loaded {
		return s.Send(common.ServerCommand{
			Type:        common.ServerCmdReady,
			ReadyResult: &common.Result[struct{}]{Err: strPtr("already ready")},
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
			CancelReadyResult: &common.Result[struct{}]{Err: strPtr("not in room")},
		})
	}

	if room.GetState() != InternalStateWaitForReady {
		return s.Send(common.ServerCommand{
			Type:              common.ServerCmdCancelReady,
			CancelReadyResult: &common.Result[struct{}]{Err: strPtr("invalid state")},
		})
	}

	if _, ok := room.started.Load(s.User.ID); !ok {
		return s.Send(common.ServerCommand{
			Type:              common.ServerCmdCancelReady,
			CancelReadyResult: &common.Result[struct{}]{Err: strPtr("not ready")},
		})
	}

	room.started.Delete(s.User.ID)

	// 房主取消则取消游戏
	if room.GetHost().ID == s.User.ID {
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
			PlayedResult: &common.Result[struct{}]{Err: strPtr("not in room")},
		})
	}

	if room.GetState() != InternalStatePlaying {
		return s.Send(common.ServerCommand{
			Type:         common.ServerCmdPlayed,
			PlayedResult: &common.Result[struct{}]{Err: strPtr("not playing")},
		})
	}

	record, err := FetchRecord(recordID)
	if err != nil {
		return s.Send(common.ServerCommand{
			Type:         common.ServerCmdPlayed,
			PlayedResult: &common.Result[struct{}]{Err: strPtr("record not found")},
		})
	}

	if record.Player != s.User.ID {
		return s.Send(common.ServerCommand{
			Type:         common.ServerCmdPlayed,
			PlayedResult: &common.Result[struct{}]{Err: strPtr("invalid record")},
		})
	}

	// 检查是否已放弃
	if _, aborted := room.aborted.Load(s.User.ID); aborted {
		return s.Send(common.ServerCommand{
			Type:         common.ServerCmdPlayed,
			PlayedResult: &common.Result[struct{}]{Err: strPtr("already aborted")},
		})
	}

	// 检查是否已上传
	if _, hasResult := room.results.Load(s.User.ID); hasResult {
		return s.Send(common.ServerCommand{
			Type:         common.ServerCmdPlayed,
			PlayedResult: &common.Result[struct{}]{Err: strPtr("already uploaded")},
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
			AbortResult: &common.Result[struct{}]{Err: strPtr("not in room")},
		})
	}

	if room.GetState() != InternalStatePlaying {
		return s.Send(common.ServerCommand{
			Type:        common.ServerCmdAbort,
			AbortResult: &common.Result[struct{}]{Err: strPtr("not playing")},
		})
	}

	// 检查是否已上传结果
	if _, hasResult := room.results.Load(s.User.ID); hasResult {
		return s.Send(common.ServerCommand{
			Type:        common.ServerCmdAbort,
			AbortResult: &common.Result[struct{}]{Err: strPtr("already uploaded")},
		})
	}

	// 检查是否已放弃
	if _, aborted := room.aborted.Load(s.User.ID); aborted {
		return s.Send(common.ServerCommand{
			Type:        common.ServerCmdAbort,
			AbortResult: &common.Result[struct{}]{Err: strPtr("already aborted")},
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
