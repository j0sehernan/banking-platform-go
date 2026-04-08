// Package domain define las entidades del llm-ms.
// El llm-ms no tiene "lógica bancaria" propia: solo refleja transacciones
// (vista materializada) y guarda explicaciones generadas por el LLM.
package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

var (
	ErrTransactionNotFound  = errors.New("transaction not found in view")
	ErrExplanationNotFound  = errors.New("explanation not found")
)

// TransactionView es la copia local que llm-ms mantiene de las
// transacciones, alimentada por eventos de Kafka.
type TransactionView struct {
	ID             uuid.UUID
	Type           string
	Status         string
	FromAccountID  *uuid.UUID
	ToAccountID    *uuid.UUID
	Amount         decimal.Decimal
	Currency       string
	RejectionCode  string
	RejectionMsg   string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Explanation es el texto generado por Claude (o el mock) para una tx.
type Explanation struct {
	TxID        uuid.UUID
	Text        string
	Model       string
	GeneratedAt time.Time
}

// ChatMessage es un mensaje en el array del chat (user o assistant).
// El front mantiene el historial y lo manda en cada request.
type ChatMessage struct {
	Role    string `json:"role"    validate:"required,oneof=user assistant"`
	Content string `json:"content" validate:"required,min=1"`
}
