package applicationinteraction

import (
	domaininteraction "DreamReel/internal/domain/interaction"
	domainmessage "DreamReel/internal/domain/message"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	defaultCommentLimit    = 20
	hotScoreLikeWeight     = 3
	hotScoreFavoriteWeight = 4
	hotScoreCommentWeight  = 5
)

type Service struct {
	repo             domaininteraction.Repository
	hotScoreRecorder HotScoreRecorder
	statCache        StatCache
	actionStateStore ActionStateStore
	actionPublisher  ActionEventPublisher
	messageWriter    MessageWriter
}

type Option func(*Service)

func New(repo domaininteraction.Repository, options ...Option) *Service {
	service := &Service{repo: repo}
	for _, option := range options {
		option(service)
	}
	return service
}

// WithHotScoreRecorder 启动缓存中的热榜写入
func WithHotScoreRecorder(recorder HotScoreRecorder) Option {
	return func(s *Service) {
		s.hotScoreRecorder = recorder
	}
}

/*
// WithStatCache 启动
func WithStatCache(cache StatCache) Option {
	return func(s *Service) {
		s.statCache = cache
	}
}
*/

// WithAsyncActionPipeline 启动点赞收藏写入缓存计数 和 异步落库
func WithAsyncActionPipeline(store ActionStateStore, publisher ActionEventPublisher) Option {
	return func(s *Service) {
		s.actionStateStore = store
		s.actionPublisher = publisher
	}
}

/*
// WithMessageWriter 为点赞和评论成功后的通知写入启用消息中心。
func WithMessageWriter(writer MessageWriter) Option {
	return func(s *Service) {
		s.messageWriter = writer
	}
}
*/

// Like 设置用户对视频的点赞状态为有效。
func (s *Service) Like(ctx context.Context, userID int64, videoID int64, idempotencyKey string) (*ActionResult, error) {
	return s.setAction(ctx, userID, videoID, domaininteraction.ActionTypeLike, true, idempotencyKey)
}

// Unlike 设置用户对视频的点赞状态为取消。
func (s *Service) Unlike(ctx context.Context, userID int64, videoID int64, idempotencyKey string) (*ActionResult, error) {
	return s.setAction(ctx, userID, videoID, domaininteraction.ActionTypeLike, false, idempotencyKey)
}

// setAction 统一处理点赞和收藏状态变更，actionType 区分点赞或收藏，active 表示目标状态。
func (s *Service) setAction(ctx context.Context, userID int64, videoID int64, actionType string, active bool, idempotencyKey string) (*ActionResult, error) {
	if userID <= 0 {
		return nil, domaininteraction.ErrInvalidUserID
	}
	if videoID <= 0 {
		return nil, domaininteraction.ErrInvalidVideoID
	}
	if len(strings.TrimSpace(idempotencyKey)) > domaininteraction.MaxIdempotencyKeyLength {
		return nil, domaininteraction.ErrIdempotencyKeyTooLong
	}
	actionType, err := domaininteraction.NormalizeActionType(actionType)
	if err != nil {
		return nil, err
	}
	if s.actionStateStore != nil && s.actionPublisher != nil {
		return s.setActionAsync(ctx, userID, videoID, actionType, active, idempotencyKey)
	}
	return s.setActionSync(ctx, userID, videoID, actionType, active, idempotencyKey)

}

func (s *Service) setActionSync(ctx context.Context, userID int64, videoID int64, actionType string, active bool, idempotencyKey string) (*ActionResult, error) {
	action, stat, delta, err := s.repo.SetAction(ctx, userID, videoID, actionType, active, idempotencyKey)
	if err != nil {
		if errors.Is(err, domaininteraction.ErrVideoNotFound) {
			return nil, domaininteraction.ErrVideoNotFound
		}
		return nil, ErrUpdateInteractionFailed
	}

	result := &ActionResult{
		VideoID:       action.VideoID,
		ActionType:    action.ActionType,
		Active:        action.Active(),
		LikeCount:     stat.LikeCount,
		FavoriteCount: stat.FavoriteCount,
	}

	s.recordActionHotScore(ctx, action.VideoID, action.ActionType, delta)
	if action.ActionType == domaininteraction.ActionTypeLike && action.Active() && delta > 0 {
		s.notifyLike(ctx, action)
	}
	return result, nil

}

