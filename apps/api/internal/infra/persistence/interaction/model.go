package infrainteraction

import "time"

// ActionModel 映射 interaction_action 表，记录用户对视频的点赞/收藏状态。
type ActionModel struct {
	ID int64 `gorm:"column:id;primaryKey;autoIncrement"`
	// user_id + video_id + action_type 唯一，保证同一用户对同一视频只有一条同类行为记录。
	UserID     int64  `gorm:"column:user_id;not null;uniqueIndex:uk_user_video_type,priority:1;index:idx_user_type_status,priority:1"`
	VideoID    int64  `gorm:"column:video_id;not null;uniqueIndex:uk_user_video_type,priority:2;index:idx_video_type_status,priority:1"`
	ActionType string `gorm:"column:action_type;size:16;not null;uniqueIndex:uk_user_video_type,priority:3;index:idx_video_type_status,priority:2;index:idx_user_type_status,priority:2"`
	Status     int    `gorm:"column:status;type:tinyint;not null;default:1;index:idx_video_type_status,priority:3;index:idx_user_type_status,priority:3"`
	// IdempotencyKey 保存最近一次请求键，用于重复请求返回稳定结果。
	IdempotencyKey *string   `gorm:"column:idempotency_key;size:128"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName 指定互动行为表名。
func (ActionModel) TableName() string {
	return "interaction_action"
}
