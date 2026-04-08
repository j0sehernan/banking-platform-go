package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/j0sehernan/banking-platform-go/pkg/httpx"
)

// NewRouter wires up the router with all the global middlewares and routes.
func NewRouter(h *Handler, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()

	// middlewares
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(httpx.CORS)
	r.Use(httpx.SlogLogger(logger))

	// healthcheck
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// API routes
	r.Post("/clients", httpx.Wrap(h.CreateClient))
	r.Get("/clients", httpx.Wrap(h.ListClients))
	r.Get("/clients/{id}", httpx.Wrap(h.GetClient))

	r.Post("/accounts", httpx.Wrap(h.CreateAccount))
	r.Get("/accounts", httpx.Wrap(h.ListAccounts))
	r.Get("/accounts/{id}", httpx.Wrap(h.GetAccount))
	r.Get("/accounts/{id}/balance", httpx.Wrap(h.GetBalance))

	return r
}
