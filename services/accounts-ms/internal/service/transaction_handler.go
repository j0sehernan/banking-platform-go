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

// TransactionHandler processes the TransactionRequested events that come
// from transactions-ms and executes the actual debit/credit on accounts.
//
// It is the heart of the event-driven flow: receives the command,
// executes atomically on its own DB, and emits the result back.
type TransactionHandler struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func NewTransactionHandler(pool *pgxpool.Pool, logger *slog.Logger) *TransactionHandler {
	return &TransactionHandler{pool: pool, logger: logger}
}

// Handle is the signature the Kafka consumer expects.
func (h *TransactionHandler) Handle(ctx context.Context, msg pkgkafka.Message) error {
	env, err := events.UnmarshalEnvelope(msg.Value)
	if err != nil {
		return fmt.Errorf("invalid envelope: %w", err)
	}

	if env.EventType != events.EventTransactionRequested {
		// not our event, ignore
		return nil
	}

	var payload events.TransactionRequestedPayload
	if err := env.Decode(&payload); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}

	// idempotency + execution in a single tx
	return h.processInTx(ctx, env.EventID, payload)
}

func (h *TransactionHandler) processInTx(ctx context.Context, eventID string, p events.TransactionRequestedPayload) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // if commit ok, this rollback is a no-op

	// 1) idempotency check
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

	// 2) execute the operation depending on the type
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
		execErr = fmt.Errorf("unknown type: %s", p.Type)
	}

	if execErr != nil {
		// Important: for business failures we do NOT return the error.
		// Instead we rollback the UPDATEs to accounts and open a NEW
		// transaction that only stores the failure event.
		// If we returned the error the consumer would retry the message,
		// and with the idempotency already marked the rejected event
		// would never be published.
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

	// validate that both accounts exist and have the same currency
	// before moving any money
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

	// atomic debit (rejects if not enough balance)
	oldFrom, newFrom, err := accountRepo.Debit(ctx, fromID, amount)
	if err != nil {
		return err
	}
	// credit (in the same tx → if it fails, automatic rollback)
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

// publishFailedInNewTx opens a new transaction to store the failure event
// and mark idempotency. Used when the business handler failed (insufficient
// funds, account missing) and the previous rollback already happened.
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
		return "insufficient_funds", "The source account does not have enough balance"
	case errors.Is(err, domain.ErrAccountNotFound):
		return "account_not_found", "One of the accounts does not exist"
	case errors.Is(err, domain.ErrCurrencyMismatch):
		return "currency_mismatch", "The accounts have different currencies"
	case errors.Is(err, domain.ErrSameAccountTransfer):
		return "same_account", "Cannot transfer to the same account"
	default:
		return "unknown_error", err.Error()
	}
}
