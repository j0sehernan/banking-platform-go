// Package http contiene los handlers HTTP de accounts-ms.
// Las request structs llevan tags de validación que el helper
// httpx.DecodeAndValidate procesa automáticamente.
package http

import "time"

type CreateClientRequest struct {
	Name  string `json:"name"  validate:"required,min=1,max=255"`
	Email string `json:"email" validate:"required,email,max=255"`
}

type CreateAccountRequest struct {
	ClientID string `json:"client_id" validate:"required,uuid"`
	Currency string `json:"currency"  validate:"required,oneof=USD EUR ARS"`
}

type ClientResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type AccountResponse struct {
	ID        string    `json:"id"`
	ClientID  string    `json:"client_id"`
	Currency  string    `json:"currency"`
	Balance   string    `json:"balance"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type BalanceResponse struct {
	AccountID string `json:"account_id"`
	Balance   string `json:"balance"`
	Currency  string `json:"currency"`
}
