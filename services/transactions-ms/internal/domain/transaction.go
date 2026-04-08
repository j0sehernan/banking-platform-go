package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/j0sehernan/banking-platform-go/pkg/events"
	"github.com/shopspring/decimal"
)

// Estados de la transacción. Usamos los del paquete events para mantener
// consistencia entre la DB y los mensajes de Kafka.
const (
	StatusPending   = events.TxStatusPending
	StatusCompleted = events.TxStatusCompleted
	StatusRejected  = events.TxStatusRejected
)

// Tipos de transacción.
const (
	TypeDeposit  = events.TxTypeDeposit
	TypeWithdraw = events.TxTypeWithdraw
	TypeTransfer = events.TxTypeTransfer
)

// Transaction es la entidad principal del servicio.
//
// IdempotencyKey es UNIQUE en DB → si el cliente reintenta el mismo POST,
// el INSERT falla con violación de UNIQUE y devolvemos la transacción
// existente (no creamos una duplicada).
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

// NewTransaction construye una transacción nueva en estado PENDING.
// La validación de campos requeridos por tipo (ej: TRANSFER necesita
// from y to) la hace el caller en service/.
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

// CanTransitionTo valida la máquina de estados.
// PENDING → COMPLETED | REJECTED. Estado terminal: COMPLETED y REJECTED.
func (t Transaction) CanTransitionTo(newStatus string) bool {
	if t.Status != StatusPending {
		return false // estado terminal
	}
	return newStatus == StatusCompleted || newStatus == StatusRejected
}
