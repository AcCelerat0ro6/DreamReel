package applicationexposure

import (
	domainexposure "DreamReel/internal/domain/exposure"
	"DreamReel/internal/infra/metrics"
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
)

var ErrSaveExposureFailed = errors.New("failed to save exposure")

type Service struct {
	repo      domainexposure.Repository
	publisher ViewEventPublisher
}

// ViewEventPublisher 投递观看行为事件，推荐画像 worker 基于该事件更新用户向量。
type ViewEventPublisher interface {
	PublishViewEventRecorded(ctx context.Context, event *ViewEventRecordedEvent) error
}

// WithViewEventPublisher 为曝光服务启用观看行为事件发布。
func WithViewEventPublisher(publisher ViewEventPublisher) Option {
	return func(s *Service) {
		s.publisher = publisher
	}
}

type Option func(*Service)

func New(repo domainexposure.Repository, options ...Option) *Service {
	service := &Service{repo: repo}
	for _, option := range options {
		option(service)
	}
	return service
}

// RecordViewEvent 写入观看行为，并在 exposed 事件时同步维护曝光聚合索引
func (s *Service) RecordViewEvent(ctx context.Context, userID int64, videoID int64, scene string, requestID string, eventType string, watchMs int, completed bool) (*RecordViewEventResult, error) {
	event, err := domainexposure.NewViewEvent(userID, videoID, scene, requestID, eventType, watchMs, completed)
	if err != nil {
		return nil, err
	}

	savedEvent, exposure, err := s.repo.SaveViewEvent(ctx, event)
	if err != nil {
		if errors.Is(err, domainexposure.ErrVideoNotFound) {
			return nil, err
		}
		return nil, ErrSaveExposureFailed
	}

	s.publishViewEventRecorded(ctx, savedEvent, exposure)
	return &RecordViewEventResult{
		Event:    savedEvent,
		Exposure: exposure,
	}, nil

}

// publishViewEventRecorded 投递观看行为已落库事件，并记录发布指标与错误日志。
func (s *Service) publishViewEventRecorded(ctx context.Context, event *domainexposure.ViewEvent, exposure *domainexposure.Exposure) {
	if s.publisher == nil {
		return
	}
	message := NewViewEventRecordedEvent(event, exposure)
	if message == nil {
		return
	}
	start := time.Now()
	err := s.publisher.PublishViewEventRecorded(ctx, message)
	metrics.ObserveMQPublish("view_event_recorded", time.Since(start), err)
	if err != nil {
		zap.L().Error("publish view event recorded failed",
			zap.String("event_id", message.EventID),
			zap.Int64("view_event_id", message.ViewEventID),
			zap.Int64("user_id", message.UserID),
			zap.Int64("video_id", message.VideoID),
			zap.String("event_type", message.EventType),
			zap.Error(err),
		)
	}
}
