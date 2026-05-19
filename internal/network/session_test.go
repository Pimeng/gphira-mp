package network

import (
	"testing"

	"github.com/Pimeng/gphira-mp-next/internal/config"
	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/internal/state"
	"github.com/Pimeng/gphira-mp-next/internal/utils"
)

func TestMarkLostStaleSessionDoesNotClearReplacement(t *testing.T) {
	cfg := config.DefaultConfig()
	st := state.NewServerState(cfg, utils.NewLogger("INFO"), "Test", "./admin.json")
	user := game.NewUser(1, "Alice", "zh-CN")

	oldSession := NewSession("old", nil, st, "127.0.0.1")
	newSession := NewSession("new", nil, st, "127.0.0.1")
	oldSession.user = user
	newSession.user = user
	user.SetSession(newSession)

	st.WithLock(func() {
		st.Users[user.ID] = user
		st.Sessions[oldSession.ID] = oldSession
		st.Sessions[newSession.ID] = newSession
	})

	oldSession.adminDisconnect(true)

	if got := user.GetSession(); got != newSession {
		t.Fatalf("stale session cleared replacement session: got %T, want %T", got, newSession)
	}
	if token := user.DangleToken(); token != nil {
		t.Fatal("stale session should not mark user dangling")
	}
	st.WithRLock(func() {
		if _, ok := st.Sessions[oldSession.ID]; ok {
			t.Fatal("old session should be removed from server state")
		}
		if _, ok := st.Sessions[newSession.ID]; !ok {
			t.Fatal("new session should remain in server state")
		}
	})
}
