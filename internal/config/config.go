package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ServerConfig holds all server configuration fields.
type ServerConfig struct {
	Monitors         []int              `yaml:"MONITORS"`
	TestAccountIDs   []int              `yaml:"TEST_ACCOUNT_IDS"`
	ServerName       string             `yaml:"SERVER_NAME"`
	Host             string             `yaml:"HOST"`
	Port             int                `yaml:"PORT"`
	HTTPService      bool               `yaml:"HTTP_SERVICE"`
	HTTPPort         int                `yaml:"HTTP_PORT"`
	RoomMaxUsers     int                `yaml:"ROOM_MAX_USERS"`
	ChatEnabled      bool               `yaml:"CHAT_ENABLED"`
	ReplayEnabled    bool               `yaml:"REPLAY_ENABLED"`
	ReplayBaseDir    string             `yaml:"REPLAY_BASE_DIR"`
	ReplayAutoUpload bool               `yaml:"REPLAY_AUTO_UPLOAD"`
	AdminToken       string             `yaml:"ADMIN_TOKEN"`
	AdminDataPath    string             `yaml:"ADMIN_DATA_PATH"`
	RoomListTip      string             `yaml:"ROOM_LIST_TIP"`
	LogLevel         string             `yaml:"LOG_LEVEL"`
	RealIPHeader     string             `yaml:"REAL_IP_HEADER"`
	HAProxyProtocol  bool               `yaml:"HAPROXY_PROTOCOL"`
	Lang             string             `yaml:"LANG"`
	PhiraAPIEndpoint string             `yaml:"PHIRA_API_ENDPOINT"`
	OutboundProxy    string             `yaml:"OUTBOUND_PROXY"`
	ShareStation     *ShareStationConfig `yaml:"SHARE_STATION"`
	Redis            *RedisConfig       `yaml:"REDIS"`
	HitokotoAPIURL      string             `yaml:"HITOKOTO_API_URL"`
	RoomCreationEnabled bool               `yaml:"ROOM_CREATION_ENABLED"`
}

// ShareStationConfig holds share station settings.
type ShareStationConfig struct {
	URL   string `yaml:"URL"`
	Token string `yaml:"TOKEN"`
}

// RedisConfig holds Redis cache settings.
type RedisConfig struct {
	Enabled  bool   `yaml:"ENABLED"`
	Host     string `yaml:"HOST"`
	Port     int    `yaml:"PORT"`
	Password string `yaml:"PASSWORD"`
	DB       int    `yaml:"DB"`
}

// DefaultConfig returns a ServerConfig populated with sensible defaults.
func DefaultConfig() *ServerConfig {
	return &ServerConfig{
		Monitors:         []int{2},
		TestAccountIDs:   []int{1739989},
		ServerName:       "Phira MP",
		Host:             "::",
		Port:             12346,
		HTTPService:      false,
		HTTPPort:         12347,
		RoomMaxUsers:     12,
		ChatEnabled:      true,
		RoomCreationEnabled: true,
		ReplayEnabled:    false,
		ReplayBaseDir:    "./record",
		ReplayAutoUpload: false,
		AdminToken:       "",
		AdminDataPath:    "./admin_data.json",
		RoomListTip:      "",
		LogLevel:         "INFO",
		RealIPHeader:     "X-Forwarded-For",
		HAProxyProtocol:  false,
		Lang:             "zh-CN",
		PhiraAPIEndpoint: "https://phira.5wyxi.com",
		OutboundProxy:    "",
		ShareStation:     nil,
		Redis: &RedisConfig{
			Enabled:  false,
			Host:     "127.0.0.1",
			Port:     6379,
			Password: "",
			DB:       0,
		},
		HitokotoAPIURL: "https://v1.hitokoto.cn/",
	}
}

// ParseBool parses a boolean value from various formats.
func ParseBool(value string) (bool, bool) {
	if value == "" {
		return false, false
	}
	v := strings.TrimSpace(strings.ToLower(value))
	switch v {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	}
	return false, false
}

// ParseString parses a non-empty string value.
func ParseString(value string) (string, bool) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", false
	}
	return v, true
}

// ParsePort parses a valid TCP/UDP port number.
func ParsePort(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	v, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || v <= 0 || v > 65535 {
		return 0, false
	}
	return v, true
}

// ParseRoomMaxUsers parses room max users (1-64).
func ParseRoomMaxUsers(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	v, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || v < 1 {
		return 0, false
	}
	if v > 64 {
		v = 64
	}
	return v, true
}

// ParseIntegerList parses a comma/space/semicolon separated list of integers.
func ParseIntegerList(value string) ([]int, bool) {
	if value == "" {
		return nil, false
	}
	parts := strings.Split(value, ",")
	var out []int
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		subParts := strings.FieldsFunc(p, func(r rune) bool {
			return r == ' ' || r == ';' || r == '，' || r == '\t'
		})
		for _, sp := range subParts {
			sp = strings.TrimSpace(sp)
			if sp == "" {
				continue
			}
			v, err := strconv.Atoi(sp)
			if err != nil {
				continue
			}
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// ParseOutboundProxy parses outbound proxy setting.
func ParseOutboundProxy(value string) (string, bool) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", false
	}
	if strings.EqualFold(v, "false") {
		return "false", true
	}
	return v, true
}

// envOrDefault returns the value of the environment variable if it exists.
func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// getEnvLang returns the language from environment variables.
func getEnvLang() string {
	if v := os.Getenv("PHIRA_MP_LANG"); v != "" {
		return v
	}
	if v := os.Getenv("LANG"); v != "" {
		return v
	}
	return ""
}

