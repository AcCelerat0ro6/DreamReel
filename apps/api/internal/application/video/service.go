package applicationvideo

import (
	domainvideo "DreamReel/internal/domain/video"
	"context"
	"errors"
	"strings"

	"go.uber.org/zap"
)

var ErrLoadVideoFailed = errors.New("failed to load video")
var ErrSaveVideoFailed = errors.New("failed to save video")
var ErrUpdateVideoFailed = errors.New("failed to update video")

type Service struct {
	repo      domainvideo.Repository
	publisher PublishedEventPublisher
}

type Option func(*Service)

func New(repo domainvideo.Repository, options ...Option) *Service {
	service := &Service{repo: repo}
	for _, option := range options {
		option(service)
	}
	return service
}

type CreateResult struct {
	Video   *domainvideo.Video
	Created bool
}

// CreatePublished 创建已发布视频；Idempotency-Key 命中时返回已有视频。
func (s *Service) CreatePublished(ctx context.Context, authorID int64, title string, description string, mediaURL string, coverURL string, modelName, modelParams, aiStyleTag *string, originVideoID *int64, idempotencyKey string) (*CreateResult, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) > domainvideo.MaxIdempotencyKeyLength {
		return nil, domainvideo.ErrIdempotencyKeyTooLong
	}
	if idempotencyKey != "" {
		// 客户端重试同一次创建请求时，先通过作者和幂等键找回原视频。
		// 若找到原视频则不需创建，这直接返回，若没有则继续创建。
		existing, err := s.repo.FindByAuthorAndIdempotencyKey(ctx, authorID, idempotencyKey)
		if err == nil {
			return &CreateResult{Video: existing, Created: false}, nil
		}
		if !errors.Is(err, domainvideo.ErrVideoNotFound) {
			zap.L().Error("load video by idempotency key failed",
				zap.Int64("author_id", authorID),
				zap.String("idempotency_key", idempotencyKey),
				zap.Error(err),
			)
			return nil, ErrLoadVideoFailed
		}
	}
	video, err := domainvideo.NewPublished(
		authorID,
		title,
		description,
		mediaURL,
		coverURL,
		modelName,
		modelParams,
		aiStyleTag,
		originVideoID,
		idempotencyKey,
	)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Save(ctx, video); err != nil {
		// 防止并发创建造成的错误
		if idempotencyKey != "" && errors.Is(err, domainvideo.ErrDuplicateIdempotencyKey) {
			existing, loadErr := s.repo.FindByAuthorAndIdempotencyKey(ctx, authorID, idempotencyKey)
			if loadErr == nil {
				return &CreateResult{Video: existing, Created: false}, nil
			}
			zap.L().Error("reload video after duplicate idempotency key failed",
				zap.Int64("author_id", authorID),
				zap.String("idempotency_key", idempotencyKey),
				zap.Error(loadErr),
			)
			return nil, ErrLoadVideoFailed
		}
		zap.L().Error("save video failed",
			zap.Int64("author_id", authorID),
			zap.String("idempotency_key", idempotencyKey),
			zap.Error(err),
		)
		return nil, ErrSaveVideoFailed
	}

	// s.publishCreatedVideo(ctx, video)

	return &CreateResult{Video: video, Created: true}, nil

}

func (s *Service) publishCreatedVideo(ctx context.Context, video *domainvideo.Video) {
	if s.publisher == nil {
		return
	}
	event := NewPublishEvent(video)
	if event == nil {
		return
	}
	_ = s.publisher.PublishVideoPublished(ctx, event)
}

// Delete 删除当前用户本人的视频
func (s *Service) Delete(ctx context.Context, authorID, videoID int64) error {
	if authorID <= 0 {
		return domainvideo.ErrInvalidAuthorID
	}
	if videoID <= 0 {
		return domainvideo.ErrInvalidVideoID
	}

	video, err := s.repo.FindByIDAnyStatus(ctx, videoID)
	if err != nil {
		if errors.Is(err, domainvideo.ErrVideoNotFound) {
			return domainvideo.ErrVideoNotFound
		}
		zap.L().Error("load video for delete failed",
			zap.Int64("video_id", videoID),
			zap.Int64("author_id", authorID),
			zap.Error(err),
		)
		return ErrLoadVideoFailed
	}
	var alreadyDeleted bool = false
	if video.Status == domainvideo.StatusDeleted {
		alreadyDeleted = true
	}
	// video.DeleteBy校验权限，同时执行状态更新
	if err := video.DeleteBy(authorID); err != nil {
		return err
	}
	if alreadyDeleted {
		return nil
	}

	if err := s.repo.UpdateStatus(ctx, video); err != nil {
		if errors.Is(err, domainvideo.ErrVideoNotFound) {
			return domainvideo.ErrVideoNotFound
		}
		zap.L().Error("update video status failed",
			zap.Int64("video_id", videoID),
			zap.Int64("author_id", authorID),
			zap.Int("target_status", video.Status),
			zap.Error(err),
		)
		return ErrUpdateVideoFailed
	}
	return nil

}

// Get 根据ID查询公开视频
func (s *Service) Get(ctx context.Context, videoID int64) (*domainvideo.Video, error) {
	if videoID <= 0 {
		return nil, domainvideo.ErrInvalidVideoID
	}

	video, err := s.repo.FindByID(ctx, videoID)
	if err != nil {
		if errors.Is(err, domainvideo.ErrVideoNotFound) {
			return nil, domainvideo.ErrVideoNotFound
		}
		zap.L().Error("load video failed",
			zap.Int64("video_id", videoID),
			zap.Error(err),
		)
		return nil, ErrLoadVideoFailed
	}

	return video, nil
}
