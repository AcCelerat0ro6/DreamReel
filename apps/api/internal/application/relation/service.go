package applicationrelation

import (
	domainfeed "DreamReel/internal/domain/feed"
	domainmessage "DreamReel/internal/domain/message"
	domainrelation "DreamReel/internal/domain/relation"
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"
)

var (
	ErrLoadRelationFailed       = errors.New("failed to load relation")
	ErrUpdateRelationFailed     = errors.New("failed to update relation")
	ErrBackfillFollowFeedFailed = errors.New("failed to backfill follow feed")
)

const (
	defaultBackfillVideoLimit  = 100
	defaultBackfillInboxMaxLen = 1000
)

// Service 编排用户关系用例：关注、取关、关注列表和粉丝列表。
type Service struct {
	// repo 用户关系仓储
	repo domainrelation.Repository
	// backfiller 关注 feed 回填器
	backfiller FollowFeedBackfiller
	// messageWriter 消息写入器
	messageWriter MessageWriter
}

// New 创建关系应用服务。
func New(repo domainrelation.Repository, options ...Option) *Service {
	service := &Service{repo: repo}
	for _, option := range options {
		option(service)
	}
	return service
}

type Option func(*Service)

type FollowFeedBackfiller interface {
	// CountFollowers 获取当前关注者的粉丝数量
	CountFollowers(ctx context.Context, authorID int64) (int, error)
	// ListAuthorRecentVideos 列出当前关注者的最近发布视频
	ListAuthorRecentVideos(ctx context.Context, authorID int64, limit int) ([]*domainfeed.FeedPageItem, error)
	// AddInboxItems 添加关注者发布视频到目标用户收件箱
	AddInboxItems(ctx context.Context, authorID int64, userIDs []int64, item *domainfeed.FeedPageItem, maxLen int64) error
}

// WithFollowFeedBackfiller 为关注成功后的关注流索引启用回填。
func WithFollowFeedBackfiller(backfiller FollowFeedBackfiller) Option {
	return func(s *Service) {
		s.backfiller = backfiller
	}
}

// MessageWriter 写入关注触发的站内消息。
type MessageWriter interface {
	CreateFromEvent(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, idempotencyKey string) (any, error)
}

// ActorMessageWriter 可在关注消息里携带触发用户资料。
type ActorMessageWriter interface {
	CreateFromActorEvent(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, idempotencyKey string, actorID int64, actorNickname string, actorAvatarURL string) (any, error)
}

// Follow 设置当前用户关注目标用户。
func (s *Service) Follow(ctx context.Context, userID int64, targetUserID int64, idempotencyKey string) (*FollowResult, error) {
	return s.setFollow(ctx, userID, targetUserID, true, idempotencyKey)
}

// Unfollow 设置当前用户取消关注目标用户。
func (s *Service) Unfollow(ctx context.Context, userID int64, targetUserID int64, idempotencyKey string) (*FollowResult, error) {
	return s.setFollow(ctx, userID, targetUserID, false, idempotencyKey)
}

// setFollow 统一处理关注和取关，active 表示目标关系状态。
func (s *Service) setFollow(ctx context.Context, userID int64, targetUserID int64, active bool, idempotencyKey string) (*FollowResult, error) {
	if _, err := domainrelation.NewFollow(userID, targetUserID, idempotencyKey); err != nil {
		return nil, err
	}

	follow, userStat, targetStat, err := s.repo.SetFollow(ctx, userID, targetUserID, active, idempotencyKey)
	if err != nil {
		if errors.Is(err, domainrelation.ErrTargetUserNotFound) {
			return nil, err
		}
		if errors.Is(err, domainrelation.ErrIdempotencyKeyConflict) {
			zap.L().Error("idempotency key conflict while setting follow",
				zap.Int64("user_id", userID),
				zap.Int64("target_user_id", targetUserID),
				zap.Bool("active", active),
				zap.String("idempotency_key", idempotencyKey),
				zap.Error(err),
			)
			return nil, err
		}
		zap.L().Error("set follow failed",
			zap.Int64("user_id", userID),
			zap.Int64("target_user_id", targetUserID),
			zap.Bool("active", active),
			zap.String("idempotency_key", idempotencyKey),
			zap.Error(err),
		)
		return nil, fmt.Errorf("%w: %v", ErrUpdateRelationFailed, err)
	}

	if active && follow.Active() {
		if err := s.backfillFollowFeed(ctx, userID, targetUserID); err != nil {
			zap.L().Error("backfill follow feed failed",
				zap.Int64("user_id", userID),
				zap.Int64("target_user_id", targetUserID),
				zap.Error(err),
			)
			return nil, fmt.Errorf("%w: %v", ErrBackfillFollowFeedFailed, err)
		}
		s.notifyFollow(ctx, userID, targetUserID)
	}

	return &FollowResult{
		UserID:         follow.UserID,
		TargetUserID:   follow.TargetUserID,
		Status:         follow.Status,
		Following:      follow.Active(),
		FollowingCount: userStat.FollowingCount,
		FollowerCount:  targetStat.FollowerCount,
	}, nil

}

