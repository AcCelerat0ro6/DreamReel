package domainrelation

import (
	"strings"
	"time"
)

const (
	FollowStatusActive   = 1
	FollowStatusCanceled = 2

	MaxIdempotencyKeyLength = 128
	MaxLimit                = 100
)

// Follow 表示一个用户对另一个用户的关注关系，取关使用软状态保留历史。
type Follow struct {
	ID             int64
	UserID         int64
	TargetUserID   int64
	Status         int
	IdempotencyKey string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Active 判断当前关系是否处于关注中。
func (f *Follow) Active() bool {
	return f.Status == FollowStatusActive
}

// RelationStat 保存用户维度的关注数和粉丝数。
type RelationStat struct {
	UserID         int64
	FollowingCount int
	FollowerCount  int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// UserProfile 是关系通知里展示触发用户所需的资料。
type UserProfile struct {
	UserID    int64
	Nickname  string
	AvatarURL string
	Bio       string
}

// UserItem 是关注列表或粉丝列表中的用户展示数据。
type UserItem struct {
	UserID     int64
	Nickname   string
	AvatarURL  string
	Bio        string
	FollowedAt time.Time
}

// ListCursor 保存关注者/粉丝分页游标
type ListCursor struct {
	FollowedAt time.Time
	UserID     int64
}

// RestoreFollow 从数据库记录恢复关注关系。
func RestoreFollow(id int64, userID int64, targetUserID int64, status int, idempotencyKey string, createdAt time.Time, updatedAt time.Time) *Follow {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if status == 0 {
		status = FollowStatusActive
	}

	return &Follow{
		ID:             id,
		UserID:         userID,
		TargetUserID:   targetUserID,
		Status:         status,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}
}

// RestoreRelationStat 从数据库统计记录恢复领域统计对象。
func RestoreRelationStat(userID int64, followingCount int, followerCount int, createdAt time.Time, updatedAt time.Time) *RelationStat {
	return &RelationStat{
		UserID:         userID,
		FollowingCount: clampCount(followingCount),
		FollowerCount:  clampCount(followerCount),
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}
}
