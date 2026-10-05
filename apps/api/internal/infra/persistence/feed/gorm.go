package infrafeed

import (
	domainfeed "DreamReel/internal/domain/feed"
	domaininteraction "DreamReel/internal/domain/interaction"
	domainvideo "DreamReel/internal/domain/video"
	"context"
	"fmt"

	"gorm.io/gorm"
)

const hotScoreExpression = "COALESCE(vs.like_count, 0) * 3 + COALESCE(vs.comment_count, 0) * 5 + COALESCE(vs.favorite_count, 0) * 4"

type Repository struct {
	db *gorm.DB
}

// New 创建 Feed 仓储实现。
func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) basePageQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("video AS v").
		Select("v.id AS video_id, v.author_id, v.published_at").
		Where("v.status = ? AND v.published_at IS NOT NULL", domainvideo.StatusPublished)
}

// ListTimelinePage 查询时间线 Feed 轻量页，卡片和计数由应用层批量组装。
func (r *Repository) ListTimelinePage(ctx context.Context, cursor *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedPageItem, error) {
	var pageitems []domainfeed.FeedPageItem
	query := r.basePageQuery(ctx)
	if cursor != nil {
		query = query.Where(
			"(v.published_at < ? OR (v.published_at = ? AND v.id < ?))",
			cursor.PublishedAt, cursor.PublishedAt, cursor.VideoID,
		)
	}

	err := query.
		Order("v.published_at DESC").
		Order("v.id DESC").
		Limit(limit).
		Scan(&pageitems).
		Error

	if err != nil {
		return nil, err
	}
	return feedPageItemsFromModels(pageitems), nil
}

func feedPageItemsFromModels(models []domainfeed.FeedPageItem) []*domainfeed.FeedPageItem {
	items := make([]*domainfeed.FeedPageItem, 0, len(models))
	for index := range models {
		items = append(items, &models[index])
	}
	return items
}

// BatchGetFeedCards 批量从视频卡片读取视频卡片展示字段。
func (r *Repository) BatchGetFeedCards(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error) {
	cards := map[int64]*domainfeed.FeedCard{}
	if len(videoIDs) == 0 {
		return cards, nil
	}

	var models []domainfeed.FeedCard
	err := r.db.WithContext(ctx).
		Table("video as v").
		Select("v.id AS video_id, v.author_id, a.nickname AS author_nickname, a.avatar_url AS author_avatar_url, v.title, v.description, v.media_url, v.cover_url, v.published_at, v.model_name").
		Joins("LEFT JOIN account AS a ON a.id = v.author_id").
		Where("v.id IN ? AND v.status = ? AND v.published_at IS NOT NULL", videoIDs, domainvideo.StatusPublished).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}
	for index := range models {
		cards[models[index].VideoID] = &models[index]
	}
	return cards, nil
}

// BatchGetFeedStats 批量读取互动计数，缺失统计记录时按 0 处理。
func (r *Repository) BatchGetFeedStats(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	stats := map[int64]*domainfeed.FeedStat{}
	if len(videoIDs) == 0 {
		return stats, nil
	}

	// 即使未在数据库中找到统计记录，我们也按0处理
	for _, videoID := range videoIDs {
		stats[videoID] = &domainfeed.FeedStat{}
	}

	var models []domainfeed.FeedStat
	err := r.db.WithContext(ctx).
		Table("video_stat").
		Select("video_id, like_count, comment_count, favorite_count").
		Where("video_id IN ?", videoIDs).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}
	for index := range models {
		stats[models[index].VideoID] = &models[index]
	}
	return stats, nil
}

// BatchGetViewerActionStates 批量读取当前用户对视频的点赞和收藏状态。
func (r *Repository) BatchGetViewerActions(ctx context.Context, viewerID int64, videoIDs []int64) (map[int64]*domainfeed.ViewerActionState, error) {
	states := map[int64]*domainfeed.ViewerActionState{}
	if viewerID <= 0 || len(videoIDs) == 0 {
		return states, nil
	}
	// 未读默认为0
	for _, videoID := range videoIDs {
		states[videoID] = &domainfeed.ViewerActionState{VideoID: videoID}
	}

	var models []viewerActionStateModel
	err := r.db.WithContext(ctx).
		Table("interaction_action").
		Select("video_id, action_type").
		Where("user_id = ? AND video_id IN ? AND status = ?", viewerID, videoIDs, domaininteraction.ActionStatusActive).
		Where("action_type IN ?", []string{domaininteraction.ActionTypeLike, domaininteraction.ActionTypeFavorite}).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}
	for _, model := range models {
		state := states[model.VideoID]
		if state == nil {
			state = &domainfeed.ViewerActionState{VideoID: model.VideoID}
			states[model.VideoID] = state
		}
		switch model.ActionType {
		case domaininteraction.ActionTypeLike:
			state.Liked = true
		case domaininteraction.ActionTypeFavorite:
			state.Favorited = true
		}
	}
	return states, nil
}

// ListHotPage 查询热榜 Feed 轻量页，按互动热度倒序稳定分页。
func (r *Repository) ListHotPage(ctx context.Context, cursor *domainfeed.HotCursor, limit int) ([]*domainfeed.FeedPageItem, error) {
	var models []domainfeed.FeedPageItem
	query := r.baseHotPageQuery(ctx)

	if cursor != nil {
		query = query.Where(
			fmt.Sprintf("((%[1]s) < ? OR ((%[1]s) = ? AND v.published_at < ?) OR ((%[1]s) = ? AND v.published_at = ? AND v.id < ?))", hotScoreExpression),
			cursor.HotScore,
			cursor.HotScore,
			cursor.PublishedAt,
			cursor.HotScore,
			cursor.PublishedAt,
			cursor.VideoID,
		)
	}

	err := query.
		Order("hot_score DESC").
		Order("v.published_at DESC").
		Order("v.id DESC").
		Limit(limit).
		Scan(&models).
		Error

	if err != nil {
		return nil, err
	}

	return feedPageItemsFromModels(models), nil

}

func (r *Repository) baseHotPageQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("video AS v").
		Select("v.id AS video_id, v.author_id, ("+hotScoreExpression+") AS hot_score, v.published_at").
		Joins("LEFT JOIN video_stat AS vs ON vs.video_id = v.id").
		Where("v.status = ? AND v.published_at IS NOT NULL", domainvideo.StatusPublished)
}
