// Package outbox implements the outbox pattern: a worker that reads
// pending rows from an `outbox` table and publishes them to Kafka.
//
// Guarantees DB+event atomicity without 2PC: business logic writes the
// state change and the outbox row in the same Postgres transaction.
// If the transaction commits, both exist. If it fails, neither exists.
package outbox

import (
	"context"
	"log/slog"
	"time"
)

// Row represents a pending row in the outbox.
type Row struct {
	ID      string
	Topic   string
	Key     string
	Payload []byte
	Headers map[string]string
}

// Repository is the interface each microservice must implement with its
// own Postgres access. Keeps this package independent of pgx (services
// import pgx, this package does not).
type Repository interface {
	// FetchPending reads up to `limit` rows from the outbox that have
	// not been published yet. Should use `FOR UPDATE SKIP LOCKED` in
	// Postgres so multiple workers can run in parallel without stepping
	// on each other.
	FetchPending(ctx context.Context, limit int) ([]Row, error)

	// MarkPublished marks rows as successfully published.
	MarkPublished(ctx context.Context, ids []string) error
}

// Publisher is what the worker uses to send to Kafka.
// Only needs to publish, not consume.
type Publisher interface {
	Publish(ctx context.Context, topic, key string, payload []byte, headers map[string]string) error
}

// Worker runs in background, reads from the outbox, publishes, marks as sent.
type Worker struct {
	repo      Repository
	publisher Publisher
	interval  time.Duration
	batchSize int
	logger    *slog.Logger
}

// NewWorker builds a worker with the given frequency and batch size.
// Reasonable values: interval=500ms, batchSize=100.
func NewWorker(repo Repository, pub Publisher, interval time.Duration, batchSize int, logger *slog.Logger) *Worker {
	return &Worker{
		repo:      repo,
		publisher: pub,
		interval:  interval,
		batchSize: batchSize,
		logger:    logger,
	}
}

// Run runs the loop until the context is cancelled.
// Idempotent: if it crashes mid-flight, on the next start the rows that
// were not marked as published get reprocessed (may produce duplicates
// in Kafka, but the consumer is idempotent via processed_events).
func (w *Worker) Run(ctx context.Context) {
	w.logger.Info("outbox worker started", "interval", w.interval, "batch_size", w.batchSize)
	defer w.logger.Info("outbox worker stopped")

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.processOnce(ctx); err != nil {
				w.logger.Error("outbox process error", "err", err)
			}
		}
	}
}

func (w *Worker) processOnce(ctx context.Context) error {
	rows, err := w.repo.FetchPending(ctx, w.batchSize)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	publishedIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if err := w.publisher.Publish(ctx, row.Topic, row.Key, row.Payload, row.Headers); err != nil {
			w.logger.Error("outbox publish failed",
				"err", err,
				"topic", row.Topic,
				"key", row.Key,
			)
			// not marking as published → retry on next tick
			continue
		}
		publishedIDs = append(publishedIDs, row.ID)
	}

	if len(publishedIDs) > 0 {
		if err := w.repo.MarkPublished(ctx, publishedIDs); err != nil {
			w.logger.Error("outbox mark published failed", "err", err)
			return err
		}
		w.logger.Debug("outbox batch published", "count", len(publishedIDs))
	}

	return nil
}
