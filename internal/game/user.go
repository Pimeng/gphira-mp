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
	ID       int32
	name     string
	lang     *l10n.Language
	session  SessionSender
	room     *Room
	monitor  bool
	gameTime float32
	mu       sync.RWMutex

	dangleToken any
	dangleRoom  *Room
}

func (u *User) ToInfo() protocol.UserInfo {
	u.mu.RLock()
	name := u.name
	monitor := u.monitor
	u.mu.RUnlock()
	return protocol.UserInfo{
		ID:      u.ID,
		Name:    name,
		Monitor: monitor,
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
		name:     name,
		lang:     l10n.New(language),
		gameTime: -1e9,
	}
}

func (u *User) GetName() string {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.name
}

func (u *User) GetLang() *l10n.Language {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.lang
}

func (u *User) SetIdentity(name, language string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.name = name
	u.lang = l10n.New(language)
}

func (u *User) GetSession() SessionSender {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.session
}

func (u *User) HasSession() bool {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.session != nil
}

func (u *User) GetRoom() *Room {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.room
}

func (u *User) IsMonitor() bool {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.monitor
}

func (u *User) SetRoom(room *Room, monitor bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.room = room
	u.monitor = monitor
}

func (u *User) ClearRoom() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.room = nil
	u.monitor = false
}

func (u *User) SetGameTime(v float32) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.gameTime = v
}

func (u *User) ResetGameTime() {
	u.SetGameTime(-1e9)
}

func (u *User) GetGameTime() float32 {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.gameTime
}

func (u *User) SetSession(sess SessionSender) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.session = sess
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
	u.dangleRoom = u.room
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
	sess := u.GetSession()
	if sess == nil {
		return false
	}
	if sender, ok := sess.(SessionSender); ok {
		return sender.Send(cmd)
	}
	return false
}
