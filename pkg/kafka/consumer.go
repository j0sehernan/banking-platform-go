package kafka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"
)

// HandlerFunc procesa un mensaje. Si devuelve error, el wrapper hará retry.
// Si tras N intentos sigue fallando, el mensaje va a la DLQ y se commitea
// el offset (para no bloquear la partición eternamente).
type HandlerFunc func(ctx context.Context, msg Message) error

// Message es la versión "limpia" del kafka.Message: solo lo que el handler
// necesita. Evita acoplar al handler con la API de kafka-go.
type Message struct {
	Topic     string
	Partition int
	Offset    int64
	Key       string
	Value     []byte
	Headers   map[string]string
	Time      time.Time
}

// ConsumerConfig agrupa lo configurable del consumer.
type ConsumerConfig struct {
	Brokers     []string
	GroupID     string
	Topic       string
	MaxRetries  int           // cuántas veces reintentar antes de DLQ (default 3)
	BackoffBase time.Duration // backoff inicial (default 1s, exponencial)
}

// Consumer es un wrapper sobre kafka.Reader con retries + DLQ + idempotencia delegada.
type Consumer struct {
	reader  *kafka.Reader
	dlq     *Producer
	cfg     ConsumerConfig
	logger  *slog.Logger
}

// NewConsumer crea un consumer para un topic con un consumer group.
// El dlq es opcional pero recomendado: si está nil, los mensajes que fallan
// definitivamente solo se loguean (no es lo ideal en prod).
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
		CommitInterval: 0,        // commit manual tras procesar
		StartOffset:    kafka.LastOffset,
	})

	return &Consumer{
		reader: r,
		dlq:    dlq,
		cfg:    cfg,
		logger: logger,
	}
}

// Run consume mensajes hasta que el contexto se cancele.
// El loop es: leer → procesar (con retries) → commit. Si tras retries falla,
// publica a la DLQ y commitea para avanzar.
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
		// FetchMessage es blocking pero respeta el contexto
		raw, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			c.logger.Error("kafka fetch error",
				"err", err,
				"topic", c.cfg.Topic,
			)
			// pequeño backoff antes de reintentar la lectura
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
				continue
			}
		}

		msg := toMessage(raw)
		if err := c.processWithRetry(ctx, msg, handle); err != nil {
			// si processWithRetry devuelve error, el contexto fue cancelado
			return nil
		}

		// commit del offset (avanzamos en la partición)
		if err := c.reader.CommitMessages(ctx, raw); err != nil {
			c.logger.Error("kafka commit error", "err", err)
		}
	}
}

// processWithRetry intenta hasta MaxRetries veces con backoff exponencial.
// Si todos los intentos fallan, manda a DLQ y devuelve nil (para que el caller
// commitee el offset y siga). Solo devuelve error si el contexto se canceló.
func (c *Consumer) processWithRetry(ctx context.Context, msg Message, handle HandlerFunc) error {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			// backoff exponencial: 1s, 2s, 4s, 8s...
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
		// éxito
		return nil
	}

	// Tras todos los retries, DLQ
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

// Close libera recursos del reader.
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
