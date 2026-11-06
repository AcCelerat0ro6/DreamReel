package infravideo

import (
	domainvideo "DreamReel/internal/domain/video"
	"context"
)

func (r *Repository) ListModelRankings(ctx context.Context, sort string, limit, offset int) ([]domainvideo.ModelRanking, error) {
	// Only fixed SQL expressions may enter ORDER BY. Missing stats still count as a work.
	order := "hot_score DESC, video_count DESC, model_name ASC"
	switch sort {
	case "works":
		order = "video_count DESC, hot_score DESC, model_name ASC"
	case "likes":
		order = "like_count DESC, hot_score DESC, video_count DESC, model_name ASC"
	}
	items := []domainvideo.ModelRanking{}
	err := r.db.WithContext(ctx).Table("video AS v").
		Select(`MIN(TRIM(v.model_name)) AS model_name, COUNT(*) AS video_count,
			SUM(COALESCE(vs.like_count, 0)) AS like_count,
			SUM(COALESCE(vs.comment_count, 0)) AS comment_count,
			SUM(COALESCE(vs.favorite_count, 0)) AS favorite_count,
			SUM(COALESCE(vs.like_count, 0) * 3 + COALESCE(vs.comment_count, 0) * 5 + COALESCE(vs.favorite_count, 0) * 4) AS hot_score`).
		Joins("LEFT JOIN video_stat AS vs ON vs.video_id = v.id").
		Where("v.status = ? AND v.published_at IS NOT NULL AND v.model_name IS NOT NULL AND TRIM(v.model_name) <> ''", domainvideo.StatusPublished).
		Group("LOWER(TRIM(v.model_name))").Order(order).Limit(limit).Offset(offset).Scan(&items).Error
	return items, err
}
