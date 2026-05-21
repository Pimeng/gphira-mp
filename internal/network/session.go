package network

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/internal/replay"
	"github.com/Pimeng/gphira-mp-next/internal/state"
	"github.com/Pimeng/gphira-mp-next/internal/utils"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
	"github.com/Pimeng/gphira-mp-next/pkg/stream"
)

const (
	heartbeatDisconnectTimeoutMs = 10000
	dangleTimeoutMs              = 10000
)

// Session represents a client connection session.
type Session struct {
	ID       string
	Conn     net.Conn
	State    *state.ServerState
	RemoteIP string

	mu                 sync.RWMutex
	stream             *stream.Stream[protocol.ServerCommand, protocol.ClientCommand]
	user               *game.User
	lastRecv           time.Time
	waitingAuth        bool
	panicked           bool
	lost               bool
	preserveRoomOnLost bool
	monitorBuf         *MonitorBuffer
}

// NewSession creates a new session.
func NewSession(id string, conn net.Conn, state *state.ServerState, remoteIP string) *Session {
	sess := &Session{
		ID:          id,
		Conn:        conn,
		State:       state,
		RemoteIP:    remoteIP,
		waitingAuth: true,
		lastRecv:    time.Now(),
	}
	sess.monitorBuf = NewMonitorBuffer(func(player int32, frames []protocol.TouchFrame, judges []protocol.JudgeEvent, ids []int32) {
		if len(frames) > 0 {
			cmd := protocol.ServerCommand{Type: protocol.ServerCmdTouches, Player: player, Frames: frames}
			for _, mid := range ids {
				if u := sess.findUser(mid); u != nil {
					_ = u.TrySend(cmd)
				}
			}
		}
		if len(judges) > 0 {
			cmd := protocol.ServerCommand{Type: protocol.ServerCmdJudges, Player: player, JudgeEvents: judges}
			for _, mid := range ids {
				if u := sess.findUser(mid); u != nil {
					_ = u.TrySend(cmd)
				}
			}
		}
	})
	return sess
}

func (s *Session) findUser(id int32) *game.User {
	var u *game.User
	s.State.WithRLock(func() {
		u = s.State.Users[id]
	})
	return u
}

// BindStream binds the negotiated stream to this session.
func (s *Session) BindStream(strm *stream.Stream[protocol.ServerCommand, protocol.ClientCommand]) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stream = strm
}

// Send sends a command to the client (implements game.SessionSender).
func (s *Session) Send(cmd protocol.ServerCommand) bool {
	s.mu.RLock()
	stream := s.stream
	s.mu.RUnlock()
	if stream == nil {
		return false
	}
	if err := stream.Send(cmd); err != nil {
		return false
	}
	return true
}

// OnCommand handles an incoming client command.
func (s *Session) OnCommand(cmd protocol.ClientCommand) error {
	runtime := s.State.SnapshotRuntime()
	s.mu.Lock()
	if s.lost || s.panicked {
		s.mu.Unlock()
		return nil
	}
	s.lastRecv = time.Now()

	if cmd.Type == protocol.ClientCmdPing {
		s.mu.Unlock()
		_ = s.Send(protocol.ServerCommand{Type: protocol.ServerCmdPong})
		return nil
	}

	if s.waitingAuth {
		s.mu.Unlock()
		if cmd.Type == protocol.ClientCmdAuthenticate {
			s.State.Logger.DebugL(runtime.ServerLang, "log-auth-received", map[string]string{"session": s.ID, "remote": s.RemoteIP})
			return s.handleAuthenticate(cmd.Token)
		}
		s.State.Logger.DebugL(runtime.ServerLang, "log-command-before-auth", map[string]string{"session": s.ID, "cmd": fmt.Sprintf("%v", cmd.Type)})
		return nil
	}
	s.mu.Unlock()

	if s.user == nil {
		return nil
	}

	resp, err := s.process(cmd)
	if err != nil {
		return err
	}
	if resp.Type != 0 {
		_ = s.Send(resp)
	}
	return nil
}

