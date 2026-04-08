package kafka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"
)

// HandlerFunc processes one message. If it returns an error, the wrapper
// will retry. If after N attempts it keeps failing, the message is sent
// to the DLQ and the offset is committed (so the partition is not blocked
// forever).
type HandlerFunc func(ctx context.Context, msg Message) error

// Message is the "clean" version of kafka.Message: only what the handler
// needs. Avoids coupling handlers to the kafka-go API.
type Message struct {
	Topic     string
	Partition int
	Offset    int64
	Key       string
	Value     []byte
	Headers   map[string]string
	Time      time.Time
}

// ConsumerConfig groups everything configurable on the consumer.
type ConsumerConfig struct {
	Brokers     []string
	GroupID     string
	Topic       string
	MaxRetries  int           // attempts before going to the DLQ (default 3)
	BackoffBase time.Duration // initial backoff (default 1s, exponential)
}

// Consumer wraps kafka.Reader with retries + DLQ + delegated idempotency.
type Consumer struct {
	reader *kafka.Reader
	dlq    *Producer
	cfg    ConsumerConfig
	logger *slog.Logger
}

// NewConsumer creates a consumer for a topic with a consumer group.
// dlq is optional but recommended: if nil, messages that fail terminally
// are only logged (not ideal in production).
func NewConsumer(cfg ConsumerConfig, dlq *Producer, logger *slog.Logger) *Consumer {
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}
	if cfg.BackoffBase == 0 {
		cfg.BackoffBase = time.Second
	}

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        cfg.Brokers,
		GroupID:        cfg.GroupID,
		Topic:          cfg.Topic,
		MinBytes:       1,
		MaxBytes:       10 << 20, // 10MB
		CommitInterval: 0,        // manual commit after processing
		StartOffset:    kafka.LastOffset,
	})

	return &Consumer{
		reader: r,
		dlq:    dlq,
		cfg:    cfg,
		logger: logger,
	}
}

// Run consumes messages until the context is cancelled.
// Loop: read → process (with retries) → commit. If after retries it still
// fails, publish to the DLQ and commit to advance.
func (c *Consumer) Run(ctx context.Context, handle HandlerFunc) error {
	c.logger.Info("kafka consumer started",
		"group", c.cfg.GroupID,
		"topic", c.cfg.Topic,
	)
	defer c.logger.Info("kafka consumer stopped",
		"group", c.cfg.GroupID,
		"topic", c.cfg.Topic,
	)

	for {
		// FetchMessage is blocking but respects the context
		raw, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			c.logger.Error("kafka fetch error",
				"err", err,
				"topic", c.cfg.Topic,
			)
			// small backoff before retrying the read
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
				continue
			}
		}

		msg := toMessage(raw)
		if err := c.processWithRetry(ctx, msg, handle); err != nil {
			// if processWithRetry returns an error, the context was cancelled
			return nil
		}

		// commit the offset (advance in the partition)
		if err := c.reader.CommitMessages(ctx, raw); err != nil {
			c.logger.Error("kafka commit error", "err", err)
		}
	}
}

// processWithRetry attempts up to MaxRetries times with exponential backoff.
// If all attempts fail, sends to the DLQ and returns nil (so the caller
// can commit the offset and continue). Only returns an error when the
// context is cancelled.
func (c *Consumer) processWithRetry(ctx context.Context, msg Message, handle HandlerFunc) error {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			// exponential backoff: 1s, 2s, 4s, 8s...
			backoff := c.cfg.BackoffBase * (1 << (attempt - 1))
			c.logger.Warn("kafka handler failed, retrying",
				"attempt", attempt,
				"max_retries", c.cfg.MaxRetries,
				"backoff", backoff,
				"err", lastErr,
			)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		if err := handle(ctx, msg); err != nil {
			lastErr = err
			continue
		}
		// success
		return nil
	}

	// After all retries, send to DLQ
	c.sendToDLQ(ctx, msg, lastErr)
	return nil
}

func (c *Consumer) sendToDLQ(ctx context.Context, msg Message, lastErr error) {
	if c.dlq == nil {
		c.logger.Error("message dropped (no DLQ configured)",
			"topic", msg.Topic,
			"key", msg.Key,
			"err", lastErr,
		)
		return
	}

	headers := map[string]string{
		"x-original-topic": msg.Topic,
		"x-error":          fmt.Sprintf("%v", lastErr),
		"x-retry-count":    fmt.Sprintf("%d", c.cfg.MaxRetries),
	}
	for k, v := range msg.Headers {
		headers[k] = v
	}

	if err := c.dlq.Publish(ctx, "dlq", msg.Key, msg.Value, headers); err != nil {
		c.logger.Error("failed to publish to DLQ",
			"err", err,
			"original_topic", msg.Topic,
		)
		return
	}
	c.logger.Warn("message moved to DLQ",
		"original_topic", msg.Topic,
		"key", msg.Key,
		"reason", lastErr,
	)
}

// Close releases reader resources.
func (c *Consumer) Close() error {
	return c.reader.Close()
}

func toMessage(m kafka.Message) Message {
	headers := make(map[string]string, len(m.Headers))
	for _, h := range m.Headers {
		headers[h.Key] = string(h.Value)
	}
	return Message{
		Topic:     m.Topic,
		Partition: m.Partition,
		Offset:    m.Offset,
		Key:       string(m.Key),
		Value:     m.Value,
		Headers:   headers,
		Time:      m.Time,
	}
}
