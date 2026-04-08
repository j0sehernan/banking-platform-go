package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/j0sehernan/banking-platform-go/pkg/events"
	pkgkafka "github.com/j0sehernan/banking-platform-go/pkg/kafka"
	"github.com/j0sehernan/banking-platform-go/services/accounts-ms/internal/domain"
	"github.com/j0sehernan/banking-platform-go/services/accounts-ms/internal/repo"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// TransactionHandler procesa los TransactionRequested que llegan desde
// transactions-ms y ejecuta el débito/crédito de cuentas.
//
// Es el corazón del flujo event-driven: recibe el comando, ejecuta
// atómicamente en su propia DB, y emite el resultado de vuelta.
type TransactionHandler struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func NewTransactionHandler(pool *pgxpool.Pool, logger *slog.Logger) *TransactionHandler {
	return &TransactionHandler{pool: pool, logger: logger}
}

// Handle es la firma que el consumer Kafka espera.
func (h *TransactionHandler) Handle(ctx context.Context, msg pkgkafka.Message) error {
	env, err := events.UnmarshalEnvelope(msg.Value)
	if err != nil {
		return fmt.Errorf("invalid envelope: %w", err)
	}

	if env.EventType != events.EventTransactionRequested {
		// no es nuestro evento, ignoramos
		return nil
	}

	var payload events.TransactionRequestedPayload
	if err := env.Decode(&payload); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}

	// idempotencia + ejecución en una sola tx
	return h.processInTx(ctx, env.EventID, payload)
}

func (h *TransactionHandler) processInTx(ctx context.Context, eventID string, p events.TransactionRequestedPayload) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // si commit ok, este rollback es no-op

	// 1) check idempotencia
	processedRepo := repo.NewProcessedEventsRepo(tx)
	isNew, err := processedRepo.MarkProcessed(ctx, eventID)
	if err != nil {
		return err
	}
	if !isNew {
		h.logger.Info("event already processed, skipping",
			"event_id", eventID,
			"transaction_id", p.TransactionID,
		)
		return tx.Commit(ctx)
	}

	// 2) ejecutar la operación según el tipo
	accountRepo := repo.NewAccountRepo(tx)
	outboxRepo := repo.NewOutboxRepo(tx)

	amount, err := decimal.NewFromString(p.Amount)
	if err != nil {
		if err := h.publishFailed(ctx, outboxRepo, p, "invalid_amount", err.Error()); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	var execErr error
	switch p.Type {
	case events.TxTypeDeposit:
		execErr = h.handleDeposit(ctx, accountRepo, outboxRepo, p, amount)
	case events.TxTypeWithdraw:
		execErr = h.handleWithdraw(ctx, accountRepo, outboxRepo, p, amount)
	case events.TxTypeTransfer:
		execErr = h.handleTransfer(ctx, accountRepo, outboxRepo, p, amount)
	default:
		execErr = fmt.Errorf("tipo desconocido: %s", p.Type)
	}

	if execErr != nil {
		// Importante: para fallos de negocio, NO devolvemos el error.
		// En su lugar, hacemos rollback de los UPDATEs a accounts y abrimos
		// una NUEVA transacción que solo guarda el evento de fallo.
		// Si retornáramos el error, el consumer reintentaría el mensaje
		// y con la idempotencia ya marcada nunca se publicaría el rejected.
		_ = tx.Rollback(ctx)
		return h.publishFailedInNewTx(ctx, eventID, p, execErr)
	}

	return tx.Commit(ctx)
}

func (h *TransactionHandler) handleDeposit(
	ctx context.Context,
	accountRepo *repo.AccountRepo,
	outboxRepo *repo.OutboxRepo,
	p events.TransactionRequestedPayload,
	amount decimal.Decimal,
) error {
	toID, err := uuid.Parse(p.ToAccountID)
	if err != nil {
		return fmt.Errorf("invalid to_account_id: %w", err)
	}
	oldBal, newBal, err := accountRepo.Credit(ctx, toID, amount)
	if err != nil {
		return err
	}
	if err := h.writeBalanceUpdated(ctx, outboxRepo, p.ToAccountID, oldBal, newBal, "deposit"); err != nil {
		return err
	}
	return h.writeApplied(ctx, outboxRepo, p)
}

func (h *TransactionHandler) handleWithdraw(
	ctx context.Context,
	accountRepo *repo.AccountRepo,
	outboxRepo *repo.OutboxRepo,
	p events.TransactionRequestedPayload,
	amount decimal.Decimal,
) error {
	fromID, err := uuid.Parse(p.FromAccountID)
	if err != nil {
		return fmt.Errorf("invalid from_account_id: %w", err)
	}
	oldBal, newBal, err := accountRepo.Debit(ctx, fromID, amount)
	if err != nil {
		return err
	}
	if err := h.writeBalanceUpdated(ctx, outboxRepo, p.FromAccountID, oldBal, newBal, "withdraw"); err != nil {
		return err
	}
	return h.writeApplied(ctx, outboxRepo, p)
}