func (s *Session) handleAuthenticate(token string) error {
	runtime := s.State.SnapshotRuntime()
	endpoint := runtime.Config.PhiraAPIEndpoint
	if endpoint == "" {
		endpoint = defaultPhiraAPIEndpoint
	}

	info, err := FetchPhiraUserInfo(endpoint, token, runtime.Config.OutboundProxy)
	if err != nil {
		s.State.Logger.DebugL(runtime.ServerLang, "log-auth-api-failed", map[string]string{"session": s.ID, "error": err.Error()})
		s.State.Logger.WarnL(runtime.ServerLang, "log-auth-failed", map[string]string{"session": s.ID, "error": err.Error()})
		_ = s.Send(protocol.ServerCommand{
			Type:       protocol.ServerCmdAuthenticate,
			AuthResult: protocol.Err[protocol.AuthenticateResult](err.Error()),
		})
		s.panicked = true
		s.markLost()
		return nil
	}

	var existingSession *Session
	s.State.WithLock(func() {
		if u := s.State.Users[info.ID]; u != nil {
			if sess, ok := u.GetSession().(*Session); ok && sess != s {
				existingSession = sess
			}
			u.SetSession(s)
			u.SetIdentity(info.Name, info.Language)
			s.user = u
		}
		if s.user == nil {
			u := game.NewUser(info.ID, info.Name, info.Language)
			u.SetSession(s)
			s.user = u
			s.State.Users[info.ID] = u
		}
	})

	if existingSession != nil {
		existingSession.adminDisconnect(true)
	}

	var isBanned bool
	s.State.WithRLock(func() {
		_, isBanned = s.State.BannedUsers[s.user.ID]
	})
	if isBanned && s.user.GetRoom() != nil {
		s.handleUserLeaveRoom(s.user, s.user.GetRoom())
	}

	var clientRoom *protocol.ClientRoomState
	if s.user.GetRoom() != nil {
		users := s.collectUsers()
		crs := s.user.GetRoom().ClientState(s.user.ID, users)
		clientRoom = &crs
	}

	authRes := protocol.AuthenticateResult{
		Me:   s.user.ToInfo(),
		Room: clientRoom,
	}
	_ = s.Send(protocol.ServerCommand{
		Type:       protocol.ServerCmdAuthenticate,
		AuthResult: protocol.Ok(authRes),
	})

	s.mu.Lock()
	s.waitingAuth = false
	s.mu.Unlock()

	SendWelcomeExtras(s.user, s.State, s.sendSystemChat)

	s.State.Logger.InfoL(runtime.ServerLang, "log-user-authenticated", map[string]string{"session": s.ID, "user": info.Name, "id": fmt.Sprintf("%d", info.ID)})

	if clientRoom != nil {
		s.State.Logger.DebugL(runtime.ServerLang, "log-auth-restored-room", map[string]string{"session": s.ID, "user": info.Name, "room": string(s.user.GetRoom().ID)})
	}
	return nil
}

func (s *Session) collectUsers() map[int32]*game.User {
	users := make(map[int32]*game.User)
	s.State.WithRLock(func() {
		for id, u := range s.State.Users {
			users[id] = u
		}
	})
	return users
}

