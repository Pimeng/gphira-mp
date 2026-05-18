package test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestParseString(t *testing.T) {
	if v, ok := config.ParseString("hello"); !ok || v != "hello" {
		t.Errorf("ParseString(\"hello\") = %q, %v, want hello, true", v, ok)
	}
	if _, ok := config.ParseString("  "); ok {
		t.Error("ParseString(\"  \") should be false")
	}
	if _, ok := config.ParseString(""); ok {
		t.Error("ParseString(\"\") should be false")
	}
}

func TestParseRoomMaxUsers(t *testing.T) {
	if _, ok := config.ParseRoomMaxUsers("0"); ok {
		t.Error("ParseRoomMaxUsers(\"0\") should be false")
	}
	if _, ok := config.ParseRoomMaxUsers("-1"); ok {
		t.Error("ParseRoomMaxUsers(\"-1\") should be false")
	}
	if v, ok := config.ParseRoomMaxUsers("16"); !ok || v != 16 {
		t.Errorf("ParseRoomMaxUsers(\"16\") = %d, %v, want 16, true", v, ok)
	}
	if v, ok := config.ParseRoomMaxUsers("100"); !ok || v != 64 {
		t.Errorf("ParseRoomMaxUsers(\"100\") = %d, %v, want 64, true", v, ok)
	}
}

func TestParseIntegerList(t *testing.T) {
	cases := []struct {
		input string
		want  []int
		ok    bool
	}{
		{"1,2,3", []int{1, 2, 3}, true},
		{"1;2;3", []int{1, 2, 3}, true},
		{"1 2 3", []int{1, 2, 3}, true},
		{"1, 2 , 3", []int{1, 2, 3}, true},
		{"", nil, false},
		{"a,b", nil, false},
		{"1,a,3", []int{1, 3}, true},
	}
	for _, tt := range cases {
		got, ok := config.ParseIntegerList(tt.input)
		if ok != tt.ok {
			t.Errorf("ParseIntegerList(%q) ok = %v, want %v", tt.input, ok, tt.ok)
			continue
		}
		if tt.ok {
			if len(got) != len(tt.want) {
				t.Errorf("ParseIntegerList(%q) = %v, want %v", tt.input, got, tt.want)
				continue
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ParseIntegerList(%q) = %v, want %v", tt.input, got, tt.want)
					break
				}
			}
		}
	}
}

func TestParseOutboundProxy(t *testing.T) {
	if v, ok := config.ParseOutboundProxy("http://proxy"); !ok || v != "http://proxy" {
		t.Errorf("ParseOutboundProxy = %q, %v, want http://proxy, true", v, ok)
	}
	if v, ok := config.ParseOutboundProxy("false"); !ok || v != "false" {
		t.Errorf("ParseOutboundProxy = %q, %v, want false, true", v, ok)
	}
	if _, ok := config.ParseOutboundProxy(""); ok {
		t.Error("ParseOutboundProxy(\"\") should be false")
	}
}

