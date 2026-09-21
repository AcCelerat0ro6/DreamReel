package migration

import (
	infraaccount "DreamReel/internal/infra/persistence/account"
	infravideo "DreamReel/internal/infra/persistence/video"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

func AutoMigrate(db *gorm.DB) error {
	if err := autoMigrateModels(db); err != nil {
		return err
	}
	// 在启动时就保证每条视频都有对应的一条统计记录
	if err := infravideo.EnsureStats(db); err != nil {
		return err
	}
	return nil
}

func autoMigrateModels(db *gorm.DB) error {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		err = db.AutoMigrate(
			&infraaccount.UserModel{},
			&infravideo.VideoModel{},
			&infravideo.VideoStatModel{},
		)
		if err == nil {
			return nil
		}
		if !isConcurrentMigrationError(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 150 * time.Millisecond)
	}
	return err
}

func isConcurrentMigrationError(err error) bool {
	var mysqlErr *mysql.MySQLError

	if !errors.As(err, &mysqlErr) {
		return false
	}
	switch mysqlErr.Number {
	case 1060, 1061:
		return true
	default:
		return false
	}
}
