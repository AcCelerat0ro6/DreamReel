package infravideo

import (
	domainvideo "DreamReel/internal/domain/video"
	"time"
)

type videoWithStatModel struct {
	ID             int64
	AuthorID       int64
	Title          string
	Description    string
	MediaURL       string
	CoverURL       string
	Status         int
	LikeCount      int
	CommentCount   int
	FavoriteCount  int
	PublishedAt    *time.Time
	IdempotencyKey *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	// AIGC 元数据
	ModelName     *string
	ModelParams   *string
	AIStyleTag    *string
	OriginVideoID *int64
}

func restorevideo(model videoWithStatModel) *domainvideo.Video {
	return &domainvideo.Video{
		ID:             model.ID,
		AuthorID:       model.AuthorID,
		Title:          model.Title,
		Description:    model.Description,
		MediaURL:       model.MediaURL,
		CoverURL:       model.CoverURL,
		Status:         model.Status,
		LikeCount:      model.LikeCount,
		CommentCount:   model.CommentCount,
		FavoriteCount:  model.FavoriteCount,
		PublishedAt:    model.PublishedAt,
		IdempotencyKey: model.IdempotencyKey,
		CreatedAt:      model.CreatedAt,
		UpdatedAt:      model.UpdatedAt,
		// AIGC 元数据
		ModelName:     model.ModelName,
		ModelParams:   model.ModelParams,
		AIStyleTag:    model.AIStyleTag,
		OriginVideoID: model.OriginVideoID,
	}
}

// VideoModel 映射 video 表
type VideoModel struct {
	ID          int64      `gorm:"column:id;primaryKey;autoIncrement"`
	AuthorID    int64      `gorm:"column:author_id;not null;index:idx_author_status,priority:1;uniqueIndex:uk_author_idempotency,priority:1"`
	Title       string     `gorm:"column:title;size:128;not null"`
	Description string     `gorm:"column:description;size:512"`
	MediaURL    string     `gorm:"column:media_url;size:512;not null"`
	CoverURL    string     `gorm:"column:cover_url;size:512;not null"`
	Status      int        `gorm:"column:status;type:tinyint;not null;default:2;index:idx_author_status,priority:2;index:idx_status_published,priority:1"`
	PublishedAt *time.Time `gorm:"column:published_at;index:idx_status_published,priority:2"`
	// IdempotencyKey 与 AuthorID 组成唯一索引，用于发布接口的安全重试。
	IdempotencyKey *string   `gorm:"column:idempotency_key;size:128;uniqueIndex:uk_author_idempotency,priority:2"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime;index:idx_author_status,priority:3"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime"`
	// AIGC 元数据字段
	ModelName     *string `gorm:"column:model_name;size:64;index:idx_model_name"`
	ModelParams   *string `gorm:"column:model_params;size:1024"`
	AIStyleTag    *string `gorm:"column:ai_style_tag;size:32"`
	OriginVideoID *int64  `gorm:"column:origin_video_id"`
}

func (VideoModel) TableName() string {
	return "video"
}

// VideoStatModel 映射 video_stat 表，保存可频繁变更的互动计数, 例如 点赞 评论，收藏。
type VideoStatModel struct {
	VideoID       int64     `gorm:"column:video_id;primaryKey"`
	LikeCount     int       `gorm:"column:like_count;not null;default:0"`
	CommentCount  int       `gorm:"column:comment_count;not null;default:0"`
	FavoriteCount int       `gorm:"column:favorite_count;not null;default:0"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (VideoStatModel) TableName() string {
	return "video_stat"
}

func videoWithStatSelect() string {
	return "v.id, v.author_id, v.title, v.description, v.media_url, v.cover_url, v.status, COALESCE(vs.like_count, 0) AS like_count, COALESCE(vs.comment_count, 0) AS comment_count, COALESCE(vs.favorite_count, 0) AS favorite_count, v.published_at, v.idempotency_key, v.created_at, v.updated_at, v.model_name, v.model_params, v.ai_style_tag, v.origin_video_id"
}
