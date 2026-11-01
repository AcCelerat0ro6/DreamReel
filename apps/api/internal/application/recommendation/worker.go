package applicationrecommendation

import (
	applicationexposure "DreamReel/internal/application/exposure"
	"DreamReel/internal/infra/metrics"
	"context"
	"time"
)

// ViewEventConsumer 消费观看行为已落库事件。
type ViewEventConsumer interface {
	ConsumeViewEventRecorded(ctx context.Context, handler func(context.Context, *applicationexposure.ViewEventRecordedEvent) error) error
}

// ProfileWorker 消费观看行为事件并异步更新用户推荐画像。
type ProfileWorker struct {
	service  *Service
	consumer ViewEventConsumer
}

// NewProfileWorker 创建推荐画像更新 Worker。
func NewProfileWorker(service *Service, consumer ViewEventConsumer) *ProfileWorker {
	return &ProfileWorker{
		service:  service,
		consumer: consumer,
	}
}

// Start 启动观看行为事件消费。
func (w *ProfileWorker) Start(ctx context.Context) error {
	if w == nil || w.consumer == nil {
		return nil
	}
	return w.consumer.ConsumeViewEventRecorded(ctx, w.HandleViewEventRecorded)
}

// HandleViewEventRecorded 根据观看行为事件刷新用户推荐兴趣向量。
func (w *ProfileWorker) HandleViewEventRecorded(ctx context.Context, event *applicationexposure.ViewEventRecordedEvent) (err error) {
	start := time.Now()
	defer func() {
		metrics.ObserveWorkerJob("recommendation_profile", time.Since(start), err)
	}()
	if w == nil || w.service == nil || event == nil {
		return nil
	}
	_, err = w.service.RefreshUserInterestProfile(ctx, RefreshProfileInput{
		UserID:      event.UserID,
		VideoID:     event.VideoID,
		EventType:   event.EventType,
		WatchMs:     event.WatchMs,
		Completed:   event.Completed,
		ViewEventID: event.ViewEventID,
	})
	return err
}
