package utils

import (
	"context"
	"fmt"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/config"
	"github.com/redis/go-redis/v9"
)

var (
	redisClient *redis.Client
	redisCtx    = context.Background()
)

// InitRedis initializes the global Redis client from config.
// If the config is nil or disabled, it disconnects any existing client.
func InitRedis(cfg *config.RedisConfig) error {
	if redisClient != nil {
		_ = redisClient.Close()
		redisClient = nil
	}

	if cfg == nil || !cfg.Enabled {
		return nil
	}

	host := cfg.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := cfg.Port
	if port == 0 {
		port = 6379
	}

	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", host, port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(redisCtx, 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return err
	}

	redisClient = client
	return nil
}

// GetRedisClient returns the global Redis client, or nil if not initialized.
func GetRedisClient() *redis.Client {
	return redisClient
}

// CloseRedis disconnects the global Redis client.
func CloseRedis() {
	if redisClient != nil {
		_ = redisClient.Close()
		redisClient = nil
	}
}
