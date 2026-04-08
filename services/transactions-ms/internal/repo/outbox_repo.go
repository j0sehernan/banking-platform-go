package repo

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/j0sehernan/banking-platform-go/pkg/outbox"
)

// OutboxRepo is identical to the one in accounts-ms but lives in its
// own service (each ms has its own DB → its own outbox table).
// I leave it duplicated on purpose instead of extracting to pkg because
// it's ~50 lines and sharing it would add coupling between services.
type OutboxRepo struct {
	db DBTX
}

func NewOutboxRepo(db DBTX) *OutboxRepo {
	return &OutboxRepo{db: db}
}

func (r *OutboxRepo) Insert(ctx context.Context, topic, key string, payload []byte, headers map[string]string) error {
	hdrJSON, _ := json.Marshal(headers)
	_, err := r.db.Exec(ctx,
		`INSERT INTO outbox (id, topic, msg_key, payload, headers)
		 VALUES ($1, $2, $3, $4, $5)`,
		uuid.New(), topic, key, payload, hdrJSON,
	)
	return err
}

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
		_ = json.Unmarshal(headersJSON, &headers)
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
