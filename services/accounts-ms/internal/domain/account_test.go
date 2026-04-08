package domain

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestNewAccount_ValidCurrency(t *testing.T) {
	clientID := uuid.New()
	acc, err := NewAccount(clientID, "USD")

	assert.NoError(t, err)
	assert.Equal(t, clientID, acc.ClientID)
	assert.Equal(t, "USD", acc.Currency)
	assert.True(t, acc.Balance.Equal(decimal.Zero))
	assert.NotEqual(t, uuid.Nil, acc.ID)
}

func TestNewAccount_InvalidCurrency(t *testing.T) {
	_, err := NewAccount(uuid.New(), "XYZ")
	assert.ErrorIs(t, err, ErrInvalidCurrency)
}

func TestAccount_CanDebit(t *testing.T) {
	tests := []struct {
		name    string
		balance string
		amount  string
		want    bool
	}{
		{"saldo suficiente", "100.00", "50.00", true},
		{"saldo justo", "100.00", "100.00", true},
		{"saldo insuficiente", "50.00", "100.00", false},
		{"saldo cero", "0.00", "0.01", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			acc := Account{
				Balance: decimal.RequireFromString(tt.balance),
			}
			amount := decimal.RequireFromString(tt.amount)
			assert.Equal(t, tt.want, acc.CanDebit(amount))
		})
	}
}
