package test

import (
	"testing"

	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
)

func TestNewUser(t *testing.T) {
	u := game.NewUser(42, "Alice", "zh-CN")
	if u.ID != 42 {
		t.Errorf("id = %d, want 42", u.ID)
	}
	if u.Name != "Alice" {
		t.Errorf("name = %q, want Alice", u.Name)
	}
	if u.GameTime != -1e9 {
		t.Errorf("game time = %v, want -1e9", u.GameTime)
	}
}

func TestUserToInfo(t *testing.T) {
	u := game.NewUser(1, "Bob", "en-US")
	info := u.ToInfo()
	if info.ID != 1 {
		t.Errorf("info id = %d, want 1", info.ID)
	}
	if info.Name != "Bob" {
		t.Errorf("info name = %q, want Bob", info.Name)
	}
}

func TestUserCanMonitor(t *testing.T) {
	u := game.NewUser(1, "Test", "zh-CN")
	u.Monitor = true
	if !u.CanMonitor([]int{1, 2, 3}) {
		t.Error("user should be able to monitor")
	}
	u.Monitor = false
	if u.CanMonitor([]int{2, 3}) {
		t.Error("user should not be able to monitor without permission")
	}
	if !u.CanMonitor([]int{1, 2}) {
		t.Error("user with id 1 should be able to monitor when monitors include 1")
	}
}

func TestUserDangle(t *testing.T) {
	u := game.NewUser(1, "Test", "zh-CN")
	token1 := u.MarkDangle()
	if token1 == "" {
		t.Error("dangle token should not be empty")
	}
	if !u.IsStillDangling(token1) {
		t.Error("user should still be dangling with correct token")
	}
	if u.IsStillDangling("wrong-token") {
		t.Error("user should not be dangling with wrong token")
	}

	// Re-dangle invalidates old token
	token2 := u.MarkDangle()
	if u.IsStillDangling(token1) {
		t.Error("old token should be invalid after re-dangle")
	}
	if !u.IsStillDangling(token2) {
		t.Error("new token should be valid")
	}
}

func TestUserTrySend(t *testing.T) {
	u := game.NewUser(1, "Test", "zh-CN")
	// Without session, TrySend should return false
	if u.TrySend(protocol.ServerCommand{}) {
		t.Error("TrySend without session should return false")
	}
}
