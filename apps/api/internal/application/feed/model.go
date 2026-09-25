package applicationfeed

import (
	domainfeed "DreamReel/internal/domain/feed"
	"context"
	"errors"
	"time"
)

var ErrLoadFeedFailed = errors.New("failed to load feed")

const (
	timelineFirstPageCacheTTL = 5 * time.Second
	timelinePageCacheTTL      = 45 * time.Second
)

// FeedRequest 是所有 Feed 场景共用的查询参数。
type FeedRequest struct {
	Scene         domainfeed.Scene
	Cursor        string
	Limit         int
	ViewerID      int64
	ClientContext map[string]string
}

// FeedResult 是游标分页结果，NextCursor 供客户端请求下一页。
type FeedResult struct {
	Scene      domainfeed.Scene
	Items      []*domainfeed.FeedItem
	NextCursor string
	HasMore    bool
}

// FeedPage 是 Feed 页缓存的清亮骨架，仅保存最小所需字段
type FeedPage struct {
	Scene      domainfeed.Scene
	Items      []*domainfeed.FeedPageItem
	NextCursor string
	HasMore    bool
}

// Strategy 定义了 Feed 策略。
type Strategy interface {
	Scene() domainfeed.Scene
	List(ctx context.Context, req FeedRequest) (*FeedResult, error)
}

// FeedCache 定义 Feed 页、卡片和计数缓存能力。
type FeedCache interface {
	GetPage(ctx context.Context, key string) (*FeedPage, bool, error)
	SetPage(ctx context.Context, key string, page *FeedPage, ttl time.Duration) error
}
