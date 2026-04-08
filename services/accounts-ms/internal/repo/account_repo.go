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

// Create inserta una cuenta nueva.
func (r *AccountRepo) Create(ctx context.Context, a domain.Account) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO accounts (id, client_id, currency, balance, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		a.ID, a.ClientID, a.Currency, a.Balance, a.CreatedAt, a.UpdatedAt,
	)
	return err
}

// GetByID lee una cuenta por id.
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

// ListByClient devuelve todas las cuentas de un cliente.
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

// ListAll devuelve todas las cuentas (paginado simple).
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

// Credit suma `amount` al balance de una cuenta de forma atómica.
// Devuelve el balance anterior y el nuevo (para emitir BalanceUpdated).
// Si la cuenta no existe, devuelve ErrAccountNotFound.
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

// Debit resta `amount` al balance solo si hay saldo suficiente.
// Operación atómica con condición: si el WHERE no matchea (saldo
// insuficiente), no afecta filas y devuelve ErrInsufficientFunds.
// Esto es idempotente con la regla "saldo nunca negativo".
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
			// puede ser que la cuenta no exista o que no tenga saldo
			// distinguimos haciendo un select adicional (raro pero útil para errores claros)
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
