package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Pimeng/gphira-mp-next/internal/config"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	if cfg.Port != 12346 {
		t.Errorf("default port = %d, want 12346", cfg.Port)
	}
	if cfg.Host != "::" {
		t.Errorf("default host = %q, want ::", cfg.Host)
	}
	if cfg.RoomMaxUsers != 12 {
		t.Errorf("default room max users = %d, want 12", cfg.RoomMaxUsers)
	}
	if !cfg.RoomCreationEnabled {
		t.Error("room creation should be enabled by default")
	}
	if !cfg.ChatEnabled {
		t.Error("chat should be enabled by default")
	}
	if len(cfg.Monitors) != 1 || cfg.Monitors[0] != 2 {
		t.Errorf("default monitors = %v, want [2]", cfg.Monitors)
	}
}

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	data := `
PORT: 9999
SERVER_NAME: Test Server
CHAT_ENABLED: false
MONITORS:
  - 2
  - 4
`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(path)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if cfg.Port != 9999 {
		t.Errorf("port = %d, want 9999", cfg.Port)
	}
	if cfg.ServerName != "Test Server" {
		t.Errorf("server name = %q, want Test Server", cfg.ServerName)
	}
	if cfg.ChatEnabled {
		t.Error("chat should be disabled")
	}
	if len(cfg.Monitors) != 2 {
		t.Errorf("monitors = %v, want [2, 4]", cfg.Monitors)
	}
}

func TestMergeConfig(t *testing.T) {
	base := &config.ServerConfig{Port: 12346, ServerName: "Base", ChatEnabled: true}
	override := &config.ServerConfig{Port: 9999, ChatEnabled: false}

	merged := config.MergeConfig(base, override)
	if merged.Port != 9999 {
		t.Errorf("port = %d, want 9999", merged.Port)
	}
	if merged.ServerName != "Base" {
		t.Errorf("server name = %q, want Base", merged.ServerName)
	}
	if merged.ChatEnabled {
		t.Error("chat should be overridden to false")
	}
}

func TestParseBool(t *testing.T) {
	tests := []struct {
		input string
		want  bool
		ok    bool
	}{
		{"true", true, true},
		{"1", true, true},
		{"yes", true, true},
		{"on", true, true},
		{"false", false, true},
		{"0", false, true},
		{"no", false, true},
		{"off", false, true},
		{"maybe", false, false},
		{"", false, false},
	}
	for _, tt := range tests {
		got, ok := config.ParseBool(tt.input)
		if ok != tt.ok {
			t.Errorf("ParseBool(%q) ok = %v, want %v", tt.input, ok, tt.ok)
			continue
		}
		if ok && got != tt.want {
			t.Errorf("ParseBool(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParsePort(t *testing.T) {
	if _, ok := config.ParsePort("0"); ok {
		t.Error("port 0 should be invalid")
	}
	if _, ok := config.ParsePort("70000"); ok {
		t.Error("port 70000 should be invalid")
	}
	v, ok := config.ParsePort("8080")
	if !ok || v != 8080 {
		t.Errorf("ParsePort(8080) = %d, %v, want 8080, true", v, ok)
	}
}
