package test

import (
	"path/filepath"
	"testing"

	"github.com/Pimeng/gphira-mp-next/internal/config"
	"github.com/Pimeng/gphira-mp-next/internal/state"
	"github.com/Pimeng/gphira-mp-next/internal/utils"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

func TestNewServerState(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := utils.NewLogger("INFO")
	s := state.NewServerState(cfg, logger, "Test", "./admin_data.json")

	if s.ServerName != "Test" {
		t.Errorf("server name = %q, want Test", s.ServerName)
	}
	if s.Config != cfg {
		t.Error("config pointer mismatch")
	}
	if len(s.Users) != 0 {
		t.Errorf("users should be empty, got %d", len(s.Users))
	}
	if len(s.Rooms) != 0 {
		t.Errorf("rooms should be empty, got %d", len(s.Rooms))
	}
	if !s.RoomCreationEnabled {
		t.Error("room creation should be enabled by default")
	}
}

func TestApplyConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := utils.NewLogger("INFO")
	s := state.NewServerState(cfg, logger, "Old", "./admin_data.json")

	newCfg := &config.ServerConfig{
		ServerName:    "New",
		Lang:          "en-US",
		ReplayEnabled: config.Bool(true),
	}
	s.ApplyConfig(newCfg)

	if s.ServerName != "New" {
		t.Errorf("server name = %q, want New", s.ServerName)
	}
	if !s.ReplayEnabled {
		t.Error("replay should be enabled")
	}
}

func TestAdminDataRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "admin.json")

	cfg := config.DefaultConfig()
	logger := utils.NewLogger("INFO")
	s := state.NewServerState(cfg, logger, "Test", path)

	// Add some banned users and room bans
	s.WithLock(func() {
		s.BannedUsers[100] = struct{}{}
		s.BannedUsers[200] = struct{}{}
		s.BannedRoomUsers[roomid.RoomID("room1")] = map[int32]struct{}{50: {}}
	})

	if err := s.SaveAdminData(); err != nil {
		t.Fatalf("save admin data failed: %v", err)
	}

	// Create a new state and load
	s2 := state.NewServerState(cfg, logger, "Test", path)
	if err := s2.LoadAdminData(); err != nil {
		t.Fatalf("load admin data failed: %v", err)
	}

	s2.WithRLock(func() {
		if _, ok := s2.BannedUsers[100]; !ok {
			t.Error("banned user 100 should be loaded")
		}
		if _, ok := s2.BannedUsers[200]; !ok {
			t.Error("banned user 200 should be loaded")
		}
		if _, ok := s2.BannedRoomUsers[roomid.RoomID("room1")]; !ok {
			t.Error("banned room users for room1 should be loaded")
		}
	})
}

func TestLoadAdminDataMissingFile(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := utils.NewLogger("INFO")
	s := state.NewServerState(cfg, logger, "Test", "/nonexistent/path/admin.json")

	// Should not error when file does not exist
	if err := s.LoadAdminData(); err != nil {
		t.Errorf("loading missing file should not error, got: %v", err)
	}
}

func TestWithLock(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := utils.NewLogger("INFO")
	s := state.NewServerState(cfg, logger, "Test", "./admin_data.json")

	var counter int
	s.WithLock(func() {
		counter++
	})
	if counter != 1 {
		t.Errorf("counter = %d, want 1", counter)
	}
}
