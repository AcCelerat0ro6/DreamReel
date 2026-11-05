package applicationvideo

import (
	domainfeed "DreamReel/internal/domain/feed"
	"DreamReel/internal/infra/metrics"
	"context"
	"time"
)

const defaultFollowerBatchSize = 500

type FanoutWorker struct {
	repo         FanoutRepository
	consumer     PublishedEventConsumer
	index        FollowingIndexCache
	preheater    FeedPreheater
	batchSize    int
	inboxMaxLen  int64
	outboxMaxLen int64
	preheatTTL   time.Duration
}

type FanoutWorkerOption func(*FanoutWorker)

func NewFanoutWorker(repo FanoutRepository, consumer PublishedEventConsumer, index FollowingIndexCache, preheater FeedPreheater, options ...FanoutWorkerOption) *FanoutWorker {
	worker := &FanoutWorker{
		repo:         repo,
		consumer:     consumer,
		index:        index,
		preheater:    preheater,
		batchSize:    defaultFollowerBatchSize,
		inboxMaxLen:  1000,
		outboxMaxLen: 500,
		preheatTTL:   15 * time.Minute,
	}
	for _, option := range options {
		option(worker)
	}
	return worker
}

func WithFanoutBatchSize(size int) FanoutWorkerOption {
	return func(w *FanoutWorker) {
		if size > 0 {
			w.batchSize = size
		}
	}
}

type FanoutRepository interface {
	// CountFollowers 计算关注者数量
	CountFollowers(ctx context.Context, authorID int64) (int, error)
	// ListFollowerIDs 列出关注者ID
	ListFollowerIDs(ctx context.Context, authorID int64, cursor int64, limit int) ([]int64, error)
	// BatchGetFeedCards 批量获取Feed卡片
	BatchGetFeedCards(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error)
	// BatchGetFeedStats 批量获取Feed计数统计信息
	BatchGetFeedStats(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error)
}

type PublishedEventConsumer interface {
	ConsumeVideoPublished(ctx context.Context, handler func(context.Context, *PublishedEvent) error) error
}

type FollowingIndexCache interface {
	// AddInboxItems 添加到用户Inbox,采取推送方式
	AddInboxItems(ctx context.Context, authorID int64, userIDs []int64, item *domainfeed.FeedPageItem, maxLen int64) error
	// AddAuthorOutboxItem 添加到作者的Outbox，采取拉取方式
	AddAuthorOutboxItem(ctx context.Context, authorID int64, item *domainfeed.FeedPageItem, maxLen int64) error
}

type FeedPreheater interface {
	PreheatFeedVideo(ctx context.Context, videoID int64, ttl time.Duration) error
}

type FeedCacheWriter interface {
	SetCards(ctx context.Context, cards map[int64]*domainfeed.FeedCard, ttl time.Duration) error
	SetStats(ctx context.Context, stats map[int64]*domainfeed.FeedStat, ttl time.Duration) error
}

type FeedPreheaterAdapter struct {
	source FanoutRepository
	cache  FeedCacheWriter
}

func NewFeedPreheater(source FanoutRepository, cache FeedCacheWriter) *FeedPreheaterAdapter {
	return &FeedPreheaterAdapter{source: source, cache: cache}
}

func (p *FeedPreheaterAdapter) PreheatFeedVideo(ctx context.Context, videoID int64, ttl time.Duration) error {
	if p == nil || p.source == nil || p.cache == nil || videoID <= 0 {
		return nil
	}
	cards, err := p.source.BatchGetFeedCards(ctx, []int64{videoID})
	if err != nil {
		return err
	}
	if len(cards) > 0 {
		if err := p.cache.SetCards(ctx, cards, ttl); err != nil {
			return err
		}
	}
	stats, err := p.source.BatchGetFeedStats(ctx, []int64{videoID})
	if err != nil {
		return err
	}
	if len(stats) > 0 {
		return p.cache.SetStats(ctx, stats, 15*time.Second)
	}
	return nil
}

func (w *FanoutWorker) HandleVideoPublished(ctx context.Context, event *PublishedEvent) (err error) {
	start := time.Now()
	defer func() {
		metrics.ObserveWorkerJob("video_fanout", time.Since(start), err)
	}()

	if event == nil || event.VideoID <= 0 || event.AuthorID <= 0 {
		return nil
	}
	// 预热视频- 将FeedStat和FeedCard添加到缓存
	if err = w.preheat(ctx, event); err != nil {
		return err
	}
	if w.repo == nil || w.index == nil {
		return nil
	}

	item := &domainfeed.FeedPageItem{
		VideoID:     event.VideoID,
		AuthorID:    event.AuthorID,
		PublishedAt: event.PublishedAt,
	}
	followerCount, err := w.repo.CountFollowers(ctx, event.AuthorID)
	if err != nil {
		return err
	}
	if followerCount >= domainfeed.BigCreatorFollowerThreshold {
		// 大V，添加到作者的Outbox
		return w.index.AddAuthorOutboxItem(ctx, event.AuthorID, item, w.outboxMaxLen)
	}

	cursor := int64(0)
	for {
		followerIDs, err := w.repo.ListFollowerIDs(ctx, event.AuthorID, cursor, w.batchSize)
		if err != nil {
			return err
		}
		if len(followerIDs) == 0 {
			return nil
		}
		if err = w.index.AddInboxItems(ctx, event.AuthorID, followerIDs, item, w.inboxMaxLen); err != nil {
			return err
		}
		cursor = followerIDs[len(followerIDs)-1]
		if len(followerIDs) < w.batchSize {
			return nil
		}
	}
}

func (w *FanoutWorker) preheat(ctx context.Context, event *PublishedEvent) error {
	if w.preheater == nil {
		return nil
	}
	return w.preheater.PreheatFeedVideo(ctx, event.VideoID, w.preheatTTL)
}

func (w *FanoutWorker) Start(ctx context.Context) error {
	if w == nil || w.consumer == nil {
		return nil
	}
	return w.consumer.ConsumeVideoPublished(ctx, w.HandleVideoPublished)
}
