package applicationfeed

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"internal/singleflight"
	"strings"
	"time"

	domainfeed "DreamReel/internal/domain/feed"

	"go.uber.org/zap"
)

// normalizeLimit 规范 limit 的取值
func normalizeLimit(limit int) int {
	if limit <= 0 {
		return domainfeed.DefaultFeedLimit
	}
	if limit > domainfeed.MaxFeedLimit {
		return domainfeed.MaxFeedLimit
	}
	return limit
}

// parseTimelineCursor 将客户端传回的字符串游标解析成领域游标。
func parseTimelineCursor(cursor string) (*domainfeed.TimelineCursor, error) {
	raw := strings.TrimSpace(cursor)
	if raw == "" {
		// 返回空游标表示从头开始
		return nil, nil
	}

	content, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		// 若 RawURL 编码解码失败 尝试使用标准编码解码
		content, err = base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, domainfeed.ErrInvalidCursor
		}
	}

	var payload timelineCursorPayload
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, domainfeed.ErrInvalidCursor
	}

	publishedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(payload.PublishedAt))
	if err != nil || payload.VideoID <= 0 {
		return nil, domainfeed.ErrInvalidCursor
	}

	return &domainfeed.TimelineCursor{
		PublishedAt: publishedAt,
		VideoID:     payload.VideoID,
	}, nil
}

func feedPageCacheKey(scene domainfeed.Scene, cursor string, limit int) string {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return fmt.Sprintf("feed:page:v1:%s:limit:%d:first", scene, limit)
	}
	sum := sha1.Sum([]byte(cursor))
	return fmt.Sprintf("feed:page:v1:%s:limit:%d:cursor:%s", scene, limit, hex.EncodeToString(sum[:]))
}

func feedPageCacheTTL(cursor string, cacheKey string, firstPageTTL time.Duration, pageTTL time.Duration) time.Duration {
	ttl := pageTTL
	// If cursor is empty, it's the first page
	if strings.TrimSpace(cursor) == "" {
		ttl = firstPageTTL
	}
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(cacheKey))
	jitterPercent := 10 + int(hasher.Sum32()%11)
	return ttl + time.Duration(jitterPercent)*ttl/100
}

// loadFeedPage 从缓存或数据库加载 Feed 的数据。
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

func encodeTimelineCursor(cursor *domainfeed.TimelineCursor) string {
	if cursor == nil || cursor.VideoID <= 0 || cursor.PublishedAt.IsZero() {
		return ""
	}

	// 首先反序列化成 string字符串
	content, err := json.Marshal(timelineCursorPayload{
		PublishedAt: cursor.PublishedAt.UTC().Format(time.RFC3339Nano),
		VideoID:     cursor.VideoID,
	})
	if err != nil {
		return ""
	}
	// 使用RawURL编码并传出.
	return base64.RawURLEncoding.EncodeToString(content)
}

func fingmissingCardIDs(videoIDs []int64, cards map[int64]*domainfeed.FeedCard) []int64 {
	missing := make([]int64, 0)
	for _, videoID := range videoIDs {
		if _, ok := cards[videoID]; !ok {
			missing = append(missing, videoID)
		}
	}
	return missing
}

func findmissingStatIDs(videoIDs []int64, stats map[int64]*domainfeed.FeedStat) []int64 {
	missing := make([]int64, 0)
	for _, videoID := range videoIDs {
		if _, ok := stats[videoID]; !ok {
			missing = append(missing, videoID)
		}
	}
	return missing
}

func mergeCards(target, source map[int64]*domainfeed.FeedCard) {
	for videoID, card := range source {
		if card != nil {
			target[videoID] = card
		}
	}
}

func mergeStats(target, source map[int64]*domainfeed.FeedStat) {
	for videoID, stat := range source {
		if stat != nil {
			target[videoID] = stat
		}
	}
}
