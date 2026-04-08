package domain

import "errors"

// Errores tipados del dominio. Se mapean a HTTP status codes en main()
// vía httpx.RegisterDomainError.
var (
	ErrClientNotFound       = errors.New("client not found")
	ErrAccountNotFound      = errors.New("account not found")
	ErrInsufficientFunds    = errors.New("insufficient funds")
	ErrInvalidAmount        = errors.New("invalid amount")
	ErrInvalidCurrency      = errors.New("invalid currency")
	ErrCurrencyMismatch     = errors.New("currency mismatch between accounts")
	ErrSameAccountTransfer  = errors.New("cannot transfer to the same account")
	ErrEmailAlreadyExists   = errors.New("email already exists")
)
