package mq

import (
	applicationinteraction "DreamReel/internal/application/interaction"
	"DreamReel/internal/infra/config"
	"DreamReel/internal/infra/metrics"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	defaultInteractionExchange      = "gcfeed.interaction"
	defaultActionChangedQueue       = "gcfeed.interaction.action_changed"
	defaultActionChangedRouting     = "interaction.action_changed"
	defaultVideoExchange            = "gcfeed.video"
	defaultVideoPublishedQueue      = "gcfeed.video.published"
	defaultVideoEmbeddingQueue      = "gcfeed.video.embedding"
	defaultVideoPublishedRouting    = "video.published"
	defaultExposureExchange         = "gcfeed.exposure"
	defaultViewEventRecordedQueue   = "gcfeed.exposure.view_event_recorded"
	defaultViewEventRecordedRouting = "exposure.view_event_recorded"
)

var ErrEmptyRabbitMQURL = errors.New("RabbitMQ URL is empty")

type RabbitMQ struct {
	conn            *amqp.Connection
	publishChannel  *amqp.Channel
	consumerChannel *amqp.Channel
	config          config.RabbitMQConfig
}

func NewRabbitMQ(cfg config.RabbitMQConfig) (*RabbitMQ, error) {
	cfg = normalizeRabbitMQConfig(cfg)
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, ErrEmptyRabbitMQURL
	}

	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, err
	}
	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	consumerChannel, err := conn.Channel()
	if err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, err
	}

	client := &RabbitMQ{
		conn:            conn,
		publishChannel:  channel,
		consumerChannel: consumerChannel,
		config:          cfg,
	}
	if err := client.ensureTopology(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func (r *RabbitMQ) Close() error {
	if r == nil {
		return nil
	}
	if r.publishChannel != nil {
		_ = r.publishChannel.Close()
	}
	if r.consumerChannel != nil {
		_ = r.consumerChannel.Close()
	}
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil

}

func (r *RabbitMQ) PublishActionChanged(ctx context.Context, event *applicationinteraction.ActionChangedEvent) error {
	if event == nil {
		return nil
	}
	content, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return r.publishChannel.PublishWithContext(
		ctx,
		r.config.InteractionExchange,
		r.config.ActionChangedRouting,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         content,
			DeliveryMode: amqp.Persistent,
			MessageId:    event.EventID,
			Timestamp:    time.Now(),
		},
	)
}

