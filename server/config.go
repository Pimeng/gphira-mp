package server

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Chart 谱面信息
type Chart struct {
	ID   int32  `json:"id"`
	Name string `json:"name"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	LiveMode bool    `yaml:"live_mode"` // 是否启用直播模式
	Monitors []int32 `yaml:"monitors"`  // 允许观察的用户ID列表（直播模式启用时生效）
	LogLevel string  `yaml:"log_level"` // debug, info, warn, error
}

// DefaultConfig 返回默认配置
func DefaultConfig() ServerConfig {
	return ServerConfig{
		LiveMode: false, // 默认关闭直播模式
		Monitors: []int32{2},
		LogLevel: "info", // 默认只输出 info 级别日志
	}
}

// LoadConfig 从文件加载配置
func LoadConfig(path string) (ServerConfig, error) {
	config := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return config, nil // 文件不存在返回默认配置
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return config, err
	}

	return config, nil
}

// IsDebugEnabled 是否启用 DEBUG 日志
func (c *ServerConfig) IsDebugEnabled() bool {
	return c.LogLevel == "debug"
}

// Record 游戏记录
type Record struct {
	ID        int32   `json:"id"`
	Player    int32   `json:"player"`
	Score     int32   `json:"score"`
	Perfect   int32   `json:"perfect"`
	Good      int32   `json:"good"`
	Bad       int32   `json:"bad"`
	Miss      int32   `json:"miss"`
	MaxCombo  int32   `json:"max_combo"`
	Accuracy  float32 `json:"accuracy"`
	FullCombo bool    `json:"full_combo"`
	Std       float32 `json:"std"`
	StdScore  float32 `json:"std_score"`
}
