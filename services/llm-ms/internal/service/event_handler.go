package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/j0sehernan/banking-platform-go/pkg/events"
	pkgkafka "github.com/j0sehernan/banking-platform-go/pkg/kafka"
	"github.com/j0sehernan/banking-platform-go/services/llm-ms/internal/domain"
	"github.com/j0sehernan/banking-platform-go/services/llm-ms/internal/repo"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// EventHandler processes events from transactions.events to:
//  1) update the materialized view (transactions_view)
//  2) generate the initial explanation automatically
//
// Both steps run in a single transaction to keep consistency: if we
// store the explanation we also store the updated state of the
// transaction.
//
// Idempotency: we use processed_events as in the other services.
type EventHandler struct {
	pool   *pgxpool.Pool
	svc    *LLMService
	logger *slog.Logger
}

func NewEventHandler(pool *pgxpool.Pool, svc *LLMService, logger *slog.Logger) *EventHandler {
	return &EventHandler{pool: pool, svc: svc, logger: logger}
}

func (h *EventHandler) Handle(ctx context.Context, msg pkgkafka.Message) error {
	env, err := events.UnmarshalEnvelope(msg.Value)
	if err != nil {
		return fmt.Errorf("invalid envelope: %w", err)
	}

	// Only process events that matter to us
	switch env.EventType {
	case events.EventTransactionCompleted, events.EventTransactionRejected:
		return h.handleTransactionEvent(ctx, env.EventID, env.EventType, env.Payload)
	default:
		return nil
	}
}

func (h *EventHandler) handleTransactionEvent(ctx context.Context, eventID, eventType string, raw []byte) error {
	// 1) Update the materialized view + mark idempotency in a single tx
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	processedRepo := repo.NewProcessedEventsRepo(tx)
	isNew, err := processedRepo.MarkProcessed(ctx, eventID)
	if err != nil {
		return err
	}
	if !isNew {
		h.logger.Info("event already processed", "event_id", eventID)
		committed = true
		return tx.Commit(ctx)
	}

	view, err := h.parseToView(eventType, raw)
	if err != nil {
		return err
	}

	viewRepo := repo.NewTransactionsViewRepo(tx)
	if err := viewRepo.Upsert(ctx, view); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	committed = true

	// 2) Generate the explanation with the LLM (outside the tx so we
	//    don't block DB connections while waiting for Claude).
	//    If it fails we log it but don't propagate: the transaction
	//    is already in the view, and the explanation can be regenerated.
	go h.generateExplanation(view)

	return nil
}

func (h *EventHandler) generateExplanation(view domain.TransactionView) {
	// We use an independent context with timeout because the caller
	// already finished the Kafka handler (offset commit done).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	text, err := h.svc.Explainer().ExplainTransaction(ctx, &view)
	if err != nil {
		h.logger.Error("explanation generation failed",
			"tx_id", view.ID,
			"err", err,
		)
		return
	}

	expRepo := repo.NewExplanationRepo(h.pool)
	if err := expRepo.Upsert(ctx, view.ID, text, h.svc.Explainer().Model()); err != nil {
		h.logger.Error("explanation save failed", "tx_id", view.ID, "err", err)
		return
	}

	h.logger.Info("explanation generated", "tx_id", view.ID, "model", h.svc.Explainer().Model())
}

// parseToView converts the event payload into a TransactionView.
// Handles the two relevant events: Completed and Rejected.
func (h *EventHandler) parseToView(eventType string, raw []byte) (domain.TransactionView, error) {
	switch eventType {
	case events.EventTransactionCompleted:
		var p events.TransactionCompletedPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return domain.TransactionView{}, err
		}
		return payloadToView(
			p.TransactionID, p.Type, events.TxStatusCompleted,
			p.FromAccountID, p.ToAccountID, p.Amount, p.Currency, "", "", p.CompletedAt,
		)
	case events.EventTransactionRejected:
		var p events.TransactionRejectedPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return domain.TransactionView{}, err
		}
		return payloadToView(
			p.TransactionID, p.Type, events.TxStatusRejected,
			p.FromAccountID, p.ToAccountID, p.Amount, p.Currency, p.Reason, p.Message, p.RejectedAt,
		)
	}
	return domain.TransactionView{}, fmt.Errorf("unsupported event type: %s", eventType)
}

func payloadToView(
	id, txType, status, fromID, toID, amount, currency, rejCode, rejMsg, atStr string,
) (domain.TransactionView, error) {
	txID, err := uuid.Parse(id)
	if err != nil {
		return domain.TransactionView{}, fmt.Errorf("invalid tx id: %w", err)
	}
	amt, err := decimal.NewFromString(amount)
	if err != nil {
		return domain.TransactionView{}, fmt.Errorf("invalid amount: %w", err)
	}
	view := domain.TransactionView{
		ID:            txID,
		Type:          txType,
		Status:        status,
		Amount:        amt,
		Currency:      currency,
		RejectionCode: rejCode,
		RejectionMsg:  rejMsg,
	}
	if fromID != "" {
		fid, err := uuid.Parse(fromID)
		if err == nil {
			view.FromAccountID = &fid
		}
	}
	if toID != "" {
		tid, err := uuid.Parse(toID)
		if err == nil {
			view.ToAccountID = &tid
		}
	}
	if at, err := time.Parse(time.RFC3339, atStr); err == nil {
		view.CreatedAt = at
		view.UpdatedAt = at
	} else {
		view.CreatedAt = time.Now().UTC()
		view.UpdatedAt = time.Now().UTC()
	}
	return view, nil
}
