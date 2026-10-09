package cache

import (
	domainfeed "DreamReel/internal/domain/feed"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	actionStatCounterShardCount = 16
	actionStatJSONTTL           = 15 * time.Second
)

func feedCardKey(videoID int64) string {
	return fmt.Sprintf("video:card:v1:%d", videoID)
}

func feedStatKey(videoID int64) string {
	return fmt.Sprintf("video:stat:v1:%d", videoID)
}

func interactionStatCounterKey(videoID int64) string {
	return fmt.Sprintf("video:stat:counter:v1:%d", videoID)
}

func interactionStatCounterBaseKey(videoID int64) string {
	return fmt.Sprintf("%s:base", interactionStatCounterKey(videoID))
}

func interactionStatCounterShardKey(videoID int64, shard int) string {
	return fmt.Sprintf("%s:shard:%02d", interactionStatCounterKey(videoID), shard)
}

func hotWindowKey(windowEnd time.Time) string {
	return fmt.Sprintf("feed:hot:window:v1:%d", windowEnd.UTC().Truncate(time.Minute).Unix())
}

func hotWindowMinuteKeys(windowEnd time.Time) []string {
	keys := make([]string, 0, hotWindowMinutes)
	for index := hotWindowMinutes - 1; index >= 0; index-- {
		keys = append(keys, hotMinuteKey(windowEnd.Add(-time.Duration(index)*time.Minute)))
	}
}

func hotMinuteKey(at time.Time) string {
	return fmt.Sprintf("feed:hot:minute:v1:%s", at.UTC().Truncate(time.Minute).Format("200601021504"))
}

func interactionStatCounterShardKeys(videoID int64) []string {
	keys := make([]string, 0, actionStatCounterShardCount)
	for shard := 0; shard < actionStatCounterShardCount; shard++ {
		keys = append(keys, interactionStatCounterShardKey(videoID, shard))
	}
	return keys
}

func cacheKeys(videoIDs []int64, build func(int64) string) []string {
	keys := make([]string, 0, len(videoIDs))
	for _, videoID := range videoIDs {
		keys = append(keys, build(videoID))
	}
	return keys
}

// 获取用户收件箱Key
func followingInboxKey(userID int64) string {
	return fmt.Sprintf("feed:following:inbox:v1:%d", userID)
}

// 大V发送箱Keys
func followingAuthorOutboxKey(authorID int64) string {
	return fmt.Sprintf("feed:following:author:v1:%d", authorID)
}

// 按照宏观发布时间生成排序分数
func followingIndexScore(publishedAt time.Time, videoID int64) float64 {
	return float64(publishedAt.UTC().Unix()*1000000 + videoID%1000000)
}

// cacheValueBytes 返回缓存值的字节表示，如果值为空则返回 false
func cacheValueBytes(value any) ([]byte, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, false
	case string:
		return []byte(typed), true
	case []byte:
		return typed, true
	default:
		return nil, false
	}
}

func applyActionStatFields(stat *domainfeed.FeedStat, values map[string]string) {
	if stat == nil {
		return
	}
	stat.LikeCount, _ = strconv.Atoi(values["like_count"])
	stat.CommentCount, _ = strconv.Atoi(values["comment_count"])
	stat.FavoriteCount, _ = strconv.Atoi(values["favorite_count"])
}

func clampRedisCount(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

// hotRankVideoID 获取int64形式的 Redis中热榜视频ID
func hotRankVideoID(member string) (int64, bool) {
	value := strings.TrimLeft(member, "0")
	if value == "" {
		return 0, false
	}
	videoID, err := strconv.ParseInt(value, 10, 64)
	return videoID, err == nil && videoID > 0
}

func int64Set(values []int64) map[int64]struct{} {
	set := map[int64]struct{}{}
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

// 对所有视频源进行按时间线排序归并整合
func sortFeedPageItemsByTimeline(items []*domainfeed.FeedPageItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].PublishedAt.Equal(items[j].PublishedAt) {
			return items[i].VideoID > items[j].VideoID
		}
		return items[i].PublishedAt.After(items[j].PublishedAt)
	})
}
