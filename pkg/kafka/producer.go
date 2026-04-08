// Package kafka wraps segmentio/kafka-go with a producer and a consumer
// supporting retries + DLQ. The idea is to keep microservice business
// logic decoupled from Kafka client details.
package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"
)

// Producer is a thin wrapper over kafka.Writer.
// kafka-go already does pooling and batching internally, no extra needed.
type Producer struct {
	writer *kafka.Writer
	logger *slog.Logger
}

// NewProducer creates a producer pointing at the given brokers.
// Topic is set per message, not on the writer (more flexible).
func NewProducer(brokers []string, logger *slog.Logger) *Producer {
	w := &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Balancer:               &kafka.Hash{}, // by key (important for ordering per account/tx)
		RequiredAcks:           kafka.RequireAll,
		AllowAutoTopicCreation: false, // topics are created by kafka-init
		BatchTimeout:           10 * time.Millisecond,
	}
	return &Producer{writer: w, logger: logger}
}

// Publish sends a message to a topic with the given key.
// The key determines the partition → messages with the same key land
// in the same partition → ordering guaranteed per key.
func (p *Producer) Publish(ctx context.Context, topic, key string, payload []byte, headers map[string]string) error {
	hdrs := make([]kafka.Header, 0, len(headers))
	for k, v := range headers {
		hdrs = append(hdrs, kafka.Header{Key: k, Value: []byte(v)})
	}

	msg := kafka.Message{
		Topic:   topic,
		Key:     []byte(key),
		Value:   payload,
		Headers: hdrs,
		Time:    time.Now(),
	}

	if err := p.writer.WriteMessages(ctx, msg); err != nil {
		return fmt.Errorf("kafka publish to %s: %w", topic, err)
	}

	p.logger.Debug("kafka message published",
		"topic", topic,
		"key", key,
		"size", len(payload),
	)
	return nil
}

// Close closes the writer (releases connections).
func (p *Producer) Close() error {
	return p.writer.Close()
}
