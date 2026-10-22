package applicationinteraction

import (
	domaininteraction "DreamReel/internal/domain/interaction"
	"context"
	"errors"
	"time"
)

var (
	ErrLoadInteractionFailed   = errors.New("failed to load interaction")
	ErrSaveInteractionFailed   = errors.New("failed to save interaction")
	ErrUpdateInteractionFailed = errors.New("failed to update interaction")
)

// ActionStateStore 保存点赞收藏的快速状态和计数。
type ActionStateStore interface {
	SetActionState(ctx context.Context, userID int64, videoID int64, actionType string, active bool, idempotencyKey string, initialStat *domaininteraction.VideoStat) (*ActionStateResult, error)
}

// ActionEventPublisher 通过消息队列投递点赞收藏变更事件。
type ActionEventPublisher interface {
	PublishActionChanged(ctx context.Context, event *ActionChangedEvent) error
}

type ActionResult struct {
	VideoID       int64
	ActionType    string
	Active        bool
	LikeCount     int
	FavoriteCount int
}

type ActionChangedEvent struct {
	EventID        string    `json:"event_id"`
	UserID         int64     `json:"user_id"`
	VideoID        int64     `json:"video_id"`
	ActionType     string    `json:"action_type"`
	Active         bool      `json:"active"`
	IdempotencyKey string    `json:"idempotency_key"`
	OccurredAt     time.Time `json:"occurred_at"`
}

type ActionStateResult struct {
	VideoID        int64
	ActionType     string
	Active         bool
	LikeCount      int
	FavoriteCount  int
	Delta          int
	IdempotencyKey string
}

// StatCache 同步 Feed 展示所需的视频互动计数缓存。
type StatCache interface {
	SetVideoStat(ctx context.Context, stat *domaininteraction.VideoStat) error
}

// HotScoreRecorder 把互动变化投递到热榜分钟桶。
type HotScoreRecorder interface {
	AddHotScore(ctx context.Context, videoID int64, scoreDelta int, at time.Time) error
}

// MessageWriter 写入互动触发的站内消息。
type MessageWriter interface {
	CreateFromEvent(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, idempotencyKey string) (any, error)
}

// ActorMessageWriter 可在消息里携带触发互动的用户资料。
type ActorMessageWriter interface {
	CreateFromActorEvent(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, idempotencyKey string, actorID int64, actorNickname string, actorAvatarURL string) (any, error)
}
