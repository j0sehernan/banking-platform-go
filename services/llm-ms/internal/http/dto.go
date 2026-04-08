package http

import (
	"time"

	"github.com/j0sehernan/banking-platform-go/services/llm-ms/internal/domain"
)

// ChatRequest es el body del POST /chat.
// El front mantiene el historial completo y lo manda en cada request.
// Stateless del lado del backend.
type ChatRequest struct {
	TxID     string               `json:"tx_id"    validate:"required,uuid"`
	Messages []domain.ChatMessage `json:"messages" validate:"required,min=1,dive"`
}

type ExplanationResponse struct {
	TxID        string    `json:"tx_id"`
	Explanation string    `json:"explanation"`
	Model       string    `json:"model"`
	GeneratedAt time.Time `json:"generated_at"`
}
