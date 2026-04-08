package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/j0sehernan/banking-platform-go/services/accounts-ms/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

type AccountRepo struct {
	db DBTX
}

func NewAccountRepo(db DBTX) *AccountRepo {
	return &AccountRepo{db: db}
}

// Create inserts a new account.
func (r *AccountRepo) Create(ctx context.Context, a domain.Account) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO accounts (id, client_id, currency, balance, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		a.ID, a.ClientID, a.Currency, a.Balance, a.CreatedAt, a.UpdatedAt,
	)
	return err
}

// GetByID reads an account by id.
func (r *AccountRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Account, error) {
	var a domain.Account
	err := r.db.QueryRow(ctx,
		`SELECT id, client_id, currency, balance, created_at, updated_at
		 FROM accounts WHERE id = $1`,
		id,
	).Scan(&a.ID, &a.ClientID, &a.Currency, &a.Balance, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrAccountNotFound
		}
		return nil, err
	}
	return &a, nil
}

// ListByClient returns all accounts for a client.
func (r *AccountRepo) ListByClient(ctx context.Context, clientID uuid.UUID) ([]domain.Account, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, client_id, currency, balance, created_at, updated_at
		 FROM accounts WHERE client_id = $1 ORDER BY created_at DESC`,
		clientID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []domain.Account
	for rows.Next() {
		var a domain.Account
		if err := rows.Scan(&a.ID, &a.ClientID, &a.Currency, &a.Balance, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

// ListAll returns all accounts (simple pagination).
func (r *AccountRepo) ListAll(ctx context.Context) ([]domain.Account, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, client_id, currency, balance, created_at, updated_at
		 FROM accounts ORDER BY created_at DESC LIMIT 100`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []domain.Account
	for rows.Next() {
		var a domain.Account
		if err := rows.Scan(&a.ID, &a.ClientID, &a.Currency, &a.Balance, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

// Credit atomically adds `amount` to an account's balance.
// Returns the previous and new balance (so we can emit BalanceUpdated).
// If the account does not exist, returns ErrAccountNotFound.
func (r *AccountRepo) Credit(ctx context.Context, id uuid.UUID, amount decimal.Decimal) (oldBal, newBal decimal.Decimal, err error) {
	err = r.db.QueryRow(ctx,
		`UPDATE accounts
		 SET balance = balance + $1, updated_at = now()
		 WHERE id = $2
		 RETURNING balance - $1 AS old_balance, balance AS new_balance`,
		amount, id,
	).Scan(&oldBal, &newBal)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return decimal.Zero, decimal.Zero, domain.ErrAccountNotFound
		}
		return decimal.Zero, decimal.Zero, err
	}
	return oldBal, newBal, nil
}

// Debit subtracts `amount` from a balance only if there's enough funds.
// Atomic operation with a condition: if the WHERE doesn't match
// (insufficient funds), no rows are affected and we return
// ErrInsufficientFunds. This is what enforces the "balance never
// negative" rule even under concurrent operations.
func (r *AccountRepo) Debit(ctx context.Context, id uuid.UUID, amount decimal.Decimal) (oldBal, newBal decimal.Decimal, err error) {
	err = r.db.QueryRow(ctx,
		`UPDATE accounts
		 SET balance = balance - $1, updated_at = now()
		 WHERE id = $2 AND balance >= $1
		 RETURNING balance + $1 AS old_balance, balance AS new_balance`,
		amount, id,
	).Scan(&oldBal, &newBal)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// could be either: the account doesn't exist, or no balance
			// we distinguish with an extra select for clearer error messages
			var exists bool
			_ = r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM accounts WHERE id = $1)`, id).Scan(&exists)
			if !exists {
				return decimal.Zero, decimal.Zero, domain.ErrAccountNotFound
			}
			return decimal.Zero, decimal.Zero, domain.ErrInsufficientFunds
		}
		return decimal.Zero, decimal.Zero, err
	}
	return oldBal, newBal, nil
}