func TestLoadConfigNotExist(t *testing.T) {
	_, err := config.LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(path, []byte("{invalid"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.LoadConfig(path)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoadConfigFull(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	data := `
MONITORS:
  - 1
  - 3
TEST_ACCOUNT_IDS:
  - 123
  - 456
SERVER_NAME: Full Server
HOST: 127.0.0.1
PORT: 8888
HTTP_SERVICE: true
HTTP_PORT: 8889
ROOM_MAX_USERS: 8
ROOM_CREATION_ENABLED: false
CHAT_ENABLED: false
REPLAY_ENABLED: true
REPLAY_BASE_DIR: /tmp/record
REPLAY_AUTO_UPLOAD: true
ADMIN_TOKEN: secret
ADMIN_DATA_PATH: /tmp/admin.json
ROOM_LIST_TIP: hello
LOG_LEVEL: DEBUG
REAL_IP_HEADER: X-Real-IP
HAPROXY_PROTOCOL: true
LANG: en-US
PHIRA_API_ENDPOINT: https://api.example.com
OUTBOUND_PROXY: http://proxy.example.com
HITOKOTO_API_URL: https://hitokoto.example.com
SHARE_STATION:
  URL: https://share.example.com
  TOKEN: share-token
REDIS:
  ENABLED: true
  HOST: redis.example.com
  PORT: 6380
  PASSWORD: redis-pass
  DB: 2
`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(path)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}

	if len(cfg.Monitors) != 2 || cfg.Monitors[0] != 1 || cfg.Monitors[1] != 3 {
		t.Errorf("monitors = %v, want [1, 3]", cfg.Monitors)
	}
	if len(cfg.TestAccountIDs) != 2 || cfg.TestAccountIDs[0] != 123 || cfg.TestAccountIDs[1] != 456 {
		t.Errorf("test account ids = %v, want [123, 456]", cfg.TestAccountIDs)
	}
	if cfg.ServerName != "Full Server" {
		t.Errorf("server name = %q, want Full Server", cfg.ServerName)
	}
	if cfg.Host != "127.0.0.1" {
		t.Errorf("host = %q, want 127.0.0.1", cfg.Host)
	}
	if cfg.Port != 8888 {
		t.Errorf("port = %d, want 8888", cfg.Port)
	}
	if !cfg.HTTPService {
		t.Error("HTTP_SERVICE should be true")
	}
	if cfg.HTTPPort != 8889 {
		t.Errorf("http port = %d, want 8889", cfg.HTTPPort)
	}
	if cfg.RoomMaxUsers != 8 {
		t.Errorf("room max users = %d, want 8", cfg.RoomMaxUsers)
	}
	if cfg.RoomCreationEnabled {
		t.Error("ROOM_CREATION_ENABLED should be false")
	}
	if cfg.ChatEnabled {
		t.Error("CHAT_ENABLED should be false")
	}
	if !cfg.ReplayEnabled {
		t.Error("REPLAY_ENABLED should be true")
	}
	if cfg.ReplayBaseDir != "/tmp/record" {
		t.Errorf("replay base dir = %q, want /tmp/record", cfg.ReplayBaseDir)
	}
	if !cfg.ReplayAutoUpload {
		t.Error("REPLAY_AUTO_UPLOAD should be true")
	}
	if cfg.AdminToken != "secret" {
		t.Errorf("admin token = %q, want secret", cfg.AdminToken)
	}
	if cfg.AdminDataPath != "/tmp/admin.json" {
		t.Errorf("admin data path = %q, want /tmp/admin.json", cfg.AdminDataPath)
	}
	if cfg.RoomListTip != "hello" {
		t.Errorf("room list tip = %q, want hello", cfg.RoomListTip)
	}
	if cfg.LogLevel != "DEBUG" {
		t.Errorf("log level = %q, want DEBUG", cfg.LogLevel)
	}
	if cfg.RealIPHeader != "X-Real-IP" {
		t.Errorf("real ip header = %q, want X-Real-IP", cfg.RealIPHeader)
	}
	if !cfg.HAProxyProtocol {
		t.Error("HAPROXY_PROTOCOL should be true")
	}
	if cfg.Lang != "en-US" {
		t.Errorf("lang = %q, want en-US", cfg.Lang)
	}
	if cfg.PhiraAPIEndpoint != "https://api.example.com" {
		t.Errorf("phira api endpoint = %q, want https://api.example.com", cfg.PhiraAPIEndpoint)
	}
	if cfg.OutboundProxy != "http://proxy.example.com" {
		t.Errorf("outbound proxy = %q, want http://proxy.example.com", cfg.OutboundProxy)
	}
	if cfg.HitokotoAPIURL != "https://hitokoto.example.com" {
		t.Errorf("hitokoto api url = %q, want https://hitokoto.example.com", cfg.HitokotoAPIURL)
	}
	if cfg.ShareStation == nil {
		t.Fatal("share station should not be nil")
	}
	if cfg.ShareStation.URL != "https://share.example.com" {
		t.Errorf("share station url = %q", cfg.ShareStation.URL)
	}
	if cfg.ShareStation.Token != "share-token" {
		t.Errorf("share station token = %q", cfg.ShareStation.Token)
	}
	if cfg.Redis == nil {
		t.Fatal("redis should not be nil")
	}
	if !cfg.Redis.Enabled {
		t.Error("redis enabled should be true")
	}
	if cfg.Redis.Host != "redis.example.com" {
		t.Errorf("redis host = %q", cfg.Redis.Host)
	}
	if cfg.Redis.Port != 6380 {
		t.Errorf("redis port = %d", cfg.Redis.Port)
	}
	if cfg.Redis.Password != "redis-pass" {
		t.Errorf("redis password = %q", cfg.Redis.Password)
	}
	if cfg.Redis.DB != 2 {
		t.Errorf("redis db = %d", cfg.Redis.DB)
	}
}

func TestLoadEnvConfig(t *testing.T) {
	t.Setenv("MONITORS", "1,3,5")
	t.Setenv("TEST_ACCOUNT_IDS", "100,200")
	t.Setenv("SERVER_NAME", "Env Server")
	t.Setenv("HOST", "0.0.0.0")
	t.Setenv("PORT", "7777")
	t.Setenv("HTTP_SERVICE", "true")
	t.Setenv("HTTP_PORT", "7778")
	t.Setenv("ROOM_MAX_USERS", "6")
	t.Setenv("ROOM_CREATION_ENABLED", "false")
	t.Setenv("CHAT_ENABLED", "false")
	t.Setenv("REPLAY_ENABLED", "true")
	t.Setenv("REPLAY_BASE_DIR", "/env/record")
	t.Setenv("REPLAY_AUTO_UPLOAD", "true")
	t.Setenv("ADMIN_TOKEN", "env-token")
	t.Setenv("ADMIN_DATA_PATH", "/env/admin.json")
	t.Setenv("ROOM_LIST_TIP", "env-tip")
	t.Setenv("LOG_LEVEL", "WARN")
	t.Setenv("REAL_IP_HEADER", "X-Env-IP")
	t.Setenv("HAPROXY_PROTOCOL", "true")
	t.Setenv("PHIRA_MP_LANG", "ja-JP")
	t.Setenv("PHIRA_API_ENDPOINT", "https://env.api.com")
	t.Setenv("OUTBOUND_PROXY", "http://env.proxy")
	t.Setenv("HITOKOTO_API_URL", "https://env.hitokoto.com")
	t.Setenv("SHARE_STATION_URL", "https://env.share.com")
	t.Setenv("SHARE_STATION_TOKEN", "env-share-token")
	t.Setenv("REDIS_ENABLED", "true")
	t.Setenv("REDIS_HOST", "env.redis")
	t.Setenv("REDIS_PORT", "6380")
	t.Setenv("REDIS_PASSWORD", "env-redis-pass")
	t.Setenv("REDIS_DB", "3")

	cfg := config.LoadEnvConfig()

	if len(cfg.Monitors) != 3 || cfg.Monitors[0] != 1 || cfg.Monitors[1] != 3 || cfg.Monitors[2] != 5 {
		t.Errorf("monitors = %v", cfg.Monitors)
	}
	if len(cfg.TestAccountIDs) != 2 || cfg.TestAccountIDs[0] != 100 || cfg.TestAccountIDs[1] != 200 {
		t.Errorf("test account ids = %v", cfg.TestAccountIDs)
	}
	if cfg.ServerName != "Env Server" {
		t.Errorf("server name = %q", cfg.ServerName)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("host = %q", cfg.Host)
	}
	if cfg.Port != 7777 {
		t.Errorf("port = %d", cfg.Port)
	}
	if !cfg.HTTPService {
		t.Error("HTTP_SERVICE should be true")
	}
	if cfg.HTTPPort != 7778 {
		t.Errorf("http port = %d", cfg.HTTPPort)
	}
	if cfg.RoomMaxUsers != 6 {
		t.Errorf("room max users = %d", cfg.RoomMaxUsers)
	}
	if cfg.RoomCreationEnabled {
		t.Error("ROOM_CREATION_ENABLED should be false")
	}
	if cfg.ChatEnabled {
		t.Error("CHAT_ENABLED should be false")
	}
	if !cfg.ReplayEnabled {
		t.Error("REPLAY_ENABLED should be true")
	}
	if cfg.ReplayBaseDir != "/env/record" {
		t.Errorf("replay base dir = %q", cfg.ReplayBaseDir)
	}
	if !cfg.ReplayAutoUpload {
		t.Error("REPLAY_AUTO_UPLOAD should be true")
	}
	if cfg.AdminToken != "env-token" {
		t.Errorf("admin token = %q", cfg.AdminToken)
	}
	if cfg.AdminDataPath != "/env/admin.json" {
		t.Errorf("admin data path = %q", cfg.AdminDataPath)
	}
	if cfg.RoomListTip != "env-tip" {
		t.Errorf("room list tip = %q", cfg.RoomListTip)
	}
	if cfg.LogLevel != "WARN" {
		t.Errorf("log level = %q", cfg.LogLevel)
	}
	if cfg.RealIPHeader != "X-Env-IP" {
		t.Errorf("real ip header = %q", cfg.RealIPHeader)
	}
	if !cfg.HAProxyProtocol {
		t.Error("HAPROXY_PROTOCOL should be true")
	}
	if cfg.Lang != "ja-JP" {
		t.Errorf("lang = %q", cfg.Lang)
	}
	if cfg.PhiraAPIEndpoint != "https://env.api.com" {
		t.Errorf("phira api endpoint = %q", cfg.PhiraAPIEndpoint)
	}
	if cfg.OutboundProxy != "http://env.proxy" {
		t.Errorf("outbound proxy = %q", cfg.OutboundProxy)
	}
	if cfg.HitokotoAPIURL != "https://env.hitokoto.com" {
		t.Errorf("hitokoto api url = %q", cfg.HitokotoAPIURL)
	}
	if cfg.ShareStation == nil {
		t.Fatal("share station should not be nil")
	}
	if cfg.ShareStation.URL != "https://env.share.com" || cfg.ShareStation.Token != "env-share-token" {
		t.Errorf("share station = %+v", cfg.ShareStation)
	}
	if cfg.Redis == nil {
		t.Fatal("redis should not be nil")
	}
	if !cfg.Redis.Enabled || cfg.Redis.Host != "env.redis" || cfg.Redis.Port != 6380 || cfg.Redis.Password != "env-redis-pass" || cfg.Redis.DB != 3 {
		t.Errorf("redis = %+v", cfg.Redis)
	}
}

func TestMergeConfigNilCases(t *testing.T) {
	// nil, nil -> defaults
	merged := config.MergeConfig(nil, nil)
	if merged == nil {
		t.Fatal("MergeConfig(nil, nil) should not be nil")
	}
	if merged.Port != 12346 {
		t.Errorf("port = %d, want default 12346", merged.Port)
	}

	// nil, override
	override := &config.ServerConfig{Port: 9999, ServerName: "Override"}
	merged = config.MergeConfig(nil, override)
	if merged.Port != 9999 || merged.ServerName != "Override" {
		t.Errorf("MergeConfig(nil, override) = %+v", merged)
	}

	// base, nil
	base := &config.ServerConfig{Port: 1111, ServerName: "Base"}
	merged = config.MergeConfig(base, nil)
	if merged.Port != 1111 || merged.ServerName != "Base" {
		t.Errorf("MergeConfig(base, nil) = %+v", merged)
	}
}

func TestMergeConfigBoolFields(t *testing.T) {
	base := config.DefaultConfig()
	override := &config.ServerConfig{}
	override.HTTPService = true
	override.ChatEnabled = false
	override.ReplayEnabled = true
	override.ReplayAutoUpload = true
	override.HAProxyProtocol = true
	override.RoomCreationEnabled = false

	merged := config.MergeConfig(base, override)
	if !merged.HTTPService {
		t.Error("HTTPService should be true")
	}
	if merged.ChatEnabled {
		t.Error("ChatEnabled should be false")
	}
	if !merged.ReplayEnabled {
		t.Error("ReplayEnabled should be true")
	}
	if !merged.ReplayAutoUpload {
		t.Error("ReplayAutoUpload should be true")
	}
	if !merged.HAProxyProtocol {
		t.Error("HAProxyProtocol should be true")
	}
	if merged.RoomCreationEnabled {
		t.Error("RoomCreationEnabled should be false")
	}
}

func TestMergeConfigNested(t *testing.T) {
	base := config.DefaultConfig()
	override := &config.ServerConfig{
		ShareStation: &config.ShareStationConfig{URL: "https://share.com", Token: "tok"},
		Redis: &config.RedisConfig{
			Enabled:  true,
			Host:     "redis.com",
			Port:     6380,
			Password: "pass",
			DB:       2,
		},
	}

	merged := config.MergeConfig(base, override)
	if merged.ShareStation == nil || merged.ShareStation.URL != "https://share.com" {
		t.Errorf("share station = %+v", merged.ShareStation)
	}
	if merged.Redis == nil || merged.Redis.Host != "redis.com" || merged.Redis.Port != 6380 {
		t.Errorf("redis = %+v", merged.Redis)
	}
}

func TestWatcher(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(path, []byte("PORT: 1111\n"), 0644); err != nil {
		t.Fatal(err)
	}

	changed := make(chan struct{}, 1)
	w := config.NewWatcher(path, 50*time.Millisecond, func() {
		changed <- struct{}{}
	})
	w.Start()
	defer w.Stop()

	// modify file
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(path, []byte("PORT: 2222\n"), 0644); err != nil {
		t.Fatal(err)
	}

	select {
	case <-changed:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not detect change")
	}
}
