package applicationfeed

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
		Scene :     req.Scene,
		Items:      items,
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}, nil
}

