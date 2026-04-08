package repo

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/j0sehernan/banking-platform-go/pkg/outbox"
)

// OutboxRepo implements pkg/outbox.Repository for Postgres.
// The idea is that the generic worker in pkg/outbox doesn't know about pgx.
type OutboxRepo struct {
	db DBTX
}

func NewOutboxRepo(db DBTX) *OutboxRepo {
	return &OutboxRepo{db: db}
}

// Insert adds a row to the outbox inside a transaction.
// Called by services when changing state, in the same tx.
func (r *OutboxRepo) Insert(ctx context.Context, topic, key string, payload []byte, headers map[string]string) error {
	hdrJSON, _ := json.Marshal(headers)
	_, err := r.db.Exec(ctx,
		`INSERT INTO outbox (id, topic, msg_key, payload, headers)
		 VALUES ($1, $2, $3, $4, $5)`,
		uuid.New(), topic, key, payload, hdrJSON,
	)
	return err
}

// FetchPending reads pending outbox rows using FOR UPDATE SKIP LOCKED.
// SKIP LOCKED is the secret sauce to allow multiple workers (or service
// replicas) to run in parallel without stepping on each other: each one
// grabs different rows.
func (r *OutboxRepo) FetchPending(ctx context.Context, limit int) ([]outbox.Row, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, topic, msg_key, payload, COALESCE(headers, '{}'::jsonb)
		 FROM outbox
		 WHERE published_at IS NULL
		 ORDER BY created_at
		 LIMIT $1
		 FOR UPDATE SKIP LOCKED`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []outbox.Row
	for rows.Next() {
		var (
			id          uuid.UUID
			topic       string
			key         string
			payload     []byte
			headersJSON []byte
		)
		if err := rows.Scan(&id, &topic, &key, &payload, &headersJSON); err != nil {
			return nil, err
		}
		var headers map[string]string
		if err := json.Unmarshal(headersJSON, &headers); err != nil {
			headers = nil
		}
		result = append(result, outbox.Row{
			ID:      id.String(),
			Topic:   topic,
			Key:     key,
			Payload: payload,
			Headers: headers,
		})
	}
	return result, rows.Err()
}

// MarkPublished marks rows as published after sending them to Kafka.
func (r *OutboxRepo) MarkPublished(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.db.Exec(ctx,
		`UPDATE outbox SET published_at = now() WHERE id = ANY($1::uuid[])`,
		ids,
	)
	return err
}
