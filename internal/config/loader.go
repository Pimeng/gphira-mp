package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadConfig loads configuration from a YAML file and merges it with defaults.
// Only fields present in the YAML file override the defaults.
func LoadConfig(path string) (*ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	cfg := DefaultConfig()

	if v, ok := raw["MONITORS"]; ok {
		if val, ok := parseIntegerListFromInterface(v); ok {
			cfg.Monitors = val
		}
	}
	if v, ok := raw["TEST_ACCOUNT_IDS"]; ok {
		if val, ok := parseIntegerListFromInterface(v); ok {
			cfg.TestAccountIDs = val
		}
	}
	if v, ok := raw["SERVER_NAME"]; ok {
		if val, ok := parseStringFromInterface(v); ok {
			cfg.ServerName = val
		}
	}
	if v, ok := raw["HOST"]; ok {
		if val, ok := parseStringFromInterface(v); ok {
			cfg.Host = val
		}
	}
	if v, ok := raw["PORT"]; ok {
		if val, ok := parsePortFromInterface(v); ok {
			cfg.Port = val
		}
	}
	if v, ok := raw["HTTP_SERVICE"]; ok {
		if val, ok := parseBoolFromInterface(v); ok {
			cfg.HTTPService = val
		}
	}
	if v, ok := raw["HTTP_PORT"]; ok {
		if val, ok := parsePortFromInterface(v); ok {
			cfg.HTTPPort = val
		}
	}
	if v, ok := raw["ROOM_MAX_USERS"]; ok {
		if val, ok := parseRoomMaxUsersFromInterface(v); ok {
			cfg.RoomMaxUsers = val
		}
	}
	if v, ok := raw["ROOM_CREATION_ENABLED"]; ok {
		if val, ok := parseBoolFromInterface(v); ok {
			cfg.RoomCreationEnabled = val
		}
	}
	if v, ok := raw["CHAT_ENABLED"]; ok {
		if val, ok := parseBoolFromInterface(v); ok {
			cfg.ChatEnabled = val
		}
	}
	if v, ok := raw["REPLAY_ENABLED"]; ok {
		if val, ok := parseBoolFromInterface(v); ok {
			cfg.ReplayEnabled = val
		}
	}
	if v, ok := raw["REPLAY_BASE_DIR"]; ok {
		if val, ok := parseStringFromInterface(v); ok {
			cfg.ReplayBaseDir = val
		}
	}
	if v, ok := raw["REPLAY_AUTO_UPLOAD"]; ok {
		if val, ok := parseBoolFromInterface(v); ok {
			cfg.ReplayAutoUpload = val
		}
	}
	if v, ok := raw["ADMIN_TOKEN"]; ok {
		if val, ok := parseStringFromInterface(v); ok {
			cfg.AdminToken = val
		}
	}
	if v, ok := raw["ADMIN_DATA_PATH"]; ok {
		if val, ok := parseStringFromInterface(v); ok {
			cfg.AdminDataPath = val
		}
	}
	if v, ok := raw["ROOM_LIST_TIP"]; ok {
		if val, ok := parseStringFromInterface(v); ok {
			cfg.RoomListTip = val
		}
	}
	if v, ok := raw["LOG_LEVEL"]; ok {
		if val, ok := parseStringFromInterface(v); ok {
			cfg.LogLevel = val
		}
	}
	if v, ok := raw["REAL_IP_HEADER"]; ok {
		if val, ok := parseStringFromInterface(v); ok {
			cfg.RealIPHeader = val
		}
	}
	if v, ok := raw["HAPROXY_PROTOCOL"]; ok {
		if val, ok := parseBoolFromInterface(v); ok {
			cfg.HAProxyProtocol = val
		}
	}
	if v, ok := raw["LANG"]; ok {
		if val, ok := parseStringFromInterface(v); ok {
			cfg.Lang = val
		}
	}
	if v, ok := raw["PHIRA_API_ENDPOINT"]; ok {
		if val, ok := parseStringFromInterface(v); ok {
			cfg.PhiraAPIEndpoint = val
		}
	}
	if v, ok := raw["OUTBOUND_PROXY"]; ok {
		if val, ok := parseOutboundProxyFromInterface(v); ok {
			cfg.OutboundProxy = val
		}
	}
	if v, ok := raw["SHARE_STATION"]; ok {
		if val, ok := parseShareStationFromInterface(v); ok {
			cfg.ShareStation = val
		}
	}
	if v, ok := raw["REDIS"]; ok {
		if val, ok := parseRedisFromInterface(v); ok {
			cfg.Redis = val
		}
	}
	if v, ok := raw["HITOKOTO_API_URL"]; ok {
		if val, ok := parseStringFromInterface(v); ok {
			cfg.HitokotoAPIURL = val
		}
	}

	return cfg, nil
}