func (r *RabbitMQ) ConsumeActionChanged(ctx context.Context, handler func(context.Context, *applicationinteraction.ActionChangedEvent) error) error {
	deliveries, err := r.consumerChannel.ConsumeWithContext(
		ctx,
		r.config.ActionChangedQueue,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	go func() {
		for delivery := range deliveries {
			start := time.Now()
			var event applicationinteraction.ActionChangedEvent
			if err := json.Unmarshal(delivery.Body, &event); err != nil {
				metrics.ObserveWorkerJob("mq_action_changed_decode", time.Since(start), err)
				_ = delivery.Nack(false, false)
				continue
			}
			if err := handler(ctx, &event); err != nil {
				metrics.ObserveWorkerJob("mq_action_changed_consume", time.Since(start), err)
				_ = delivery.Nack(false, true)
				continue
			}
			metrics.ObserveWorkerJob("mq_action_changed_consume", time.Since(start), nil)
			_ = delivery.Ack(false)
		}
	}()
	return nil
}

func (r *RabbitMQ) ensureTopology() error {
	if err := r.publishChannel.ExchangeDeclare(
		r.config.InteractionExchange,
		"topic",
		true,  // durable
		false, // auto-delete
		false, // internal
		false, // no-wait
		nil,
	); err != nil {
		return err
	}
	if _, err := r.publishChannel.QueueDeclare(
		r.config.ActionChangedQueue,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,
	); err != nil {
		return err
	}
	if err := r.publishChannel.QueueBind(
		r.config.ActionChangedQueue,
		r.config.ActionChangedRouting,
		r.config.InteractionExchange,
		false,
		nil,
	); err != nil {
		return err
	}
	if err := r.publishChannel.ExchangeDeclare(
		r.config.VideoExchange,
		"topic",
		true,  // durable
		false, // auto-delete
		false, // internal
		false, // no-wait
		nil,
	); err != nil {
		return err
	}
	if _, err := r.publishChannel.QueueDeclare(
		r.config.VideoPublishedQueue,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,
	); err != nil {
		return err
	}
	if err := r.publishChannel.QueueBind(
		r.config.VideoPublishedQueue,
		r.config.VideoPublishedRouting,
		r.config.VideoExchange,
		false,
		nil,
	); err != nil {
		return err
	}
	if _, err := r.publishChannel.QueueDeclare(
		r.config.VideoEmbeddingQueue,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,
	); err != nil {
		return err
	}
	if err := r.publishChannel.QueueBind(
		r.config.VideoEmbeddingQueue,
		r.config.VideoPublishedRouting,
		r.config.VideoExchange,
		false,
		nil,
	); err != nil {
		return err
	}
	if err := r.publishChannel.ExchangeDeclare(
		r.config.ExposureExchange,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return err
	}
	if _, err := r.publishChannel.QueueDeclare(
		r.config.ViewEventRecordedQueue,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return err
	}
	if err := r.publishChannel.QueueBind(
		r.config.ViewEventRecordedQueue,
		r.config.ViewEventRecordedRouting,
		r.config.ExposureExchange,
		false,
		nil,
	); err != nil {
		return err
	}
	return nil
}

func normalizeRabbitMQConfig(cfg config.RabbitMQConfig) config.RabbitMQConfig {
	// URL
	cfg.URL = strings.TrimSpace(cfg.URL)

	// Interaction
	cfg.InteractionExchange = strings.TrimSpace(cfg.InteractionExchange)
	cfg.ActionChangedQueue = strings.TrimSpace(cfg.ActionChangedQueue)
	cfg.ActionChangedRouting = strings.TrimSpace(cfg.ActionChangedRouting)
	if cfg.InteractionExchange == "" {
		cfg.InteractionExchange = defaultInteractionExchange
	}
	if cfg.ActionChangedQueue == "" {
		cfg.ActionChangedQueue = defaultActionChangedQueue
	}
	if cfg.ActionChangedRouting == "" {
		cfg.ActionChangedRouting = defaultActionChangedRouting
	}

	// Video
	cfg.VideoExchange = strings.TrimSpace(cfg.VideoExchange)
	cfg.VideoPublishedQueue = strings.TrimSpace(cfg.VideoPublishedQueue)
	cfg.VideoEmbeddingQueue = strings.TrimSpace(cfg.VideoEmbeddingQueue)
	cfg.VideoPublishedRouting = strings.TrimSpace(cfg.VideoPublishedRouting)
	if cfg.VideoExchange == "" {
		cfg.VideoExchange = defaultVideoExchange
	}
	if cfg.VideoPublishedQueue == "" {
		cfg.VideoPublishedQueue = defaultVideoPublishedQueue
	}
	if cfg.VideoEmbeddingQueue == "" {
		cfg.VideoEmbeddingQueue = defaultVideoEmbeddingQueue
	}
	if cfg.VideoPublishedRouting == "" {
		cfg.VideoPublishedRouting = defaultVideoPublishedRouting
	}

	// Exposure
	cfg.ExposureExchange = strings.TrimSpace(cfg.ExposureExchange)
	cfg.ViewEventRecordedQueue = strings.TrimSpace(cfg.ViewEventRecordedQueue)
	cfg.ViewEventRecordedRouting = strings.TrimSpace(cfg.ViewEventRecordedRouting)
	if cfg.ExposureExchange == "" {
		cfg.ExposureExchange = defaultExposureExchange
	}
	if cfg.ViewEventRecordedQueue == "" {
		cfg.ViewEventRecordedQueue = defaultViewEventRecordedQueue
	}
	if cfg.ViewEventRecordedRouting == "" {
		cfg.ViewEventRecordedRouting = defaultViewEventRecordedRouting
	}
	return cfg
}
