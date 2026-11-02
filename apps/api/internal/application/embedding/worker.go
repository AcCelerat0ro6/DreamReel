package applicationembedding

import (
	applicationvideo "DreamReel/internal/application/video"
	"DreamReel/internal/infra/metrics"
	"context"
	"time"
)

// PublishedEventConsumer 消费视频发布事件。
type PublishedEventConsumer interface {
	ConsumeVideoPublishedForEmbedding(ctx context.Context, handler func(context.Context, *applicationvideo.PublishedEvent) error) error
}

type VideoEmbeddingWorker struct {
	service  *Service
	consumer PublishedEventConsumer
}

func (w *VideoEmbeddingWorker) Start(ctx context.Context) error {
	if w == nil || w.consumer == nil {
		return nil
	}
	return w.consumer.ConsumeVideoPublishedForEmbedding(ctx, w.HandleVideoPublished)
}

func NewVideoEmbeddingWorker(service *Service, consumer PublishedEventConsumer) *VideoEmbeddingWorker {
	return &VideoEmbeddingWorker{
		service:  service,
		consumer: consumer,
	}
}

func (w *VideoEmbeddingWorker) HandleVideoPublished(ctx context.Context, event *applicationvideo.PublishedEvent) (err error) {
	start := time.Now()
	defer func() {
		metrics.ObserveWorkerJob("video_embedding", time.Since(start), err)
	}()
	if w == nil || w.service == nil || event == nil {
		return nil
	}
	_, err = w.service.GenerateForPublishedVideo(ctx, event)
	return err
}
