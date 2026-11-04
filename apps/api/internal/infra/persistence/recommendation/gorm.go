package infrarecommendation

import (
	domainembedding "DreamReel/internal/domain/embedding"
	domainexposure "DreamReel/internal/domain/exposure"
	domainrecommendation "DreamReel/internal/domain/recommendation"
	domainvideo "DreamReel/internal/domain/video"
	infraexposure "DreamReel/internal/infra/persistence/exposure"
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// LoadUserInterestVector 优先读取有效窗口内的推荐画像表向量，缺失或过期时回退为近期观看行为在线聚合。
func (r *Repository) LoadUserInterestVector(ctx context.Context, userID int64) ([]float64, bool, error) {
	if userID <= 0 {
		return nil, false, nil
	}

	since := time.Now().Add(-domainrecommendation.PositiveBehaviorWindow)
	var profile UserInterestProfileModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND model = ? AND updated_at >= ?", userID, domainembedding.HashNgramModel, since).
		Take(&profile).
		Error
	if err == nil {
		vector, decodeErr := decodeVector(profile.EmbeddingJSON)
		if decodeErr == nil && len(vector) == domainembedding.HashNgramDimension && domainrecommendation.IsValidInterestVector(vector) {
			return vector, true, nil
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	samples, err := r.ListRecentBehaviorSamples(
		ctx,
		userID,
		domainembedding.HashNgramModel,
		since,
		domainrecommendation.MaxProfileSampleEvents,
	)
	if err != nil {
		return nil, false, err
	}
	vector, sampleCount := domainrecommendation.AggregateInterestVector(samples)
	if !domainrecommendation.IsValidInterestVector(vector) || sampleCount == 0 {
		return nil, false, nil
	}
	return vector, true, nil
}

// ListRecentBehaviorSamples 读取用户近期对已发布视频的正反馈观看行为，并在视频向量尚未落表时按标题描述兜底计算。
func (r *Repository) ListRecentBehaviorSamples(ctx context.Context, userID int64, model string, since time.Time, limit int) ([]*domainrecommendation.BehaviorSample, error) {
	if userID <= 0 || limit <= 0 {
		return []*domainrecommendation.BehaviorSample{}, nil
	}
	if model == "" {
		model = domainembedding.HashNgramModel
	}
	rows, err := r.db.WithContext(ctx).
		Table("video_view_events AS ev").
		Select("ve.embedding_json, v.title, v.description, ev.event_type, ev.watch_ms, ev.completed").
		Joins("JOIN video AS v ON v.id = ev.video_id AND v.status = ?", domainvideo.StatusPublished).
		Joins("LEFT JOIN video_embedding AS ve ON ve.video_id = ev.video_id AND ve.model = ?", model).
		Where(
			"ev.user_id = ? AND ev.created_at >= ? AND (ev.event_type IN ? OR (ev.completed = ? AND ev.event_type <> ?))",
			userID,
			since,
			domainrecommendation.PositiveBehaviorEventTypes(),
			true,
			domainrecommendation.BehaviorEventExposed,
		).
		Order("ev.created_at DESC").
		Order("ev.id DESC").
		Limit(limit).
		Rows()

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fallbackVectorizer *domainembedding.HashNgramVectorizer
	samples := make([]*domainrecommendation.BehaviorSample, 0, limit)
	for rows.Next() {
		var embeddingJSON sql.NullString
		var title string
		var description string
		var eventType string
		var watchMs int
		var completed bool
		if err := rows.Scan(&embeddingJSON, &title, &description, &eventType, &watchMs, &completed); err != nil {
			return nil, err
		}

		var vector []float64
		if embeddingJSON.Valid && strings.TrimSpace(embeddingJSON.String) != "" {
			if decoded, decodeErr := decodeVector(embeddingJSON.String); decodeErr == nil {
				vector = decoded
			}
		}
		if len(vector) == 0 && model == domainembedding.HashNgramModel {
			if fallbackVectorizer == nil {
				fallbackVectorizer = domainembedding.NewHashNgramVectorizer()
			}
			vector = fallbackVectorizer.Vectorize(domainembedding.BuildVideoText(title, description))
		}
		if !domainrecommendation.IsValidInterestVector(vector) {
			continue
		}
		samples = append(samples, &domainrecommendation.BehaviorSample{
			EventType: eventType,
			WatchMs:   watchMs,
			Completed: completed,
			Embedding: vector,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return samples, nil
}

// SaveUserInterestProfile 使用 user_id + model upsert，重复或乱序行为事件覆盖最新兴趣向量且保持 last_event_id 单调递增。
func (r *Repository) SaveUserInterestProfile(ctx context.Context, profile *domainrecommendation.UserInterestProfile) error {
	if profile == nil {
		return nil
	}
	model := UserInterestProfileModel{
		UserID:        profile.UserID,
		Model:         profile.Model,
		Dimension:     profile.Dimension,
		EmbeddingJSON: profile.EmbeddingJSON,
		SampleCount:   profile.SampleCount,
		LastEventID:   profile.LastEventID,
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"},
			{Name: "model"},
		},
		DoUpdates: clause.Assignments(map[string]any{
			"dimension":      gorm.Expr("VALUES(dimension)"),
			"embedding_json": gorm.Expr("VALUES(embedding_json)"),
			"sample_count":   gorm.Expr("VALUES(sample_count)"),
			"last_event_id":  gorm.Expr("GREATEST(last_event_id, VALUES(last_event_id))"),
			"updated_at":     gorm.Expr("VALUES(updated_at)"),
		}),
	}).Create(&model).Error
}

// ListRecentExposures 查询指定时间窗口内的曝光聚合记录。
func (r *Repository) ListRecentExposures(ctx context.Context, userID int64, videoIDs []int64, since time.Time) ([]*domainrecommendation.Exposure, error) {
	if len(videoIDs) == 0 {
		return []*domainrecommendation.Exposure{}, nil
	}

	var models []infraexposure.ExposureModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND video_id IN ? AND last_exposed_at >= ?", userID, videoIDs, since).
		Find(&models).
		Error
	if err != nil {
		return nil, err
	}
	exposures := make([]*domainrecommendation.Exposure, 0, len(models))
	for _, model := range models {
		exposures = append(exposures, restoreExposure(model))
	}
	return exposures, nil
}

// SaveExposures 在同一事务内写入观看行为流水并聚合更新 exposures 表。
func (r *Repository) SaveExposures(ctx context.Context, writes []*domainrecommendation.ExposureWrite) ([]*domainrecommendation.Exposure, error) {
	if len(writes) == 0 {
		return []*domainrecommendation.Exposure{}, nil
	}

	exposures := make([]*domainrecommendation.Exposure, 0, len(writes))
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, write := range writes {
			if write == nil {
				continue
			}
			if err := ensurePublishedVideo(tx, write.VideoID); err != nil {
				return err
			}
			event := infraexposure.ViewEventModel{
				UserID:    write.UserID,
				VideoID:   write.VideoID,
				Scene:     write.Scene,
				RequestID: stringPtr(write.RequestID),
				EventType: domainexposure.EventTypeExposed,
				WatchMs:   0,
				Completed: false,
			}
			if err := tx.Create(&event).Error; err != nil {
				return err
			}
			model := infraexposure.ExposureModel{
				UserID:         write.UserID,
				VideoID:        write.VideoID,
				FirstExposedAt: event.CreatedAt,
				LastExposedAt:  event.CreatedAt,
				ExposureCount:  1,
				LastScene:      write.Scene,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "user_id"},
					{Name: "video_id"},
				},
				DoUpdates: clause.Assignments(map[string]any{
					"last_exposed_at": gorm.Expr("VALUES(last_exposed_at)"),
					"exposure_count":  gorm.Expr("exposure_count + 1"),
					"last_scene":      gorm.Expr("VALUES(last_scene)"),
					"updated_at":      gorm.Expr("VALUES(updated_at)"),
				}),
			}).Create(&model).Error; err != nil {
				return err
			}

			var saved infraexposure.ExposureModel
			if err := tx.Where("user_id = ? AND video_id = ?", write.UserID, write.VideoID).Take(&saved).Error; err != nil {
				return err
			}
			exposures = append(exposures, restoreExposure(saved))
		}
		return nil
	})
	if err != nil {
		return nil, mapRecommendationError(err)
	}
	return exposures, nil
}

func ensurePublishedVideo(tx *gorm.DB, videoID int64) error {
	var item struct {
		ID int64
	}
	err := tx.Table("video").
		Select("id").
		Where("id = ? AND status = ?", videoID, domainvideo.StatusPublished).
		Take(&item).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domainrecommendation.ErrVideoNotFound
		}
		return err
	}
	return nil
}

func restoreExposure(model infraexposure.ExposureModel) *domainrecommendation.Exposure {
	return domainrecommendation.RestoreExposure(
		model.ID,
		model.UserID,
		model.VideoID,
		model.FirstExposedAt,
		model.LastExposedAt,
		model.ExposureCount,
		model.LastScene,
	)
}

func mapRecommendationError(err error) error {
	if errors.Is(err, domainrecommendation.ErrVideoNotFound) {
		return err
	}
	return err
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
