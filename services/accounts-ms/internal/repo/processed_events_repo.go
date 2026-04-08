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

// MarkProcessed tries to mark an event_id as processed.
// Returns true if it was inserted (was new), false if it was already
// there (skip).
//
// This is the central piece of the Inbox pattern: it makes consumers
// idempotent even when Kafka redelivers messages.
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
			// already processed
			return false, nil
		}
		return false, err
	}
	return true, nil
}
