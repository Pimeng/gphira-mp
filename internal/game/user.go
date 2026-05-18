package game

import (
	"sync"

	"github.com/Pimeng/gphira-mp-next/internal/l10n"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
)

// SessionSender is the interface required by User to send commands.
type SessionSender interface {
	Send(cmd protocol.ServerCommand) bool
}

type User struct {
	ID          int32
	Name        string
	Lang        *l10n.Language
	Session     SessionSender
	Room        *Room
	Monitor     bool
	GameTime    float32
	mu          sync.Mutex
	dangleToken any
	dangleRoom  *Room
}

func (u *User) ToInfo() protocol.UserInfo {
	return protocol.UserInfo{
		ID:      u.ID,
		Name:    u.Name,
		Monitor: u.Monitor,
	}
}

func (u *User) CanMonitor(monitors []int) bool {
	for _, m := range monitors {
		if int(u.ID) == m {
			return true
		}
	}
	return false
}

func NewUser(id int32, name, language string) *User {
	return &User{
		ID:       id,
		Name:     name,
		Lang:     l10n.New(language),
		GameTime: -1e9,
	}
}

func (u *User) SetSession(sess SessionSender) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.Session = sess
	u.dangleToken = nil
	u.dangleRoom = nil
}

// MarkDangle marks the user as dangling and returns a token for verification.
// The caller should preserve the user's room association; dangleRoom is set
// to the current room so it can be restored or cleaned up later.
func (u *User) MarkDangle() any {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.dangleToken = new(int)
	u.dangleRoom = u.Room
	return u.dangleToken
}

func (u *User) IsStillDangling(token any) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.dangleToken == token
}

// DangleRoom returns the room the user was in when marked as dangling.
func (u *User) DangleRoom() *Room {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.dangleRoom
}

// ClearDangle removes the dangling state without clearing the session.
func (u *User) ClearDangle() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.dangleToken = nil
	u.dangleRoom = nil
}

// DangleToken returns the current dangle token (for scheduling cleanup).
func (u *User) DangleToken() any {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.dangleToken
}

func (u *User) TrySend(cmd protocol.ServerCommand) bool {
	if u.Session == nil {
		return false
	}
	if sender, ok := u.Session.(SessionSender); ok {
		return sender.Send(cmd)
	}
	return false
}
