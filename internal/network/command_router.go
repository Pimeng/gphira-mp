package network

import (
	"fmt"

	"github.com/Pimeng/gphira-mp-next/internal/config"
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
	runtime := st.SnapshotRuntime()

	if st.Logger != nil {
		st.Logger.DebugL(runtime.ServerLang, "log-process-command", map[string]string{"user": fmt.Sprintf("%d", user.ID), "name": user.GetName(), "cmd": fmt.Sprintf("%v", cmd.Type)})
	}

	switch cmd.Type {
	case protocol.ClientCmdAuthenticate:
		if st.Logger != nil {
			st.Logger.DebugL(runtime.ServerLang, "log-repeated-authenticate", map[string]string{"user": fmt.Sprintf("%d", user.ID), "name": user.GetName()})
		}
		return protocol.ServerCommand{
			Type:       protocol.ServerCmdAuthenticate,
			AuthResult: protocol.Err[protocol.AuthenticateResult](user.GetLang().Format("auth-repeated-authenticate", nil)),
		}, nil

	case protocol.ClientCmdChat:
		room := ctx.RequireRoom()
		if st.Logger != nil {
			st.Logger.DebugL(runtime.ServerLang, "log-chat", map[string]string{"user": fmt.Sprintf("%d", user.ID), "name": user.GetName(), "room": string(room.ID), "content": cmd.Message})
		}
		content := cmd.Message
		if !config.DerefBool(runtime.Config.ChatEnabled, true) {
			content = runtime.ServerLang.Format("chat-disabled-by-server", nil)
		}
		_ = ctx.BroadcastRoomMessage(room, protocol.Message{Type: protocol.MessageChat, User: user.ID, Content: content})
		if st.Logger != nil {
			st.Logger.LogRoomInfo(runtime.ServerLang, room.ID, "log-user-chat", map[string]string{"user": user.GetName(), "room": string(room.ID)})
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdChat, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdTouches:
		room := user.GetRoom()
		if room == nil {
			return protocol.ServerCommand{}, nil
		}
		if !room.CanAcceptTouches(user.ID) {
			return protocol.ServerCommand{}, nil
		}
		if len(cmd.Frames) > 0 {
			last := cmd.Frames[len(cmd.Frames)-1]
			user.SetGameTime(last.Time)
		}
		if len(room.MonitorIDs()) > 0 {
			ctx.MonitorBuffer.BufferTouches(user.ID, cmd.Frames, room.MonitorIDs())
		}
		if runtime.ReplayEnabled && ctx.State.ReplayRecorder != nil && room.ReplayEligible {
			ctx.State.ReplayRecorder.AppendTouches(room.ID, user.ID, cmd.Frames)
		}
		return protocol.ServerCommand{}, nil

	case protocol.ClientCmdJudges:
		room := user.GetRoom()
		if room == nil {
			return protocol.ServerCommand{}, nil
		}
		if !room.CanAcceptTouches(user.ID) {
			return protocol.ServerCommand{}, nil
		}
		if len(room.MonitorIDs()) > 0 {
			ctx.MonitorBuffer.BufferJudges(user.ID, cmd.Judges, room.MonitorIDs())
		}
		if runtime.ReplayEnabled && ctx.State.ReplayRecorder != nil && room.ReplayEligible {
			ctx.State.ReplayRecorder.AppendJudges(room.ID, user.ID, cmd.Judges)
		}
		return protocol.ServerCommand{}, nil

	case protocol.ClientCmdCreateRoom:
		if st.Logger != nil {
			st.Logger.DebugL(runtime.ServerLang, "log-create-room", map[string]string{"user": fmt.Sprintf("%d", user.ID), "name": user.GetName(), "room": string(cmd.RoomID)})
		}
		if err := ctx.ProcessCreateRoom(cmd.RoomID); err != nil {
			return protocol.ServerCommand{Type: protocol.ServerCmdCreateRoom, Result: protocol.Err[struct{}](err.Error())}, nil
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdCreateRoom, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdJoinRoom:
		if st.Logger != nil {
			st.Logger.DebugL(runtime.ServerLang, "log-join-room", map[string]string{"user": fmt.Sprintf("%d", user.ID), "name": user.GetName(), "room": string(cmd.RoomID), "monitor": fmt.Sprintf("%v", cmd.Monitor)})
		}
		resp, err := ctx.ProcessJoinRoom(cmd.RoomID, cmd.Monitor)
		if err != nil {
			return protocol.ServerCommand{Type: protocol.ServerCmdJoinRoom, JoinResult: protocol.Err[protocol.JoinRoomResponse](err.Error())}, nil
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdJoinRoom, JoinResult: protocol.Ok(*resp)}, nil

	case protocol.ClientCmdLeaveRoom:
		room := ctx.RequireRoom()
		if st.Logger != nil {
			st.Logger.DebugL(runtime.ServerLang, "log-leave-room", map[string]string{"user": fmt.Sprintf("%d", user.ID), "name": user.GetName(), "room": string(room.ID)})
		}
		participantIDs := room.AllParticipantIDs()
		shouldDisband := room.OnUserLeave(user, &game.RoomCallbacks{
			Broadcast: func(cmd protocol.ServerCommand) error {
				for _, id := range participantIDs {
					if u := ctx.findUser(id); u != nil {
						_ = u.TrySend(cmd)
					}
				}
				return nil
			},
			UsersById: func(id int32) *game.User {
				return ctx.findUser(id)
			},
			PickRandomUserId: utils.RandomPickInt32,
			Lang:             runtime.ServerLang,
			Logger:           st.Logger,
			NotifyWebSocket: func(rid roomid.RoomID) {
				if st.WSServer != nil {
					st.WSServer.BroadcastRoomUpdate(rid, nil)
				}
			},
		})
		suffix := ""
		if user.IsMonitor() {
			suffix = runtime.ServerLang.Format("label-monitor-suffix", nil)
		}
		if st.Logger != nil {
			st.Logger.LogRoomMark(runtime.ServerLang, room.ID, "log-room-left", map[string]string{"user": user.GetName(), "suffix": suffix, "room": string(room.ID)})
		}
		if shouldDisband {
			if st.Logger != nil {
				st.Logger.LogRoomInfo(runtime.ServerLang, room.ID, "log-room-recycled", map[string]string{"room": string(room.ID)})
			}
			ctx.DisbandRoom(room)
		} else {
			room.RefreshLive(runtime.ReplayEnabled)
			ctx.CheckRoomAllReady(room)
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdLeaveRoom, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdLockRoom:
		room := ctx.RequireRoom()
		if st.Logger != nil {
			st.Logger.DebugL(runtime.ServerLang, "log-lock-room", map[string]string{"user": fmt.Sprintf("%d", user.ID), "name": user.GetName(), "room": string(room.ID), "lock": fmt.Sprintf("%v", cmd.Lock)})
		}
		if err := room.CheckHost(user.ID); err != nil {
			return protocol.ServerCommand{Type: protocol.ServerCmdLockRoom, Result: protocol.Err[struct{}](err.Error())}, nil
		}
		room.SetLocked(cmd.Lock)
		_ = ctx.BroadcastRoomMessage(room, protocol.Message{Type: protocol.MessageLockRoom, Lock: cmd.Lock})
		if st.Logger != nil {
			lockStr := runtime.ServerLang.Format("log-room-lock-locked", nil)
			if !cmd.Lock {
				lockStr = runtime.ServerLang.Format("log-room-lock-unlocked", nil)
			}
			st.Logger.LogRoomMark(runtime.ServerLang, room.ID, "log-room-lock", map[string]string{"user": user.GetName(), "room": string(room.ID), "lock": lockStr})
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdLockRoom, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdCycleRoom:
		room := ctx.RequireRoom()
		if st.Logger != nil {
			st.Logger.DebugL(runtime.ServerLang, "log-cycle-room", map[string]string{"user": fmt.Sprintf("%d", user.ID), "name": user.GetName(), "room": string(room.ID), "cycle": fmt.Sprintf("%v", cmd.Cycle)})
		}
		if err := room.CheckHost(user.ID); err != nil {
			return protocol.ServerCommand{Type: protocol.ServerCmdCycleRoom, Result: protocol.Err[struct{}](err.Error())}, nil
		}
		room.SetCycle(cmd.Cycle)
		_ = ctx.BroadcastRoomMessage(room, protocol.Message{Type: protocol.MessageCycleRoom, Cycle: cmd.Cycle})
		if st.Logger != nil {
			cycleStr := runtime.ServerLang.Format("log-room-cycle-on", nil)
			if !cmd.Cycle {
				cycleStr = runtime.ServerLang.Format("log-room-cycle-off", nil)
			}
			st.Logger.LogRoomMark(runtime.ServerLang, room.ID, "log-room-cycle", map[string]string{"user": user.GetName(), "room": string(room.ID), "cycle": cycleStr})
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdCycleRoom, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdSelectChart:
		room := ctx.RequireRoom()
		if st.Logger != nil {
			st.Logger.DebugL(runtime.ServerLang, "log-select-chart", map[string]string{"user": fmt.Sprintf("%d", user.ID), "name": user.GetName(), "room": string(room.ID), "chartId": fmt.Sprintf("%d", cmd.ChartID)})
		}
		if err := room.ValidateSelectChart(user.ID); err != nil {
			return protocol.ServerCommand{Type: protocol.ServerCmdSelectChart, Result: protocol.Err[struct{}](err.Error())}, nil
		}
		chart, err := ctx.FetchChart(cmd.ChartID)
		if err != nil {
			return protocol.ServerCommand{Type: protocol.ServerCmdSelectChart, Result: protocol.Err[struct{}](err.Error())}, nil
		}
		room.SetChart(chart)
		_ = ctx.BroadcastRoomMessage(room, protocol.Message{Type: protocol.MessageSelectChart, User: user.ID, Name: chart.Name, ChartID: int32(chart.ID)})
		_ = ctx.BroadcastRoom(room, protocol.ServerCommand{Type: protocol.ServerCmdChangeState, State: room.ClientRoomState()})
		if st.Logger != nil {
			st.Logger.LogRoomMark(runtime.ServerLang, room.ID, "log-room-select-chart", map[string]string{"user": user.GetName(), "userId": fmt.Sprintf("%d", user.ID), "room": string(room.ID), "chart": chart.Name})
		}
		if st.WSServer != nil {
			st.WSServer.BroadcastRoomUpdate(room.ID, nil)
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdSelectChart, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdRequestStart:
		room := ctx.RequireRoom()
		if st.Logger != nil {
			st.Logger.DebugL(runtime.ServerLang, "log-request-start", map[string]string{"user": fmt.Sprintf("%d", user.ID), "name": user.GetName(), "room": string(room.ID)})
		}
		if err := room.ValidateStart(user.ID); err != nil {
			return protocol.ServerCommand{Type: protocol.ServerCmdRequestStart, Result: protocol.Err[struct{}](err.Error())}, nil
		}
		room.ResetGameTime(ctx.findUser)
		_ = ctx.BroadcastRoomMessage(room, protocol.Message{Type: protocol.MessageGameStart, User: user.ID})
		if err := room.StartWaitForReady(user.ID); err != nil {
			return protocol.ServerCommand{Type: protocol.ServerCmdRequestStart, Result: protocol.Err[struct{}](err.Error())}, nil
		}
		_ = ctx.BroadcastRoom(room, protocol.ServerCommand{Type: protocol.ServerCmdChangeState, State: room.ClientRoomState()})
		ctx.CheckRoomAllReady(room)
		if st.Logger != nil {
			st.Logger.LogRoomMark(runtime.ServerLang, room.ID, "log-room-request-start", map[string]string{"user": user.GetName(), "room": string(room.ID)})
		}
		if st.WSServer != nil {
			st.WSServer.BroadcastRoomUpdate(room.ID, nil)
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdRequestStart, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdReady:
		room := ctx.RequireRoom()
		if st.Logger != nil {
			st.Logger.DebugL(runtime.ServerLang, "log-ready", map[string]string{"user": fmt.Sprintf("%d", user.ID), "name": user.GetName(), "room": string(room.ID)})
		}
		already, err := room.SetReady(user.ID)
		if err != nil {
			return protocol.ServerCommand{Type: protocol.ServerCmdReady, Result: protocol.Err[struct{}](user.GetLang().Format(err.Error(), nil))}, nil
		}
		if already {
			// Already ready or not in WaitForReady state - silently succeed
			return protocol.ServerCommand{Type: protocol.ServerCmdReady, Result: protocol.Ok(struct{}{})}, nil
		}
		_ = ctx.BroadcastRoomMessage(room, protocol.Message{Type: protocol.MessageReady, User: user.ID})
		ctx.CheckRoomAllReady(room)
		if ctx.State.WSServer != nil {
			ctx.State.WSServer.BroadcastRoomUpdate(room.ID, nil)
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdReady, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdCancelReady:
		room := ctx.RequireRoom()
		if st.Logger != nil {
			st.Logger.DebugL(runtime.ServerLang, "log-cancel-ready", map[string]string{"user": fmt.Sprintf("%d", user.ID), "name": user.GetName(), "room": string(room.ID)})
		}
		wasHost, err := room.CancelReady(user.ID)
		if err != nil {
			return protocol.ServerCommand{Type: protocol.ServerCmdCancelReady, Result: protocol.Err[struct{}](user.GetLang().Format(err.Error(), nil))}, nil
		}
		if wasHost {
			room.ResetToSelectChart()
			_ = ctx.BroadcastRoomMessage(room, protocol.Message{Type: protocol.MessageCancelGame, User: user.ID})
			_ = ctx.BroadcastRoom(room, protocol.ServerCommand{Type: protocol.ServerCmdChangeState, State: room.ClientRoomState()})
		} else {
			_ = ctx.BroadcastRoomMessage(room, protocol.Message{Type: protocol.MessageCancelReady, User: user.ID})
		}
		if ctx.State.WSServer != nil {
			ctx.State.WSServer.BroadcastRoomUpdate(room.ID, nil)
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdCancelReady, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdPlayed:
		room := ctx.RequireRoom()
		if st.Logger != nil {
			st.Logger.DebugL(runtime.ServerLang, "log-played", map[string]string{"user": fmt.Sprintf("%d", user.ID), "name": user.GetName(), "room": string(room.ID), "recordId": fmt.Sprintf("%d", cmd.RecordID)})
		}
		record, err := ctx.FetchRecord(cmd.RecordID)
		if err != nil {
			return protocol.ServerCommand{Type: protocol.ServerCmdPlayed, Result: protocol.Err[struct{}](err.Error())}, nil
		}
		if record.Player != user.ID {
			return protocol.ServerCommand{Type: protocol.ServerCmdPlayed, Result: protocol.Err[struct{}](user.GetLang().Format("record-invalid", nil))}, nil
		}
		_ = ctx.BroadcastRoomMessage(room, protocol.Message{Type: protocol.MessagePlayed, User: user.ID, Score: int32(record.Score), Accuracy: record.Accuracy, FullCombo: record.FullCombo})
		err = room.AddResult(user.ID, &game.RecordData{
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
		})
		if err != nil {
			if err.Error() == "room-invalid-state" {
				return protocol.ServerCommand{Type: protocol.ServerCmdPlayed, Result: protocol.Ok(struct{}{})}, nil
			}
			return protocol.ServerCommand{Type: protocol.ServerCmdPlayed, Result: protocol.Err[struct{}](user.GetLang().Format(err.Error(), nil))}, nil
		}
		if runtime.ReplayEnabled && ctx.State.ReplayRecorder != nil && room.ReplayEligible {
			ctx.State.ReplayRecorder.SetRecordID(room.ID, user.ID, record.ID)
		}
		ctx.CheckRoomAllReady(room)
		if ctx.State.WSServer != nil {
			ctx.State.WSServer.BroadcastRoomUpdate(room.ID, nil)
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdPlayed, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdAbort:
		room := ctx.RequireRoom()
		if st.Logger != nil {
			st.Logger.DebugL(runtime.ServerLang, "log-abort", map[string]string{"user": fmt.Sprintf("%d", user.ID), "name": user.GetName(), "room": string(room.ID)})
		}
		err := room.SetAborted(user.ID)
		if err != nil {
			if err.Error() == "room-invalid-state" {
				// Not in Playing state - silently succeed
				return protocol.ServerCommand{Type: protocol.ServerCmdAbort, Result: protocol.Ok(struct{}{})}, nil
			}
			return protocol.ServerCommand{Type: protocol.ServerCmdAbort, Result: protocol.Err[struct{}](user.GetLang().Format(err.Error(), nil))}, nil
		}
		_ = ctx.BroadcastRoomMessage(room, protocol.Message{Type: protocol.MessageAbort, User: user.ID})
		ctx.CheckRoomAllReady(room)
		if ctx.State.WSServer != nil {
			ctx.State.WSServer.BroadcastRoomUpdate(room.ID, nil)
		}
		return protocol.ServerCommand{Type: protocol.ServerCmdAbort, Result: protocol.Ok(struct{}{})}, nil

	case protocol.ClientCmdPing:
		return protocol.ServerCommand{}, nil
	}

	return protocol.ServerCommand{}, fmt.Errorf("%s", user.GetLang().Format("log-unknown-command-type", map[string]string{"type": fmt.Sprintf("%d", cmd.Type)}))
}

func (ctx *CommandContext) findUser(id int32) *game.User {
	var u *game.User
	ctx.State.WithRLock(func() {
		u = ctx.State.Users[id]
	})
	return u
}
