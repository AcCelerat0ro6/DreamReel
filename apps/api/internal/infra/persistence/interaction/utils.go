package infrainteraction

import (
	domaininteraction "DreamReel/internal/domain/interaction"
	domainvideo "DreamReel/internal/domain/video"
	infravideo "DreamReel/internal/infra/persistence/video"
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
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

// commentWithUserSelect 统一评论查询字段，并附带用户昵称和头像。
func commentWithUserSelect() string {
	return "c.id, c.video_id, c.user_id, a.nickname AS user_nickname, a.avatar_url AS user_avatar_url, c.content, c.status, c.idempotency_key, c.created_at, c.updated_at"
}

// restoreComment 把评论联表查询结果转换为领域对象。
func restoreComment(model commentWithUserModel) *domaininteraction.Comment {
	return domaininteraction.RestoreComment(
		model.ID,
		model.VideoID,
		model.UserID,
		model.UserNickname,
		model.UserAvatarURL,
		model.Content,
		model.Status,
		idempotencyKeyValue(model.IdempotencyKey),
		model.CreatedAt,
		model.UpdatedAt,
	)
}

// isDuplicateKeyError 兼容 GORM 标准错误和 MySQL 1062 唯一键冲突。
func isDuplicateKeyError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

func clampCount(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func currentVideoStatCounter(tx *gorm.DB, videoID int64, field string) (int, error) {
	var stat infravideo.VideoStatModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("video_id = ?", videoID).Take(&stat).Error; err != nil {
		return 0, mapVideoError(err)
	}
	switch field {
	case "like_count":
		return stat.LikeCount, nil
	case "favorite_count":
		return stat.FavoriteCount, nil
	case "comment_count":
		return stat.CommentCount, nil
	default:
		return 0, nil
	}
}