func (s *Session) checkRoomAllReady(room *game.Room) {
	runtime := s.State.SnapshotRuntime()
	users := s.collectUsers()
	participantIDs := room.AllParticipantIDs()
	userIDs := room.UserIDs()
	monitorIDs := room.MonitorIDs()
	_ = room.CheckAllReady(&game.RoomCallbacks{
		UsersById: func(id int32) *game.User {
			return users[id]
		},
		Broadcast: func(cmd protocol.ServerCommand) error {
			for _, id := range participantIDs {
				if u := users[id]; u != nil {
					_ = u.TrySend(cmd)
				}
			}
			return nil
		},
		BroadcastToMonitors: func(cmd protocol.ServerCommand) {
			for _, id := range monitorIDs {
				if u := users[id]; u != nil {
					_ = u.TrySend(cmd)
				}
			}
		},
		PickRandomUserId: utils.RandomPickInt32,
		Lang:             runtime.ServerLang,
		Logger:           s.State.Logger,
		OnEnterPlaying: func(r *game.Room) {
			if runtime.ReplayEnabled && s.State.ReplayRecorder != nil && r.ReplayEligible {
				var participants []replay.Participant
				for _, uid := range userIDs {
					name := ""
					if u := users[uid]; u != nil {
						name = u.GetName()
					}
					participants = append(participants, replay.Participant{ID: uid, Name: name})
				}
				chartID := 0
				chartName := ""
				if r.Chart != nil {
					chartID = r.Chart.ID
					chartName = r.Chart.Name
				}
				s.State.ReplayRecorder.StartRoom(r.ID, chartID, chartName, participants)
			}
		},
		OnGameEnd: func(r *game.Room) {
			if runtime.ReplayEnabled && s.State.ReplayRecorder != nil && r.ReplayEligible {
				s.State.ReplayRecorder.EndRoom(r.ID)
			}
			if s.State.AutoUploadCallback != nil && r.Chart != nil {
				if playing, ok := r.State.(*game.StatePlaying); ok {
					chartID := int32(r.Chart.ID)
					var roomFiles []replay.FileInfo
					if s.State.ReplayRecorder != nil {
						roomFiles = s.State.ReplayRecorder.ListRoomFiles(r.ID)
					}
					for userID, recordData := range playing.Results {
						for _, fi := range roomFiles {
							if fi.UserID == userID {
								s.State.AutoUploadCallback(userID, chartID, fi.Timestamp, recordData.ID)
								break
							}
						}
					}
				}
			}
			if s.State.ReplayRecorder != nil {
				s.State.ReplayRecorder.ClearRoomFiles(r.ID)
			}
		},
		NotifyWebSocket: func(rid roomid.RoomID) {
			if s.State.WSServer != nil {
				s.State.WSServer.BroadcastRoomUpdate(rid, nil)
			}
		},
		DisbandRoom: func(r *game.Room) {
			s.State.WithLock(func() {
				delete(s.State.Rooms, r.ID)
				for _, u := range s.State.Users {
					if u.GetRoom() == r {
						u.ClearRoom()
					}
				}
			})
			if runtime.ReplayEnabled && s.State.ReplayRecorder != nil {
				s.State.ReplayRecorder.EndRoom(r.ID)
			}
		},
	})
}

func clampRoomMaxUsers(v int) int {
	if v == 0 {
		return 8
	}
	if v < 1 {
		return 1
	}
	if v > 64 {
		return 64
	}
	return v
}

