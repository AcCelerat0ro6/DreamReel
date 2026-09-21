package rabbitmq

import (
	"DreamReel/internal/infra/config"
	"errors"
	"strings"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

var ErrEmptyRabbitMQURL = errors.New("rabbitmq url is empty")

const defaultInteractionExchange = "dreamreel.interaction"
const defaultActionChangedQueue = "dreamreel.interaction.action_changed"
const defaultActionChangedRouting = "interaction.action_changed"
const defaultVideoExchange = "dreamreel.video"
const defaultVideoPublishedQueue = "dreamreel.video.published"
const defaultVideoEmbeddingQueue = "dreamreel.video.embedding"
const defaultVideoPublishedRouting = "dreamreel.published"
const defaultExposureExchange = "dreamreel.exposure"
const defaultViewEventRecordedQueue = "dreamreel.exposure.view_event_recorded"
const defaultViewEventRecordedRouting = "exposure.view_event_recorded"

type RabbitMQ struct {
	conn            *amqp.Connection
	publishChannel  *amqp.Channel
	consumerChannel *amqp.Channel
	config          config.RabbitMQConfig
}

func NewRabbitMQ(cfg config.RabbitMQConfig) (*RabbitMQ, error) {
	normalizeRabbitMQConfig(&cfg)
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, ErrEmptyRabbitMQURL
	}
	// 1. Create a connection to the RabbitMQ server
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
	/*
		if err := client.ensureTopology(); err != nil {
			_ = client.Close()
			return nil, err
		}
	*/
	return client, nil
}

func (r *RabbitMQ) Close() error {
	if r == nil {
		return nil
	}
	if r.publishChannel != nil {
		err := r.publishChannel.Close()
		if err != nil {
			zap.L().Error("failed to close publish channel", zap.Error(err))
		}
	}
	if r.consumerChannel != nil {
		err := r.consumerChannel.Close()
		if err != nil {
			zap.L().Error("failed to close consumer channel", zap.Error(err))
		}
	}
	if r.conn != nil {
		err := r.conn.Close()
		if err != nil {
			zap.L().Error("failed to close connection", zap.Error(err))
		}
	}
	return nil
}

func normalizeRabbitMQConfig(cfg *config.RabbitMQConfig) {
	cfg.URL = strings.TrimSpace(cfg.URL)
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
	if cfg.VideoExchange == "" {
		cfg.VideoExchange = defaultVideoExchange
	}
	if cfg.VideoPublishedQueue == "" {
		cfg.VideoPublishedQueue = defaultVideoPublishedQueue
	}
	cfg.VideoEmbeddingQueue = strings.TrimSpace(cfg.VideoEmbeddingQueue)
	if cfg.VideoEmbeddingQueue == "" {
		cfg.VideoEmbeddingQueue = defaultVideoEmbeddingQueue
	}
	if cfg.VideoPublishedRouting == "" {
		cfg.VideoPublishedRouting = defaultVideoPublishedRouting
	}
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
}
