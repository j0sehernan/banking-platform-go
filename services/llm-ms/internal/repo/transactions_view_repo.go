package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/j0sehernan/banking-platform-go/services/llm-ms/internal/domain"
	"github.com/jackc/pgx/v5"
)

// TransactionsViewRepo maneja la copia local de transacciones (read model).
type TransactionsViewRepo struct {
	db DBTX
}

func NewTransactionsViewRepo(db DBTX) *TransactionsViewRepo {
	return &TransactionsViewRepo{db: db}
}

// Upsert inserta o actualiza una transacción en la vista.
// Lo llama el consumer cada vez que llega un evento de transactions-ms.
func (r *TransactionsViewRepo) Upsert(ctx context.Context, t domain.TransactionView) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO transactions_view
		 (id, type, status, from_account_id, to_account_id, amount, currency,
		  rejection_code, rejection_msg, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT (id) DO UPDATE SET
		   status = EXCLUDED.status,
		   rejection_code = EXCLUDED.rejection_code,
		   rejection_msg = EXCLUDED.rejection_msg,
		   updated_at = EXCLUDED.updated_at`,
		t.ID, t.Type, t.Status, t.FromAccountID, t.ToAccountID, t.Amount, t.Currency,
		nullIfEmpty(t.RejectionCode), nullIfEmpty(t.RejectionMsg),
		t.CreatedAt, t.UpdatedAt,
	)
	return err
}

// GetByID lee una transacción por id.
func (r *TransactionsViewRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.TransactionView, error) {
	var t domain.TransactionView
	var rejCode, rejMsg *string
	err := r.db.QueryRow(ctx,
		`SELECT id, type, status, from_account_id, to_account_id, amount, currency,
		        rejection_code, rejection_msg, created_at, updated_at
		 FROM transactions_view WHERE id = $1`,
		id,
	).Scan(&t.ID, &t.Type, &t.Status, &t.FromAccountID, &t.ToAccountID, &t.Amount, &t.Currency,
		&rejCode, &rejMsg, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTransactionNotFound
		}
		return nil, err
	}
	if rejCode != nil {
		t.RejectionCode = *rejCode
	}
	if rejMsg != nil {
		t.RejectionMsg = *rejMsg
	}
	return &t, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