func (s *Session) sendFakeMonitorJoin(targetUser *game.User, room *game.Room) {
	runtime := s.State.SnapshotRuntime()
	if !runtime.ReplayEnabled || !room.ReplayEligible || s.State.ReplayRecorder == nil {
		if s.State.Logger != nil {
			s.State.Logger.Debug(fmt.Sprintf("sendFakeMonitorJoin skipped: replayEnabled=%v replayEligible=%v recorder=%v",
				runtime.ReplayEnabled, room.ReplayEligible, s.State.ReplayRecorder != nil))
		}
		return
	}
	fake := protocol.UserInfo{
		ID:      2000000000,
		Name:    runtime.ServerLang.Format("replay-recorder-name", nil),
		Monitor: true,
	}
	go func() {
		// 10ms 后向房主单播一个伪造的 monitor 加入事件——Phira 客户端据此把
		// 本地 room.live 翻为 true，从而开始上报 touches/judges。
		time.Sleep(10 * time.Millisecond)
		if targetUser.GetRoom() != room {
			if s.State.Logger != nil {
				s.State.Logger.Debug(fmt.Sprintf("sendFakeMonitorJoin aborted: user %d no longer in room %s", targetUser.ID, room.ID))
			}
			return
		}
		_ = targetUser.TrySend(protocol.ServerCommand{Type: protocol.ServerCmdOnJoinRoom, UserInfo: fake})
		_ = targetUser.TrySend(protocol.ServerCommand{
			Type:    protocol.ServerCmdMessage,
			Message: protocol.Message{Type: protocol.MessageJoinRoom, User: fake.ID, Name: fake.Name},
		})
		if s.State.Logger != nil {
			s.State.Logger.Debug(fmt.Sprintf("sendFakeMonitorJoin sent to user %d room %s", targetUser.ID, room.ID))
		}

		// 再 10ms 后单播一个伪造的 monitor 离开事件，清理客户端房间视图中
		// 的占位用户。Phira 一旦确认过 live 状态就不会再因为离开而停发数据。
		time.Sleep(10 * time.Millisecond)
		if targetUser.GetRoom() != room {
			return
		}
		_ = targetUser.TrySend(protocol.ServerCommand{
			Type:    protocol.ServerCmdMessage,
			Message: protocol.Message{Type: protocol.MessageLeaveRoom, User: fake.ID, Name: fake.Name},
		})
		if s.State.Logger != nil {
			s.State.Logger.Debug(fmt.Sprintf("sendFakeMonitorLeave sent to user %d room %s", targetUser.ID, room.ID))
		}
	}()
}

