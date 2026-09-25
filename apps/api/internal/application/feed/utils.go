package applicationfeed

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	domainfeed "DreamReel/internal/domain/feed"
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
