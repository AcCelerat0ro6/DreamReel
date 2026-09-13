package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"DreamReel/internal/infra/config"
	"DreamReel/internal/infra/database"
	"DreamReel/internal/infra/httpgin"
	"DreamReel/internal/infra/log"
	"DreamReel/internal/infra/redis"
	"DreamReel/internal/interfaces/http/router"

	"go.uber.org/zap"
)

const defaultConfigPath = "G:\\GoCode\\DreamReel\\apps\\api\\configs\\config.yaml"

func main() {
	if err := run(); err != nil {
		zap.L().Error("启动失败", zap.Error(err))
		_ = zap.L().Sync()
		os.Exit(1)
	}
}

// 启动顺序：配置 -> 日志 -> 信号 ctx -> 数据库 -> Redis -> Gin -> 路由注册 -> 启动后端服务。
func run() error {
	// 获取配置文件路径
	configPath := flag.String("config", defaultConfigPath, "配置文件路径")
	flag.Parse()

	// 加载配置
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	// 初始化日志
	if err := log.Init(&cfg.Logger); err != nil {
		return fmt.Errorf("初始化日志失败: %w", err)
	}
	defer func() { _ = zap.L().Sync() }()

	// 监听 SIGINT / SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 初始化数据库连接
	db, err := database.New(&cfg.Database)
	if err != nil {
		return fmt.Errorf("初始化数据库失败: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			zap.L().Warn("关闭数据库连接失败", zap.Error(err))
		}
	}()
	zap.L().Info("数据库连接成功")

	// 初始化Redis连接
	rdb, err := redis.NewRedisClient(ctx, cfg.Redis)
	if err != nil {
		return fmt.Errorf("初始化 Redis 失败: %w", err)
	}
	defer func() {
		if err := rdb.Close(); err != nil {
			zap.L().Warn("关闭 Redis 连接失败", zap.Error(err))
		}
	}()
	zap.L().Info("Redis 连接成功")

	// 初始化gin引擎
	g := httpgin.Init(cfg.Mode)

	if err := router.Register(g, cfg, db, rdb); err != nil {
		return fmt.Errorf("注册路由失败: %w", err)
	}
	zap.L().Info("路由注册成功")

	zap.L().Info("服务器正在运行中",
		zap.Int("port", cfg.Port),
		zap.String("mode", cfg.Mode),
	)

	// 启动后端服务
	if err := httpgin.Run(ctx, cfg, g); err != nil {
		return fmt.Errorf("后端服务出错: %w", err)
	}

	zap.L().Info("后端服务已关闭")
	return nil
}
