// Package kafka envuelve segmentio/kafka-go con un producer y un consumer
// con retries + DLQ. La idea es que la lógica de los microservicios
// no tenga que conocer detalles del cliente Kafka.
package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"
)

// Producer es un wrapper liviano sobre kafka.Writer.
// kafka-go ya hace pooling y batching internamente, no necesitamos más.
type Producer struct {
	writer *kafka.Writer
	logger *slog.Logger
}

// NewProducer crea un producer apuntando a los brokers dados.
// Topic se setea por mensaje, no en el writer (más flexible).
func NewProducer(brokers []string, logger *slog.Logger) *Producer {
	w := &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Balancer:               &kafka.Hash{}, // por key (importante para orden por cuenta/tx)
		RequiredAcks:           kafka.RequireAll,
		AllowAutoTopicCreation: false, // los topics los crea kafka-init
		BatchTimeout:           10 * time.Millisecond,
	}
	return &Producer{writer: w, logger: logger}
}

// Publish manda un mensaje a un topic con un key específico.
// El key define la partición → mensajes con el mismo key van a la misma
// partición → orden garantizado por key.
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

// Close cierra el writer (libera conexiones).
func (p *Producer) Close() error {
	return p.writer.Close()
}
