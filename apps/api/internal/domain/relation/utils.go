package domainrelation

import (
	"strings"
	"time"
)

// NewFollow 创建关注关系，负责基础 ID 和幂等键校验。
func NewFollow(userID int64, targetUserID int64, idempotencyKey string) (*Follow, error) {
	if userID <= 0 {
		return nil, ErrInvalidUserID
	}
	if targetUserID <= 0 {
		return nil, ErrInvalidTargetUserID
	}
	if userID == targetUserID {
		return nil, ErrFollowSelfForbidden
	}

	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) > MaxIdempotencyKeyLength {
		return nil, ErrIdempotencyKeyTooLong
	}
	return &Follow{
		UserID:         userID,
		TargetUserID:   targetUserID,
		Status:         FollowStatusActive,
		IdempotencyKey: idempotencyKey,
	}, nil
}

func clampCount(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

// RestoreUserProfile 从账号记录恢复通知展示资料。
func RestoreUserProfile(userID int64, nickname string, avatarURL string, bio string) *UserProfile {
	return &UserProfile{
		UserID:    userID,
		Nickname:  strings.TrimSpace(nickname),
		AvatarURL: strings.TrimSpace(avatarURL),
		Bio:       strings.TrimSpace(bio),
	}
}

func RestoreUserItem(userID int64, nickname string, avatarURL string, bio string, followedAt time.Time) *UserItem {
	return &UserItem{
		UserID:     userID,
		Nickname:   strings.TrimSpace(nickname),
		AvatarURL:  strings.TrimSpace(avatarURL),
		Bio:        strings.TrimSpace(bio),
		FollowedAt: followedAt,
	}
}
