package repo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/j0sehernan/banking-platform-go/services/llm-ms/internal/domain"
	"github.com/jackc/pgx/v5"
)

// ExplanationRepo cachea las explicaciones generadas por el LLM
// para no llamar a Claude cada vez que el front pide una explicación.
type ExplanationRepo struct {
	db DBTX
}

func NewExplanationRepo(db DBTX) *ExplanationRepo {
	return &ExplanationRepo{db: db}
}

// Upsert guarda o actualiza la explicación de una transacción.
func (r *ExplanationRepo) Upsert(ctx context.Context, txID uuid.UUID, text, model string) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO transaction_explanations (tx_id, explanation, model, generated_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (tx_id) DO UPDATE SET
		   explanation = EXCLUDED.explanation,
		   model = EXCLUDED.model,
		   generated_at = EXCLUDED.generated_at`,
		txID, text, model, time.Now().UTC(),
	)
	return err
}

// GetByTxID lee la explicación de una transacción.
func (r *ExplanationRepo) GetByTxID(ctx context.Context, txID uuid.UUID) (*domain.Explanation, error) {
	var e domain.Explanation
	err := r.db.QueryRow(ctx,
		`SELECT tx_id, explanation, model, generated_at
		 FROM transaction_explanations WHERE tx_id = $1`,
		txID,
	).Scan(&e.TxID, &e.Text, &e.Model, &e.GeneratedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrExplanationNotFound
		}
		return nil, err
	}
	return &e, nil
}
