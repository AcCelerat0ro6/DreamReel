package applicationinteraction

import (
	domaininteraction "DreamReel/internal/domain/interaction"
	"DreamReel/internal/infra/metrics"
	"context"
	"time"
)

type ActionEventConsumer interface {
	ConsumeActionChanged(ctx context.Context, handler func(context.Context, *ActionChangedEvent) error) error
}

type ActionWorker struct {
	repo     domaininteraction.Repository
	consumer ActionEventConsumer
}

func NewActionWorker(repo domaininteraction.Repository, consumer ActionEventConsumer) *ActionWorker {
	return &ActionWorker{
		repo:     repo,
		consumer: consumer,
	}
}

func (w *ActionWorker) Start(ctx context.Context) error {
	if w == nil || w.consumer == nil {
		return nil
	}
	return w.consumer.ConsumeActionChanged(ctx, w.HandleActionChanged)
}

func (w *ActionWorker) HandleActionChanged(ctx context.Context, event *ActionChangedEvent) error {
	start := time.Now()
	var err error
	defer func() {
		metrics.ObserveWorkerJob("interaction_action_changed", time.Since(start), err)
	}()

	if event == nil {
		return nil
	}
	_, _, _, err = w.repo.SetAction(ctx, event.UserID, event.VideoID, event.ActionType, event.Active, event.IdempotencyKey)
	return err
}
