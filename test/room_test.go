package test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/internal/l10n"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

func TestRoomCreateAndJoin(t *testing.T) {
	room := game.NewRoom(roomid.RoomID("test-room"), 1, 3, false)
	if room.ID != roomid.RoomID("test-room") {
		t.Errorf("room id = %q, want test-room", room.ID)
	}
	if room.HostID != 1 {
		t.Errorf("host id = %d, want 1", room.HostID)
	}
	if !room.IsHost(1) {
		t.Error("user 1 should be host")
	}
	if room.IsHost(2) {
		t.Error("user 2 should not be host")
	}

	// Add users up to max
	if !room.AddUser(1, false) {
		t.Error("failed to add user 1")
	}
	if !room.AddUser(2, false) {
		t.Error("failed to add user 2")
	}
	if !room.AddUser(3, false) {
		t.Error("failed to add user 3")
	}
	if room.AddUser(4, false) {
		t.Error("should not add user 4 beyond max")
	}
	// Monitor can still join
	if !room.AddUser(5, true) {
		t.Error("failed to add monitor 5")
	}

	uids := room.UserIDs()
	if len(uids) != 3 {
		t.Errorf("user count = %d, want 3", len(uids))
	}
	mids := room.MonitorIDs()
	if len(mids) != 1 {
		t.Errorf("monitor count = %d, want 1", len(mids))
	}
}

func TestRoomStateMachine(t *testing.T) {
	room := game.NewRoom(roomid.RoomID("state-room"), 1, 4, false)
	room.AddUser(1, false)
	room.AddUser(2, false)

	// Initial state: SelectChart
	if room.ClientRoomState().Type != protocol.RoomStateSelectChart {
		t.Errorf("initial state = %d, want SelectChart", room.ClientRoomState().Type)
	}

	// Only host can select chart
	if err := room.ValidateSelectChart(2); err == nil {
		t.Error("non-host should not be allowed to select chart")
	}
	if err := room.ValidateSelectChart(1); err != nil {
		t.Errorf("host select chart failed: %v", err)
	}

	room.Chart = &game.Chart{ID: 1, Name: "Test Chart"}

	// Only host can start
	if err := room.ValidateStart(2); err == nil {
		t.Error("non-host should not be allowed to start")
	}
	if err := room.ValidateStart(1); err != nil {
		t.Errorf("host start failed: %v", err)
	}

	// Transition to WaitForReady
	room.State = &game.StateWaitForReady{Started: map[int32]struct{}{1: {}}}
	if room.ClientRoomState().Type != protocol.RoomStateWaitingForReady {
		t.Errorf("state = %d, want WaitForReady", room.ClientRoomState().Type)
	}

	// CheckAllReady with only host ready should not transition
	cb := &game.RoomCallbacks{
		UsersById: func(id int32) *game.User {
			return &game.User{ID: id, Lang: l10n.New("zh-CN")}
		},
		Broadcast: func(cmd protocol.ServerCommand) error { return nil },
		BroadcastToMonitors: func(cmd protocol.ServerCommand) {},
		PickRandomUserId: func(ids []int32) int32 {
			if len(ids) > 0 {
				return ids[0]
			}
			return 0
		},
		Lang: l10n.New("zh-CN"),
	}
	_ = room.CheckAllReady(cb)
	if room.ClientRoomState().Type != protocol.RoomStateWaitingForReady {
		t.Error("should still be WaitForReady")
	}

	// Both ready → Playing
	room.State.(*game.StateWaitForReady).Started[2] = struct{}{}
	_ = room.CheckAllReady(cb)
	if room.ClientRoomState().Type != protocol.RoomStatePlaying {
		t.Errorf("state = %d, want Playing", room.ClientRoomState().Type)
	}

	// Game end → SelectChart
	room.State.(*game.StatePlaying).Results[1] = &game.RecordData{Player: 1, Score: 100}
	room.State.(*game.StatePlaying).Results[2] = &game.RecordData{Player: 2, Score: 200}
	_ = room.CheckAllReady(cb)
	if room.ClientRoomState().Type != protocol.RoomStateSelectChart {
		t.Errorf("state = %d, want SelectChart after game end", room.ClientRoomState().Type)
	}
}

func TestRoomOnUserLeave(t *testing.T) {
	room := game.NewRoom(roomid.RoomID("leave-room"), 1, 4, false)
	room.AddUser(1, false)
	room.AddUser(2, false)

	cb := &game.RoomCallbacks{
		UsersById: func(id int32) *game.User {
			return &game.User{ID: id, Lang: l10n.New("zh-CN")}
		},
		Broadcast:           func(cmd protocol.ServerCommand) error { return nil },
		BroadcastToMonitors: func(cmd protocol.ServerCommand) {},
		PickRandomUserId: func(ids []int32) int32 {
			if len(ids) > 0 {
				return ids[0]
			}
			return 0
		},
		Lang: l10n.New("zh-CN"),
	}

	u2 := &game.User{ID: 2, Lang: l10n.New("zh-CN")}
	shouldDisband := room.OnUserLeave(u2, cb)
	if shouldDisband {
		t.Error("should not disband when non-host leaves")
	}
	if len(room.UserIDs()) != 1 {
		t.Errorf("user count = %d, want 1", len(room.UserIDs()))
	}

	// Host leaves → disband
	u1 := &game.User{ID: 1, Lang: l10n.New("zh-CN")}
	shouldDisband = room.OnUserLeave(u1, cb)
	if !shouldDisband {
		t.Error("should disband when host leaves")
	}
}

