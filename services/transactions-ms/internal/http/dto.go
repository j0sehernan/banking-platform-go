package http

import "time"

// I accept amount as string to keep decimal precision in JSON.
// validator/v10 with the `numeric` tag checks the format.

type DepositRequest struct {
	ToAccountID    string `json:"to_account_id"   validate:"required,uuid"`
	Amount         string `json:"amount"          validate:"required,numeric"`
	Currency       string `json:"currency"        validate:"required,oneof=USD EUR ARS"`
	IdempotencyKey string `json:"idempotency_key" validate:"required,min=8,max=128"`
}

type WithdrawRequest struct {
	FromAccountID  string `json:"from_account_id" validate:"required,uuid"`
	Amount         string `json:"amount"          validate:"required,numeric"`
	Currency       string `json:"currency"        validate:"required,oneof=USD EUR ARS"`
	IdempotencyKey string `json:"idempotency_key" validate:"required,min=8,max=128"`
}

type TransferRequest struct {
	FromAccountID  string `json:"from_account_id" validate:"required,uuid"`
	ToAccountID    string `json:"to_account_id"   validate:"required,uuid,nefield=FromAccountID"`
	Amount         string `json:"amount"          validate:"required,numeric"`
	Currency       string `json:"currency"        validate:"required,oneof=USD EUR ARS"`
	IdempotencyKey string `json:"idempotency_key" validate:"required,min=8,max=128"`
}

type TransactionResponse struct {
	ID             string    `json:"id"`
	Type           string    `json:"type"`
	FromAccountID  *string   `json:"from_account_id,omitempty"`
	ToAccountID    *string   `json:"to_account_id,omitempty"`
	Amount         string    `json:"amount"`
	Currency       string    `json:"currency"`
	Status         string    `json:"status"`
	IdempotencyKey string    `json:"idempotency_key"`
	RejectionCode  string    `json:"rejection_code,omitempty"`
	RejectionMsg   string    `json:"rejection_msg,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
