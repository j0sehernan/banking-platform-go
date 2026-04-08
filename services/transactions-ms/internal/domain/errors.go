package domain

import "errors"

var (
	ErrTransactionNotFound     = errors.New("transaction not found")
	ErrInvalidTransactionType  = errors.New("invalid transaction type")
	ErrDuplicateIdempotencyKey = errors.New("duplicate idempotency key")
	ErrInvalidStateTransition  = errors.New("invalid state transition")
	ErrSameAccountTransfer     = errors.New("cannot transfer to the same account")
)