func TestRoomValidateJoin(t *testing.T) {
	room := game.NewRoom(roomid.RoomID("join-room"), 1, 2, false)
	room.Locked = true

	if err := room.ValidateJoin(2, false, []int{2}, room.State); err == nil {
		t.Error("should reject join when room is locked")
	}

	room.Locked = false
	if err := room.ValidateJoin(2, false, []int{2}, room.State); err != nil {
		t.Errorf("join failed: %v", err)
	}

	// Monitor join without permission
	if err := room.ValidateJoin(2, true, []int{}, room.State); err == nil {
		t.Error("should reject monitor join without permission")
	}

	// Non-monitor join during WaitForReady
	room.State = &game.StateWaitForReady{}
	if err := room.ValidateJoin(2, false, []int{2}, room.State); err == nil {
		t.Error("should reject player join during WaitForReady")
	}
}

func TestRoomLog(t *testing.T) {
	room := game.NewRoom(roomid.RoomID("log-room"), 1, 4, false)
	room.AddLog("hello")
	room.AddLog("world")

	logs := room.GetRecentLogs()
	if len(logs) != 2 {
		t.Errorf("log count = %d, want 2", len(logs))
	}
	if logs[0].Message != "hello" {
		t.Errorf("first log = %q, want hello", logs[0].Message)
	}
	if logs[1].Message != "world" {
		t.Errorf("second log = %q, want world", logs[1].Message)
	}
}

func TestRoomGameSummary(t *testing.T) {
	room := game.NewRoom(roomid.RoomID("summary-room"), 1, 4, false)
	room.AddUser(1, false)
	room.AddUser(2, false)
	room.Chart = &game.Chart{ID: 1, Name: "Test Chart"}
	room.State = &game.StatePlaying{
		Results: map[int32]*game.RecordData{
			1: {Player: 1, Score: 100, Accuracy: 0.95, Std: 0.05},
			2: {Player: 2, Score: 200, Accuracy: 0.98, Std: 0.02},
		},
		Aborted: map[int32]struct{}{},
	}

	var chatMessages []protocol.Message
	cb := &game.RoomCallbacks{
		UsersById: func(id int32) *game.User {
			return &game.User{ID: id, Name: fmt.Sprintf("User%d", id), Lang: l10n.New("zh-CN")}
		},
		Broadcast: func(cmd protocol.ServerCommand) error {
			if cmd.Type == protocol.ServerCmdMessage && cmd.Message.Type == protocol.MessageChat {
				chatMessages = append(chatMessages, cmd.Message)
			}
			return nil
		},
		Lang: l10n.New("zh-CN"),
	}

	_ = room.CheckAllReady(cb)

	if room.ClientRoomState().Type != protocol.RoomStateSelectChart {
		t.Errorf("state = %d, want SelectChart after game end", room.ClientRoomState().Type)
	}
	if len(chatMessages) != 1 {
		t.Errorf("chat message count = %d, want 1", len(chatMessages))
	}
	msg := chatMessages[0]
	if msg.Type != protocol.MessageChat {
		t.Errorf("message type = %d, want Chat", msg.Type)
	}
	if msg.User != 0 {
		t.Errorf("chat user = %d, want 0 (system)", msg.User)
	}
	if !strings.Contains(msg.Content, "User2") {
		t.Errorf("summary should mention best score user, got: %s", msg.Content)
	}
}

type mockSessionSender struct {
	commands []protocol.ServerCommand
}

func (m *mockSessionSender) Send(cmd protocol.ServerCommand) bool {
	m.commands = append(m.commands, cmd)
	return true
}

func TestRoomContestWhitelist(t *testing.T) {
	room := game.NewRoom(roomid.RoomID("contest-room"), 1, 4, false)
	room.Contest = &game.ContestConfig{
		Whitelist:   map[int32]struct{}{1: {}, 2: {}},
		ManualStart: true,
		AutoDisband: true,
	}
	room.AddUser(1, false)

	// User 2 is whitelisted
	if err := room.ValidateJoin(2, false, []int{}, room.State); err != nil {
		t.Errorf("user 2 should be allowed: %v", err)
	}

	// User 3 is not whitelisted
	if err := room.ValidateJoin(3, false, []int{}, room.State); err == nil {
		t.Error("user 3 should be rejected")
	} else if err.Error() != "room-not-whitelisted" {
		t.Errorf("unexpected error: %v", err)
	}

	// Monitors also require whitelist in contest mode
	if err := room.ValidateJoin(3, true, []int{3}, room.State); err == nil {
		t.Error("monitor 3 should be rejected without whitelist")
	} else if err.Error() != "room-not-whitelisted" {
		t.Errorf("unexpected error: %v", err)
	}

	// Whitelisted monitor is allowed
	if err := room.ValidateJoin(2, true, []int{2}, room.State); err != nil {
		t.Errorf("whitelisted monitor should be allowed: %v", err)
	}
}

