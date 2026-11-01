package applicationmessage

import (
	domainmessage "DreamReel/internal/domain/message"
	"context"
	"errors"
	"strings"

	"go.uber.org/zap"
)

var (
	ErrLoadMessageFailed   = errors.New("failed to load message")
	ErrSaveMessageFailed   = errors.New("failed to save message")
	ErrUpdateMessageFailed = errors.New("failed to update message")
)

type MarkReadResult struct {
	UpdatedCount int
}

type Service struct {
	repo domainmessage.Repository
}

func New(repo domainmessage.Repository) *Service {
	return &Service{repo: repo}
}

// List 查询当前用户消息列表，使用游标分页。
func (s *Service) List(ctx context.Context, userID int64, cursor string, limit int) (*ListResult, error) {
	if userID <= 0 {
		return nil, domainmessage.ErrInvalidUserID
	}

	parsedCursor, err := parseCursor(cursor)
	if err != nil {
		return nil, err
	}
	limit = normalizeLimit(limit)
	items, err := s.repo.ListByUser(ctx, userID, parsedCursor, limit+1)
	if err != nil {
		zap.L().Error("list messages failed",
			zap.Int64("user_id", userID),
			zap.String("cursor", cursor),
			zap.Int("limit", limit),
			zap.Error(err),
		)
		return nil, ErrLoadMessageFailed
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	nextCursor := ""
	if len(items) > 0 {
		nextCursor = encodeCursor(&domainmessage.Cursor{
			CreatedAt: items[len(items)-1].CreatedAt,
			MessageID: items[len(items)-1].ID,
		})
	}

	return &ListResult{Items: items, NextCursor: nextCursor, HasMore: hasMore}, nil
}

// MarkRead 将当前用户消息标记为已读，空 messageIDs 表示全部已读。
func (s *Service) MarkRead(ctx context.Context, userID int64, messageIDs []int64) (*MarkReadResult, error) {
	if userID <= 0 {
		return nil, domainmessage.ErrInvalidUserID
	}

	ids := make([]int64, 0, len(messageIDs))
	seen := map[int64]struct{}{}
	for _, id := range messageIDs {
		if id <= 0 {
			return nil, domainmessage.ErrInvalidMessageID
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	count, err := s.repo.MarkRead(ctx, userID, ids)
	if err != nil {
		zap.L().Error("mark messages read failed",
			zap.Int64("user_id", userID),
			zap.Int("message_count", len(ids)),
			zap.Error(err),
		)
		return nil, ErrUpdateMessageFailed
	}
	return &MarkReadResult{UpdatedCount: count}, nil

}

// CountUnread 查询当前用户未读数。
func (s *Service) CountUnread(ctx context.Context, userID int64) (*UnreadStat, error) {
	if userID <= 0 {
		return nil, domainmessage.ErrInvalidUserID
	}
	count, err := s.repo.CountUnread(ctx, userID)
	if err != nil {
		zap.L().Error("count unread messages failed",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return nil, ErrLoadMessageFailed
	}
	return &UnreadStat{UnreadCount: count}, nil
}

// CreateFromEvent 将内部事件转换成用户消息，eventID/idempotencyKey 命中时返回既有消息。
func (s *Service) CreateFromEvent(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, idempotencyKey string) (*CreateResult, error) {
	return s.CreateFromActorEvent(ctx, userID, messageType, title, content, eventID, idempotencyKey, 0, "", "")
}

// CreateFromActorEvent 创建用户站内消息。
func (s *Service) CreateFromActorEvent(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, idempotencyKey string, actorID int64, actorNickname string, actorAvatarURL string) (*CreateResult, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) > domainmessage.MaxIdempotencyKeyLength {
		return nil, domainmessage.ErrIdempotencyKeyTooLong
	}
	message, err := domainmessage.New(userID, messageType, title, content, eventID)
	if err != nil {
		return nil, err
	}
	message.WithActor(actorID, actorNickname, actorAvatarURL)
	created, inserted, err := s.repo.Create(ctx, message, idempotencyKey)
	if err != nil {
		zap.L().Error("create message failed",
			zap.Int64("user_id", userID),
			zap.String("message_type", message.Type),
			zap.String("event_id", message.EventID),
			zap.String("idempotency_key", idempotencyKey),
			zap.Int64("actor_id", message.ActorID),
			zap.Error(err),
		)
		return nil, ErrSaveMessageFailed
	}
	return &CreateResult{Message: created, Created: inserted}, nil
}
