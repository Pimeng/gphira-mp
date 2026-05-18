package network

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/internal/l10n"
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
			s.State.Logger.DebugL(s.State.ServerLang, "log-auth-received", map[string]string{"session": s.ID, "remote": s.RemoteIP})
			return s.handleAuthenticate(cmd.Token)
		}
		s.State.Logger.DebugL(s.State.ServerLang, "log-command-before-auth", map[string]string{"session": s.ID, "cmd": fmt.Sprintf("%v", cmd.Type)})
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
	endpoint := s.State.Config.PhiraAPIEndpoint
	if endpoint == "" {
		endpoint = defaultPhiraAPIEndpoint
	}

	info, err := FetchPhiraUserInfo(endpoint, token, s.State.Config.OutboundProxy)
	if err != nil {
		s.State.Logger.DebugL(s.State.ServerLang, "log-auth-api-failed", map[string]string{"session": s.ID, "error": err.Error()})
		s.State.Logger.WarnL(s.State.ServerLang, "log-auth-failed", map[string]string{"session": s.ID, "error": err.Error()})
		_ = s.Send(protocol.ServerCommand{
			Type:       protocol.ServerCmdAuthenticate,
			AuthResult: protocol.Err[protocol.AuthenticateResult](err.Error()),
		})
		s.panicked = true
		s.markLost()
		return nil
	}

	var existingSession *Session
	var wasDangling bool
	s.State.WithLock(func() {
		if u := s.State.Users[info.ID]; u != nil {
			if sess, ok := u.Session.(*Session); ok && sess != s {
				existingSession = sess
			}
			wasDangling = u.Session == nil && u.DangleRoom() != nil
			u.SetSession(s)
			u.Name = info.Name
			u.Lang = l10n.New(info.Language)
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

	// If the user was dangling, restore room state broadcast to notify others.
	if wasDangling && s.user.Room != nil {
		room := s.user.Room
		s.State.Logger.InfoL(s.State.ServerLang, "log-user-reconnected", map[string]string{"session": s.ID, "user": s.user.Name, "room": string(room.ID)})
		_ = s.broadcastRoom(room, protocol.ServerCommand{Type: protocol.ServerCmdOnJoinRoom, UserInfo: s.user.ToInfo()})
		_ = s.broadcastRoomMessage(room, protocol.Message{Type: protocol.MessageJoinRoom, User: s.user.ID, Name: s.user.Name})
	}

	var clientRoom *protocol.ClientRoomState
	if s.user.Room != nil {
		users := s.collectUsers()
		crs := s.user.Room.ClientState(s.user.ID, users)
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

	s.State.Logger.InfoL(s.State.ServerLang, "log-user-authenticated", map[string]string{"session": s.ID, "user": info.Name, "id": fmt.Sprintf("%d", info.ID)})

	if clientRoom != nil {
		s.State.Logger.DebugL(s.State.ServerLang, "log-auth-restored-room", map[string]string{"session": s.ID, "user": info.Name, "room": string(s.user.Room.ID)})
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

func (s *Session) process(cmd protocol.ClientCommand) (protocol.ServerCommand, error) {
	ctx := &CommandContext{
		State: s.State,
		User:  s.user,
		MonitorBuffer: s.monitorBuf,
		RequireRoom: func() *game.Room {
			if s.user.Room == nil {
				panic(s.user.Lang.Format("room-no-room", nil))
			}
			return s.user.Room
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
			if msg.Type == protocol.MessageChat {
				room.AddLog(msg.Content)
				if s.State.WSServer != nil {
					s.State.WSServer.BroadcastRoomLog(room.ID, msg.Content, time.Now().UnixMilli())
				}
			}
			return nil
		},
		BroadcastToMonitors: func(room *game.Room, cmd protocol.ServerCommand) {
			for _, id := range room.MonitorIDs() {
				if u := s.findUser(id); u != nil {
					_ = u.TrySend(cmd)
				}
			}
		},
		ProcessCreateRoom: func(id roomid.RoomID) error {
			if s.user.Room != nil {
				return fmt.Errorf("%s", s.user.Lang.Format("room-already-in-room", nil))
			}
			if !s.State.RoomCreationEnabled {
				return fmt.Errorf("%s", s.user.Lang.Format("room-creation-disabled", nil))
			}
			maxUsers := s.State.Config.RoomMaxUsers
			if maxUsers < 1 || maxUsers > 64 {
				maxUsers = 8
			}
			room := game.NewRoom(id, s.user.ID, maxUsers, s.State.ReplayEnabled)
			s.State.WithLock(func() {
				if _, exists := s.State.Rooms[id]; exists {
					panic(s.user.Lang.Format("create-id-occupied", nil))
				}
				s.State.Rooms[id] = room
			})
			s.user.Room = room
			s.user.Monitor = false
			_ = s.broadcastRoomMessage(room, protocol.Message{Type: protocol.MessageCreateRoom, User: s.user.ID})
			s.State.Logger.LogRoomMark(s.State.ServerLang, room.ID, "log-room-created", map[string]string{"user": s.user.Name, "room": string(room.ID)})
			return nil
		},
		ProcessJoinRoom: func(id roomid.RoomID, monitor bool) (*protocol.JoinRoomResponse, error) {
			if s.user.Room != nil {
				return nil, fmt.Errorf("%s", s.user.Lang.Format("room-already-in-room", nil))
			}
			var room *game.Room
			s.State.WithRLock(func() {
			room = s.State.Rooms[id]
			})
			if room == nil {
				return nil, fmt.Errorf("%s", s.user.Lang.Format("room-not-found", nil))
			}
			if err := room.ValidateJoin(s.user.ID, monitor, s.State.Config.Monitors, room.State); err != nil {
				return nil, err
			}
			if !room.AddUser(s.user.ID, monitor) {
				return nil, fmt.Errorf("%s", s.user.Lang.Format("join-room-full", nil))
			}
			s.user.Room = room
			s.user.Monitor = monitor
			room.HandleJoin(s.user.ID, room.State)

			users := s.collectUsers()
			resp := protocol.JoinRoomResponse{
				State: room.ClientRoomState(),
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
			_ = s.broadcastRoomMessage(room, protocol.Message{Type: protocol.MessageJoinRoom, User: s.user.ID, Name: s.user.Name})

			suffix := ""
			if monitor {
				suffix = s.State.ServerLang.Format("label-monitor-suffix", nil)
			}
			s.State.Logger.LogRoomMark(s.State.ServerLang, room.ID, "log-room-joined", map[string]string{"user": s.user.Name, "suffix": suffix, "room": string(room.ID)})

			return &resp, nil
		},
		DisbandRoom: func(room *game.Room) {
			s.State.WithLock(func() {
				delete(s.State.Rooms, room.ID)
			})
		},
		CheckRoomAllReady: func(room *game.Room) {
			users := s.collectUsers()
			_ = room.CheckAllReady(&game.RoomCallbacks{
				UsersById: func(id int32) *game.User {
					return users[id]
				},
				Broadcast: func(cmd protocol.ServerCommand) error {
					for _, id := range room.AllParticipantIDs() {
						if u := users[id]; u != nil {
							_ = u.TrySend(cmd)
						}
					}
					return nil
				},
				BroadcastToMonitors: func(cmd protocol.ServerCommand) {
					for _, id := range room.MonitorIDs() {
						if u := users[id]; u != nil {
							_ = u.TrySend(cmd)
						}
					}
				},
				PickRandomUserId: utils.RandomPickInt32,
				Lang:   s.State.ServerLang,
				Logger: s.State.Logger,
				OnEnterPlaying: func(r *game.Room) {
					if s.State.ReplayEnabled && s.State.ReplayRecorder != nil && r.ReplayEligible {
						var participants []replay.Participant
						for _, uid := range r.UserIDs() {
							name := ""
							if u := users[uid]; u != nil {
								name = u.Name
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
					if s.State.ReplayEnabled && s.State.ReplayRecorder != nil && r.ReplayEligible {
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
				},
				NotifyWebSocket: func(rid roomid.RoomID) {
					if s.State.WSServer != nil {
						s.State.WSServer.BroadcastRoomUpdate(rid, nil)
					}
				},
			})
		},
		FetchChart: func(id int32) (*game.Chart, error) {
			if s.State.ChartCache != nil {
				if cached := s.State.ChartCache.Get(id); cached != nil {
					return &game.Chart{ID: int(cached.ID), Name: cached.Name}, nil
				}
			}
			endpoint := s.State.Config.PhiraAPIEndpoint
			if endpoint == "" {
				endpoint = defaultPhiraAPIEndpoint
			}
			chart, err := FetchPhiraChart(endpoint, id, s.State.Config.OutboundProxy)
			if err != nil {
				return nil, fmt.Errorf("%s", s.user.Lang.Format("chart-fetch-failed", nil))
			}
			if s.State.ChartCache != nil {
				s.State.ChartCache.Set(chart.ID, chart.Name)
			}
			return &game.Chart{ID: int(chart.ID), Name: chart.Name}, nil
		},
		FetchRecord: func(id int32) (*PhiraRecord, error) {
			endpoint := s.State.Config.PhiraAPIEndpoint
			if endpoint == "" {
				endpoint = defaultPhiraAPIEndpoint
			}
			record, err := FetchPhiraRecord(endpoint, id, s.State.Config.OutboundProxy)
			if err != nil {
				return nil, fmt.Errorf("%s", s.user.Lang.Format("record-fetch-failed", nil))
			}
			return record, nil
		},
	}
	return ProcessClientCommand(ctx, cmd)
}

// CheckHeartbeat checks if the session has timed out.
func (s *Session) CheckHeartbeat(now time.Time) {
	s.mu.Lock()
	if s.lost {
		s.mu.Unlock()
		return
	}
	if now.Sub(s.lastRecv) > heartbeatDisconnectTimeoutMs*time.Millisecond {
		s.mu.Unlock()
		s.State.Logger.DebugL(s.State.ServerLang, "log-heartbeat-timeout", map[string]string{"session": s.ID, "user": fmt.Sprintf("%d", s.userID())})
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
		return s.user.Name
	}
	return ""
}

func (s *Session) markLost() {
	s.mu.Lock()
	if s.lost {
		s.mu.Unlock()
		return
	}
	s.lost = true
	preserveRoom := s.preserveRoomOnLost
	s.mu.Unlock()

	s.State.Logger.DebugL(s.State.ServerLang, "log-session-marked-lost", map[string]string{"session": s.ID, "user": fmt.Sprintf("%d", s.userID()), "preserveRoom": fmt.Sprintf("%v", preserveRoom)})

	s.monitorBuf.Destroy()
	if s.stream != nil {
		s.stream.Close()
	}

	var user *game.User
	s.State.WithLock(func() {
		delete(s.State.Sessions, s.ID)
		if s.user != nil {
			s.user.SetSession(nil)
			user = s.user
		}
	})

	if user == nil {
		return
	}

	// If the user is banned, remove immediately without dangling.
	var isBanned bool
	s.State.WithRLock(func() {
		_, isBanned = s.State.BannedUsers[user.ID]
	})
	if isBanned {
		s.State.Logger.DebugL(s.State.ServerLang, "log-banned-user-disconnected", map[string]string{"session": s.ID, "user": fmt.Sprintf("%d", user.ID), "name": user.Name})
		s.handleUserLeaveAndRemove(user)
		return
	}

	// If currently playing, leave room immediately (no dangling).
	if user.Room != nil {
		if _, playing := user.Room.State.(*game.StatePlaying); playing {
			s.State.Logger.DebugL(s.State.ServerLang, "log-user-disconnected-playing", map[string]string{"session": s.ID, "user": fmt.Sprintf("%d", user.ID), "name": user.Name, "room": string(user.Room.ID)})
			s.handleUserLeaveAndRemove(user)
			return
		}
	}

	// If admin disconnect requested to preserve room (e.g. kicking stale session),
	// just mark dangling but do NOT leave the room.
	if preserveRoom {
		user.MarkDangle()
		s.scheduleDangleCleanup(user)
		return
	}

	// Normal disconnect: mark dangling, keep room association.
	user.MarkDangle()
	s.State.Logger.DebugL(s.State.ServerLang, "log-user-dangling", map[string]string{"session": s.ID, "user": fmt.Sprintf("%d", user.ID), "name": user.Name, "room": string(user.Room.ID)})
	s.scheduleDangleCleanup(user)
}

// handleUserLeaveAndRemove immediately removes the user from their room and
// deletes them from the global user map. Used for banned users and playing-state
// disconnects where dangling is not desired.
func (s *Session) handleUserLeaveAndRemove(user *game.User) {
	s.State.Logger.DebugL(s.State.ServerLang, "log-user-leave-remove", map[string]string{"session": s.ID, "user": fmt.Sprintf("%d", user.ID), "name": user.Name})
	if user.Room != nil {
		room := user.Room
		shouldDisband := room.OnUserLeave(user, &game.RoomCallbacks{
			Broadcast: func(cmd protocol.ServerCommand) error {
				for _, id := range room.AllParticipantIDs() {
					if u := s.findUser(id); u != nil {
						_ = u.TrySend(cmd)
					}
				}
				return nil
			},
			UsersById:      s.findUser,
			PickRandomUserId: utils.RandomPickInt32,
			Lang:           s.State.ServerLang,
			Logger:         s.State.Logger,
		})
		if shouldDisband {
			s.State.Logger.LogRoomInfo(s.State.ServerLang, room.ID, "log-room-recycled", map[string]string{"room": string(room.ID)})
			s.State.WithLock(func() {
				delete(s.State.Rooms, room.ID)
			})
		}
		user.Room = nil
	}
	s.State.WithLock(func() {
		delete(s.State.Users, user.ID)
	})
}

// scheduleDangleCleanup waits for the dangle timeout. If the user hasn't
// reconnected (session still nil), it performs the actual room leave and
// removes the user from the global map.
func (s *Session) scheduleDangleCleanup(user *game.User) {
	token := user.DangleToken()
	go func() {
		time.Sleep(dangleTimeoutMs * time.Millisecond)
		if !user.IsStillDangling(token) {
			return
		}
		// Double-check: if user reconnected, don't remove.
		if user.Session != nil {
			s.State.Logger.DebugL(s.State.ServerLang, "log-dangle-cleanup-skipped", map[string]string{"session": s.ID, "user": fmt.Sprintf("%d", user.ID), "name": user.Name})
			return
		}
		s.State.Logger.DebugL(s.State.ServerLang, "log-dangle-cleanup-started", map[string]string{"session": s.ID, "user": fmt.Sprintf("%d", user.ID), "name": user.Name})

		dangleRoom := user.DangleRoom()
		if dangleRoom != nil {
			room := dangleRoom
			s.State.Logger.DebugL(s.State.ServerLang, "log-dangle-cleanup-leaving", map[string]string{"session": s.ID, "user": fmt.Sprintf("%d", user.ID), "name": user.Name, "room": string(room.ID)})
			shouldDisband := room.OnUserLeave(user, &game.RoomCallbacks{
				Broadcast: func(cmd protocol.ServerCommand) error {
					for _, id := range room.AllParticipantIDs() {
						if u := s.findUser(id); u != nil {
							_ = u.TrySend(cmd)
						}
					}
					return nil
				},
				UsersById:      s.findUser,
				PickRandomUserId: utils.RandomPickInt32,
				Lang:           s.State.ServerLang,
				Logger:         s.State.Logger,
				NotifyWebSocket: func(rid roomid.RoomID) {
					if s.State.WSServer != nil {
						s.State.WSServer.BroadcastRoomUpdate(rid, nil)
					}
				},
			})
			if shouldDisband {
				s.State.Logger.LogRoomInfo(s.State.ServerLang, room.ID, "log-room-recycled", map[string]string{"room": string(room.ID)})
				s.State.WithLock(func() {
					delete(s.State.Rooms, room.ID)
				})
			}
			user.Room = nil
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
