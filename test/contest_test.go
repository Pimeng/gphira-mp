package test

import (
	"testing"

	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

func TestContestEnableDisable(t *testing.T) {
	room := game.NewRoom(roomid.RoomID("contest-toggle"), 1, 4, false)
	room.AddUser(1, false)
	room.AddUser(2, false)

	if room.Contest != nil {
		t.Error("new room should not have contest enabled")
	}

	// Enable contest without explicit whitelist → uses current participants
	room.Contest = &game.ContestConfig{
		Whitelist:   map[int32]struct{}{1: {}, 2: {}},
		ManualStart: true,
		AutoDisband: true,
	}

	if room.Contest == nil {
		t.Fatal("contest should be enabled")
	}
	if !room.Contest.ManualStart {
		t.Error("manualStart should be true")
	}
	if !room.Contest.AutoDisband {
		t.Error("autoDisband should be true")
	}

	// Disable contest
	room.Contest = nil
	if room.Contest != nil {
		t.Error("contest should be disabled")
	}

	// After disabling, anyone can join
	if err := room.ValidateJoin(99, false, []int{}, room.State); err != nil {
		t.Errorf("any user should be able to join after disable: %v", err)
	}
}

func TestContestWhitelistUpdatePreservesParticipants(t *testing.T) {
	room := game.NewRoom(roomid.RoomID("contest-wl"), 1, 4, false)
	room.AddUser(1, false)
	room.AddUser(2, false)
	room.AddUser(3, true) // monitor

	room.Contest = &game.ContestConfig{
		Whitelist:   map[int32]struct{}{1: {}, 2: {}, 3: {}},
		ManualStart: true,
		AutoDisband: true,
	}

	// Update whitelist to only user 100
	newWhitelist := map[int32]struct{}{100: {}}
	// Re-add current participants (matching TS behavior)
	for _, id := range room.UserIDs() {
		newWhitelist[id] = struct{}{}
	}
	for _, id := range room.MonitorIDs() {
		newWhitelist[id] = struct{}{}
	}
	room.Contest.Whitelist = newWhitelist

	// Original participants should still be whitelisted
	if err := room.ValidateJoin(1, false, []int{}, room.State); err != nil {
		t.Errorf("user 1 should still be whitelisted: %v", err)
	}
	if err := room.ValidateJoin(3, true, []int{3}, room.State); err != nil {
		t.Errorf("monitor 3 should still be whitelisted: %v", err)
	}
	// User 100 is now whitelisted
	if err := room.ValidateJoin(100, false, []int{}, room.State); err != nil {
		t.Errorf("user 100 should be whitelisted: %v", err)
	}
	// User 99 is not whitelisted
	if err := room.ValidateJoin(99, false, []int{}, room.State); err == nil {
		t.Error("user 99 should not be whitelisted")
	}
}

func TestContestManualStartRequiresReady(t *testing.T) {
	room := game.NewRoom(roomid.RoomID("contest-start"), 1, 4, false)
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

	// CheckAllReady should NOT auto-start in contest mode
	_ = room.CheckAllReady(cb)
	if _, ok := room.State.(*game.StatePlaying); ok {
		t.Error("contest room should not auto-start even when all ready")
	}
}

func TestContestAutoDisbandLogsResults(t *testing.T) {
	room := game.NewRoom(roomid.RoomID("contest-ad"), 1, 4, false)
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
			return &game.User{ID: id, Name: "User"}
		},
		Broadcast:   func(cmd protocol.ServerCommand) error { return nil },
		DisbandRoom: func(r *game.Room) { disbanded = true },
	}

	_ = room.CheckAllReady(cb)
	if !disbanded {
		t.Error("room should auto-disband after game ends in contest mode")
	}
}