func (h *TransactionHandler) handleTransfer(
	ctx context.Context,
	accountRepo *repo.AccountRepo,
	outboxRepo *repo.OutboxRepo,
	p events.TransactionRequestedPayload,
	amount decimal.Decimal,
) error {
	fromID, err := uuid.Parse(p.FromAccountID)
	if err != nil {
		return fmt.Errorf("invalid from_account_id: %w", err)
	}
	toID, err := uuid.Parse(p.ToAccountID)
	if err != nil {
		return fmt.Errorf("invalid to_account_id: %w", err)
	}
	if fromID == toID {
		return domain.ErrSameAccountTransfer
	}

	// validar que ambas cuentas existen y misma moneda antes de mover plata
	from, err := accountRepo.GetByID(ctx, fromID)
	if err != nil {
		return err
	}
	to, err := accountRepo.GetByID(ctx, toID)
	if err != nil {
		return err
	}
	if from.Currency != to.Currency {
		return domain.ErrCurrencyMismatch
	}

	// débito atómico (rechaza si no hay saldo)
	oldFrom, newFrom, err := accountRepo.Debit(ctx, fromID, amount)
	if err != nil {
		return err
	}
	// crédito (en la misma tx → si falla, rollback automático)
	oldTo, newTo, err := accountRepo.Credit(ctx, toID, amount)
	if err != nil {
		return err
	}

	if err := h.writeBalanceUpdated(ctx, outboxRepo, p.FromAccountID, oldFrom, newFrom, "transfer-out"); err != nil {
		return err
	}
	if err := h.writeBalanceUpdated(ctx, outboxRepo, p.ToAccountID, oldTo, newTo, "transfer-in"); err != nil {
		return err
	}
	return h.writeApplied(ctx, outboxRepo, p)
}

// ===== outbox writers =====

func (h *TransactionHandler) writeBalanceUpdated(
	ctx context.Context,
	outboxRepo *repo.OutboxRepo,
	accountID string,
	oldBal, newBal decimal.Decimal,
	reason string,
) error {
	payload := events.BalanceUpdatedPayload{
		AccountID:  accountID,
		OldBalance: oldBal.String(),
		NewBalance: newBal.String(),
		Reason:     reason,
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	env, err := events.NewEnvelope(events.EventBalanceUpdated, payload, "")
	if err != nil {
		return err
	}
	bytes, _ := json.Marshal(env)
	return outboxRepo.Insert(ctx, events.TopicAccountsEvents, accountID, bytes, nil)
}

func (h *TransactionHandler) writeApplied(
	ctx context.Context,
	outboxRepo *repo.OutboxRepo,
	p events.TransactionRequestedPayload,
) error {
	payload := events.AccountsTransferAppliedPayload{
		TransactionID: p.TransactionID,
		AppliedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	env, err := events.NewEnvelope(events.EventAccountsTransferApplied, payload, p.TransactionID)
	if err != nil {
		return err
	}
	bytes, _ := json.Marshal(env)
	return outboxRepo.Insert(ctx, events.TopicAccountsTxResults, p.TransactionID, bytes, nil)
}

func (h *TransactionHandler) publishFailed(
	ctx context.Context,
	outboxRepo *repo.OutboxRepo,
	p events.TransactionRequestedPayload,
	reason, message string,
) error {
	payload := events.AccountsTransferFailedPayload{
		TransactionID: p.TransactionID,
		Reason:        reason,
		Message:       message,
		FailedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	env, err := events.NewEnvelope(events.EventAccountsTransferFailed, payload, p.TransactionID)
	if err != nil {
		return err
	}
	bytes, _ := json.Marshal(env)
	return outboxRepo.Insert(ctx, events.TopicAccountsTxResults, p.TransactionID, bytes, nil)
}

// publishFailedInNewTx abre una nueva transacción para guardar el evento
// de fallo + marcar como procesado. Esto se usa cuando el handler de
// negocio falló (saldo insuficiente, cuenta no existe) y el rollback ya
// pasó.
func (h *TransactionHandler) publishFailedInNewTx(
	ctx context.Context,
	eventID string,
	p events.TransactionRequestedPayload,
	execErr error,
) error {
	reason, message := mapErrToReason(execErr)
	h.logger.Info("transaction rejected",
		"transaction_id", p.TransactionID,
		"reason", reason,
		"err", execErr,
	)

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	processedRepo := repo.NewProcessedEventsRepo(tx)
	if _, err := processedRepo.MarkProcessed(ctx, eventID); err != nil {
		return err
	}

	outboxRepo := repo.NewOutboxRepo(tx)
	if err := h.publishFailed(ctx, outboxRepo, p, reason, message); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func mapErrToReason(err error) (reason, message string) {
	switch {
	case errors.Is(err, domain.ErrInsufficientFunds):
		return "insufficient_funds", "La cuenta de origen no tiene saldo suficiente"
	case errors.Is(err, domain.ErrAccountNotFound):
		return "account_not_found", "Una de las cuentas no existe"
	case errors.Is(err, domain.ErrCurrencyMismatch):
		return "currency_mismatch", "Las cuentas tienen monedas distintas"
	case errors.Is(err, domain.ErrSameAccountTransfer):
		return "same_account", "No se puede transferir a la misma cuenta"
	default:
		return "unknown_error", err.Error()
	}
}