func (s *Service) setActionAsync(ctx context.Context, userID int64, videoID int64, actionType string, active bool, idempotencyKey string) (*ActionResult, error) {
	initialStat, err := s.repo.GetVideoStat(ctx, videoID)
	if err != nil {
		if errors.Is(err, domaininteraction.ErrVideoNotFound) {
			return nil, domaininteraction.ErrVideoNotFound
		}
		return nil, ErrUpdateInteractionFailed
	}
	// 进行视频互动计数缓存相关的操作， 包括更新用户点赞/收藏缓存记录， 更新视频完整点赞/收藏数Stat记录，更新视频点赞/收藏数基准计数Base记录和分片Shard记录
	state, err := s.actionStateStore.SetActionState(ctx, userID, videoID, actionType, active, idempotencyKey, initialStat)
	if err != nil {
		return nil, ErrUpdateInteractionFailed
	}
	if state.Delta != 0 {
		// 发布点赞收藏变更事件, 接收处执行数据库更新
		event := NewActionChangedEvent(userID, videoID, actionType, active, idempotencyKey)
		if err := s.actionPublisher.PublishActionChanged(ctx, event); err != nil {
			return s.setActionSync(ctx, userID, videoID, actionType, active, idempotencyKey)
		}
		// 将互动变化投递到热榜分钟桶
		s.recordActionHotScore(ctx, state.VideoID, state.ActionType, state.Delta)

		// 如果是点赞且状态为有效且变化量为正，则向视频作者发送点赞通知
		if state.ActionType == domaininteraction.ActionTypeLike && state.Active && state.Delta > 0 {
			s.notifyLike(ctx, &domaininteraction.Action{
				UserID:     userID,
				VideoID:    state.VideoID,
				ActionType: state.ActionType,
				Status:     domaininteraction.ActionStatusActive,
			})
		}

		return &ActionResult{
			VideoID:       state.VideoID,
			ActionType:    state.ActionType,
			Active:        state.Active,
			LikeCount:     state.LikeCount,
			FavoriteCount: state.FavoriteCount,
		}, nil
	}

	return &ActionResult{
		VideoID:       state.VideoID,
		ActionType:    state.ActionType,
		Active:        state.Active,
		LikeCount:     state.LikeCount,
		FavoriteCount: state.FavoriteCount,
	}, nil
}

func (s *Service) notifyLike(ctx context.Context, action *domaininteraction.Action) {
	if s.messageWriter == nil || action == nil {
		return
	}
	authorID, err := s.repo.GetVideoAuthorID(ctx, action.VideoID)
	if err != nil || authorID == action.UserID {
		return
	}
	eventID := fmt.Sprintf("interaction:like:%d:%d", action.VideoID, action.UserID)
	s.createInteractionMessage(ctx, authorID, domainmessage.TypeLike, "收到点赞", "点赞了你的视频", eventID, action.UserID)
}

func (s *Service) createInteractionMessage(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, actorID int64) {
	actor, _ := s.repo.GetUserProfile(ctx, actorID)
	if writer, ok := s.messageWriter.(ActorMessageWriter); ok {
		actorNickname := ""
		actorAvatarURL := ""
		if actor != nil {
			actorNickname = actor.Nickname
			actorAvatarURL = actor.AvatarURL
		}
		_, _ = writer.CreateFromActorEvent(ctx, userID, messageType, title, content, eventID, eventID, actorID, actorNickname, actorAvatarURL)
		return
	}
	actorName := fmt.Sprintf("用户 %d", actorID)
	if actor != nil && actor.Nickname != "" {
		actorName = actor.Nickname
	}
	_, _ = s.messageWriter.CreateFromEvent(ctx, userID, messageType, title, fmt.Sprintf("%s %s", actorName, content), eventID, eventID)
}

func (s *Service) recordActionHotScore(ctx context.Context, videoID int64, actionType string, delta int) {
	if actionType == domaininteraction.ActionTypeLike {
		recordHotScore(ctx, s.hotScoreRecorder, videoID, delta*hotScoreLikeWeight)
		return
	}
	recordHotScore(ctx, s.hotScoreRecorder, videoID, delta*hotScoreFavoriteWeight)
}

func recordHotScore(ctx context.Context, recorder HotScoreRecorder, videoID int64, scoreDelta int) {
	if recorder == nil || scoreDelta == 0 {
		return
	}
	_ = recorder.AddHotScore(ctx, videoID, scoreDelta, time.Now())
}

func newEventID() string {
	content := make([]byte, 12)
	if _, err := rand.Read(content); err == nil {
		return hex.EncodeToString(content)
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func NewActionChangedEvent(userID, videoID int64, actionType string, active bool, idempotencyKey string) *ActionChangedEvent {
	return &ActionChangedEvent{
		EventID:        newEventID(),
		UserID:         userID,
		VideoID:        videoID,
		ActionType:     actionType,
		Active:         active,
		IdempotencyKey: strings.TrimSpace(idempotencyKey),
		OccurredAt:     time.Now().UTC(),
	}

}

func (s *Service) Favorite(ctx context.Context, userID int64, videoID int64, idempotencyKey string) (*ActionResult, error) {
	return s.setAction(ctx, userID, videoID, domaininteraction.ActionTypeFavorite, true, idempotencyKey)
}

func (s *Service) Unfavorite(ctx context.Context, userID int64, videoID int64, idempotencyKey string) (*ActionResult, error) {
	return s.setAction(ctx, userID, videoID, domaininteraction.ActionTypeFavorite, false, idempotencyKey)
}
