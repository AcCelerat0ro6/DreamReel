package applicationfeed

import (
	domainfeed "DreamReel/internal/domain/feed"
	"context"
	"fmt"

	"go.uber.org/zap"
)

type FollowingIndexCache interface {
	ListFollowingIndexPage(ctx context.Context, viewerID int64, authorIDs []int64, cursor *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedPageItem, bool, error)
}

// FollowingStrategy 使用推拉混合模式读取关注流。
type FollowingStrategy struct {
	repo           domainfeed.Repository
	cache          FeedCache
	followingIndex FollowingIndexCache
}

// NewFollowingStrategy 创建关注流推拉混合策略。
func NewFollowingStrategy(repo domainfeed.Repository) *FollowingStrategy {
	return &FollowingStrategy{repo: repo}
}

// Scene 返回关注流场景。
func (s *FollowingStrategy) Scene() domainfeed.Scene {
	return domainfeed.SceneFollowing
}

// List 根据当前登录用户读取关注流。
func (s *FollowingStrategy) List(ctx context.Context, req FeedRequest) (*FeedResult, error) {
	if req.ViewerID <= 0 {
		return nil, domainfeed.ErrViewerRequired
	}
	parsedCursor, err := parseTimelineCursor(req.Cursor)
	if err != nil {
		return nil, err
	}
	limit := normalizeLimit(req.Limit)
	page, err := s.listPageFromRepo(ctx, req.ViewerID, parsedCursor, limit)
	if err != nil {
		return nil, err
	}

	// 拼接关注流页面
	items, err := assembleFeedItems(ctx, s.repo, s.cache, page.Items, req.ViewerID)
	if err != nil {
		zap.L().Error("failed to assemble following feed items", zap.Int64("viewer_id", req.ViewerID), zap.Error(err))
		return nil, fmt.Errorf("%w: %v", ErrLoadFeedFailed, err)
	}

	return &FeedResult{
		Scene:      domainfeed.SceneFollowing,
		Items:      items,
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}, nil
}

func (s *FollowingStrategy) listPageFromRepo(ctx context.Context, viewerID int64, parsedCursor *domainfeed.TimelineCursor, limit int) (*FeedPage, error) {
	items, err := s.listFollowingItems(ctx, viewerID, parsedCursor, limit+1)
	if err != nil {
		zap.L().Error("failed to list following page", zap.Int64("viewer_id", viewerID), zap.Error(err))
		return nil, fmt.Errorf("%w: %v", ErrLoadFeedFailed, err)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	nextCursor := ""
	if len(items) > 0 {
		nextCursor = encodeTimelineCursor(&domainfeed.TimelineCursor{
			PublishedAt: items[len(items)-1].PublishedAt,
			VideoID:     items[len(items)-1].VideoID,
		})
	}

	return &FeedPage{
		Scene:      domainfeed.SceneFollowing,
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (s *FollowingStrategy) listFollowingItems(ctx context.Context, viewerID int64, parsedCursor *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedPageItem, error) {
	if s.followingIndex != nil {
		// 主动拉取
		authorIDs, err := s.repo.ListFollowingPullAuthorIDs(ctx, viewerID)
		if err != nil {
			return nil, err
		}
		items, ok, err := s.followingIndex.ListFollowingIndexPage(ctx, viewerID, authorIDs, parsedCursor, limit)
		if err != nil {
			return nil, err
		}
		if ok {
			return items, nil
		}
	}
	return s.repo.ListFollowingPage(ctx, viewerID, parsedCursor, limit)
}
