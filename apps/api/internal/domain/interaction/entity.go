package domaininteraction

import (
	"context"
	"strings"
	"time"
)

const (
	ActionTypeLike     = "LIKE"
	ActionTypeFavorite = "FAVORITE"

	ActionStatusActive   = 1
	ActionStatusCanceled = 2

	CommentStatusNormal  = 1
	CommentStatusDeleted = 2

	MaxCommentContentLength = 1000
	MaxIdempotencyKeyLength = 128
	MaxLimit                = 100
)

// VideoStat 保存互动模块需要的视频统计快照。
type VideoStat struct {
	VideoID       int64
	LikeCount     int
	CommentCount  int
	FavoriteCount int
}

type Repository interface {
	// GetVideoStat 读取公开视频当前互动计数。
	GetVideoStat(ctx context.Context, videoID int64) (*VideoStat, error)
	// GetVideoAuthorID 读取公开视频作者，用于互动通知。
	GetVideoAuthorID(ctx context.Context, videoID int64) (int64, error)
	// GetUserProfile 读取触发互动的用户展示资料。
	GetUserProfile(ctx context.Context, userID int64) (*UserProfile, error)
	// SetAction 设置点赞或收藏状态，并返回更新后的完整视频统计快照。
	SetAction(ctx context.Context, userID int64, videoID int64, actionType string, active bool, idempotencyKey string) (*Action, *VideoStat, int, error)
}

// Action 表示用户对视频的一类互动状态，例如点赞或收藏。
type Action struct {
	ID             int64
	UserID         int64
	VideoID        int64
	ActionType     string
	Status         int
	IdempotencyKey string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// UserProfile 保存互动消息需要展示的用户资料。
type UserProfile struct {
	ID        int64
	Nickname  string
	AvatarURL string
}

// RestoreAction 从数据库记录恢复互动行为，供仓储层返回领域对象。
func RestoreAction(id int64, userID int64, videoID int64, actionType string, status int, idempotencyKey string, createdAt time.Time, updatedAt time.Time) *Action {
	actionType, _ = NormalizeActionType(actionType)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if status == 0 {
		status = ActionStatusActive
	}

	return &Action{
		ID:             id,
		UserID:         userID,
		VideoID:        videoID,
		ActionType:     actionType,
		Status:         status,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}
}