func TestRoomContestManualStart(t *testing.T) {
	room := game.NewRoom(roomid.RoomID("contest-manual"), 1, 4, false)
	room.Contest = &game.ContestConfig{
		Whitelist:   map[int32]struct{}{1: {}, 2: {}},
		ManualStart: true,
		AutoDisband: false,
	}
	room.AddUser(1, false)
	room.AddUser(2, false)
	room.Chart = &game.Chart{ID: 1, Name: "Test"}
	room.State = &game.StateWaitForReady{Started: map[int32]struct{}{1: {}, 2: {}}}

	cb := &game.RoomCallbacks{
		Broadcast: func(cmd protocol.ServerCommand) error { return nil },
	}

	_ = room.CheckAllReady(cb)
	if _, ok := room.State.(*game.StatePlaying); ok {
		t.Error("should not auto-start when manualStart is enabled")
	}
}

func TestRoomContestAutoDisband(t *testing.T) {
	room := game.NewRoom(roomid.RoomID("contest-disband"), 1, 4, false)
	room.Contest = &game.ContestConfig{
		Whitelist:   map[int32]struct{}{1: {}, 2: {}},
		ManualStart: false,
		AutoDisband: true,
	}
	room.AddUser(1, false)
	room.AddUser(2, false)
	room.Chart = &game.Chart{ID: 1, Name: "Test Chart"}
	room.State = &game.StatePlaying{
		Results: map[int32]*game.RecordData{
			1: {Player: 1, Score: 100, Accuracy: 0.95, Std: 0.05},
			2: {Player: 2, Score: 200, Accuracy: 0.98, Std: 0.02},
		},
		Aborted: map[int32]struct{}{},
	}

	disbanded := false
	cb := &game.RoomCallbacks{
		UsersById: func(id int32) *game.User {
			return &game.User{ID: id, Name: fmt.Sprintf("User%d", id)}
		},
		Broadcast:  func(cmd protocol.ServerCommand) error { return nil },
		DisbandRoom: func(r *game.Room) { disbanded = true },
		Lang:       l10n.New("zh-CN"),
	}

	_ = room.CheckAllReady(cb)
	if !disbanded {
		t.Error("room should be disbanded when autoDisband is enabled")
	}
}

func TestRoomCycleHostRotation(t *testing.T) {
	room := game.NewRoom(roomid.RoomID("cycle-room"), 1, 4, false)
	room.Cycle = true
	room.AddUser(1, false)
	room.AddUser(2, false)
	room.Chart = &game.Chart{ID: 1, Name: "Test Chart"}
	room.State = &game.StatePlaying{
		Results: map[int32]*game.RecordData{
			1: {Player: 1, Score: 100},
			2: {Player: 2, Score: 200},
		},
		Aborted: map[int32]struct{}{},
	}

	sender1 := &mockSessionSender{}
	sender2 := &mockSessionSender{}

	cb := &game.RoomCallbacks{
		UsersById: func(id int32) *game.User {
			var sess game.SessionSender
			if id == 1 {
				sess = sender1
			} else {
				sess = sender2
			}
			return &game.User{ID: id, Name: fmt.Sprintf("User%d", id), Lang: l10n.New("zh-CN"), Session: sess}
		},
		Broadcast: func(cmd protocol.ServerCommand) error { return nil },
		Lang:      l10n.New("zh-CN"),
	}

	_ = room.CheckAllReady(cb)

	if room.HostID != 2 {
		t.Errorf("host id = %d, want 2 after cycle", room.HostID)
	}

	var changeHostCmds []protocol.ServerCommand
	changeHostCmds = append(changeHostCmds, sender1.commands...)
	changeHostCmds = append(changeHostCmds, sender2.commands...)

	if len(changeHostCmds) != 2 {
		t.Errorf("change host commands = %d, want 2", len(changeHostCmds))
	}
	foundOld := false
	foundNew := false
	for _, cmd := range changeHostCmds {
		if cmd.Type != protocol.ServerCmdChangeHost {
			continue
		}
		if !cmd.IsHost {
			foundOld = true
		} else {
			foundNew = true
		}
	}
	if !foundOld {
		t.Error("old host should receive ChangeHost(false)")
	}
	if !foundNew {
		t.Error("new host should receive ChangeHost(true)")
	}
}
