// Per-command helpers extracted from Session.process() to keep that function
// focused on wiring the CommandContext. Each doXxx method holds the body of
// what was previously an inline closure inside process(). The runtime snapshot
// is taken once per process() call and threaded through these methods so that
// admin config edits cannot make different callbacks observe inconsistent state
// mid-command.
package network

import (
	"fmt"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/internal/state"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

// doBroadcastRoomMessage sends a Message to every participant in `room` and
// appends a formatted log line (if any) to the room's recent log and the
// WebSocket admin stream.
func (s *Session) doBroadcastRoomMessage(runtime state.RuntimeSnapshot, room *game.Room, msg protocol.Message) error {
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
}

// doBroadcastToMonitors fan-outs a server command to all monitors of `room` in a
// background goroutine. Panics inside the goroutine are recovered and logged.
func (s *Session) doBroadcastToMonitors(room *game.Room, cmd protocol.ServerCommand) {
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
}

func (s *Session) doProcessCreateRoom(runtime state.RuntimeSnapshot, id roomid.RoomID) error {
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
}

func (s *Session) doProcessJoinRoom(runtime state.RuntimeSnapshot, id roomid.RoomID, monitor bool) (*protocol.JoinRoomResponse, error) {
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
}

func (s *Session) doDisbandRoom(runtime state.RuntimeSnapshot, room *game.Room) {
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
}

func (s *Session) doFetchChart(runtime state.RuntimeSnapshot, id int32) (*game.Chart, error) {
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
}

func (s *Session) doFetchRecord(runtime state.RuntimeSnapshot, id int32) (*PhiraRecord, error) {
	endpoint := runtime.Config.PhiraAPIEndpoint
	if endpoint == "" {
		endpoint = defaultPhiraAPIEndpoint
	}
	record, err := FetchPhiraRecord(endpoint, id, runtime.Config.OutboundProxy)
	if err != nil {
		return nil, fmt.Errorf("%s", s.user.GetLang().Format("record-fetch-failed", nil))
	}
	return record, nil
}
