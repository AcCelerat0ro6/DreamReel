package infrarecommendation

import (
	domainembedding "DreamReel/internal/domain/embedding"
	domainrecommendation "DreamReel/internal/domain/recommendation"
	domainvideo "DreamReel/internal/domain/video"
	"context"
	"time"
)

func (r *Repository) ListCandidatePool(ctx context.Context, userID int64, limit int) ([]*domainrecommendation.Candidate, error) {
	if limit <= 0 {
		return []*domainrecommendation.Candidate{}, nil
	}
	var models []candidateModel

	err := r.db.WithContext(ctx).
		Table("video AS v").
		Select("v.id AS video_id, v.author_id, ("+hotScoreExpression+") AS hot_score, v.published_at").
		Joins("LEFT JOIN video_stat AS vs ON vs.video_id = v.id").
		Joins(
			"LEFT JOIN exposures AS e ON e.user_id = ? AND e.video_id = v.id AND e.last_exposed_at >= ?",
			userID,
			time.Now().Add(-domainrecommendation.RecentExposureWindow),
		).
		Where("v.status = ? AND v.published_at IS NOT NULL AND e.video_id IS NULL", domainvideo.StatusPublished).
		Order("hot_score DESC").
		Order("v.published_at DESC").
		Order("v.id DESC").
		Limit(limit).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}

	candidates := make([]*domainrecommendation.Candidate, 0, len(models))
	for _, model := range models {
		candidates = append(candidates, domainrecommendation.RestoreCandidate(
			model.VideoID,
			model.AuthorID,
			0,
			0,
			model.HotScore,
			0,
			"",
			model.PublishedAt,
		))
	}
	return candidates, nil
}

func (r *Repository) LoadVideoVectors(ctx context.Context, videoIDs []int64) (map[int64][]float64, error) {
	vectors := map[int64][]float64{}
	if len(videoIDs) == 0 {
		return vectors, nil
	}

	var models []videoVectorModel
	err := r.db.WithContext(ctx).
		Table("video_embedding").
		Select("video_id, embedding_json").
		Where("video_id IN ? AND model = ?", videoIDs, domainembedding.HashNgramModel).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}
	for _, model := range models {
		vector, err := decodeVector(model.EmbeddingJSON)
		if err != nil {
			continue
		}
		vectors[model.VideoID] = vector
	}
	return vectors, nil
}

func (r *Repository) LoadUserInterestVector(ctx context.Context, userID int64) ([]float64, bool, error) {
	// 从数据库中查询指定用户最近30天内的观看视频记录和观看视频的向量， 至多200条.
	rows, err := r.db.WithContext(ctx).
		Table("video_view_events AS ev").
		Select("ve.embedding_json, ev.event_type, ev.watch_ms, ev.completed").
		Joins("JOIN video_embedding AS ve ON ve.video_id = ev.video_id AND ve.model = ?", domainembedding.HashNgramModel).
		Where("ev.user_id = ? AND ev.created_at >= ? AND ev.event_type IN ?", userID, time.Now().Add(-positiveEventWindow), positiveEventTypes()).
		Order("ev.created_at DESC").
		Limit(200).
		Rows()

	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var sum []float64
	var totalWeight float64
	for rows.Next() {
		var embeddingJSON string
		var eventType string
		var watchMs int
		var completed bool
		if err := rows.Scan(&embeddingJSON, &eventType, &watchMs, &completed); err != nil {
			return nil, false, err
		}
		vector, err := decodeVector(embeddingJSON)
		if err != nil || len(vector) == 0 {
			continue
		}
		if len(sum) == 0 {
			sum = make([]float64, len(vector))
		}
		if len(vector) != len(sum) {
			continue
		}
		weight := eventWeight(eventType, watchMs, completed)
		for i := range vector {
			sum[i] += vector[i] * weight
		}
		totalWeight += weight
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if len(sum) == 0 || totalWeight == 0 {
		return nil, false, nil
	}
	for i := range sum {
		sum[i] = sum[i] / totalWeight
	}
	return sum, true, nil
}