func (s *Session) process(cmd protocol.ClientCommand) (protocol.ServerCommand, error) {
	runtime := s.State.SnapshotRuntime()
	ctx := &CommandContext{
		State:         s.State,
		User:          s.user,
		MonitorBuffer: s.monitorBuf,
		RequireRoom: func() *game.Room {
			if s.user.GetRoom() == nil {
				panic(s.user.GetLang().Format("room-no-room", nil))
			}
			return s.user.GetRoom()
		},
		BroadcastRoom: func(room *game.Room, cmd protocol.ServerCommand) error {
			for _, id := range room.AllParticipantIDs() {
				if u := s.findUser(id); u != nil {
					_ = u.TrySend(cmd)
				}
			}
			return nil
		},
		BroadcastRoomMessage: func(room *game.Room, msg protocol.Message) error {
			cmd := protocol.ServerCommand{Type: protocol.ServerCmdMessage, Message: msg}
			for _, id := range room.AllParticipantIDs() {
				if u := s.findUser(id); u != nil {
					_ = u.TrySend(cmd)
				}
			}
			logText := game.FormatMessageForLog(msg, runtime.ServerLang, func(id int32) string {
				if u := s.findUser(id); u != nil {
					return u.GetName()
				}
				return ""
			})
			if logText != "" {
				room.AddLog(logText)
				if s.State.WSServer != nil {
					s.State.WSServer.BroadcastRoomLog(room.ID, logText, time.Now().UnixMilli())
				}
			}
			return nil
		},
		BroadcastToMonitors: func(room *game.Room, cmd protocol.ServerCommand) {
			ids := room.MonitorIDs()
			if len(ids) == 0 {
				return
			}
			go func() {
				defer func() {
					if rec := recover(); rec != nil && s.State != nil && s.State.Logger != nil {
						s.State.Logger.Error("broadcast monitors panic", "err", fmt.Sprintf("%v", rec))
					}
				}()
				for _, id := range ids {
					if u := s.findUser(id); u != nil {
						_ = u.TrySend(cmd)
					}
				}
			}()
		},
		ProcessCreateRoom: func(id roomid.RoomID) error {
			var isBanned bool
			s.State.WithRLock(func() {
				_, isBanned = s.State.BannedUsers[s.user.ID]
			})
			if isBanned {
				return fmt.Errorf("%s", s.user.GetLang().Format("user-banned-by-server", nil))
			}
			if !s.State.RoomCreationEnabled {
				return fmt.Errorf("%s", s.user.GetLang().Format("room-creation-disabled", nil))
			}
			if s.user.GetRoom() != nil {
				return fmt.Errorf("%s", s.user.GetLang().Format("room-already-in-room", nil))
			}
			maxUsers := clampRoomMaxUsers(runtime.Config.RoomMaxUsers)
			room := game.NewRoom(id, s.user.ID, maxUsers, runtime.ReplayEnabled)
			s.State.WithLock(func() {
				if _, exists := s.State.Rooms[id]; exists {
					panic(s.user.GetLang().Format("create-id-occupied", nil))
				}
				s.State.Rooms[id] = room
				s.user.SetRoom(room, false)
			})
			room.RefreshLive(runtime.ReplayEnabled)
			_ = s.broadcastRoomMessage(room, protocol.Message{Type: protocol.MessageCreateRoom, User: s.user.ID})
			s.sendFakeMonitorJoin(s.user, room)
			s.State.Logger.LogRoomMark(runtime.ServerLang, room.ID, "log-room-created", map[string]string{"user": s.user.GetName(), "room": string(room.ID)})
			return nil
		},
		ProcessJoinRoom: func(id roomid.RoomID, monitor bool) (*protocol.JoinRoomResponse, error) {
			var globalBanned bool
			s.State.WithRLock(func() {
				_, globalBanned = s.State.BannedUsers[s.user.ID]
			})
			if globalBanned {
				return nil, fmt.Errorf("%s", s.user.GetLang().Format("user-banned-by-server", nil))
			}
			if s.user.GetRoom() != nil {
				return nil, fmt.Errorf("%s", s.user.GetLang().Format("room-already-in-room", nil))
			}
			var roomBanned bool
			s.State.WithRLock(func() {
				if bannedRoom, ok := s.State.BannedRoomUsers[id]; ok {
					_, roomBanned = bannedRoom[s.user.ID]
				}
			})
			if roomBanned {
				return nil, fmt.Errorf("%s", s.user.GetLang().Format("room-banned", map[string]string{"id": string(id)}))
			}
			var room *game.Room
			s.State.WithRLock(func() {
				room = s.State.Rooms[id]
			})
			if room == nil {
				return nil, fmt.Errorf("%s", s.user.GetLang().Format("room-not-found", nil))
			}
			if err := room.ValidateJoin(s.user.ID, monitor, runtime.Config.Monitors, room.State); err != nil {
				return nil, err
			}
			if !room.AddUser(s.user.ID, monitor) {
				return nil, fmt.Errorf("%s", s.user.GetLang().Format("join-room-full", nil))
			}
			s.user.SetRoom(room, monitor)
			room.OnUserJoin(s.user.ID, monitor)
			room.RefreshLive(runtime.ReplayEnabled)

			users := s.collectUsers()
			// ProtocolHack: if room is not in SelectChart state but has a chart,
			// respond with SelectChart so client gets chart info first, then send real state.
			respState := room.ClientRoomState()
			if _, isSelectChart := room.State.(*game.StateSelectChart); !isSelectChart && room.Chart != nil {
				chartID := int32(room.Chart.ID)
				respState = protocol.RoomState{Type: protocol.RoomStateSelectChart, ChartID: &chartID}
			}
			resp := protocol.JoinRoomResponse{
				State: respState,
				Users: make([]protocol.UserInfo, 0),
				Live:  room.Live,
			}
			for _, pid := range room.AllParticipantIDs() {
				if u, ok := users[pid]; ok {
					resp.Users = append(resp.Users, u.ToInfo())
				}
			}

			// Broadcast OnJoinRoom to existing participants, then JoinRoom message
			_ = s.broadcastRoom(room, protocol.ServerCommand{Type: protocol.ServerCmdOnJoinRoom, UserInfo: s.user.ToInfo()})
			_ = s.broadcastRoomMessage(room, protocol.Message{Type: protocol.MessageJoinRoom, User: s.user.ID, Name: s.user.GetName()})

			suffix := ""
			if monitor {
				suffix = runtime.ServerLang.Format("label-monitor-suffix", nil)
			}
			s.State.Logger.LogRoomMark(runtime.ServerLang, room.ID, "log-room-joined", map[string]string{"user": s.user.GetName(), "suffix": suffix, "room": string(room.ID)})

			// ProtocolHack: schedule deferred real-state correction for client
			if _, isSelectChart := room.State.(*game.StateSelectChart); !isSelectChart && room.Chart != nil {
				realState := room.ClientRoomState()
				chartID := int32(room.Chart.ID)
				go func() {
					time.Sleep(2 * time.Millisecond)
					_ = s.user.TrySend(protocol.ServerCommand{
						Type:  protocol.ServerCmdChangeState,
						State: protocol.RoomState{Type: protocol.RoomStateSelectChart, ChartID: &chartID},
					})
					time.Sleep(2 * time.Millisecond)
					_ = s.user.TrySend(protocol.ServerCommand{
						Type:  protocol.ServerCmdChangeState,
						State: realState,
					})
				}()
			}

			s.sendFakeMonitorJoin(s.user, room)
			return &resp, nil
		},
		DisbandRoom: func(room *game.Room) {
			s.State.WithLock(func() {
				delete(s.State.Rooms, room.ID)
				for _, u := range s.State.Users {
					if u.GetRoom() == room {
						u.ClearRoom()
					}
				}
			})
			if runtime.ReplayEnabled && s.State.ReplayRecorder != nil {
				s.State.ReplayRecorder.EndRoom(room.ID)
			}
		},
		CheckRoomAllReady: s.checkRoomAllReady,
		FetchChart: func(id int32) (*game.Chart, error) {
			if s.State.ChartCache != nil {
				if cached := s.State.ChartCache.Get(id); cached != nil {
					return &game.Chart{ID: int(cached.ID), Name: cached.Name}, nil
				}
			}
			endpoint := runtime.Config.PhiraAPIEndpoint
			if endpoint == "" {
				endpoint = defaultPhiraAPIEndpoint
			}
			chart, err := FetchPhiraChart(endpoint, id, runtime.Config.OutboundProxy)
			if err != nil {
				return nil, fmt.Errorf("%s", s.user.GetLang().Format("chart-fetch-failed", nil))
			}
			if s.State.ChartCache != nil {
				s.State.ChartCache.Set(chart.ID, chart.Name)
			}
			return &game.Chart{ID: int(chart.ID), Name: chart.Name}, nil
		},
		FetchRecord: func(id int32) (*PhiraRecord, error) {
			endpoint := runtime.Config.PhiraAPIEndpoint
			if endpoint == "" {
				endpoint = defaultPhiraAPIEndpoint
			}
			record, err := FetchPhiraRecord(endpoint, id, runtime.Config.OutboundProxy)
			if err != nil {
				return nil, fmt.Errorf("%s", s.user.GetLang().Format("record-fetch-failed", nil))
			}
			return record, nil
		},
	}
	return ProcessClientCommand(ctx, cmd)
}

