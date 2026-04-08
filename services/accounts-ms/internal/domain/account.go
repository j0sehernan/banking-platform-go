package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Currencies soportadas. Mantengo lista cerrada para evitar entradas
// raras y para que coincida con el CHECK constraint en la DB.
var SupportedCurrencies = map[string]bool{
	"USD": true,
	"EUR": true,
	"ARS": true,
}

// Account es una cuenta bancaria. El balance es decimal (NUMERIC en PG)
// para no perder precisión, NUNCA float.
type Account struct {
	ID        uuid.UUID
	ClientID  uuid.UUID
	Currency  string
	Balance   decimal.Decimal
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewAccount construye una cuenta con balance 0 y la moneda dada.
// Devuelve error si la currency no está soportada.
func NewAccount(clientID uuid.UUID, currency string) (Account, error) {
	if !SupportedCurrencies[currency] {
		return Account{}, ErrInvalidCurrency
	}
	now := time.Now().UTC()
	return Account{
		ID:        uuid.New(),
		ClientID:  clientID,
		Currency:  currency,
		Balance:   decimal.Zero,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// CanDebit verifica que haya saldo suficiente. Es un chequeo "temprano"
// para devolver errores claros, pero la verdadera atomicidad la da
// el UPDATE condicional en repo (`balance >= $1`).
func (a Account) CanDebit(amount decimal.Decimal) bool {
	return a.Balance.GreaterThanOrEqual(amount)
}
