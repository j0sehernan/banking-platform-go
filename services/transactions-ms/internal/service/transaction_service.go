// Package service implements the use cases for transactions-ms.
// The service starts transactions (HTTP) and reacts to the responses
// from accounts-ms (Kafka) to close the saga loop.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/j0sehernan/banking-platform-go/pkg/events"
	"github.com/j0sehernan/banking-platform-go/services/transactions-ms/internal/domain"
	"github.com/j0sehernan/banking-platform-go/services/transactions-ms/internal/repo"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

type TransactionService struct {
	pool *pgxpool.Pool
}

func NewTransactionService(pool *pgxpool.Pool) *TransactionService {
	return &TransactionService{pool: pool}
}

// CreateCommand carries the data to create a new transaction.
// The Type field tells whether it is DEPOSIT, WITHDRAW or TRANSFER and
// from there From/To are required or not (validated here).
type CreateCommand struct {
	Type           string
	FromAccountID  *uuid.UUID
	ToAccountID    *uuid.UUID
	Amount         decimal.Decimal
	Currency       string
	IdempotencyKey string
}

// Create starts a new transaction: persists it as PENDING and publishes
// TransactionRequested via outbox. All in a single PG transaction.
//
// If the idempotency_key already exists, it returns the existing
// transaction (the operation is idempotent from the client's view).
func (s *TransactionService) Create(ctx context.Context, cmd CreateCommand) (*domain.Transaction, error) {
	if err := s.validateCommand(cmd); err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	txRepo := repo.NewTransactionRepo(tx)
	outboxRepo := repo.NewOutboxRepo(tx)

	t := domain.NewTransaction(cmd.Type, cmd.FromAccountID, cmd.ToAccountID, cmd.Amount, cmd.Currency, cmd.IdempotencyKey)

	if err := txRepo.Create(ctx, t); err != nil {
		if errors.Is(err, domain.ErrDuplicateIdempotencyKey) {
			// fetch the existing one
			_ = tx.Rollback(ctx)
			return s.getByIdempotencyKey(ctx, cmd.IdempotencyKey)
		}
		return nil, err
	}

	// publish the command so accounts-ms executes
	payload := events.TransactionRequestedPayload{
		TransactionID: t.ID.String(),
		Type:          t.Type,
		Amount:        t.Amount.String(),
		Currency:      t.Currency,
	}
	if t.FromAccountID != nil {
		payload.FromAccountID = t.FromAccountID.String()
	}
	if t.ToAccountID != nil {
		payload.ToAccountID = t.ToAccountID.String()
	}

	env, err := events.NewEnvelope(events.EventTransactionRequested, payload, t.ID.String())
	if err != nil {
		return nil, err
	}
	bytes, _ := json.Marshal(env)
	if err := outboxRepo.Insert(ctx, events.TopicTransactionsCommands, t.ID.String(), bytes, nil); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *TransactionService) getByIdempotencyKey(ctx context.Context, key string) (*domain.Transaction, error) {
	r := repo.NewTransactionRepo(s.pool)
	return r.GetByIdempotencyKey(ctx, key)
}

// validateCommand checks business rules before touching the DB.
// Syntactic validation (UUID format, amount > 0) was already done by the handler.
func (s *TransactionService) validateCommand(cmd CreateCommand) error {
	switch cmd.Type {
	case domain.TypeDeposit:
		if cmd.ToAccountID == nil {
			return invalidErr("to_account_id is required for DEPOSIT")
		}
	case domain.TypeWithdraw:
		if cmd.FromAccountID == nil {
			return invalidErr("from_account_id is required for WITHDRAW")
		}
	case domain.TypeTransfer:
		if cmd.FromAccountID == nil || cmd.ToAccountID == nil {
			return invalidErr("from_account_id and to_account_id are required for TRANSFER")
		}
		if *cmd.FromAccountID == *cmd.ToAccountID {
			return domain.ErrSameAccountTransfer
		}
	default:
		return domain.ErrInvalidTransactionType
	}
	if !cmd.Amount.IsPositive() {
		return invalidErr("amount must be positive")
	}
	return nil
}

// GetByID reads a transaction by id.
func (s *TransactionService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Transaction, error) {
	r := repo.NewTransactionRepo(s.pool)
	return r.GetByID(ctx, id)
}