// CheckHeartbeat checks if the session has timed out.
func (s *Session) CheckHeartbeat(now time.Time) {
	runtime := s.State.SnapshotRuntime()
	s.mu.Lock()
	if s.lost {
		s.mu.Unlock()
		return
	}
	// If the underlying stream is already closed (e.g. client killed process,
	// network dropped, or TCP RST), mark lost immediately instead of waiting
	// for the heartbeat timeout.
	if s.stream != nil && s.stream.IsClosed() {
		s.mu.Unlock()
		s.State.Logger.DebugL(runtime.ServerLang, "log-stream-closed", map[string]string{"session": s.ID, "user": fmt.Sprintf("%d", s.userID())})
		s.markLost()
		return
	}
	if now.Sub(s.lastRecv) > heartbeatDisconnectTimeoutMs*time.Millisecond {
		s.mu.Unlock()
		s.State.Logger.DebugL(runtime.ServerLang, "log-heartbeat-timeout", map[string]string{"session": s.ID, "user": fmt.Sprintf("%d", s.userID())})
		s.markLost()
		return
	}
	s.mu.Unlock()
}

func (s *Session) userID() int32 {
	if s.user != nil {
		return s.user.ID
	}
	return 0
}

func (s *Session) userName() string {
	if s.user != nil {
		return s.user.GetName()
	}
	return ""
}

