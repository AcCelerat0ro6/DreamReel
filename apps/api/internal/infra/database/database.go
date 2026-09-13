package database

import (
	"DreamReel/internal/infra/config"
	"database/sql"
	"fmt"
	"time"

	// 匿名导入以注册 "mysql" driver，供 sql.Open 使用。
	_ "github.com/go-sql-driver/mysql"
)

// 连接池默认值，当配置未提供时使用。
// ConnMaxLifetime 默认 1h，远小于 MySQL wait_timeout（默认 8h），
// 避免长连接被服务端主动关闭后客户端拿到 invalid connection。
const (
	defaultMaxOpenConns    = 50
	defaultMaxIdleConns    = 10
	defaultConnMaxLifetime = time.Hour
	defaultConnMaxIdleTime = 5 * time.Minute
)

// New 根据配置创建 MySQL 连接池
func New(dbcfg *config.DatabaseConfig) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		dbcfg.User,
		dbcfg.Password,
		dbcfg.Host,
		dbcfg.Port,
		dbcfg.Name,
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(config.PositiveInt(dbcfg.MaxOpenConns, defaultMaxOpenConns))
	db.SetMaxIdleConns(config.PositiveInt(dbcfg.MaxIdleConns, defaultMaxIdleConns))
	db.SetConnMaxLifetime(config.ParseDuration(dbcfg.ConnMaxLifetime, defaultConnMaxLifetime))
	db.SetConnMaxIdleTime(config.ParseDuration(dbcfg.ConnMaxIdleTime, defaultConnMaxIdleTime))

	if err := db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}
