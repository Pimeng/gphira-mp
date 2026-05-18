package test

import (
	"testing"

	"github.com/Pimeng/gphira-mp-next/internal/config"
	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/internal/network"
	"github.com/Pimeng/gphira-mp-next/internal/state"
	"github.com/Pimeng/gphira-mp-next/internal/utils"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

func setupTestContext() *network.CommandContext {
	cfg := config.DefaultConfig()
	logger := utils.NewLogger("INFO")
	st := state.NewServerState(cfg, logger, "Test", "./admin.json")
	user := game.NewUser(1, "Alice", "zh-CN")

	return &network.CommandContext{
		State: st,
		User:  user,
		MonitorBuffer: network.NewMonitorBuffer(func(player int32, frames []protocol.TouchFrame, judges []protocol.JudgeEvent, ids []int32) {}),
		RequireRoom: func() *game.Room {
			return nil
		},
		BroadcastRoom: func(r *game.Room, cmd protocol.ServerCommand) error {
			return nil
		},
		BroadcastRoomMessage: func(r *game.Room, msg protocol.Message) error {
			return nil
		},
		BroadcastToMonitors: func(r *game.Room, cmd protocol.ServerCommand) {},
		ProcessCreateRoom: func(id roomid.RoomID) error {
			return nil
		},
		ProcessJoinRoom: func(id roomid.RoomID, monitor bool) (*protocol.JoinRoomResponse, error) {
			return nil, nil
		},
		DisbandRoom:     func(r *game.Room) {},
		CheckRoomAllReady: func(r *game.Room) {},
		FetchChart:      func(id int32) (*game.Chart, error) { return nil, nil },
		FetchRecord:     func(id int32) (*network.PhiraRecord, error) { return nil, nil },
	}
}

func TestProcessAuthenticate(t *testing.T) {
	ctx := setupTestContext()
	resp, err := network.ProcessClientCommand(ctx, protocol.ClientCommand{Type: protocol.ClientCmdAuthenticate})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Type != protocol.ServerCmdAuthenticate {
		t.Errorf("resp type = %d, want Authenticate", resp.Type)
	}
	if resp.AuthResult.Ok {
		t.Error("auth should fail on repeated authenticate")
	}
}

func TestProcessPing(t *testing.T) {
	ctx := setupTestContext()
	resp, err := network.ProcessClientCommand(ctx, protocol.ClientCommand{Type: protocol.ClientCmdPing})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Type != 0 {
		t.Errorf("ping resp should be empty, got type %d", resp.Type)
	}
}

func TestProcessCreateRoom(t *testing.T) {
	ctx := setupTestContext()
	created := false
	ctx.ProcessCreateRoom = func(id roomid.RoomID) error {
		created = true
		return nil
	}

	resp, err := network.ProcessClientCommand(ctx, protocol.ClientCommand{Type: protocol.ClientCmdCreateRoom, RoomID: roomid.RoomID("test")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Type != protocol.ServerCmdCreateRoom {
		t.Errorf("resp type = %d, want CreateRoom", resp.Type)
	}
	if !resp.Result.Ok {
		t.Errorf("create room failed: %s", resp.Result.Error)
	}
	if !created {
		t.Error("ProcessCreateRoom should be called")
	}
}

func TestProcessUnknownCommand(t *testing.T) {
	ctx := setupTestContext()
	_, err := network.ProcessClientCommand(ctx, protocol.ClientCommand{Type: 255})
	if err == nil {
		t.Error("expected error for unknown command")
	}
}
