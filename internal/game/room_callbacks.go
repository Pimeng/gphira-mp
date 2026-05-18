package game

import (
	"github.com/Pimeng/gphira-mp-next/internal/l10n"
	"github.com/Pimeng/gphira-mp-next/internal/utils"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

type RoomCallbacks struct {
	UsersById           func(int32) *User
	Broadcast           func(protocol.ServerCommand) error
	BroadcastToMonitors func(protocol.ServerCommand)
	PickRandomUserId    func([]int32) int32
	Lang                *l10n.Language
	Logger              *utils.Logger
	OnEnterPlaying      func(*Room)
	OnGameEnd           func(*Room)
	DisbandRoom         func(*Room)
	NotifyWebSocket     func(roomid.RoomID)
}
