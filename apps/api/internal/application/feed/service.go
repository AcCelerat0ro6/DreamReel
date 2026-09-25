package applicationfeed

import (
	domainfeed "DreamReel/internal/domain/feed"
	metrics "DreamReel/internal/infra/metrics"
	"context"
	"internal/singleflight"
	"time"

	"go.uber.org/zap"
)

type Service struct {
	strategies map[domainfeed.Scene]Strategy
}

func (s *Service) GetFeed(ctx context.Context, request FeedRequest) (FeedResponse, error) {
	start := time.Now()
	request.Scene = domainfeed.NormalizeScene(request.Scene)
	strategy, ok := s.strategies[request.Scene]
	if !ok {
		metrics.ObserveFeed(string(request.Scene), time.Since(start), 0, domainfeed.ErrUnsupportedScene)
		return nil, domainfeed.ErrUnsupportedScene
	}
	result, err := strategy.List(ctx, request)
	itemCount := 0
	if result != nil {
		itemCount = len(result.Items)
	}
	metrics.ObserveFeed(string(request.Scene), time.Since(start), itemCount, err)
	return result, err
}

// loadFeedPage 从缓存或数据库加载时间线 Feed 的一页数据。
func loadFeedPage(ctx context.Context, cache FeedCache, scene domainfeed.Scene, cursor string, limit int, firstPageTTL time.Duration, pageTTL time.Duration, group *singleflight.Group, load func() (*FeedPage, error)) (*FeedPage, error) {
	if cache == nil || group == nil {
		// 没有配置缓存，无并发保护查数据库
		zap.L().Warn("no cache configured, loading feed page from database without concurrency protection...")
		return load()
	}

	cacheKey := feedPageCacheKey(scene, cursor, limit)
	if page, ok, err := cache.GetPage(ctx, cacheKey); err == nil && ok {
		// 缓存命中 直接返回
		return page, nil
	}

	value, err, _ := group.Do(cacheKey, func() (any, error) {
		// 二次检查缓存, 若命中则再次返回
		if page, ok, err := cache.GetPage(ctx, cacheKey); err == nil && ok {
			return page, nil
		}
		// 查库
		page, err := load()
		if err != nil {
			return nil, err
		}

		// 写缓存
		_ = cache.SetPage(ctx, cacheKey, page, feedPageCacheTTL(cursor, cacheKey, firstPageTTL, pageTTL))
		return page, nil
	})
	if err != nil {
		return nil, err
	}
	page, ok := value.(*FeedPage)
	if !ok {
		return nil, ErrLoadFeedFailed
	}
	return page, nil
}
