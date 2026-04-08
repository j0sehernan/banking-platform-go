package domain

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestNewTransaction_Defaults(t *testing.T) {
	from := uuid.New()
	to := uuid.New()
	amount := decimal.RequireFromString("100.50")

	tx := NewTransaction(TypeTransfer, &from, &to, amount, "USD", "key-1")

	assert.Equal(t, TypeTransfer, tx.Type)
	assert.Equal(t, StatusPending, tx.Status)
	assert.Equal(t, "key-1", tx.IdempotencyKey)
	assert.True(t, tx.Amount.Equal(amount))
	assert.NotEqual(t, uuid.Nil, tx.ID)
}

func TestTransaction_StateMachine(t *testing.T) {
	tx := Transaction{Status: StatusPending}

	t.Run("pending can move to completed", func(t *testing.T) {
		assert.True(t, tx.CanTransitionTo(StatusCompleted))
	})

	t.Run("pending can move to rejected", func(t *testing.T) {
		assert.True(t, tx.CanTransitionTo(StatusRejected))
	})

	t.Run("completed is terminal", func(t *testing.T) {
		completed := Transaction{Status: StatusCompleted}
		assert.False(t, completed.CanTransitionTo(StatusRejected))
		assert.False(t, completed.CanTransitionTo(StatusPending))
	})

	t.Run("rejected is terminal", func(t *testing.T) {
		rejected := Transaction{Status: StatusRejected}
		assert.False(t, rejected.CanTransitionTo(StatusCompleted))
	})
}
