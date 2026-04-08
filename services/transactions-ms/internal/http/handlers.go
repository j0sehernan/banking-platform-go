package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/j0sehernan/banking-platform-go/pkg/httpx"
	"github.com/j0sehernan/banking-platform-go/services/transactions-ms/internal/domain"
	"github.com/j0sehernan/banking-platform-go/services/transactions-ms/internal/service"
	"github.com/shopspring/decimal"
)

type Handler struct {
	svc *service.TransactionService
}

func NewHandler(svc *service.TransactionService) *Handler {
	return &Handler{svc: svc}
}

// CreateDeposit POST /transactions/deposit
func (h *Handler) CreateDeposit(w http.ResponseWriter, r *http.Request) error {
	var req DepositRequest
	if err := httpx.DecodeAndValidate(r, &req); err != nil {
		return err
	}
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil || !amount.IsPositive() {
		return httpx.NewError(http.StatusUnprocessableEntity, "invalid_amount", "El monto debe ser positivo")
	}
	to, _ := uuid.Parse(req.ToAccountID)

	tx, err := h.svc.Create(r.Context(), service.CreateCommand{
		Type:           domain.TypeDeposit,
		ToAccountID:    &to,
		Amount:         amount,
		Currency:       req.Currency,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusAccepted, transactionToResponse(tx))
	return nil
}

// CreateWithdraw POST /transactions/withdraw
func (h *Handler) CreateWithdraw(w http.ResponseWriter, r *http.Request) error {
	var req WithdrawRequest
	if err := httpx.DecodeAndValidate(r, &req); err != nil {
		return err
	}
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil || !amount.IsPositive() {
		return httpx.NewError(http.StatusUnprocessableEntity, "invalid_amount", "El monto debe ser positivo")
	}
	from, _ := uuid.Parse(req.FromAccountID)

	tx, err := h.svc.Create(r.Context(), service.CreateCommand{
		Type:           domain.TypeWithdraw,
		FromAccountID:  &from,
		Amount:         amount,
		Currency:       req.Currency,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusAccepted, transactionToResponse(tx))
	return nil
}

// CreateTransfer POST /transactions/transfer
func (h *Handler) CreateTransfer(w http.ResponseWriter, r *http.Request) error {
	var req TransferRequest
	if err := httpx.DecodeAndValidate(r, &req); err != nil {
		return err
	}
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil || !amount.IsPositive() {
		return httpx.NewError(http.StatusUnprocessableEntity, "invalid_amount", "El monto debe ser positivo")
	}
	from, _ := uuid.Parse(req.FromAccountID)
	to, _ := uuid.Parse(req.ToAccountID)

	tx, err := h.svc.Create(r.Context(), service.CreateCommand{
		Type:           domain.TypeTransfer,
		FromAccountID:  &from,
		ToAccountID:    &to,
		Amount:         amount,
		Currency:       req.Currency,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusAccepted, transactionToResponse(tx))
	return nil
}

// GetTransaction GET /transactions/{id}
func (h *Handler) GetTransaction(w http.ResponseWriter, r *http.Request) error {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return httpx.NewError(http.StatusBadRequest, "invalid_id", "El id no es un UUID válido")
	}
	tx, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, transactionToResponse(tx))
	return nil
}

// ListTransactions GET /transactions
func (h *Handler) ListTransactions(w http.ResponseWriter, r *http.Request) error {
	txs, err := h.svc.ListAll(r.Context())
	if err != nil {
		return err
	}
	resp := make([]TransactionResponse, 0, len(txs))
	for i := range txs {
		resp = append(resp, transactionToResponse(&txs[i]))
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
	return nil
}

// ListByAccount GET /accounts/{id}/transactions
func (h *Handler) ListByAccount(w http.ResponseWriter, r *http.Request) error {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return httpx.NewError(http.StatusBadRequest, "invalid_id", "El id no es un UUID válido")
	}
	txs, err := h.svc.ListByAccount(r.Context(), id)
	if err != nil {
		return err
	}
	resp := make([]TransactionResponse, 0, len(txs))
	for i := range txs {
		resp = append(resp, transactionToResponse(&txs[i]))
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
	return nil
}

func transactionToResponse(t *domain.Transaction) TransactionResponse {
	resp := TransactionResponse{
		ID:             t.ID.String(),
		Type:           t.Type,
		Amount:         t.Amount.String(),
		Currency:       t.Currency,
		Status:         t.Status,
		IdempotencyKey: t.IdempotencyKey,
		RejectionCode:  t.RejectionCode,
		RejectionMsg:   t.RejectionMsg,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
	}
	if t.FromAccountID != nil {
		s := t.FromAccountID.String()
		resp.FromAccountID = &s
	}
	if t.ToAccountID != nil {
		s := t.ToAccountID.String()
		resp.ToAccountID = &s
	}
	return resp
}
