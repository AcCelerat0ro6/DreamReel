package infrarelation

import (
	domainaccount "DreamReel/internal/domain/account"
	domainrelation "DreamReel/internal/domain/relation"
	infraaccount "DreamReel/internal/infra/persistence/account"
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const hotScoreExpression = "COALESCE(vs.like_count, 0) * 3 + COALESCE(vs.comment_count, 0) * 5 + COALESCE(vs.favorite_count, 0) * 4"

type Repository struct {
	db *gorm.DB
}

// New 创建 Feed 仓储实现。
func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func orderedUserPair(a, b int64) (int64, int64) {
	if a <= b {
		return a, b
	}
	return b, a
}

func lockNormalUser(tx *gorm.DB, userID int64, role string) error {
	var user infraaccount.UserModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND status = ?", userID, domainaccount.StatusNormal).
		Take(&user).
		Error
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		zap.L().Warn("relation user not found or not normal",
			zap.String("role", role),
			zap.Int64("user_id", userID),
		)
		return fmt.Errorf("%s user %d not found or not normal: %w", role, userID, domainrelation.ErrTargetUserNotFound)
	}
	zap.L().Error("lock relation user failed",
		zap.String("role", role),
		zap.Int64("user_id", userID),
		zap.Error(err),
	)
	return err
}

func lockUserPair(tx *gorm.DB, actorID int64, targetID int64) error {
	first, second := orderedUserPair(actorID, targetID)
	firstRole, secondRole := "actor", "target"
	if first == targetID {
		firstRole, secondRole = "target", "actor"
	}
	if err := lockNormalUser(tx, first, firstRole); err != nil {
		return err
	}
	if first == second {
		return nil
	}
	return lockNormalUser(tx, second, secondRole)
}

func ensureStat(tx *gorm.DB, userID int64) error {
	stat := RelationStatModel{UserID: userID}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&stat).Error; err != nil {
		zap.L().Error("ensure relation stat failed",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return err
	}
	return nil
}

func ensureStatPair(tx *gorm.DB, actorID int64, targetID int64) error {
	first, second := orderedUserPair(actorID, targetID)
	if err := ensureStat(tx, first); err != nil {
		return err
	}
	if first == second {
		return nil
	}
	return ensureStat(tx, second)
}

func statusFromActive(active bool) int {
	if active {
		return domainrelation.FollowStatusActive
	}
	return domainrelation.FollowStatusCanceled
}

func idempotencyKeyPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func idempotencyKeyValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func currentStat(tx *gorm.DB, userID int64) (RelationStatModel, error) {
	var stat RelationStatModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ?", userID).
		Take(&stat).
		Error
	if err != nil {
		zap.L().Error("lock relation stat failed",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return RelationStatModel{}, err
	}
	return stat, nil
}

func clampCount(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func updateStat(tx *gorm.DB, userID int64, followingDelta int, followerDelta int) (RelationStatModel, error) {
	// 获取原有 StatModel，同时加锁
	stat, err := currentStat(tx, userID)
	if err != nil {
		return RelationStatModel{}, err
	}
	// 更改粉丝和关注数量
	stat.FollowingCount = clampCount(stat.FollowingCount + followingDelta)
	stat.FollowerCount = clampCount(stat.FollowerCount + followerDelta)
	if err := tx.Save(&stat).Error; err != nil {
		zap.L().Error("save relation stat failed",
			zap.Int64("user_id", userID),
			zap.Int("following_delta", followingDelta),
			zap.Int("follower_delta", followerDelta),
			zap.Error(err),
		)
		return RelationStatModel{}, err
	}
	return stat, nil
}

func updateStatPair(tx *gorm.DB, actorID int64, targetID int64, followingDelta int, followerDelta int) (RelationStatModel, RelationStatModel, error) {
	first, second := orderedUserPair(actorID, targetID)
	firstIsActor := first == actorID

	var firstFollowing, firstFollower, secondFollowing, secondFollower int
	if firstIsActor {
		firstFollowing = followingDelta
		secondFollower = followerDelta
	} else {
		firstFollower = followerDelta
		secondFollowing = followingDelta
	}

	firstStat, err := updateStat(tx, first, firstFollowing, firstFollower)
	if err != nil {
		return RelationStatModel{}, RelationStatModel{}, err
	}
	secondStat := firstStat
	if first != second {
		secondStat, err = updateStat(tx, second, secondFollowing, secondFollower)
		if err != nil {
			return RelationStatModel{}, RelationStatModel{}, err
		}
	}

	if firstIsActor {
		return firstStat, secondStat, nil
	}
	return secondStat, firstStat, nil
}

func readStatPair(tx *gorm.DB, actorID int64, targetID int64) (RelationStatModel, RelationStatModel, error) {
	first, second := orderedUserPair(actorID, targetID)
	firstStat, err := currentStat(tx, first)
	if err != nil {
		return RelationStatModel{}, RelationStatModel{}, err
	}
	secondStat := firstStat
	if first != second {
		secondStat, err = currentStat(tx, second)
		if err != nil {
			return RelationStatModel{}, RelationStatModel{}, err
		}
	}
	if first == actorID {
		return firstStat, secondStat, nil
	}
	return secondStat, firstStat, nil
}

func restoreFollow(model FollowModel) *domainrelation.Follow {
	return domainrelation.RestoreFollow(
		model.ID,
		model.UserID,
		model.TargetUserID,
		model.Status,
		idempotencyKeyValue(model.IdempotencyKey),
		model.CreatedAt,
		model.UpdatedAt,
	)
}

func restoreStat(model RelationStatModel) *domainrelation.RelationStat {
	return domainrelation.RestoreRelationStat(
		model.UserID,
		model.FollowingCount,
		model.FollowerCount,
		model.CreatedAt,
		model.UpdatedAt,
	)
}

// SetFollow 设置关注或取关状态，并在同一事务中维护双方计数。
func (r *Repository) SetFollow(ctx context.Context, userID int64, targetUserID int64, active bool, idempotencyKey string) (*domainrelation.Follow, *domainrelation.RelationStat, *domainrelation.RelationStat, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	var follow FollowModel
	var userStat RelationStatModel
	var targetStat RelationStatModel
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockUserPair(tx, userID, targetUserID); err != nil {
			return err
		}
		if err := ensureStatPair(tx, userID, targetUserID); err != nil {
			return err
		}

		findErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND target_user_id = ?", userID, targetUserID).
			Take(&follow).
			Error
		notFound := errors.Is(findErr, gorm.ErrRecordNotFound)
		if findErr != nil && !notFound {
			zap.L().Error("load follow record failed",
				zap.Int64("user_id", userID),
				zap.Int64("target_user_id", targetUserID),
				zap.Error(findErr),
			)
			return findErr
		}

		nextStatus := statusFromActive(active)
		delta := 0
		if notFound {
			follow = FollowModel{
				UserID:         userID,
				TargetUserID:   targetUserID,
				Status:         nextStatus,
				IdempotencyKey: idempotencyKeyPtr(idempotencyKey),
			}
			if err := tx.Create(&follow).Error; err != nil {
				zap.L().Error("create follow record failed",
					zap.Int64("user_id", userID),
					zap.Int64("target_user_id", targetUserID),
					zap.Bool("active", active),
					zap.Error(err),
				)
				return err
			}
			if active {
				delta = 1
			}
		} else {
			if idempotencyKey != "" && idempotencyKeyValue(follow.IdempotencyKey) == idempotencyKey {
				if follow.Status != nextStatus {
					zap.L().Warn("idempotency key reused across conflicting follow states",
						zap.Int64("user_id", userID),
						zap.Int64("target_user_id", targetUserID),
						zap.String("idempotency_key", idempotencyKey),
						zap.Int("stored_status", follow.Status),
						zap.Int("requested_status", nextStatus),
					)
					return fmt.Errorf("idempotency key %q reused: stored status %d, requested %d: %w",
						idempotencyKey, follow.Status, nextStatus, domainrelation.ErrIdempotencyKeyConflict)
				}
				actorStat, targetStatNow, err := readStatPair(tx, userID, targetUserID)
				if err != nil {
					return err
				}
				userStat, targetStat = actorStat, targetStatNow
				return nil
			}

			previousStatus := follow.Status
			previousIdempotencyKey := idempotencyKeyValue(follow.IdempotencyKey)
			if previousStatus != nextStatus {
				if active {
					delta = 1
				} else {
					delta = -1
				}
			}
			follow.Status = nextStatus
			if idempotencyKey != "" {
				follow.IdempotencyKey = idempotencyKeyPtr(idempotencyKey)
			}
			nextIdempotencyKey := idempotencyKeyValue(follow.IdempotencyKey)
			if previousStatus != nextStatus || previousIdempotencyKey != nextIdempotencyKey {
				if err := tx.Save(&follow).Error; err != nil {
					zap.L().Error("update follow record failed",
						zap.Int64("user_id", userID),
						zap.Int64("target_user_id", targetUserID),
						zap.Int("previous_status", previousStatus),
						zap.Int("next_status", nextStatus),
						zap.Error(err),
					)
					return err
				}
			}
		}

		if delta != 0 {
			actorStat, targetStatNow, err := updateStatPair(tx, userID, targetUserID, delta, delta)
			if err != nil {
				return err
			}
			userStat, targetStat = actorStat, targetStatNow
			return nil
		}

		actorStat, targetStatNow, err := readStatPair(tx, userID, targetUserID)
		if err != nil {
			return err
		}
		userStat, targetStat = actorStat, targetStatNow
		return nil
	})

	if err != nil {
		if errors.Is(err, domainrelation.ErrTargetUserNotFound) || errors.Is(err, domainrelation.ErrIdempotencyKeyConflict) {
			return nil, nil, nil, err
		}
		zap.L().Error("set follow transaction failed",
			zap.Int64("user_id", userID),
			zap.Int64("target_user_id", targetUserID),
			zap.Bool("active", active),
			zap.String("idempotency_key", idempotencyKey),
			zap.Error(err),
		)
		return nil, nil, nil, err
	}

	return restoreFollow(follow), restoreStat(userStat), restoreStat(targetStat), nil
}

