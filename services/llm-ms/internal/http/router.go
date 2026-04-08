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
	// NOTE: 5min timeout to support long LLM streams
	r.Use(middleware.Timeout(5 * time.Minute))
	r.Use(httpx.CORS)
	r.Use(httpx.SlogLogger(logger))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	r.Get("/transactions/{id}/explanation", httpx.Wrap(h.GetExplanation))

	// Chat does NOT use httpx.Wrap because it handles SSE headers manually
	r.Post("/chat", h.Chat)

	return r
}
