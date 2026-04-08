package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Supported currencies. Closed list to avoid weird inputs and to match
// the CHECK constraint in the database.
var SupportedCurrencies = map[string]bool{
	"USD": true,
	"EUR": true,
	"ARS": true,
}

// Account is a bank account. The balance is decimal (NUMERIC in PG)
// to avoid losing precision — NEVER float.
type Account struct {
	ID        uuid.UUID
	ClientID  uuid.UUID
	Currency  string
	Balance   decimal.Decimal
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewAccount builds an account with balance 0 and the given currency.
// Returns an error if the currency is not supported.
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

// CanDebit checks that there is enough balance. It is an "early" check
// to return clear errors, but the real atomicity is provided by the
// conditional UPDATE in repo (`balance >= $1`).
func (a Account) CanDebit(amount decimal.Decimal) bool {
	return a.Balance.GreaterThanOrEqual(amount)
}
