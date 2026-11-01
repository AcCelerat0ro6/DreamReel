package infrarecommendation

import (
	"time"

	"gorm.io/gorm"
)

const (
	hotScoreExpression  = "COALESCE(vs.like_count, 0) * 3 + COALESCE(vs.comment_count, 0) * 5 + COALESCE(vs.favorite_count, 0) * 4"
	positiveEventWindow = 30 * 24 * time.Hour
)

type Repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type candidateModel struct {
	VideoID     int64
	AuthorID    int64
	HotScore    int
	PublishedAt time.Time
}

type videoVectorModel struct {
	VideoID       int64
	EmbeddingJSON string
}

// UserInterestProfileModel 映射 user_interest_profile 表，保存用户推荐兴趣向量。
type UserInterestProfileModel struct {
	UserID        int64     `gorm:"column:user_id;primaryKey;autoIncrement:false;uniqueIndex:uk_user_model,priority:1"`
	Model         string    `gorm:"column:model;size:64;not null;primaryKey;uniqueIndex:uk_user_model,priority:2;index:idx_model_updated,priority:1"`
	Dimension     int       `gorm:"column:dimension;not null"`
	EmbeddingJSON string    `gorm:"column:embedding_json;type:json;not null"`
	SampleCount   int       `gorm:"column:sample_count;not null;default:0"`
	LastEventID   int64     `gorm:"column:last_event_id;not null;default:0"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime;index:idx_model_updated,priority:2"`
}

// TableName 指定用户推荐兴趣向量表名。
func (UserInterestProfileModel) TableName() string {
	return "user_interest_profile"
}
