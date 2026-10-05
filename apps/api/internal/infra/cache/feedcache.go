package cache

import (
	applicationfeed "DreamReel/internal/application/feed"
	domainfeed "DreamReel/internal/domain/feed"
	domaininteraction "DreamReel/internal/domain/interaction"
	"DreamReel/internal/infra/metrics"
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	hotWindowCacheTTL = 2 * time.Minute
	hotWindowMinutes  = 60
)

// redis 相关接口。
type redisWatchCmdable interface {
	redis.Cmdable
	Pipeline() redis.Pipeliner
	Watch(ctx context.Context, fn func(*redis.Tx) error, keys ...string) error
}

type FeedCache struct {
	client redisWatchCmdable
}

// GetPage 尝试读取缓存中的轻量 Feed 页。
func (c *FeedCache) GetPage(ctx context.Context, key string) (*applicationfeed.FeedPage, bool, error) {
	content, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		metrics.ObserveCacheRead("page", 1, 0, nil)
		return nil, false, nil
	}
	if err != nil {
		metrics.ObserveCacheRead("page", 1, 0, err)
		return nil, false, err
	}

	var page applicationfeed.FeedPage
	if err := json.Unmarshal(content, &page); err != nil {
		metrics.ObserveCacheRead("page", 1, 0, err)
		return nil, false, err
	}
	metrics.ObserveCacheRead("page", 1, 1, nil)
	return &page, true, nil
}

// SetPage 写入轻量 Feed 页，并设置过期时间。
func (c *FeedCache) SetPage(ctx context.Context, key string, page *applicationfeed.FeedPage, ttl time.Duration) error {
	content, err := json.Marshal(page)
	if err != nil {
		metrics.ObserveCacheWrite("page", 1, err)
		return err
	}

	err = c.client.Set(ctx, key, content, ttl).Err()
	metrics.ObserveCacheWrite("page", 1, err)
	return err
}

// GetCards 批量读取视频卡片缓存。
func (c *FeedCache) GetCards(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error) {
	cards := map[int64]*domainfeed.FeedCard{}
	if len(videoIDs) == 0 {
		return cards, nil
	}

	results, err := c.client.MGet(ctx, cacheKeys(videoIDs, feedCardKey)...).Result()
	if err != nil {
		metrics.ObserveCacheRead("card", len(videoIDs), 0, err)
		return nil, err
	}
	for index, result := range results {
		content, ok := cacheValueBytes(result)
		if !ok {
			continue
		}
		var card domainfeed.FeedCard
		if err := json.Unmarshal(content, &card); err != nil {
			continue
		}
		if card.VideoID <= 0 {
			card.VideoID = videoIDs[index]
		}
		cards[card.VideoID] = &card
	}
	metrics.ObserveCacheRead("card", len(videoIDs), len(cards), nil)
	return cards, nil
}

