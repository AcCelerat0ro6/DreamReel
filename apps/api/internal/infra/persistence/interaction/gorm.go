package infrainteraction

import (
	domainaccount "DreamReel/internal/domain/account"
	domaininteraction "DreamReel/internal/domain/interaction"
	domainvideo "DreamReel/internal/domain/video"
	infravideo "DreamReel/internal/infra/persistence/video"
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// GetVideoStat 读取公开视频当前互动计数。
func (r *Repository) GetVideoStat(ctx context.Context, videoID int64) (*domaininteraction.VideoStat, error) {
	var stat infravideo.VideoStatModel
	err := r.db.WithContext(ctx).
		Table("video_stat AS vs").
		Select("vs.video_id, vs.like_count, vs.comment_count, vs.favorite_count, vs.created_at, vs.updated_at").
		Joins("JOIN video AS v ON v.id = vs.video_id").
		Where("vs.video_id = ? AND v.status = ?", videoID, domainvideo.StatusPublished).
		Take(&stat).
		Error
	if err != nil {
		return nil, mapVideoError(err)
	}
	return &domaininteraction.VideoStat{
		VideoID:       stat.VideoID,
		LikeCount:     stat.LikeCount,
		CommentCount:  stat.CommentCount,
		FavoriteCount: stat.FavoriteCount,
	}, nil
}

// mapVideoError 转换领域错误
func mapVideoError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domaininteraction.ErrVideoNotFound
	}
	return err
}

// GetVideoAuthorID 读取公开视频作者 ID，用于点赞消息通知。
func (r *Repository) GetVideoAuthorID(ctx context.Context, videoID int64) (int64, error) {
	var video infravideo.VideoModel
	err := r.db.WithContext(ctx).
		Where("id = ? AND status = ?", videoID, domainvideo.StatusPublished).
		Take(&video).
		Error
	if err != nil {
		return 0, mapVideoError(err)
	}
	return video.AuthorID, nil
}

// GetUserProfile 读取用户展示资料，用于互动消息展示触发者。
func (r *Repository) GetUserProfile(ctx context.Context, userID int64) (*domaininteraction.UserProfile, error) {
	var model userProfileModel
	err := r.db.WithContext(ctx).
		Table("account").
		Select("id, nickname, avatar_url").
		Where("id = ? AND status = ?", userID, domainaccount.StatusNormal).
		Take(&model).
		Error
	if err != nil {
		return nil, mapUserError(err)
	}
	return &domaininteraction.UserProfile{
		ID:        model.ID,
		Nickname:  strings.TrimSpace(model.Nickname),
		AvatarURL: strings.TrimSpace(model.AvatarURL),
	}, nil
}

// mapUserError 把 GORM 找不到记录转换为互动领域的用户 ID 错误。
func mapUserError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domaininteraction.ErrInvalidUserID
	}
	return err
}

// SetAction 写入点赞或收藏状态，并在同一事务内返回更新后的完整视频统计
func (r *Repository) SetAction(ctx context.Context, userID int64, videoID int64, actionType string, active bool, idempotencyKey string) (*domaininteraction.Action, *domaininteraction.VideoStat, int, error) {
	actionType, err := domaininteraction.NormalizeActionType(actionType)
	if err != nil {
		return nil, nil, 0, err
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	var action ActionModel
	var stat *domaininteraction.VideoStat
	var statDelta int

	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 锁定视频信息记录
		if err := lockPublishedVideo(tx, videoID); err != nil {
			return err
		}

		// 锁定用户在该视频的互动记录
		findErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND video_id = ? AND action_type = ?", userID, videoID, actionType).
			Take(&action).
			Error
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}

		delta := 0
		nextStatus := actionStatusFromActive(active)
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			action = ActionModel{
				UserID:         userID,
				VideoID:        videoID,
				ActionType:     actionType,
				Status:         nextStatus,
				IdempotencyKey: idempotencyKeyPtr(idempotencyKey),
			}
			if err := tx.Create(&action).Error; err != nil {
				return err
			}
			if active {
				delta = 1
			}
		} else {
			// 同一幂等键直接返回当前统计快照，避免客户端重试重复变更计数。
			if idempotencyKey != "" && idempotencyKeyValue(action.IdempotencyKey) == idempotencyKey {
				// 注意 这里需要对视频统计进行加锁，避免并发读写数据不一致。
				currentStat, err := currentVideoStat(tx, videoID)
				if err != nil {
					return err
				}
				stat = currentStat
				return nil
			}

			// 只有状态真正变化时才更新 video_stat
			previousStatus := action.Status
			previousIdempotencyKey := idempotencyKeyValue(action.IdempotencyKey)
			if action.Status != nextStatus {
				if active {
					delta = 1
				} else {
					delta = -1
				}
			}
			action.Status = nextStatus
			action.IdempotencyKey = idempotencyKeyPtr(idempotencyKey)
			if previousStatus != nextStatus || previousIdempotencyKey != idempotencyKey {
				if err := tx.Save(&action).Error; err != nil {
					return err
				}
			}
		}

		if delta == 0 {
			currentStat, err := currentVideoStat(tx, videoID)
			if err != nil {
				return err
			}
			stat = currentStat
			return nil
		}

		updatedStat, err := updateActionStat(tx, videoID, actionType, delta)
		if err != nil {
			return err
		}
		stat = updatedStat
		statDelta = delta
		return nil
	})
	if err != nil {
		return nil, nil, 0, mapVideoError(err)
	}
	return restoreAction(action), stat, statDelta, nil
}

// currentVideoStat 事务内锁定并读取视频完整互动统计，用于幂等命中和零变更返回。
func currentVideoStat(tx *gorm.DB, videoID int64) (*domaininteraction.VideoStat, error) {
	var stat infravideo.VideoStatModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("video_id = ?", videoID).Take(&stat).Error; err != nil {
		return nil, mapVideoError(err)
	}
	return toDomainVideoStat(videoID, stat), nil
}

// toDomainVideoStat 把持久层统计模型转换为互动领域的统计快照。
func toDomainVideoStat(videoID int64, stat infravideo.VideoStatModel) *domaininteraction.VideoStat {
	return &domaininteraction.VideoStat{
		VideoID:       videoID,
		LikeCount:     stat.LikeCount,
		CommentCount:  stat.CommentCount,
		FavoriteCount: stat.FavoriteCount,
	}
}

// restoreAction 把数据库互动行为转换为领域对象。
func restoreAction(model ActionModel) *domaininteraction.Action {
	return domaininteraction.RestoreAction(
		model.ID,
		model.UserID,
		model.VideoID,
		model.ActionType,
		model.Status,
		idempotencyKeyValue(model.IdempotencyKey),
		model.CreatedAt,
		model.UpdatedAt,
	)
}

// updateActionStat 根据行为类型更新点赞或收藏计数，并返回更新后的完整互动统计。
func updateActionStat(tx *gorm.DB, videoID int64, actionType string, delta int) (*domaininteraction.VideoStat, error) {
	var stat infravideo.VideoStatModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("video_id = ?", videoID).Take(&stat).Error; err != nil {
		return nil, mapVideoError(err)
	}
	if actionType == domaininteraction.ActionTypeLike {
		stat.LikeCount = clampCount(stat.LikeCount + delta)
	} else {
		stat.FavoriteCount = clampCount(stat.FavoriteCount + delta)
	}
	if err := tx.Save(&stat).Error; err != nil {
		return nil, err
	}
	return toDomainVideoStat(videoID, stat), nil
}