func (s *Service) backfillFollowFeed(ctx context.Context, userID int64, targetUserID int64) error {
	if s.backfiller == nil {
		zap.L().Warn("backfiller not configured, skip follow feed backfill",
			zap.Int64("user_id", userID),
			zap.Int64("target_user_id", targetUserID),
		)
		return nil
	}
	followerCount, err := s.backfiller.CountFollowers(ctx, targetUserID)
	if err != nil {
		zap.L().Error("count target followers for backfill failed",
			zap.Int64("user_id", userID),
			zap.Int64("target_user_id", targetUserID),
			zap.Error(err),
		)
		return fmt.Errorf("count followers: %w", err)
	}
	// 大V, 不作回填
	if followerCount >= domainfeed.BigCreatorFollowerThreshold {
		return nil
	}

	// 列出当前关注者的最近发布视频
	items, err := s.backfiller.ListAuthorRecentVideos(ctx, targetUserID, defaultBackfillVideoLimit)
	if err != nil {
		zap.L().Error("list target recent videos for backfill failed",
			zap.Int64("user_id", userID),
			zap.Int64("target_user_id", targetUserID),
			zap.Int("limit", defaultBackfillVideoLimit),
			zap.Error(err),
		)
		return fmt.Errorf("list author recent videos: %w", err)
	}

	for _, item := range items {
		if item == nil {
			continue
		}
		if err := s.backfiller.AddInboxItems(ctx, targetUserID, []int64{userID}, item, defaultBackfillInboxMaxLen); err != nil {
			zap.L().Error("add inbox item during backfill failed",
				zap.Int64("user_id", userID),
				zap.Int64("target_user_id", targetUserID),
				zap.Int64("max_len", defaultBackfillInboxMaxLen),
				zap.Error(err),
			)
			return fmt.Errorf("add inbox items: %w", err)
		}
	}
	return nil
}

func (s *Service) notifyFollow(ctx context.Context, userID int64, targetUserID int64) {
	if s.messageWriter == nil {
		zap.L().Warn("message writer not configured, skip follow notification",
			zap.Int64("user_id", userID),
			zap.Int64("target_user_id", targetUserID),
		)
		return
	}

	//eventID 作为幂等键 确保用户关注时只会发送单条消息
	eventID := fmt.Sprintf("relation:follow:%d:%d", targetUserID, userID)
	actor, err := s.repo.GetUserProfile(ctx, userID)
	if err != nil {
		zap.L().Warn("load actor profile for follow notification failed",
			zap.Int64("user_id", userID),
			zap.Int64("target_user_id", targetUserID),
			zap.String("event_id", eventID),
			zap.Error(err),
		)
	}
	if writer, ok := s.messageWriter.(ActorMessageWriter); ok {
		actorNickname := ""
		actorAvatarURL := ""
		if actor != nil {
			actorNickname = actor.Nickname
			actorAvatarURL = actor.AvatarURL
		}
		if _, err := writer.CreateFromActorEvent(
			ctx,
			targetUserID,
			domainmessage.TypeFollow,
			"新增关注",
			"关注了你",
			eventID,
			eventID,
			userID,
			actorNickname,
			actorAvatarURL,
		); err != nil {
			zap.L().Error("create follow notification with actor failed",
				zap.Int64("user_id", userID),
				zap.Int64("target_user_id", targetUserID),
				zap.String("event_id", eventID),
				zap.Error(err),
			)
		}
		return
	}
	actorName := fmt.Sprintf("用户 %d", userID)
	if actor != nil && actor.Nickname != "" {
		actorName = actor.Nickname
	}
	if _, err := s.messageWriter.CreateFromEvent(
		ctx,
		targetUserID,
		domainmessage.TypeFollow,
		"新增关注",
		fmt.Sprintf("%s 关注了你", actorName),
		eventID,
		eventID,
	); err != nil {
		zap.L().Error("create follow notification failed",
			zap.Int64("user_id", userID),
			zap.Int64("target_user_id", targetUserID),
			zap.String("event_id", eventID),
			zap.Error(err),
		)
	}
}

// ListFollowing 查询当前用户的关注列表。
func (s *Service) ListFollowing(ctx context.Context, userID int64, cursor string, limit int) (*ListResult, error) {
	if userID <= 0 {
		return nil, domainrelation.ErrInvalidUserID
	}
	parsedCursor, err := parseListCursor(cursor)
	if err != nil {
		return nil, err
	}
	limit = normalizeLimit(limit)

	items, err := s.repo.ListFollowing(ctx, userID, parsedCursor, limit+1)
	if err != nil {
		zap.L().Error("list following failed",
			zap.Int64("user_id", userID),
			zap.String("cursor", cursor),
			zap.Int("limit", limit),
			zap.Error(err),
		)
		return nil, fmt.Errorf("%w: %v", ErrLoadRelationFailed, err)
	}
	return listResult(items, limit), nil
}

// ListFollowers 查询当前用户的粉丝列表。
func (s *Service) ListFollowers(ctx context.Context, userID int64, cursor string, limit int) (*ListResult, error) {
	if userID <= 0 {
		return nil, domainrelation.ErrInvalidUserID
	}
	parsedCursor, err := parseListCursor(cursor)
	if err != nil {
		return nil, err
	}
	limit = normalizeLimit(limit)

	items, err := s.repo.ListFollowers(ctx, userID, parsedCursor, limit+1)
	if err != nil {
		zap.L().Error("list followers failed",
			zap.Int64("user_id", userID),
			zap.String("cursor", cursor),
			zap.Int("limit", limit),
			zap.Error(err),
		)
		return nil, fmt.Errorf("%w: %v", ErrLoadRelationFailed, err)
	}
	return listResult(items, limit), nil
}