// parseShareStationFromEnv builds ShareStationConfig from environment variables.
func parseShareStationFromEnv() (*ShareStationConfig, bool) {
	url, urlOK := ParseString(os.Getenv("SHARE_STATION_URL"))
	token, tokenOK := ParseString(os.Getenv("SHARE_STATION_TOKEN"))
	if !urlOK || !tokenOK {
		return nil, false
	}
	return &ShareStationConfig{URL: url, Token: token}, true
}

// parseRedisFromEnv builds RedisConfig from environment variables.
func parseRedisFromEnv() (*RedisConfig, bool) {
	enabled, ok := ParseBool(os.Getenv("REDIS_ENABLED"))
	if !ok {
		return nil, false
	}
	host := envOrDefault("REDIS_HOST", "127.0.0.1")
	port, _ := ParsePort(os.Getenv("REDIS_PORT"))
	if port == 0 {
		port = 6379
	}
	password, _ := ParseString(os.Getenv("REDIS_PASSWORD"))
	dbRaw := os.Getenv("REDIS_DB")
	db := 0
	if dbRaw != "" {
		v, err := strconv.Atoi(strings.TrimSpace(dbRaw))
		if err == nil && v >= 0 {
			db = v
		}
	}
	return &RedisConfig{
		Enabled:  enabled,
		Host:     host,
		Port:     port,
		Password: password,
		DB:       db,
	}, true
}

// loadEnvConfigInternal loads configuration from environment variables.
func loadEnvConfigInternal() *ServerConfig {
	cfg := &ServerConfig{}
	if v, ok := ParseIntegerList(os.Getenv("MONITORS")); ok {
		cfg.Monitors = v
	}
	if v, ok := ParseIntegerList(os.Getenv("TEST_ACCOUNT_IDS")); ok {
		cfg.TestAccountIDs = v
	}
	if v, ok := ParseString(os.Getenv("SERVER_NAME")); ok {
		cfg.ServerName = v
	}
	if v, ok := ParseString(os.Getenv("HOST")); ok {
		cfg.Host = v
	}
	if v, ok := ParsePort(os.Getenv("PORT")); ok {
		cfg.Port = v
	}
	if v, ok := ParseBool(os.Getenv("HTTP_SERVICE")); ok {
		cfg.HTTPService = v
	}
	if v, ok := ParsePort(os.Getenv("HTTP_PORT")); ok {
		cfg.HTTPPort = v
	}
	if v, ok := ParseRoomMaxUsers(os.Getenv("ROOM_MAX_USERS")); ok {
		cfg.RoomMaxUsers = v
	}
	if v, ok := ParseBool(os.Getenv("CHAT_ENABLED")); ok {
		cfg.ChatEnabled = v
	}
	if v, ok := ParseBool(os.Getenv("REPLAY_ENABLED")); ok {
		cfg.ReplayEnabled = v
	}
	if v, ok := ParseString(os.Getenv("REPLAY_BASE_DIR")); ok {
		cfg.ReplayBaseDir = v
	}
	if v, ok := ParseBool(os.Getenv("REPLAY_AUTO_UPLOAD")); ok {
		cfg.ReplayAutoUpload = v
	}
	if v, ok := ParseString(os.Getenv("ADMIN_TOKEN")); ok {
		cfg.AdminToken = v
	}
	if v, ok := ParseString(os.Getenv("ADMIN_DATA_PATH")); ok {
		cfg.AdminDataPath = v
	}
	if v, ok := ParseString(os.Getenv("ROOM_LIST_TIP")); ok {
		cfg.RoomListTip = v
	}
	if v, ok := ParseString(os.Getenv("LOG_LEVEL")); ok {
		cfg.LogLevel = v
	}
	if v, ok := ParseString(os.Getenv("REAL_IP_HEADER")); ok {
		cfg.RealIPHeader = v
	}
	if v, ok := ParseBool(os.Getenv("HAPROXY_PROTOCOL")); ok {
		cfg.HAProxyProtocol = v
	}
	if v, ok := ParseString(getEnvLang()); ok {
		cfg.Lang = v
	}
	if v, ok := ParseString(os.Getenv("PHIRA_API_ENDPOINT")); ok {
		cfg.PhiraAPIEndpoint = v
	}
	if v, ok := ParseOutboundProxy(os.Getenv("OUTBOUND_PROXY")); ok {
		cfg.OutboundProxy = v
	}
	if ss, ok := parseShareStationFromEnv(); ok {
		cfg.ShareStation = ss
	}
	if r, ok := parseRedisFromEnv(); ok {
		cfg.Redis = r
	}
	if v, ok := ParseBool(os.Getenv("ROOM_CREATION_ENABLED")); ok {
		cfg.RoomCreationEnabled = v
	}
	if v, ok := ParseString(os.Getenv("HITOKOTO_API_URL")); ok {
		cfg.HitokotoAPIURL = v
	}
	return cfg
}

// cloneIntSlice returns a copy of an int slice.
func cloneIntSlice(src []int) []int {
	if src == nil {
		return nil
	}
	dst := make([]int, len(src))
	copy(dst, src)
	return dst
}

// cloneServerConfig returns a deep copy of ServerConfig.
func cloneServerConfig(src *ServerConfig) *ServerConfig {
	if src == nil {
		return nil
	}
	dst := *src
	dst.Monitors = cloneIntSlice(src.Monitors)
	dst.TestAccountIDs = cloneIntSlice(src.TestAccountIDs)
	if src.ShareStation != nil {
		ss := *src.ShareStation
		dst.ShareStation = &ss
	}
	if src.Redis != nil {
		r := *src.Redis
		dst.Redis = &r
	}
	return &dst
}

// fmtErrorf is a helper to format errors.
func fmtErrorf(format string, args ...interface{}) error {
	return fmt.Errorf(format, args...)
}
