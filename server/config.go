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
	Monitors []int32 `yaml:"monitors"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() ServerConfig {
	return ServerConfig{
		Monitors: []int32{2},
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

// Record 游戏记录
type Record struct {
	ID         int32   `json:"id"`
	Player     int32   `json:"player"`
	Score      int32   `json:"score"`
	Perfect    int32   `json:"perfect"`
	Good       int32   `json:"good"`
	Bad        int32   `json:"bad"`
	Miss       int32   `json:"miss"`
	MaxCombo   int32   `json:"max_combo"`
	Accuracy   float32 `json:"accuracy"`
	FullCombo  bool    `json:"full_combo"`
	Std        float32 `json:"std"`
	StdScore   float32 `json:"std_score"`
}
