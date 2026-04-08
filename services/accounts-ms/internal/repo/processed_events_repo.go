package repo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type ProcessedEventsRepo struct {
	db DBTX
}

func NewProcessedEventsRepo(db DBTX) *ProcessedEventsRepo {
	return &ProcessedEventsRepo{db: db}
}

// MarkProcessed intenta marcar un event_id como procesado.
// Devuelve true si fue insertado (era nuevo), false si ya estaba (skip).
//
// Esta es la pieza central del Inbox pattern: hace que los consumers
// sean idempotentes incluso ante re-deliveries de Kafka.
func (r *ProcessedEventsRepo) MarkProcessed(ctx context.Context, eventID string) (bool, error) {
	var inserted string
	err := r.db.QueryRow(ctx,
		`INSERT INTO processed_events (event_id)
		 VALUES ($1)
		 ON CONFLICT DO NOTHING
		 RETURNING event_id`,
		eventID,
	).Scan(&inserted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// ya estaba procesado
			return false, nil
		}
		return false, err
	}
	return true, nil
}