// GetUserProfile 读取用户展示资料，用于关注通知。
func (r *Repository) GetUserProfile(ctx context.Context, userID int64) (*domainrelation.UserProfile, error) {
	var model relationUserProfileModel
	err := r.db.WithContext(ctx).
		Table("account").
		Select("id AS user_id, nickname, avatar_url, bio").
		Where("id = ? AND status = ?", userID, domainaccount.StatusNormal).
		Take(&model).
		Error
	if err != nil {
		return nil, mapUserError(err)
	}
	return domainrelation.RestoreUserProfile(model.UserID, model.Nickname, model.AvatarURL, model.Bio), nil
}

func mapUserError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domainrelation.ErrTargetUserNotFound
	}
	return err
}

// ListFollowing 查询当前用户关注的人。
func (r *Repository) ListFollowing(ctx context.Context, userID int64, cursor *domainrelation.ListCursor, limit int) ([]*domainrelation.UserItem, error) {
	var models []relationUserModel
	query := r.db.WithContext(ctx).
		Table("user_follow AS f").
		Select("a.id AS user_id, a.nickname, a.avatar_url, a.bio, f.updated_at AS followed_at").
		Joins("LEFT JOIN account AS a ON a.id = f.target_user_id").
		Where("f.user_id = ? AND f.status = ? AND a.status = ?", userID, domainrelation.FollowStatusActive, domainaccount.StatusNormal)

	if cursor != nil {
		query = query.Where(
			"(f.updated_at < ? OR (f.updated_at = ? AND f.target_user_id < ?))",
			cursor.FollowedAt,
			cursor.FollowedAt,
			cursor.UserID,
		)
	}
	err := query.Order("f.updated_at DESC").
		Order("f.target_user_id DESC").
		Limit(limit).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}
	items := make([]*domainrelation.UserItem, 0, len(models))
	for _, model := range models {
		items = append(items, domainrelation.RestoreUserItem(
			model.UserID,
			model.Nickname,
			model.AvatarURL,
			model.Bio,
			model.FollowedAt,
		))
	}
	return items, nil
}

// ListFollowers 查询关注当前用户的人。
func (r *Repository) ListFollowers(ctx context.Context, userID int64, cursor *domainrelation.ListCursor, limit int) ([]*domainrelation.UserItem, error) {
	var models []relationUserModel
	query := r.db.WithContext(ctx).
		Table("user_follow AS f").
		Select("a.id AS user_id, a.nickname, a.avatar_url, a.bio, f.updated_at AS followed_at").
		Joins("LEFT JOIN account AS a ON a.id = f.user_id").
		Where("f.target_user_id = ? AND f.status = ? AND a.status = ?", userID, domainrelation.FollowStatusActive, domainaccount.StatusNormal)

	if cursor != nil {
		query = query.Where(
			"(f.updated_at < ? OR (f.updated_at = ? AND f.user_id < ?))",
			cursor.FollowedAt,
			cursor.FollowedAt,
			cursor.UserID,
		)
	}

	err := query.Order("f.updated_at DESC").
		Order("f.user_id DESC").
		Limit(limit).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}
	items := make([]*domainrelation.UserItem, 0, len(models))
	for _, model := range models {
		items = append(items, domainrelation.RestoreUserItem(
			model.UserID,
			model.Nickname,
			model.AvatarURL,
			model.Bio,
			model.FollowedAt,
		))
	}
	return items, nil

}