// LoadEnvConfig loads configuration from environment variables.
// Only fields that are explicitly set in the environment are populated;
// the returned config is intended to be merged with a base config.
func LoadEnvConfig() *ServerConfig {
	return loadEnvConfigInternal()
}

// MergeConfig merges two configurations where override takes precedence over base.
// Fields in override that have non-zero values (or non-empty slices) replace the
// corresponding fields in base. This function is intended for merging sparse
// override configs (e.g., from environment variables) into complete base configs.
func MergeConfig(base, override *ServerConfig) *ServerConfig {
	if base == nil && override == nil {
		return DefaultConfig()
	}
	if base == nil {
		return cloneServerConfig(override)
	}
	if override == nil {
		return cloneServerConfig(base)
	}

	merged := cloneServerConfig(base)

	if len(override.Monitors) > 0 {
		merged.Monitors = cloneIntSlice(override.Monitors)
	}
	if len(override.TestAccountIDs) > 0 {
		merged.TestAccountIDs = cloneIntSlice(override.TestAccountIDs)
	}
	if override.ServerName != "" {
		merged.ServerName = override.ServerName
	}
	if override.Host != "" {
		merged.Host = override.Host
	}
	if override.Port != 0 {
		merged.Port = override.Port
	}
	if override.HTTPPort != 0 {
		merged.HTTPPort = override.HTTPPort
	}
	if override.RoomMaxUsers != 0 {
		merged.RoomMaxUsers = override.RoomMaxUsers
	}
	if override.ReplayBaseDir != "" {
		merged.ReplayBaseDir = override.ReplayBaseDir
	}
	if override.AdminToken != "" {
		merged.AdminToken = override.AdminToken
	}
	if override.AdminDataPath != "" {
		merged.AdminDataPath = override.AdminDataPath
	}
	if override.RoomListTip != "" {
		merged.RoomListTip = override.RoomListTip
	}
	if override.LogLevel != "" {
		merged.LogLevel = override.LogLevel
	}
	if override.RealIPHeader != "" {
		merged.RealIPHeader = override.RealIPHeader
	}
	if override.Lang != "" {
		merged.Lang = override.Lang
	}
	if override.PhiraAPIEndpoint != "" {
		merged.PhiraAPIEndpoint = override.PhiraAPIEndpoint
	}
	if override.OutboundProxy != "" {
		merged.OutboundProxy = override.OutboundProxy
	}
	if override.HitokotoAPIURL != "" {
		merged.HitokotoAPIURL = override.HitokotoAPIURL
	}
	if override.ShareStation != nil {
		ss := *override.ShareStation
		merged.ShareStation = &ss
	}
	if override.Redis != nil {
		r := *override.Redis
		merged.Redis = &r
	}

	// For bool fields, we copy them unconditionally because we cannot distinguish
	// "unset" from "explicitly false" in a sparse override config. Callers should
	// ensure that override configs only set bools that are explicitly intended to
	// override the base (e.g., from LoadEnvConfig which only sets bools when the
	// corresponding environment variable is present).
	merged.HTTPService = override.HTTPService
	merged.ChatEnabled = override.ChatEnabled
	merged.ReplayEnabled = override.ReplayEnabled
	merged.ReplayAutoUpload = override.ReplayAutoUpload
	merged.HAProxyProtocol = override.HAProxyProtocol
	merged.RoomCreationEnabled = override.RoomCreationEnabled

	return merged
}