// ListAll returns all transactions (the latest 100).
func (s *TransactionService) ListAll(ctx context.Context) ([]domain.Transaction, error) {
	r := repo.NewTransactionRepo(s.pool)
	return r.ListAll(ctx)
}

// ListByAccount returns the transactions of an account.
func (s *TransactionService) ListByAccount(ctx context.Context, accountID uuid.UUID) ([]domain.Transaction, error) {
	r := repo.NewTransactionRepo(s.pool)
	return r.ListByAccount(ctx, accountID)
}

// ===== handler for accounts-ms events (closes the saga loop) =====

// HandleAccountsResult processes the AccountsTransferApplied/Failed
// events that come from accounts-ms and updates the transaction state.
// Then publishes TransactionCompleted or TransactionRejected so llm-ms
// and the front know about the final result.
func (s *TransactionService) HandleAccountsResult(ctx context.Context, eventID, eventType string, raw []byte) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// idempotency
	processedRepo := repo.NewProcessedEventsRepo(tx)
	isNew, err := processedRepo.MarkProcessed(ctx, eventID)
	if err != nil {
		return err
	}
	if !isNew {
		return tx.Commit(ctx)
	}

	txRepo := repo.NewTransactionRepo(tx)
	outboxRepo := repo.NewOutboxRepo(tx)

	switch eventType {
	case events.EventAccountsTransferApplied:
		var p events.AccountsTransferAppliedPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return err
		}
		txID, err := uuid.Parse(p.TransactionID)
		if err != nil {
			return err
		}
		// fetch the transaction to know its data
		t, err := txRepo.GetByID(ctx, txID)
		if err != nil {
			return err
		}
		if err := txRepo.MarkCompleted(ctx, txID); err != nil {
			return err
		}
		if err := s.publishCompleted(ctx, outboxRepo, t); err != nil {
			return err
		}

	case events.EventAccountsTransferFailed:
		var p events.AccountsTransferFailedPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return err
		}
		txID, err := uuid.Parse(p.TransactionID)
		if err != nil {
			return err
		}
		t, err := txRepo.GetByID(ctx, txID)
		if err != nil {
			return err
		}
		if err := txRepo.MarkRejected(ctx, txID, p.Reason, p.Message); err != nil {
			return err
		}
		if err := s.publishRejected(ctx, outboxRepo, t, p.Reason, p.Message); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (s *TransactionService) publishCompleted(
	ctx context.Context,
	outboxRepo *repo.OutboxRepo,
	t *domain.Transaction,
) error {
	payload := events.TransactionCompletedPayload{
		TransactionID: t.ID.String(),
		Type:          t.Type,
		Amount:        t.Amount.String(),
		Currency:      t.Currency,
		CompletedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if t.FromAccountID != nil {
		payload.FromAccountID = t.FromAccountID.String()
	}
	if t.ToAccountID != nil {
		payload.ToAccountID = t.ToAccountID.String()
	}
	env, err := events.NewEnvelope(events.EventTransactionCompleted, payload, t.ID.String())
	if err != nil {
		return err
	}
	bytes, _ := json.Marshal(env)
	return outboxRepo.Insert(ctx, events.TopicTransactionsEvents, t.ID.String(), bytes, nil)
}

func (s *TransactionService) publishRejected(
	ctx context.Context,
	outboxRepo *repo.OutboxRepo,
	t *domain.Transaction,
	reason, message string,
) error {
	payload := events.TransactionRejectedPayload{
		TransactionID: t.ID.String(),
		Type:          t.Type,
		Amount:        t.Amount.String(),
		Currency:      t.Currency,
		Reason:        reason,
		Message:       message,
		RejectedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	if t.FromAccountID != nil {
		payload.FromAccountID = t.FromAccountID.String()
	}
	if t.ToAccountID != nil {
		payload.ToAccountID = t.ToAccountID.String()
	}
	env, err := events.NewEnvelope(events.EventTransactionRejected, payload, t.ID.String())
	if err != nil {
		return err
	}
	bytes, _ := json.Marshal(env)
	return outboxRepo.Insert(ctx, events.TopicTransactionsEvents, t.ID.String(), bytes, nil)
}

// invalidErr creates a quick validation error.
type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }
func invalidErr(msg string) error        { return &validationError{msg: msg} }