func (s *Session) markLost() {
	runtime := s.State.SnapshotRuntime()
	s.mu.Lock()
	if s.lost {
		s.mu.Unlock()
		return
	}
	s.lost = true
	preserveRoom := s.preserveRoomOnLost
	s.mu.Unlock()

	s.State.Logger.DebugL(runtime.ServerLang, "log-session-marked-lost", map[string]string{"session": s.ID, "user": fmt.Sprintf("%d", s.userID()), "preserveRoom": fmt.Sprintf("%v", preserveRoom)})

	s.monitorBuf.Destroy()
	if s.stream != nil {
		s.stream.Close()
	}

	var user *game.User
	detachedUserSession := false
	s.State.WithLock(func() {
		delete(s.State.Sessions, s.ID)
		if s.user != nil {
			user = s.user
			if user.GetSession() == s {
				user.SetSession(nil)
				detachedUserSession = true
			}
		}
	})

	if user == nil || !detachedUserSession {
		return
	}

	// If the user is banned, remove immediately without dangling.
	var isBanned bool
	s.State.WithRLock(func() {
		_, isBanned = s.State.BannedUsers[user.ID]
	})
	if isBanned {
		s.State.Logger.DebugL(runtime.ServerLang, "log-banned-user-disconnected", map[string]string{"session": s.ID, "user": fmt.Sprintf("%d", user.ID), "name": user.GetName()})
		s.handleUserLeaveAndRemove(user)
		return
	}

	// If currently playing, leave room immediately (no dangling).
	if user.GetRoom() != nil {
		if _, playing := user.GetRoom().State.(*game.StatePlaying); playing {
			s.State.Logger.DebugL(runtime.ServerLang, "log-user-disconnected-playing", map[string]string{"session": s.ID, "user": fmt.Sprintf("%d", user.ID), "name": user.GetName(), "room": string(user.GetRoom().ID)})
			s.handleUserLeaveAndRemove(user)
			return
		}
	}

	// If admin disconnect requested to preserve room (e.g. kicking stale session),
	// leave the user attached to its room without scheduling cleanup.
	if preserveRoom {
		return
	}

	// Normal disconnect: mark dangling, keep room association.
	user.MarkDangle()
	roomID := ""
	if user.GetRoom() != nil {
		roomID = string(user.GetRoom().ID)
	}
	s.State.Logger.DebugL(runtime.ServerLang, "log-user-dangling", map[string]string{"session": s.ID, "user": fmt.Sprintf("%d", user.ID), "name": user.GetName(), "room": roomID})
	s.scheduleDangleCleanup(user)
}