// parseStringFromInterface extracts a non-empty string from an interface{}.
func parseStringFromInterface(v interface{}) (string, bool) {
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return ParseString(s)
}

// parseBoolFromInterface extracts a bool from an interface{}.
func parseBoolFromInterface(v interface{}) (bool, bool) {
	switch val := v.(type) {
	case bool:
		return val, true
	case int:
		if val == 1 {
			return true, true
		}
		if val == 0 {
			return false, true
		}
		return false, false
	case string:
		return ParseBool(val)
	default:
		return false, false
	}
}

// parsePortFromInterface extracts a valid port from an interface{}.
func parsePortFromInterface(v interface{}) (int, bool) {
	switch val := v.(type) {
	case int:
		if val > 0 && val <= 65535 {
			return val, true
		}
		return 0, false
	case string:
		return ParsePort(val)
	default:
		return 0, false
	}
}

// parseRoomMaxUsersFromInterface extracts room max users from an interface{}.
func parseRoomMaxUsersFromInterface(v interface{}) (int, bool) {
	switch val := v.(type) {
	case int:
		if val < 1 {
			return 0, false
		}
		if val > 64 {
			val = 64
		}
		return val, true
	case string:
		return ParseRoomMaxUsers(val)
	default:
		return 0, false
	}
}

// parseIntegerListFromInterface extracts a list of integers from an interface{}.
func parseIntegerListFromInterface(v interface{}) ([]int, bool) {
	switch val := v.(type) {
	case []interface{}:
		var out []int
		for _, item := range val {
			switch iv := item.(type) {
			case int:
				out = append(out, iv)
			case string:
				if n, err := strconv.Atoi(strings.TrimSpace(iv)); err == nil {
					out = append(out, n)
				}
			}
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	case string:
		return ParseIntegerList(val)
	case int:
		return []int{val}, true
	default:
		return nil, false
	}
}

// parseOutboundProxyFromInterface extracts outbound proxy from an interface{}.
func parseOutboundProxyFromInterface(v interface{}) (string, bool) {
	switch val := v.(type) {
	case bool:
		if !val {
			return "false", true
		}
		return "", false
	case string:
		return ParseOutboundProxy(val)
	default:
		return "", false
	}
}

// parseShareStationFromInterface extracts ShareStationConfig from an interface{}.
func parseShareStationFromInterface(v interface{}) (*ShareStationConfig, bool) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, false
	}
	url, urlOK := parseStringFromInterface(m["URL"])
	token, tokenOK := parseStringFromInterface(m["TOKEN"])
	if !urlOK || !tokenOK {
		return nil, false
	}
	return &ShareStationConfig{URL: url, Token: token}, true
}

// parseRedisFromInterface extracts RedisConfig from an interface{}.
func parseRedisFromInterface(v interface{}) (*RedisConfig, bool) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, false
	}
	enabled, ok := parseBoolFromInterface(m["ENABLED"])
	if !ok {
		return nil, false
	}
	host := "127.0.0.1"
	if h, ok := parseStringFromInterface(m["HOST"]); ok {
		host = h
	}
	port := 6379
	if p, ok := parsePortFromInterface(m["PORT"]); ok {
		port = p
	}
	password, _ := parseStringFromInterface(m["PASSWORD"])
	db := 0
	if d, ok := m["DB"]; ok {
		switch dv := d.(type) {
		case int:
			if dv >= 0 {
				db = dv
			}
		case string:
			if v, err := strconv.Atoi(strings.TrimSpace(dv)); err == nil && v >= 0 {
				db = v
			}
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
