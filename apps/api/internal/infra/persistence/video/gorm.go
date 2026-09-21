package infravideo

import (
	domainvideo "DreamReel/internal/domain/video"
	"context"
	"errors"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

// New 创建视频仓储实现。
func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// EnsureStats 确保每条视频视频sql记录都有对应的位于sql中的统计信息记录。
func EnsureStats(db *gorm.DB) error {
	return db.Exec(`
		Insert INTO video_stat(video_id, like_count, comment_count, favorite_count, created_at, updated_at)
		Select v.id, 0, 0, 0, NOW(), NOW()
		From video v
		LEFT JOIN video_stat AS vs ON vs.video_id = v.id
		WHERE vs.video_id IS NULL
	`).Error
}

// FindByAuthorAndIdempotencyKey 根据作者和幂等键查找已创建视频。
func (r *Repository) FindByAuthorAndIdempotencyKey(ctx context.Context, authorID int64, idempotencyKey string) (*domainvideo.Video, error) {
	if idempotencyKey == "" {
		return nil, domainvideo.ErrVideoNotFound
	}

	var model videoWithStatModel
	err := r.db.WithContext(ctx).
		Table("video as v").
		Select(videoWithStatSelect()).
		Joins("LEFT JOIN video_stat AS vs ON vs.video_id = v.id").
		Where("v.author_id = ? AND v.idempotency_key = ?", authorID, idempotencyKey).
		Take(&model).
		Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainvideo.ErrVideoNotFound
		}
		return nil, err
	}
	return restorevideo(model), nil
}

func (r *Repository) Save(ctx context.Context, video *domainvideo.Video) error {
	var model VideoModel
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		model = VideoModel{
			AuthorID:       video.AuthorID,
			Title:          video.Title,
			Description:    video.Description,
			MediaURL:       video.MediaURL,
			CoverURL:       video.CoverURL,
			Status:         video.Status,
			PublishedAt:    video.PublishedAt,
			IdempotencyKey: video.IdempotencyKey,
			ModelName:      video.ModelName,
			ModelParams:    video.ModelParams,
			AIStyleTag:     video.AIStyleTag,
			OriginVideoID:  video.OriginVideoID,
		}
		if err := tx.Create(&model).Error; err != nil {
			if isDuplicateKeyError(err) {
				return domainvideo.ErrDuplicateIdempotencyKey
			}
			return err
		}

		stat := VideoStatModel{
			VideoID:       model.ID,
			LikeCount:     video.LikeCount,
			CommentCount:  video.CommentCount,
			FavoriteCount: video.FavoriteCount,
		}
		if err := tx.Create(&stat).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	// 回填ID, 时间字段
	video.ID = model.ID
	video.CreatedAt = model.CreatedAt
	video.UpdatedAt = model.UpdatedAt
	return nil
}

func isDuplicateKeyError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
