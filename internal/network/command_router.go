package network

import (
	"fmt"

	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/internal/state"
	"github.com/Pimeng/gphira-mp-next/internal/utils"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

// CommandContext holds dependencies for command routing.
type CommandContext struct {
	State                *state.ServerState
	User                 *game.User
	MonitorBuffer        *MonitorBuffer
	RequireRoom          func() *game.Room
	BroadcastRoom        func(*game.Room, protocol.ServerCommand) error
	BroadcastRoomMessage func(*game.Room, protocol.Message) error
	BroadcastToMonitors  func(*game.Room, protocol.ServerCommand)
	ProcessCreateRoom    func(id roomid.RoomID) error
	ProcessJoinRoom      func(id roomid.RoomID, monitor bool) (*protocol.JoinRoomResponse, error)
	DisbandRoom          func(*game.Room)
	CheckRoomAllReady    func(*game.Room)
	FetchChart           func(id int32) (*game.Chart, error)
	FetchRecord          func(id int32) (*PhiraRecord, error)
}

// ProcessClientCommand routes a client command to its handler.
func ProcessClientCommand(ctx *CommandContext, cmd protocol.ClientCommand) (protocol.ServerCommand, error) {
	st := ctx.State
	user := ctx.User

	switch cmd.Type {
	case protocol.ClientCmdAuthenticate:
		return protocol.ServerCommand{
			Type:       protocol.ServerCmdAuthenticate,
			AuthResult: protocol.Err[protocol.AuthenticateResult](user.Lang.Format("auth-repeated-authenticate", nil)),
		}, nil

	case protocol.ClientCmdChat:
		room := ctx.RequireRoom()
		content := cmd.Message
		if !st.Config.ChatEnabled {
			content = st.ServerLang.Format("chat-disabled-by-server", nil)
		}
		_ = ctx.BroadcastRoomMessage(room, protocol.Message{Type: protocol.MessageChat, User: user.ID, Content: content})
		if st.Logger != nil {
			st.Logger.LogRoomInfo(st.ServerLang, room.ID, "log-user-chat", map[string]string{"user": user.Name, "room": string(room.ID)})
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdChat, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdTouches:
		room := user.Room
		if room == nil {
			return protocol.ServerCommand{}, nil
		}
		playing, ok := room.State.(*game.StatePlaying)
		if !ok {
			return protocol.ServerCommand{}, nil
		}
		if _, aborted := playing.Aborted[user.ID]; aborted {
			return protocol.ServerCommand{}, nil
		}
		if _, results := playing.Results[user.ID]; results {
			return protocol.ServerCommand{}, nil
		}
		if len(cmd.Frames) > 0 {
			last := cmd.Frames[len(cmd.Frames)-1]
			user.GameTime = last.Time
		}
		if len(room.MonitorIDs()) > 0 {
			ctx.MonitorBuffer.BufferTouches(user.ID, cmd.Frames, room.MonitorIDs())
		}
		if ctx.State.ReplayEnabled && ctx.State.ReplayRecorder != nil && room.ReplayEligible {
			ctx.State.ReplayRecorder.AppendTouches(room.ID, user.ID, cmd.Frames)
		}
		return protocol.ServerCommand{}, nil

	case protocol.ClientCmdJudges:
		room := user.Room
		if room == nil {
			return protocol.ServerCommand{}, nil
		}
		st, ok := room.State.(*game.StatePlaying)
		if !ok {
			return protocol.ServerCommand{}, nil
		}
		if _, aborted := st.Aborted[user.ID]; aborted {
			return protocol.ServerCommand{}, nil
		}
		if _, results := st.Results[user.ID]; results {
			return protocol.ServerCommand{}, nil
		}
		if len(room.MonitorIDs()) > 0 {
			ctx.MonitorBuffer.BufferJudges(user.ID, cmd.Judges, room.MonitorIDs())
		}
		if ctx.State.ReplayEnabled && ctx.State.ReplayRecorder != nil && room.ReplayEligible {
			ctx.State.ReplayRecorder.AppendJudges(room.ID, user.ID, cmd.Judges)
		}
		return protocol.ServerCommand{}, nil

	case protocol.ClientCmdCreateRoom:
		if err := ctx.ProcessCreateRoom(cmd.RoomID); err != nil {
			return protocol.ServerCommand{Type: protocol.ServerCmdCreateRoom, Result: protocol.Err[struct{}](err.Error())}, nil
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdCreateRoom, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdJoinRoom:
		resp, err := ctx.ProcessJoinRoom(cmd.RoomID, cmd.Monitor)
		if err != nil {
			return protocol.ServerCommand{Type: protocol.ServerCmdJoinRoom, JoinResult: protocol.Err[protocol.JoinRoomResponse](err.Error())}, nil
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdJoinRoom, JoinResult: protocol.Ok(*resp)}, nil

	case protocol.ClientCmdLeaveRoom:
		room := ctx.RequireRoom()
		shouldDisband := room.OnUserLeave(user, &game.RoomCallbacks{
			Broadcast: func(cmd protocol.ServerCommand) error {
				return ctx.BroadcastRoom(room, cmd)
			},
			UsersById: func(id int32) *game.User {
				return ctx.findUser(id)
			},
			PickRandomUserId: utils.RandomPickInt32,
			Lang:   st.ServerLang,
			Logger: st.Logger,
			NotifyWebSocket: func(rid roomid.RoomID) {
				if st.WSServer != nil {
					st.WSServer.BroadcastRoomUpdate(rid, nil)
				}
			},
		})
		suffix := ""
		if user.Monitor {
			suffix = st.ServerLang.Format("label-monitor-suffix", nil)
		}
		if st.Logger != nil {
			st.Logger.LogRoomMark(st.ServerLang, room.ID, "log-room-left", map[string]string{"user": user.Name, "suffix": suffix, "room": string(room.ID)})
		}
		if shouldDisband {
			if st.Logger != nil {
				st.Logger.LogRoomInfo(st.ServerLang, room.ID, "log-room-recycled", map[string]string{"room": string(room.ID)})
			}
			ctx.DisbandRoom(room)
		}
		user.Room = nil
		return protocol.ServerCommand{Type: protocol.ServerCmdLeaveRoom, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdLockRoom:
		room := ctx.RequireRoom()
		if err := room.CheckHost(user.ID); err != nil {
			return protocol.ServerCommand{Type: protocol.ServerCmdLockRoom, Result: protocol.Err[struct{}](err.Error())}, nil
		}
		room.Locked = cmd.Lock
		_ = ctx.BroadcastRoomMessage(room, protocol.Message{Type: protocol.MessageLockRoom, Lock: cmd.Lock})
		if st.Logger != nil {
			lockStr := st.ServerLang.Format("log-room-lock-locked", nil)
			if !cmd.Lock {
				lockStr = st.ServerLang.Format("log-room-lock-unlocked", nil)
			}
			st.Logger.LogRoomMark(st.ServerLang, room.ID, "log-room-lock", map[string]string{"user": user.Name, "room": string(room.ID), "lock": lockStr})
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdLockRoom, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdCycleRoom:
		room := ctx.RequireRoom()
		if err := room.CheckHost(user.ID); err != nil {
			return protocol.ServerCommand{Type: protocol.ServerCmdCycleRoom, Result: protocol.Err[struct{}](err.Error())}, nil
		}
		room.Cycle = cmd.Cycle
		_ = ctx.BroadcastRoomMessage(room, protocol.Message{Type: protocol.MessageCycleRoom, Cycle: cmd.Cycle})
		if st.Logger != nil {
			cycleStr := st.ServerLang.Format("log-room-cycle-on", nil)
			if !cmd.Cycle {
				cycleStr = st.ServerLang.Format("log-room-cycle-off", nil)
			}
			st.Logger.LogRoomMark(st.ServerLang, room.ID, "log-room-cycle", map[string]string{"user": user.Name, "room": string(room.ID), "cycle": cycleStr})
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdCycleRoom, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdSelectChart:
		room := ctx.RequireRoom()
		if err := room.ValidateSelectChart(user.ID); err != nil {
			return protocol.ServerCommand{Type: protocol.ServerCmdSelectChart, Result: protocol.Err[struct{}](err.Error())}, nil
		}
		chart, err := ctx.FetchChart(cmd.ChartID)
		if err != nil {
			return protocol.ServerCommand{Type: protocol.ServerCmdSelectChart, Result: protocol.Err[struct{}](err.Error())}, nil
		}
		room.Chart = chart
		_ = ctx.BroadcastRoomMessage(room, protocol.Message{Type: protocol.MessageSelectChart, User: user.ID, Name: chart.Name, ChartID: int32(chart.ID)})
		_ = ctx.BroadcastRoom(room, protocol.ServerCommand{Type: protocol.ServerCmdChangeState, State: room.ClientRoomState()})
		if st.Logger != nil {
			st.Logger.LogRoomMark(st.ServerLang, room.ID, "log-room-select-chart", map[string]string{"user": user.Name, "userId": fmt.Sprintf("%d", user.ID), "room": string(room.ID), "chart": chart.Name})
		}
		if st.WSServer != nil {
			st.WSServer.BroadcastRoomUpdate(room.ID, nil)
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdSelectChart, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdRequestStart:
		room := ctx.RequireRoom()
		if err := room.ValidateStart(user.ID); err != nil {
			return protocol.ServerCommand{Type: protocol.ServerCmdRequestStart, Result: protocol.Err[struct{}](err.Error())}, nil
		}
		_ = ctx.BroadcastRoomMessage(room, protocol.Message{Type: protocol.MessageGameStart, User: user.ID})
		room.State = &game.StateWaitForReady{Started: map[int32]struct{}{user.ID: {}}}
		_ = ctx.BroadcastRoom(room, protocol.ServerCommand{Type: protocol.ServerCmdChangeState, State: room.ClientRoomState()})
		ctx.CheckRoomAllReady(room)
		if st.Logger != nil {
			st.Logger.LogRoomMark(st.ServerLang, room.ID, "log-room-request-start", map[string]string{"user": user.Name, "room": string(room.ID)})
		}
		if st.WSServer != nil {
			st.WSServer.BroadcastRoomUpdate(room.ID, nil)
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdRequestStart, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdReady:
		room := ctx.RequireRoom()
		if _, ok := room.State.(*game.StatePlaying); ok {
			return protocol.ServerCommand{Type: protocol.ServerCmdReady, Result: protocol.Err[struct{}](user.Lang.Format("room-invalid-state", nil))}, nil
		}
		if st, ok := room.State.(*game.StateWaitForReady); ok {
			if _, already := st.Started[user.ID]; already {
				return protocol.ServerCommand{Type: protocol.ServerCmdReady, Result: protocol.Err[struct{}](user.Lang.Format("room-already-ready", nil))}, nil
			}
			st.Started[user.ID] = struct{}{}
			_ = ctx.BroadcastRoomMessage(room, protocol.Message{Type: protocol.MessageReady, User: user.ID})
			ctx.CheckRoomAllReady(room)
			if ctx.State.WSServer != nil {
				ctx.State.WSServer.BroadcastRoomUpdate(room.ID, nil)
			}
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdReady, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdCancelReady:
		room := ctx.RequireRoom()
		if _, ok := room.State.(*game.StatePlaying); ok {
			return protocol.ServerCommand{Type: protocol.ServerCmdCancelReady, Result: protocol.Err[struct{}](user.Lang.Format("room-invalid-state", nil))}, nil
		}
		if st, ok := room.State.(*game.StateWaitForReady); ok {
			if _, exists := st.Started[user.ID]; !exists {
				return protocol.ServerCommand{Type: protocol.ServerCmdCancelReady, Result: protocol.Err[struct{}](user.Lang.Format("room-not-ready", nil))}, nil
			}
			delete(st.Started, user.ID)
			if room.IsHost(user.ID) {
				room.State = &game.StateSelectChart{}
				_ = ctx.BroadcastRoomMessage(room, protocol.Message{Type: protocol.MessageCancelGame, User: user.ID})
				_ = ctx.BroadcastRoom(room, protocol.ServerCommand{Type: protocol.ServerCmdChangeState, State: room.ClientRoomState()})
			} else {
				_ = ctx.BroadcastRoomMessage(room, protocol.Message{Type: protocol.MessageCancelReady, User: user.ID})
			}
			if ctx.State.WSServer != nil {
				ctx.State.WSServer.BroadcastRoomUpdate(room.ID, nil)
			}
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdCancelReady, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdPlayed:
		room := ctx.RequireRoom()
		record, err := ctx.FetchRecord(cmd.RecordID)
		if err != nil {
			return protocol.ServerCommand{Type: protocol.ServerCmdPlayed, Result: protocol.Err[struct{}](err.Error())}, nil
		}
		if record.Player != user.ID {
			return protocol.ServerCommand{Type: protocol.ServerCmdPlayed, Result: protocol.Err[struct{}](user.Lang.Format("record-invalid", nil))}, nil
		}
		_ = ctx.BroadcastRoomMessage(room, protocol.Message{Type: protocol.MessagePlayed, User: user.ID, Score: int32(record.Score), Accuracy: record.Accuracy, FullCombo: record.FullCombo})
		if st, ok := room.State.(*game.StatePlaying); ok {
			if _, aborted := st.Aborted[user.ID]; aborted {
				return protocol.ServerCommand{Type: protocol.ServerCmdPlayed, Result: protocol.Err[struct{}](user.Lang.Format("room-game-aborted", nil))}, nil
			}
			if _, results := st.Results[user.ID]; results {
				return protocol.ServerCommand{Type: protocol.ServerCmdPlayed, Result: protocol.Err[struct{}](user.Lang.Format("record-already-uploaded", nil))}, nil
			}
			st.Results[user.ID] = &game.RecordData{
				ID:        record.ID,
				Player:    record.Player,
				Score:     record.Score,
				Perfect:   record.Perfect,
				Good:      record.Good,
				Bad:       record.Bad,
				Miss:      record.Miss,
				MaxCombo:  record.MaxCombo,
				Accuracy:  record.Accuracy,
				FullCombo: record.FullCombo,
				Std:       record.Std,
				StdScore:  record.StdScore,
			}
			if ctx.State.ReplayEnabled && ctx.State.ReplayRecorder != nil && room.ReplayEligible {
				ctx.State.ReplayRecorder.SetRecordID(room.ID, user.ID, record.ID)
			}
			ctx.CheckRoomAllReady(room)
			if ctx.State.WSServer != nil {
				ctx.State.WSServer.BroadcastRoomUpdate(room.ID, nil)
			}
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdPlayed, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdAbort:
		room := ctx.RequireRoom()
		if st, ok := room.State.(*game.StatePlaying); ok {
			if _, results := st.Results[user.ID]; results {
				return protocol.ServerCommand{Type: protocol.ServerCmdAbort, Result: protocol.Err[struct{}](user.Lang.Format("record-already-uploaded", nil))}, nil
			}
			if _, aborted := st.Aborted[user.ID]; aborted {
				return protocol.ServerCommand{Type: protocol.ServerCmdAbort, Result: protocol.Err[struct{}](user.Lang.Format("room-game-aborted", nil))}, nil
			}
			st.Aborted[user.ID] = struct{}{}
			_ = ctx.BroadcastRoomMessage(room, protocol.Message{Type: protocol.MessageAbort, User: user.ID})
			ctx.CheckRoomAllReady(room)
			if ctx.State.WSServer != nil {
				ctx.State.WSServer.BroadcastRoomUpdate(room.ID, nil)
			}
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdAbort, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdPing:
		return protocol.ServerCommand{}, nil
	}

	return protocol.ServerCommand{}, fmt.Errorf("unknown command type: %d", cmd.Type)
}

func (ctx *CommandContext) findUser(id int32) *game.User {
	var u *game.User
	ctx.State.WithRLock(func() {
		u = ctx.State.Users[id]
	})
	return u
}
