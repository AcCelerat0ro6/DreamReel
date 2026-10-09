package applicationfeed

import (
	domainfeed "DreamReel/internal/domain/feed"
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

type HotStrategy struct {
	repo  domainfeed.Repository
	cache FeedCache
}

// NewHotStrategy 创建热榜排序策略。
func NewHotStrategy(repo domainfeed.Repository) *HotStrategy {
	return &HotStrategy{repo: repo}
}

type hotCursorPayload struct {
	HotScore    int    `json:"hot_score"`
	PublishedAt string `json:"published_at"`
	VideoID     int64  `json:"video_id"`
	WindowEnd   string `json:"window_end,omitempty"`
	Offset      int    `json:"offset,omitempty"`
}

// Scene 返回热榜场景。
func (s *HotStrategy) Scene() domainfeed.Scene {
	return domainfeed.SceneHot
}

// List 读取热榜；Redis 场景使用最近一小时分钟桶，基础场景使用仓储累计热度。
func (s *HotStrategy) List(ctx context.Context, req FeedRequest) (*FeedResult, error) {
	parsedCursor, err := parseHotCursor(req.Cursor)
	if err != nil {
		return nil, err
	}
	limit := normalizeLimit(req.Limit)

	var page *FeedPage
	if s.cache != nil {
		if strings.TrimSpace(req.Cursor) != "" && (parsedCursor == nil || parsedCursor.WindowEnd.IsZero()) {
			return nil, domainfeed.ErrInvalidCursor
		}
		// 分支1. 有热度截止窗口，走Redis 最近一小时分钟桶
		page, err = s.listPageFromHotWindow(ctx, parsedCursor, limit)
	} else {
		// 分支2. 无热度截止窗口，走基础数据累计热度
		page, err = s.listPageFromRepo(ctx, parsedCursor, limit)
	}
	if err != nil {
		return nil, err
	}
	items, err := assembleFeedItems(ctx, s.repo, s.cache, page.Items, req.ViewerID)
	if err != nil {
		return nil, ErrLoadFeedFailed
	}
	return &FeedResult{
		Scene:      domainfeed.SceneHot,
		Items:      items,
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}, nil
}

// parseHotCursor 将客户端传回的热榜游标解析成领域游标。
func parseHotCursor(raw string) (*domainfeed.HotCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	content, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		content, err = base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, domainfeed.ErrInvalidCursor
		}
	}
	var payload hotCursorPayload
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, domainfeed.ErrInvalidCursor
	}

	// 分支1. 有热度截止窗口，走Redis 最近一小时分钟桶
	if strings.TrimSpace(payload.WindowEnd) != "" {
		windowEnd, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(payload.WindowEnd))
		if err != nil || payload.Offset < 0 {
			return nil, domainfeed.ErrInvalidCursor
		}
		return &domainfeed.HotCursor{
			WindowEnd: windowEnd,
			Offset:    payload.Offset,
		}, nil
	}

	// 分支2. 无热度截止窗口，走基础数据累计热度
	publishedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(payload.PublishedAt))
	if err != nil || payload.VideoID <= 0 {
		return nil, domainfeed.ErrInvalidCursor
	}

	return &domainfeed.HotCursor{
		HotScore:    payload.HotScore,
		PublishedAt: publishedAt,
		VideoID:     payload.VideoID,
	}, nil

}

// listPageFromHotWindow 从 Redis 热度窗口列表中读取一页数据。使用Redis 最近一小时分钟桶
func (s *HotStrategy) listPageFromHotWindow(ctx context.Context, parsedCursor *domainfeed.HotCursor, limit int) (*FeedPage, error) {
	// 为空游标赋一个默认的初始值：截止窗口为当前时间，偏移量为0
	windowEnd := time.Now().UTC().Truncate(time.Minute)
	offset := 0
	if parsedCursor != nil && !parsedCursor.WindowEnd.IsZero() {
		windowEnd = parsedCursor.WindowEnd.UTC().Truncate(time.Minute)
		offset = parsedCursor.Offset
	}
	items, err := s.cache.ListHotWindowPage(ctx, windowEnd, offset, limit+1)
	if err != nil {
		return nil, ErrLoadFeedFailed
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	nextCursor := ""
	if len(items) > 0 {
		nextCursor = encodeHotWindowCursor(&domainfeed.HotCursor{
			WindowEnd: windowEnd,
			Offset:    offset + len(items),
		})
	}

	return &FeedPage{
		Scene:      domainfeed.SceneHot,
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// encodeHotWindowCursor 把热榜滑动窗口位置编码成 URL 安全的游标字符串。
func encodeHotWindowCursor(cursor *domainfeed.HotCursor) string {
	if cursor == nil || cursor.WindowEnd.IsZero() || cursor.Offset < 0 {
		return ""
	}

	content, err := json.Marshal(hotCursorPayload{
		WindowEnd: cursor.WindowEnd.UTC().Format(time.RFC3339Nano),
		Offset:    cursor.Offset,
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(content)
}

// listPageFromRepo 走基础数据累计热度查询视频页
func (s *HotStrategy) listPageFromRepo(ctx context.Context, parsedCursor *domainfeed.HotCursor, limit int) (*FeedPage, error) {
	items, err := s.repo.ListHotPage(ctx, parsedCursor, limit+1)
	if err != nil {
		return nil, ErrLoadFeedFailed
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	nextCursor := ""
	if len(items) > 0 {
		last := items[len(items)-1]
		nextCursor = encodeHotCursor(&domainfeed.HotCursor{
			HotScore:    last.HotScore,
			PublishedAt: last.PublishedAt,
			VideoID:     last.VideoID,
		})
	}

	return &FeedPage{
		Scene:      domainfeed.SceneHot,
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}
