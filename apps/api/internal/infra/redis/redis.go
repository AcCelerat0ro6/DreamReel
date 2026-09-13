package redis

import (
	"context"
	"time"

	"DreamReel/internal/infra/config"

	"github.com/redis/go-redis/v9"
)

const (
	defaultPoolSize     = 50
	defaultMinIdleConns = 10
	defaultDialTimeout  = 2 * time.Second
	defaultReadTimeout  = 1 * time.Second
	defaultWriteTimeout = 1 * time.Second
	defaultPoolTimeout  = 2 * time.Second
)

// NewRedisClient 创建 Redis 客户端
func NewRedisClient(ctx context.Context, rediscfg config.RedisConfig) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         rediscfg.Addr,
		Password:     rediscfg.Password,
		DB:           rediscfg.DB,
		PoolSize:     config.PositiveInt(rediscfg.PoolSize, defaultPoolSize),
		MinIdleConns: config.PositiveInt(rediscfg.MinIdleConns, defaultMinIdleConns),
		DialTimeout:  config.ParseDuration(rediscfg.DialTimeout, defaultDialTimeout),
		ReadTimeout:  config.ParseDuration(rediscfg.ReadTimeout, defaultReadTimeout),
		WriteTimeout: config.ParseDuration(rediscfg.WriteTimeout, defaultWriteTimeout),
		PoolTimeout:  config.ParseDuration(rediscfg.PoolTimeout, defaultPoolTimeout),
	})

	// 执行Ping操作
	pingTimeout := config.ParseDuration(rediscfg.DialTimeout, defaultDialTimeout) +
		config.ParseDuration(rediscfg.ReadTimeout, defaultReadTimeout)
	pingCtx, cancelPing := context.WithTimeout(ctx, pingTimeout)
	defer cancelPing()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, err
	}
	return rdb, nil
}
