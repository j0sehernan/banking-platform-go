package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/j0sehernan/banking-platform-go/services/transactions-ms/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const pgUniqueViolation = "23505"

type TransactionRepo struct {
	db DBTX
}

func NewTransactionRepo(db DBTX) *TransactionRepo {
	return &TransactionRepo{db: db}
}

// Create inserts a new transaction. If the idempotency_key already
// exists, returns ErrDuplicateIdempotencyKey and the caller can fetch
// the existing one with GetByIdempotencyKey.
func (r *TransactionRepo) Create(ctx context.Context, t domain.Transaction) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO transactions
		 (id, type, from_account_id, to_account_id, amount, currency, status, idempotency_key, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		t.ID, t.Type, t.FromAccountID, t.ToAccountID, t.Amount, t.Currency,
		t.Status, t.IdempotencyKey, t.CreatedAt, t.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return domain.ErrDuplicateIdempotencyKey
		}
		return err
	}
	return nil
}

// GetByID reads a transaction by id.
func (r *TransactionRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Transaction, error) {
	return r.querySingle(ctx,
		`SELECT id, type, from_account_id, to_account_id, amount, currency, status,
		        idempotency_key, COALESCE(rejection_code, ''), COALESCE(rejection_msg, ''),
		        created_at, updated_at
		 FROM transactions WHERE id = $1`,
		id,
	)
}

// GetByIdempotencyKey looks up a transaction by its idempotency_key.
func (r *TransactionRepo) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Transaction, error) {
	return r.querySingle(ctx,
		`SELECT id, type, from_account_id, to_account_id, amount, currency, status,
		        idempotency_key, COALESCE(rejection_code, ''), COALESCE(rejection_msg, ''),
		        created_at, updated_at
		 FROM transactions WHERE idempotency_key = $1`,
		key,
	)
}

func (r *TransactionRepo) querySingle(ctx context.Context, sql string, args ...any) (*domain.Transaction, error) {
	var t domain.Transaction
	err := r.db.QueryRow(ctx, sql, args...).Scan(
		&t.ID, &t.Type, &t.FromAccountID, &t.ToAccountID, &t.Amount, &t.Currency,
		&t.Status, &t.IdempotencyKey, &t.RejectionCode, &t.RejectionMsg,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTransactionNotFound
		}
		return nil, err
	}
	return &t, nil
}

// MarkCompleted updates the state to COMPLETED.
func (r *TransactionRepo) MarkCompleted(ctx context.Context, id uuid.UUID) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE transactions
		 SET status = 'COMPLETED', updated_at = now()
		 WHERE id = $1 AND status = 'PENDING'`,
		id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// either it was not in PENDING or does not exist; treat both equally
		return nil
	}
	return nil
}

// MarkRejected updates the state to REJECTED with a reason.
func (r *TransactionRepo) MarkRejected(ctx context.Context, id uuid.UUID, code, msg string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE transactions
		 SET status = 'REJECTED', rejection_code = $2, rejection_msg = $3, updated_at = now()
		 WHERE id = $1 AND status = 'PENDING'`,
		id, code, msg,
	)
	return err
}

// ListAll returns all transactions (the latest 100).
func (r *TransactionRepo) ListAll(ctx context.Context) ([]domain.Transaction, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, type, from_account_id, to_account_id, amount, currency, status,
		        idempotency_key, COALESCE(rejection_code, ''), COALESCE(rejection_msg, ''),
		        created_at, updated_at
		 FROM transactions ORDER BY created_at DESC LIMIT 100`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []domain.Transaction
	for rows.Next() {
		var t domain.Transaction
		if err := rows.Scan(
			&t.ID, &t.Type, &t.FromAccountID, &t.ToAccountID, &t.Amount, &t.Currency,
			&t.Status, &t.IdempotencyKey, &t.RejectionCode, &t.RejectionMsg,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		txs = append(txs, t)
	}
	return txs, rows.Err()
}

// ListByAccount returns transactions where the account is source or destination.
func (r *TransactionRepo) ListByAccount(ctx context.Context, accountID uuid.UUID) ([]domain.Transaction, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, type, from_account_id, to_account_id, amount, currency, status,
		        idempotency_key, COALESCE(rejection_code, ''), COALESCE(rejection_msg, ''),
		        created_at, updated_at
		 FROM transactions
		 WHERE from_account_id = $1 OR to_account_id = $1
		 ORDER BY created_at DESC LIMIT 100`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []domain.Transaction
	for rows.Next() {
		var t domain.Transaction
		if err := rows.Scan(
			&t.ID, &t.Type, &t.FromAccountID, &t.ToAccountID, &t.Amount, &t.Currency,
			&t.Status, &t.IdempotencyKey, &t.RejectionCode, &t.RejectionMsg,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		txs = append(txs, t)
	}
	return txs, rows.Err()
}
