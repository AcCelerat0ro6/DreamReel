package domaininteraction

import (
	"errors"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

func NormalizeActionType(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value != ActionTypeLike && value != ActionTypeFavorite {
		return "", ErrInvalidActionType
	}
	return value, nil
}

// Active 判断点赞或收藏当前是否处于有效状态。
func (a *Action) Active() bool {
	return a.Status == ActionStatusActive
}

// RestoreComment 从数据库查询结果恢复评论对象，并清洗展示字段。
func RestoreComment(id int64, videoID int64, userID int64, userNickname string, userAvatarURL string, content string, status int, idempotencyKey string, createdAt time.Time, updatedAt time.Time) *Comment {
	content = strings.TrimSpace(content)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if status == 0 {
		status = CommentStatusNormal
	}

	return &Comment{
		ID:             id,
		VideoID:        videoID,
		UserID:         userID,
		UserNickname:   strings.TrimSpace(userNickname),
		UserAvatarURL:  strings.TrimSpace(userAvatarURL),
		Content:        content,
		Status:         status,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}
}

func NewComment(videoID int64, userID int64, content string, idempotencyKey string) (*Comment, error) {
	if videoID <= 0 {
		return nil, ErrInvalidVideoID
	}
	if userID <= 0 {
		return nil, ErrInvalidUserID
	}

	content = strings.TrimSpace(content)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if content == "" {
		return nil, ErrEmptyCommentContent
	}
	if len(content) > MaxCommentContentLength {
		return nil, ErrCommentContentTooLong
	}
	if len(idempotencyKey) > MaxIdempotencyKeyLength {
		return nil, ErrIdempotencyKeyTooLong
	}

	// 新评论默认处于正常状态，删除时再切换为 CommentStatusDeleted。
	return &Comment{
		VideoID:        videoID,
		UserID:         userID,
		Content:        content,
		Status:         CommentStatusNormal,
		IdempotencyKey: idempotencyKey,
	}, nil
}

// isDuplicateKeyError 兼容 GORM 标准错误和 MySQL 1062 唯一键冲突。
func isDuplicateKeyError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
