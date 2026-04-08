package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/j0sehernan/banking-platform-go/pkg/httpx"
)

func NewRouter(h *Handler, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(httpx.CORS)
	r.Use(httpx.SlogLogger(logger))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	r.Post("/transactions/deposit", httpx.Wrap(h.CreateDeposit))
	r.Post("/transactions/withdraw", httpx.Wrap(h.CreateWithdraw))
	r.Post("/transactions/transfer", httpx.Wrap(h.CreateTransfer))
	r.Get("/transactions", httpx.Wrap(h.ListTransactions))
	r.Get("/transactions/{id}", httpx.Wrap(h.GetTransaction))
	r.Get("/accounts/{id}/transactions", httpx.Wrap(h.ListByAccount))

	return r
}