// GetStats 批量读取视频动态计数缓存。
func (c *FeedCache) GetStats(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	stats := map[int64]*domainfeed.FeedStat{}
	if len(videoIDs) == 0 {
		return stats, nil
	}
	results, err := c.client.MGet(ctx, cacheKeys(videoIDs, feedStatKey)...).Result()
	if err != nil {
		metrics.ObserveCacheRead("stat", len(videoIDs), 0, err)
		return nil, err
	}

	for index, result := range results {
		content, ok := cacheValueBytes(result)
		if !ok {
			continue
		}
		var stat domainfeed.FeedStat
		if err := json.Unmarshal(content, &stat); err != nil {
			continue
		}
		if stat.VideoID <= 0 {
			stat.VideoID = videoIDs[index]
		}
		stats[stat.VideoID] = &stat
	}
	for _, videoID := range videoIDs {
		if stats[videoID] != nil {
			continue
		}
		stat, ok, err := c.actionStatFromCache(ctx, videoID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		stats[videoID] = stat
		_ = c.setActionStatJSON(ctx, feedStatKey(videoID), stat)

	}
	metrics.ObserveCacheRead("stat", len(videoIDs), len(stats), nil)
	return stats, nil
}

func (c *FeedCache) actionStatFromCache(ctx context.Context, videoID int64) (*domainfeed.FeedStat, bool, error) {
	return c.actionStatWithPresence(ctx, interactionStatCounterBaseKey(videoID), interactionStatCounterShardKeys(videoID), feedStatKey(videoID), videoID, nil)
}

func (c *FeedCache) actionStatWithPresence(ctx context.Context, counterBaseKey string, counterShardKeys []string, jsonKey string, videoID int64, initialStat *domaininteraction.VideoStat) (*domainfeed.FeedStat, bool, error) {
	stat := &domainfeed.FeedStat{VideoID: videoID}
	found := false
	result, err := c.client.HGetAll(ctx, counterBaseKey).Result()
	if err != nil {
		return nil, false, err
	}
	if len(result) > 0 {
		// 基准计数缓存已经找到，回填即可
		applyActionStatFields(stat, result)
		found = true
	} else {
		// 未找到基准计数缓存， 二次查找完整计数缓存
		fallbackStat, ok, err := c.actionStatFallback(ctx, jsonKey, videoID, initialStat)
		if err != nil {
			return nil, false, err
		}
		if ok {
			stat = fallbackStat
			found = true
		}
	}

	sharedFound, err := c.applyActionStatShardDeltas(ctx, stat, counterShardKeys)
	if err != nil {
		return nil, false, err
	}
	found = found || sharedFound
	if !found {
		return nil, false, nil
	}
	return stat, true, nil
}

// actionStatFallback再次查询完整计数缓存
func (c *FeedCache) actionStatFallback(ctx context.Context, jsonKey string, videoID int64, initialStat *domaininteraction.VideoStat) (*domainfeed.FeedStat, bool, error) {
	stat := &domainfeed.FeedStat{VideoID: videoID}
	content, err := c.client.Get(ctx, jsonKey).Bytes()
	if err != redis.Nil {
		if initialStat != nil {
			stat.LikeCount = initialStat.LikeCount
			stat.CommentCount = initialStat.CommentCount
			stat.FavoriteCount = initialStat.FavoriteCount
			return stat, true, nil
		}
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if err := json.Unmarshal(content, stat); err != nil {
		return nil, false, nil
	}
	if stat.VideoID <= 0 {
		stat.VideoID = videoID
	}
	return stat, true, nil
}

func (c *FeedCache) applyActionStatShardDeltas(ctx context.Context, stat *domainfeed.FeedStat, sharedKeys []string) (bool, error) {
	if stat == nil || len(sharedKeys) == 0 {
		return false, nil
	}
	sharedValues, err := c.loadActionStatShardValues(ctx, sharedKeys)
	if err != nil {
		return false, err
	}
	found := false
	// 逐一计算增量
	likeDelta := 0
	favoriteDelta := 0
	for _, value := range sharedValues {
		if len(value) > 0 {
			found = true
		}
		likePartDelta, _ := strconv.Atoi(value["like_count"])
		favoritePartDelta, _ := strconv.Atoi(value["favorite_count"])
		likeDelta += likePartDelta
		favoriteDelta += favoritePartDelta
	}
	stat.LikeCount = clampRedisCount(stat.LikeCount + likeDelta)
	stat.FavoriteCount = clampRedisCount(stat.FavoriteCount + favoriteDelta)
	return found, nil
}

func (c *FeedCache) loadActionStatShardValues(ctx context.Context, shardKeys []string) ([]map[string]string, error) {
	// 获取所有计数分片的值总和
	type pipelineProvider interface {
		Pipeline() redis.Pipeliner
	}

	if provider, ok := c.client.(pipelineProvider); ok {
		// 构建pipeline, 加入所有命令
		pipe := provider.Pipeline()
		cmds := make([]*redis.MapStringStringCmd, 0, len(shardKeys))
		for _, key := range shardKeys {
			cmd := pipe.HGetAll(ctx, key)
			cmds = append(cmds, cmd)
		}

		// 执行
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			return nil, err
		}
		values := make([]map[string]string, 0, len(cmds))
		for _, cmd := range cmds {
			value, err := cmd.Result()
			if err != nil && err != redis.Nil {
				return nil, err
			}
			values = append(values, value)
		}
		return values, nil
	}

	// 如果没有实现pipelineProvider接口，则逐条命令执行查询
	values := make([]map[string]string, 0, len(shardKeys))
	for _, key := range shardKeys {
		value, err := c.client.HGetAll(ctx, key).Result()
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

// 往缓存中写入FeedStat
func (c *FeedCache) setActionStatJSON(ctx context.Context, jsonKey string, stat *domainfeed.FeedStat) error {
	content, err := json.Marshal(stat)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, jsonKey, content, actionStatJSONTTL).Err()
}

// SetStats 批量写入视频计数缓存。
func (c *FeedCache) SetStats(ctx context.Context, stats map[int64]*domainfeed.FeedStat, ttl time.Duration) error {
	pipe := c.client.Pipeline()
	queued := false
	for _, stat := range stats {
		if stat == nil || stat.VideoID <= 0 {
			continue
		}
		content, err := json.Marshal(stat)
		if err != nil {
			return err
		}
		pipe.Set(ctx, feedStatKey(stat.VideoID), content, ttl)
		queued = true
	}
	if !queued {
		return nil
	}
	_, err := pipe.Exec(ctx)
	metrics.ObserveCacheWrite("stat", len(stats), err)
	return err
}

// ListHotWindowPage 合并最近 60 个分钟桶，返回一小时滑动窗口内的热榜页。
func (c *FeedCache) ListHotWindowPage(ctx context.Context, windowEnd time.Time, offset int, limit int) ([]*domainfeed.FeedPageItem, error) {
	items := []*domainfeed.FeedPageItem{}
	if limit <= 0 {
		return items, nil
	}
	if offset < 0 {
		offset = 0
	}
	windowEnd = windowEnd.UTC().Truncate(time.Minute)
	windowKey := hotWindowKey(windowEnd)
	exists, err := c.client.Exists(ctx, windowKey).Result()
	if err != nil {
		return nil, err
	}
	if exists == 0 {
		// 热榜窗口不存在，需要重建
		if err := c.rebuildHotWindow(ctx, windowKey, windowEnd); err != nil {
			return nil, err
		}
	}
	return c.listHotWindowPage(ctx, windowKey, offset, limit)
}

func (c *FeedCache) listHotWindowPage(ctx context.Context, windowKey string, offset int, limit int) ([]*domainfeed.FeedPageItem, error) {
	items := []*domainfeed.FeedPageItem{}
	values, err := c.client.ZRevRangeWithScores(ctx, windowKey, int64(offset), int64(offset+limit-1)).Result()
	if err != nil {
		return nil, err
	}
	for _, value := range values {
		member, ok := value.Member.(string)
		if !ok {
			continue
		}
		videoID, ok := hotRankVideoID(member)
		if !ok {
			continue
		}
		items = append(items, &domainfeed.FeedPageItem{
			VideoID:  videoID,
			HotScore: int(value.Score),
		})
	}
	return items, nil
}

// rebuildHotWindow 重建热榜窗口
func (c *FeedCache) rebuildHotWindow(ctx context.Context, windowKey string, windowEnd time.Time) error {
	// 获取最近的60分钟桶对应的Redis Key, 并将他们对应的ZSet的各个元素求和
	if _, err := c.client.ZUnionStore(ctx, windowKey, &redis.ZStore{
		Keys:      hotWindowMinuteKeys(windowEnd),
		Aggregate: "SUM",
	}).Result(); err != nil {
		return err
	}
	pipe := c.client.Pipeline()
	// 去掉ZSet中的负数元素
	pipe.ZRemRangeByScore(ctx, windowKey, "-inf", "0")
	// 设置过期时间
	pipe.Expire(ctx, windowKey, hotWindowCacheTTL)
	_, err := pipe.Exec(ctx)
	return err
}
