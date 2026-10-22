package infrainteraction

import (
	domaininteraction "DreamReel/internal/domain/interaction"
	domainvideo "DreamReel/internal/domain/video"
	infravideo "DreamReel/internal/infra/persistence/video"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// lockPublishedVideo 校验并锁定已发布视频，互动写入前都会经过这里。
func lockPublishedVideo(tx *gorm.DB, videoID int64) error {
	var video infravideo.VideoModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND status = ?", videoID, domainvideo.StatusPublished).
		Take(&video).
		Error
	return mapVideoError(err)
}

// actionStatusFromActive 将接口目标状态转换为数据库状态枚举。
func actionStatusFromActive(active bool) int {
	if active {
		return domaininteraction.ActionStatusActive
	}
	return domaininteraction.ActionStatusCanceled
}

// idempotencyKeyValue 将数据库可空字段还原为领域层字符串。
func idempotencyKeyValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// idempotencyKeyPtr 将空幂等键存为 NULL，减少唯一索引冲突。
func idempotencyKeyPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

// clampCount 防止并发或脏数据导致计数变成负数。
func clampCount(value int) int {
	if value < 0 {
		return 0
	}
	return value
}
