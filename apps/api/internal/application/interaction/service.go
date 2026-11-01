package applicationinteraction

import (
	domaininteraction "DreamReel/internal/domain/interaction"
	domainmessage "DreamReel/internal/domain/message"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
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

// WithStatCache 启动保存视频互动计数缓存功能
func WithStatCache(cache StatCache) Option {
	return func(s *Service) {
		s.statCache = cache
	}
}

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

// notifyLike 在首次有效点赞非本人视频时向视频作者发送点赞通知。
func (s *Service) notifyLike(ctx context.Context, action *domaininteraction.Action) {
	if action == nil {
		return
	}
	if s.messageWriter == nil {
		zap.L().Warn("message writer not configured, skip like notification",
			zap.Int64("user_id", action.UserID),
			zap.Int64("video_id", action.VideoID),
		)
		return
	}
	authorID, err := s.repo.GetVideoAuthorID(ctx, action.VideoID)
	if err != nil {
		zap.L().Error("get video author for like notification failed",
			zap.Int64("user_id", action.UserID),
			zap.Int64("video_id", action.VideoID),
			zap.Error(err),
		)
		return
	}
	if authorID == action.UserID {
		return
	}
	eventID := fmt.Sprintf("interaction:like:%d:%d", action.VideoID, action.UserID)
	s.createInteractionMessage(ctx, authorID, domainmessage.TypeLike, "收到点赞", "点赞了你的视频", eventID, action.UserID)
}

// createInteractionMessage 组装触发者展示资料并写入互动站内消息。
func (s *Service) createInteractionMessage(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, actorID int64) {
	actor, err := s.repo.GetUserProfile(ctx, actorID)
	if err != nil {
		zap.L().Warn("load actor profile for interaction notification failed",
			zap.Int64("user_id", userID),
			zap.Int64("actor_id", actorID),
			zap.String("message_type", messageType),
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
		if _, err := writer.CreateFromActorEvent(ctx, userID, messageType, title, content, eventID, eventID, actorID, actorNickname, actorAvatarURL); err != nil {
			zap.L().Error("create interaction notification with actor failed",
				zap.Int64("user_id", userID),
				zap.Int64("actor_id", actorID),
				zap.String("message_type", messageType),
				zap.String("event_id", eventID),
				zap.Error(err),
			)
		}
		return
	}
	actorName := fmt.Sprintf("用户 %d", actorID)
	if actor != nil && actor.Nickname != "" {
		actorName = actor.Nickname
	}
	if _, err := s.messageWriter.CreateFromEvent(ctx, userID, messageType, title, fmt.Sprintf("%s %s", actorName, content), eventID, eventID); err != nil {
		zap.L().Error("create interaction notification failed",
			zap.Int64("user_id", userID),
			zap.Int64("actor_id", actorID),
			zap.String("message_type", messageType),
			zap.String("event_id", eventID),
			zap.Error(err),
		)
	}
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

// CreateComment 创建评论
func (s *Service) CreateComment(ctx context.Context, userID int64, videoID int64, content string, idempotencyKey string) (*CreateCommentResult, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) > domaininteraction.MaxIdempotencyKeyLength {
		return nil, domaininteraction.ErrIdempotencyKeyTooLong
	}
	if idempotencyKey != "" {
		comment, count, err := s.repo.FindCommentByUserAndIdempotencyKey(ctx, userID, idempotencyKey)
		if err == nil {
			return &CreateCommentResult{Comment: comment, CommentCount: count}, nil
		}
		if !errors.Is(err, domaininteraction.ErrCommentNotFound) {
			return nil, fmt.Errorf("%w: find comment by idempotency key: %v", ErrLoadInteractionFailed, err)
		}
	}
	comment, err := domaininteraction.NewComment(videoID, userID, content, idempotencyKey)
	if err != nil {
		return nil, err
	}

	created, count, delta, err := s.repo.CreateComment(ctx, comment)
	if err != nil {
		if errors.Is(err, domaininteraction.ErrVideoNotFound) {
			return nil, domaininteraction.ErrVideoNotFound
		}
		return nil, fmt.Errorf("%w: create comment: %v", ErrSaveInteractionFailed, err)
	}
	s.recordHotScore(ctx, created.VideoID, delta*hotScoreCommentWeight)
	s.syncCommentCount(ctx, created.VideoID, count)
	if delta > 0 {
		s.notifyComment(ctx, created)
	}
	return &CreateCommentResult{Comment: created, CommentCount: count}, nil
}

// recordHotScore 增加热榜计数
func (s *Service) recordHotScore(ctx context.Context, videoID int64, scoreDelta int) {
	if s.hotScoreRecorder == nil || scoreDelta == 0 {
		return
	}
	_ = s.hotScoreRecorder.AddHotScore(ctx, videoID, scoreDelta, time.Now())
}

func (s *Service) syncCommentCount(ctx context.Context, videoID int64, commentCount int) {
	if s.statCache == nil || videoID <= 0 {
		return
	}
	stat, err := s.repo.GetVideoStat(ctx, videoID)
	if err != nil {
		stat = &domaininteraction.VideoStat{VideoID: videoID}
	}
	stat.CommentCount = commentCount
	_ = s.statCache.SetVideoStat(ctx, stat)

}

// notifyComment 在评论创建成功后向视频作者发送评论通知。
func (s *Service) notifyComment(ctx context.Context, comment *domaininteraction.Comment) {
	if comment == nil {
		return
	}
	if s.messageWriter == nil {
		zap.L().Warn("message writer not configured, skip comment notification",
			zap.Int64("comment_id", comment.ID),
			zap.Int64("user_id", comment.UserID),
			zap.Int64("video_id", comment.VideoID),
		)
		return
	}
	authorID, err := s.repo.GetVideoAuthorID(ctx, comment.VideoID)
	if err != nil {
		zap.L().Error("get video author for comment notification failed",
			zap.Int64("comment_id", comment.ID),
			zap.Int64("user_id", comment.UserID),
			zap.Int64("video_id", comment.VideoID),
			zap.Error(err),
		)
		return
	}
	if authorID == comment.UserID {
		return
	}
	eventID := fmt.Sprintf("interaction:comment:%d", comment.ID)
	s.createInteractionMessage(ctx, authorID, domainmessage.TypeComment, "收到评论", comment.Content, eventID, comment.UserID)
}

func (s *Service) ListComments(ctx context.Context, videoID int64, cursor string, limit int) (*CommentListResult, error) {
	if videoID <= 0 {
		return nil, domaininteraction.ErrInvalidVideoID
	}

	parsedCursor, err := parseCommentCursor(cursor)
	if err != nil {
		return nil, err
	}
	limit = normalizeCommentLimit(limit)

	items, err := s.repo.ListComments(ctx, videoID, parsedCursor, limit+1)
	if err != nil {
		return nil, fmt.Errorf("%w: list comments: %v", ErrLoadInteractionFailed, err)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	nextCursor := ""
	if len(items) > 0 {
		nextCursor = encodeCommentCursor(&domaininteraction.CommentCursor{
			CreatedAt: items[len(items)-1].CreatedAt,
			CommentID: items[len(items)-1].ID,
		})
	}

	return &CommentListResult{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func parseCommentCursor(raw string) (*domaininteraction.CommentCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	content, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		content, err = base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, domaininteraction.ErrInvalidCursor
		}
	}
	var payload commentCursorPayload
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, domaininteraction.ErrInvalidCursor
	}
	createdAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(payload.CreatedAt))
	if err != nil || payload.CommentID <= 0 {
		return nil, domaininteraction.ErrInvalidCursor
	}

	return &domaininteraction.CommentCursor{
		CreatedAt: createdAt,
		CommentID: payload.CommentID,
	}, nil
}

func normalizeCommentLimit(limit int) int {
	if limit <= 0 {
		return defaultCommentLimit
	}
	if limit > domaininteraction.MaxLimit {
		return domaininteraction.MaxLimit
	}
	return limit
}

func encodeCommentCursor(cursor *domaininteraction.CommentCursor) string {
	if cursor == nil || cursor.CommentID <= 0 || cursor.CreatedAt.IsZero() {
		return ""
	}

	content, err := json.Marshal(commentCursorPayload{
		CreatedAt: cursor.CreatedAt.UTC().Format(time.RFC3339Nano),
		CommentID: cursor.CommentID,
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(content)
}

// DeleteComment 删除评论并返回删除后的评论状态和视频评论数。
func (s *Service) DeleteComment(ctx context.Context, commentID int64, userID int64, role string) (*DeleteCommentResult, error) {
	if commentID <= 0 {
		return nil, domaininteraction.ErrInvalidCommentID
	}
	if userID <= 0 {
		return nil, domaininteraction.ErrInvalidUserID
	}

	// 将删除写入数据库，获取删除后的评论状态和视频评论数
	comment, count, delta, err := s.repo.DeleteComment(ctx, commentID, userID, role)
	if err != nil {
		if errors.Is(err, domaininteraction.ErrCommentNotFound) ||
			errors.Is(err, domaininteraction.ErrCommentPermissionDenied) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: delete comment: %v", ErrUpdateInteractionFailed, err)
	}

	// 维护热榜计数
	s.recordHotScore(ctx, comment.VideoID, delta*hotScoreCommentWeight)
	// 同步视频评论数
	s.syncCommentCount(ctx, comment.VideoID, count)
	return &DeleteCommentResult{
		CommentID:    comment.ID,
		Status:       comment.Status,
		CommentCount: count,
	}, nil
}
