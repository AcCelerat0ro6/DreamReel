package main

import (
	applicationinteraction "DreamReel/internal/application/interaction"
	"DreamReel/internal/infra/config"
	"DreamReel/internal/infra/database"
	applog "DreamReel/internal/infra/log"
	"DreamReel/internal/infra/metrics"
	mq "DreamReel/internal/infra/mq"
	infrainteraction "DreamReel/internal/infra/persistence/interaction"
	"DreamReel/internal/infra/persistence/migration"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
	gormmysql "gorm.io/driver/mysql"

	"gorm.io/gorm"
)

const configPath = "./configs/config.yaml"

func main() {
	if err := run(); err != nil {
		zap.L().Error("gcfeed worker 启动失败", zap.Error(err))
		_ = zap.L().Sync()
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		// 日志初始化前 zap.L() 仍是空实现，用标准库 log 写 stderr 保证早期启动错误可见
		log.Printf("load config failed: %v", err)
		return fmt.Errorf("加载配置失败: %w", err)
	}
	if err := applog.Init(&cfg.Logger); err != nil {
		log.Printf("init logger failed: %v", err)
		return fmt.Errorf("初始化日志失败: %w", err)
	}
	defer func() { _ = zap.L().Sync() }()

	if cfg.RabbitMQ.URL == "" {
		return errors.New("rabbitmq url is required for worker")
	}
	if cfg.Redis.Addr == "" {
		return errors.New("redis addr is required for worker")
	}

	sqlDB, err := database.New(&cfg.Database)
	if err != nil {
		return fmt.Errorf("初始化数据库失败: %w", err)
	}
	defer closeSQL(sqlDB)

	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("初始化 gorm 失败: %w", err)
	}
	if err := migration.AutoMigrate(gormDB); err != nil {
		return fmt.Errorf("auto migrate 失败: %w", err)
	}
	rabbitMQ, err := mq.NewRabbitMQ(cfg.RabbitMQ)
	if err != nil {
		return fmt.Errorf("初始化 rabbitmq 失败: %w", err)
	}
	defer closeRabbitMQ(rabbitMQ)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		if err := metrics.RunServer(ctx, ":9091"); err != nil {
			zap.L().Error("metrics server failed", zap.Error(err))
		}
	}()

	if err := startWorkers(ctx, cfg, gormDB, rabbitMQ); err != nil {
		return fmt.Errorf("启动 worker 失败: %w", err)
	}
	zap.L().Info("gcfeed worker is running")
	<-ctx.Done()
	zap.L().Info("gcfeed worker stopped")
	return nil
}

func startWorkers(ctx context.Context, cfg *config.Config, gormDB *gorm.DB, rabbitMQ *mq.RabbitMQ) error {
	// redisClient := cache.NewRedisClient(cfg.Redis)
	// feedCache := cache.NewFeedCache(redisClient)
	interactionRepo := infrainteraction.New(gormDB)
	actionWorker := applicationinteraction.NewActionWorker(interactionRepo, rabbitMQ)
	if err := actionWorker.Start(ctx); err != nil {
		return err
	}
	return nil
}

func closeSQL(db *sql.DB) {
	if db != nil {
		_ = db.Close()
	}
}

func closeRabbitMQ(rabbitMQ *mq.RabbitMQ) {
	if rabbitMQ != nil {
		_ = rabbitMQ.Close()
	}
}
