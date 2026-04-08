// Package service implementa los casos de uso de transactions-ms.
// El servicio inicia las transacciones (HTTP) y reacciona a las
// respuestas de accounts-ms (Kafka) para cerrar el ciclo del saga.
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

// CreateCommand son los datos para crear una transacción nueva.
// El campo Type indica si es DEPOSIT, WITHDRAW o TRANSFER y según
// eso From/To son requeridos o no (se valida acá).
type CreateCommand struct {
	Type           string
	FromAccountID  *uuid.UUID
	ToAccountID    *uuid.UUID
	Amount         decimal.Decimal
	Currency       string
	IdempotencyKey string
}

// Create inicia una nueva transacción: la persiste en estado PENDING
// y publica TransactionRequested via outbox. Todo en una sola tx PG.
//
// Si el idempotency_key ya existe, devuelve la transacción existente
// (la operación es idempotente desde el punto de vista del cliente).
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
			// recupera la existente
			_ = tx.Rollback(ctx)
			return s.getByIdempotencyKey(ctx, cmd.IdempotencyKey)
		}
		return nil, err
	}

	// publica el comando para que accounts-ms ejecute
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

// validateCommand chequea reglas de negocio antes de tocar la DB.
// La validación sintáctica (formato UUID, monto > 0) ya pasó en el handler.
func (s *TransactionService) validateCommand(cmd CreateCommand) error {
	switch cmd.Type {
	case domain.TypeDeposit:
		if cmd.ToAccountID == nil {
			return invalidErr("to_account_id requerido para DEPOSIT")
		}
	case domain.TypeWithdraw:
		if cmd.FromAccountID == nil {
			return invalidErr("from_account_id requerido para WITHDRAW")
		}
	case domain.TypeTransfer:
		if cmd.FromAccountID == nil || cmd.ToAccountID == nil {
			return invalidErr("from_account_id y to_account_id requeridos para TRANSFER")
		}
		if *cmd.FromAccountID == *cmd.ToAccountID {
			return domain.ErrSameAccountTransfer
		}
	default:
		return domain.ErrInvalidTransactionType
	}
	if !cmd.Amount.IsPositive() {
		return invalidErr("amount debe ser positivo")
	}
	return nil
}

// GetByID lee una transacción por id.
func (s *TransactionService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Transaction, error) {
	r := repo.NewTransactionRepo(s.pool)
	return r.GetByID(ctx, id)
}

// ListAll devuelve todas las transacciones (las últimas 100).
func (s *TransactionService) ListAll(ctx context.Context) ([]domain.Transaction, error) {
	r := repo.NewTransactionRepo(s.pool)
	return r.ListAll(ctx)
}

// ListByAccount devuelve las transacciones de una cuenta.
func (s *TransactionService) ListByAccount(ctx context.Context, accountID uuid.UUID) ([]domain.Transaction, error) {
	r := repo.NewTransactionRepo(s.pool)
	return r.ListByAccount(ctx, accountID)
}

// ===== handler de eventos de accounts-ms (cierra el ciclo del saga) =====

// HandleAccountsResult procesa los AccountsTransferApplied/Failed que
// llegan desde accounts-ms y actualiza el estado de la transacción.
// Después publica TransactionCompleted o TransactionRejected para que
// el llm-ms y el front se enteren del resultado final.
func (s *TransactionService) HandleAccountsResult(ctx context.Context, eventID, eventType string, raw []byte) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// idempotencia
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
		// recuperar la transacción para saber sus datos
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

// invalidErr crea un error de validación rápido.
type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }
func invalidErr(msg string) error        { return &validationError{msg: msg} }
