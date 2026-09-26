package applicationfeed

import (
	domainfeed "DreamReel/internal/domain/feed"
	"context"
	"internal/singleflight"
	"time"

	"go.uber.org/zap"
)

type timelineCursorPayload struct {
	PublishedAt string `json:"published_at"`
	VideoID     int64  `json:"video_id"`
}

// TimelineStrategy 按时间查询策略
type TimelineStrategy struct {
	scene        domainfeed.Scene
	repo         domainfeed.Repository
	cache        FeedCache
	firstPageTTL time.Duration
	pageTTL      time.Duration
	group        singleflight.Group
}

// NewTimelineStrategy 创建一个时间线排序策略
func NewTimelineStrategy(scene domainfeed.Scene, repo domainfeed.Repository) *TimelineStrategy {
	return &TimelineStrategy{
		scene:        domainfeed.NormalizeScene(scene),
		repo:         repo,
		firstPageTTL: timelineFirstPageCacheTTL,
		pageTTL:      timelinePageCacheTTL,
	}
}

// List 使用 cursor+limit 读取时间线 Feed。
func (s *TimelineStrategy) List(ctx context.Context, req FeedRequest) (*FeedResult, error) {
	parsedCursor, err := parseTimelineCursor(req.Cursor)
	// parsedCursor包含： videoID, publishedAt
	if err != nil {
		return nil, err
	}
	limit := normalizeLimit(req.Limit)

	page, err := loadFeedPage(ctx, s.cache, s.scene, req.Cursor, limit, s.firstPageTTL, s.pageTTL, &s.group, func() (*FeedPage, error) {
		// 闭包
		return s.listPageFromRepo(ctx, parsedCursor, limit)
	})
	if err != nil {
		return nil, err
	}
	items, err := assembleFeedItems(ctx, s.repo, s.cache, page.Items, req.ViewerID)
	if err != nil {
		return nil, ErrLoadFeedFailed
	}
	return &FeedResult{
		Scene:      req.Scene,
		Items:      items,
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}, nil
}

func (s *TimelineStrategy) listPageFromRepo(ctx context.Context, parsedCursor *domainfeed.TimelineCursor, limit int) (*FeedPage, error) {
	items, err := s.repo.ListTimelinePage(ctx, parsedCursor, limit+1)
	// 多查一条可以以较低性能损耗判断是否还有多余数据.
	if err != nil {
		zap.L().Error("failed to list timeline page by querying database.", zap.Error(err))
		return nil, ErrLoadFeedFailed
	}
	var hasMore bool = false
	if len(items) > limit {
		hasMore = true
	}
	if hasMore {
		items = items[:limit]
	}

	var NextCursor string
	if len(items) > 0 {
		// cursor记录最后一条视频卡片数据， 在下一页查找时会跳过它
		NextCursor = encodeTimelineCursor(&domainfeed.TimelineCursor{
			PublishedAt: items[len(items)-1].PublishedAt,
			VideoID:     items[len(items)-1].VideoID,
		})
	}

	return &FeedPage{
		Scene:      s.scene,
		Items:      items,
		NextCursor: NextCursor,
		HasMore:    hasMore,
	}, nil
}

// assembleFeedItems 拼凑完整的单视频卡片
func assembleFeedItems(ctx context.Context, repo domainfeed.Repository, cache FeedCache, pageItems []*domainfeed.FeedPageItem, viewerID int64) ([]*domainfeed.FeedItem, error) {
	// 视频ID去重
	videoIDs := feedPageVideoIDs(pageItems)
	if len(videoIDs) == 0 {
		return []*domainfeed.FeedItem{}, nil
	}

	// 首先从缓存查询
	cards := map[int64]*domainfeed.FeedCard{}
	stats := map[int64]*domainfeed.FeedStat{}
	if cache != nil {
		if cacheCards, err := cache.GetCards(ctx, videoIDs); err == nil {
			cards = cacheCards
		}
		if cacheStats, err := cache.GetStats(ctx, videoIDs); err == nil {
			stats = cacheStats
		}
	}

	// 补齐缓存中不存在的卡片 并将其写回缓存。
	missingCardIDs := fingmissingCardIDs(videoIDs, cards)
	if len(missingCardIDs) > 0 {
		loadedCards, err := repo.BatchGetFeedCards(ctx, missingCardIDs)
		if err != nil {
			return nil, err
		}
		mergeCards(cards, loadedCards)
		if cache != nil {
			_ = cache.SetCards(ctx, loadedCards, feedCardCacheTTL)
		}
	}

	missingStatIDs := findmissingStatIDs(videoIDs, stats)
	if len(missingStatIDs) > 0 {
		loadedStats, err := repo.BatchGetFeedStats(ctx, missingStatIDs)
		if err != nil {
			return nil, err
		}
		mergeStats(stats, loadedStats)
		if cache != nil {
			_ = cache.SetStats(ctx, loadedStats, feedStatCacheTTL)
		}
	}

	//

}

func feedPageVideoIDs(items []*domainfeed.FeedPageItem) []int64 {
	videoIDs := make([]int64, 0, len(items))
	seen := map[int64]struct{}{}
	for _, item := range items {
		if item == nil || item.VideoID <= 0 {
			continue
		}
		if _, ok := seen[item.VideoID]; ok {
			continue
		}
		seen[item.VideoID] = struct{}{}
		videoIDs = append(videoIDs, item.VideoID)
	}
	return videoIDs
}