func (s *Session) handleUserLeaveRoom(user *game.User, room *game.Room) {
	runtime := s.State.SnapshotRuntime()
	participantIDs := room.AllParticipantIDs()
	shouldDisband := room.OnUserLeave(user, &game.RoomCallbacks{
		Broadcast: func(cmd protocol.ServerCommand) error {
			for _, id := range participantIDs {
				if u := s.findUser(id); u != nil {
					_ = u.TrySend(cmd)
				}
			}
			return nil
		},
		UsersById:        s.findUser,
		PickRandomUserId: utils.RandomPickInt32,
		Lang:             runtime.ServerLang,
		Logger:           s.State.Logger,
		NotifyWebSocket: func(rid roomid.RoomID) {
			if s.State.WSServer != nil {
				s.State.WSServer.BroadcastRoomUpdate(rid, nil)
			}
		},
	})
	if shouldDisband {
		s.State.Logger.LogRoomInfo(runtime.ServerLang, room.ID, "log-room-recycled", map[string]string{"room": string(room.ID)})
		s.State.WithLock(func() {
			delete(s.State.Rooms, room.ID)
		})
		return
	}
	room.RefreshLive(runtime.ReplayEnabled)
	s.checkRoomAllReady(room)
}

// handleUserLeaveAndRemove immediately removes the user from their room and
// deletes them from the global user map. Used for banned users and playing-state
// disconnects where dangling is not desired.
func (s *Session) handleUserLeaveAndRemove(user *game.User) {
	runtime := s.State.SnapshotRuntime()
	s.State.Logger.DebugL(runtime.ServerLang, "log-user-leave-remove", map[string]string{"session": s.ID, "user": fmt.Sprintf("%d", user.ID), "name": user.GetName()})
	if user.GetRoom() != nil {
		room := user.GetRoom()
		s.handleUserLeaveRoom(user, room)
	}
	s.State.WithLock(func() {
		delete(s.State.Users, user.ID)
	})
}

// scheduleDangleCleanup waits for the dangle timeout. If the user hasn't
// reconnected (session still nil), it performs the actual room leave and
// removes the user from the global map.
func (s *Session) scheduleDangleCleanup(user *game.User) {
	runtime := s.State.SnapshotRuntime()
	token := user.DangleToken()
	go func() {
		time.Sleep(dangleTimeoutMs * time.Millisecond)
		if !user.IsStillDangling(token) {
			return
		}
		// Double-check: if user reconnected, don't remove.
		if user.HasSession() {
			s.State.Logger.DebugL(runtime.ServerLang, "log-dangle-cleanup-skipped", map[string]string{"session": s.ID, "user": fmt.Sprintf("%d", user.ID), "name": user.GetName()})
			return
		}
		s.State.Logger.DebugL(runtime.ServerLang, "log-dangle-cleanup-started", map[string]string{"session": s.ID, "user": fmt.Sprintf("%d", user.ID), "name": user.GetName()})

		dangleRoom := user.DangleRoom()
		if dangleRoom != nil {
			room := dangleRoom
			s.State.Logger.DebugL(runtime.ServerLang, "log-dangle-cleanup-leaving", map[string]string{"session": s.ID, "user": fmt.Sprintf("%d", user.ID), "name": user.GetName(), "room": string(room.ID)})
			s.handleUserLeaveRoom(user, room)
		}

		s.State.WithLock(func() {
			delete(s.State.Users, user.ID)
		})
	}()
}

func (s *Session) adminDisconnect(preserveRoom bool) {
	s.mu.Lock()
	if preserveRoom {
		s.preserveRoomOnLost = true
	}
	s.mu.Unlock()
	s.markLost()
}

func (s *Session) broadcastRoom(room *game.Room, cmd protocol.ServerCommand) error {
	for _, id := range room.AllParticipantIDs() {
		if u := s.findUser(id); u != nil {
			_ = u.TrySend(cmd)
		}
	}
	return nil
}

func (s *Session) broadcastRoomMessage(room *game.Room, msg protocol.Message) error {
	cmd := protocol.ServerCommand{Type: protocol.ServerCmdMessage, Message: msg}
	for _, id := range room.AllParticipantIDs() {
		if u := s.findUser(id); u != nil {
			_ = u.TrySend(cmd)
		}
	}
	return nil
}

// MarkLost marks the session as lost (public for admin operations).
func (s *Session) MarkLost() {
	s.markLost()
}

// IsLost returns true if the session is lost.
func (s *Session) IsLost() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lost
}
