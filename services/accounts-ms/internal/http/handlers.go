package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/j0sehernan/banking-platform-go/pkg/httpx"
	"github.com/j0sehernan/banking-platform-go/services/accounts-ms/internal/domain"
	"github.com/j0sehernan/banking-platform-go/services/accounts-ms/internal/service"
)

type Handler struct {
	svc *service.AccountService
}

func NewHandler(svc *service.AccountService) *Handler {
	return &Handler{svc: svc}
}

// CreateClient POST /clients
func (h *Handler) CreateClient(w http.ResponseWriter, r *http.Request) error {
	var req CreateClientRequest
	if err := httpx.DecodeAndValidate(r, &req); err != nil {
		return err
	}

	client, err := h.svc.CreateClient(r.Context(), req.Name, req.Email)
	if err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusCreated, ClientResponse{
		ID:        client.ID.String(),
		Name:      client.Name,
		Email:     client.Email,
		CreatedAt: client.CreatedAt,
	})
	return nil
}

// GetClient GET /clients/{id}
func (h *Handler) GetClient(w http.ResponseWriter, r *http.Request) error {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		return httpx.NewError(http.StatusBadRequest, "invalid_id", "El id no es un UUID válido")
	}

	client, err := h.svc.GetClient(r.Context(), id)
	if err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusOK, ClientResponse{
		ID:        client.ID.String(),
		Name:      client.Name,
		Email:     client.Email,
		CreatedAt: client.CreatedAt,
	})
	return nil
}

// CreateAccount POST /accounts
func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) error {
	var req CreateAccountRequest
	if err := httpx.DecodeAndValidate(r, &req); err != nil {
		return err
	}

	clientID, _ := uuid.Parse(req.ClientID) // ya validado por validator
	acc, err := h.svc.CreateAccount(r.Context(), clientID, req.Currency)
	if err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusCreated, accountToResponse(acc))
	return nil
}

// GetAccount GET /accounts/{id}
func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) error {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return httpx.NewError(http.StatusBadRequest, "invalid_id", "El id no es un UUID válido")
	}

	acc, err := h.svc.GetAccount(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, accountToResponse(acc))
	return nil
}

// GetBalance GET /accounts/{id}/balance
func (h *Handler) GetBalance(w http.ResponseWriter, r *http.Request) error {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return httpx.NewError(http.StatusBadRequest, "invalid_id", "El id no es un UUID válido")
	}

	acc, err := h.svc.GetAccount(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, BalanceResponse{
		AccountID: acc.ID.String(),
		Balance:   acc.Balance.String(),
		Currency:  acc.Currency,
	})
	return nil
}

// ListAccounts GET /accounts
func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) error {
	accounts, err := h.svc.ListAccounts(r.Context())
	if err != nil {
		return err
	}
	resp := make([]AccountResponse, 0, len(accounts))
	for _, a := range accounts {
		resp = append(resp, accountToResponse(&a))
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
	return nil
}

func accountToResponse(a *domain.Account) AccountResponse {
	return AccountResponse{
		ID:        a.ID.String(),
		ClientID:  a.ClientID.String(),
		Currency:  a.Currency,
		Balance:   a.Balance.String(),
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
}
