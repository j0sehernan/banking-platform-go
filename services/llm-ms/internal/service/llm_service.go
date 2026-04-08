// Package service tiene los casos de uso del llm-ms.
// Este servicio NO tiene lógica bancaria — solo refleja transacciones
// (vista materializada) y delega al Explainer para generar texto.
package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/j0sehernan/banking-platform-go/services/llm-ms/internal/domain"
	"github.com/j0sehernan/banking-platform-go/services/llm-ms/internal/explainer"
	"github.com/j0sehernan/banking-platform-go/services/llm-ms/internal/repo"
	"github.com/jackc/pgx/v5/pgxpool"
)

type LLMService struct {
	pool      *pgxpool.Pool
	explainer explainer.Explainer
}

func NewLLMService(pool *pgxpool.Pool, exp explainer.Explainer) *LLMService {
	return &LLMService{pool: pool, explainer: exp}
}

// GetExplanation devuelve la explicación cacheada de una transacción.
// Si no existe, devuelve ErrExplanationNotFound (404 al cliente).
func (s *LLMService) GetExplanation(ctx context.Context, txID uuid.UUID) (*domain.Explanation, error) {
	r := repo.NewExplanationRepo(s.pool)
	return r.GetByTxID(ctx, txID)
}

// GetTransactionView devuelve la copia local de una transacción.
// Lo usa el chat handler para construir el contexto.
func (s *LLMService) GetTransactionView(ctx context.Context, txID uuid.UUID) (*domain.TransactionView, error) {
	r := repo.NewTransactionsViewRepo(s.pool)
	return r.GetByID(ctx, txID)
}

// ChatStream abre un stream con el LLM scoped a una transacción.
// El caller (handler HTTP) debe iterar el stream y escribir SSE.
func (s *LLMService) ChatStream(ctx context.Context, tx *domain.TransactionView, messages []domain.ChatMessage) (explainer.Stream, error) {
	return s.explainer.ChatStream(ctx, tx, messages)
}

// Explainer devuelve el explainer (lo necesita el consumer para
// generar la explicación inicial al recibir el evento de transacción).
func (s *LLMService) Explainer() explainer.Explainer {
	return s.explainer
}
