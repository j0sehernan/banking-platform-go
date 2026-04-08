package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/j0sehernan/banking-platform-go/pkg/events"
	"github.com/shopspring/decimal"
)

// Transaction states. We use the ones from the events package to keep
// consistency between the DB and the Kafka messages.
const (
	StatusPending   = events.TxStatusPending
	StatusCompleted = events.TxStatusCompleted
	StatusRejected  = events.TxStatusRejected
)

// Transaction types.
const (
	TypeDeposit  = events.TxTypeDeposit
	TypeWithdraw = events.TxTypeWithdraw
	TypeTransfer = events.TxTypeTransfer
)

// Transaction is the main entity of the service.
//
// IdempotencyKey is UNIQUE in the DB → if the client retries the same
// POST, the INSERT fails with a UNIQUE violation and we return the
// existing transaction (no duplicates created).
type Transaction struct {
	ID             uuid.UUID
	Type           string
	FromAccountID  *uuid.UUID
	ToAccountID    *uuid.UUID
	Amount         decimal.Decimal
	Currency       string
	Status         string
	IdempotencyKey string
	RejectionCode  string
	RejectionMsg   string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// NewTransaction builds a new transaction in PENDING state.
// The validation of required fields per type (e.g. TRANSFER needs
// from and to) is done by the caller in service/.
func NewTransaction(txType string, from, to *uuid.UUID, amount decimal.Decimal, currency, idempotencyKey string) Transaction {
	now := time.Now().UTC()
	return Transaction{
		ID:             uuid.New(),
		Type:           txType,
		FromAccountID:  from,
		ToAccountID:    to,
		Amount:         amount,
		Currency:       currency,
		Status:         StatusPending,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// CanTransitionTo validates the state machine.
// PENDING → COMPLETED | REJECTED. Terminal states: COMPLETED and REJECTED.
func (t Transaction) CanTransitionTo(newStatus string) bool {
	if t.Status != StatusPending {
		return false // terminal state
	}
	return newStatus == StatusCompleted || newStatus == StatusRejected
}
