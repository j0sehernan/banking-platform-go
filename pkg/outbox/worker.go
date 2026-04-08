// Package outbox implementa el patrón outbox: un worker que lee filas
// pendientes de una tabla `outbox` y las publica a Kafka.
//
// Garantiza atomicidad DB+evento sin 2PC: la lógica de negocio escribe
// el cambio de estado y la fila del outbox en la misma transacción
// Postgres. Si la transacción commitea, ambas cosas existen. Si falla,
// nada existe.
package outbox

import (
	"context"
	"log/slog"
	"time"
)

// Row representa una fila pendiente del outbox.
type Row struct {
	ID      string
	Topic   string
	Key     string
	Payload []byte
	Headers map[string]string
}

// Repository es la interface que cada microservicio debe implementar
// con su propio acceso a Postgres. Mantiene a este package agnóstico
// de pgx (lo importan los servicios).
type Repository interface {
	// FetchPending lee como máximo `limit` filas del outbox que aún no
	// fueron publicadas. Debe usar `FOR UPDATE SKIP LOCKED` en Postgres
	// para que múltiples workers puedan correr en paralelo sin pisarse.
	FetchPending(ctx context.Context, limit int) ([]Row, error)

	// MarkPublished marca filas como publicadas exitosamente.
	MarkPublished(ctx context.Context, ids []string) error
}

// Publisher es lo que el worker usa para enviar a Kafka.
// Solo necesita publicar, no consumir.
type Publisher interface {
	Publish(ctx context.Context, topic, key string, payload []byte, headers map[string]string) error
}

// Worker corre en background, lee del outbox, publica, marca como enviado.
type Worker struct {
	repo      Repository
	publisher Publisher
	interval  time.Duration
	batchSize int
	logger    *slog.Logger
}

// NewWorker crea un worker con la frecuencia y batch dados.
// Valores razonables: interval=500ms, batchSize=100.
func NewWorker(repo Repository, pub Publisher, interval time.Duration, batchSize int, logger *slog.Logger) *Worker {
	return &Worker{
		repo:      repo,
		publisher: pub,
		interval:  interval,
		batchSize: batchSize,
		logger:    logger,
	}
}

// Run corre el loop hasta que el contexto se cancele.
// Idempotente: si crash en el medio, en el próximo arranque las filas
// no marcadas como publicadas se vuelven a procesar (puede haber duplicados
// en Kafka, pero el consumer es idempotente vía processed_events).
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
			// no marcamos como publicado → reintenta en el próximo tick
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
