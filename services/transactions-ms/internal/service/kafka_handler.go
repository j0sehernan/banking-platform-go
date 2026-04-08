package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/j0sehernan/banking-platform-go/pkg/events"
	pkgkafka "github.com/j0sehernan/banking-platform-go/pkg/kafka"
)

// AccountsResultHandler listens to the accounts.tx-results topic and
// delegates to the service to close the saga loop.
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
		// not our event, ignore without error
		return nil
	}
}
