package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/j0sehernan/banking-platform-go/pkg/events"
	pkgkafka "github.com/j0sehernan/banking-platform-go/pkg/kafka"
)

// AccountsResultHandler escucha el topic accounts.tx-results y delega
// al service para que cierre el ciclo del saga.
type AccountsResultHandler struct {
	svc    *TransactionService
	logger *slog.Logger
}

func NewAccountsResultHandler(svc *TransactionService, logger *slog.Logger) *AccountsResultHandler {
	return &AccountsResultHandler{svc: svc, logger: logger}
}

func (h *AccountsResultHandler) Handle(ctx context.Context, msg pkgkafka.Message) error {
	env, err := events.UnmarshalEnvelope(msg.Value)
	if err != nil {
		return fmt.Errorf("invalid envelope: %w", err)
	}

	switch env.EventType {
	case events.EventAccountsTransferApplied, events.EventAccountsTransferFailed:
		return h.svc.HandleAccountsResult(ctx, env.EventID, env.EventType, env.Payload)
	default:
		// no es nuestro evento, ignoramos sin error
		return nil
	}
}
