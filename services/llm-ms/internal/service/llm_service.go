// Package service has the use cases of llm-ms.
// This service has NO banking logic — it only mirrors transactions
// (materialized view) and delegates to the Explainer to generate text.
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

// GetExplanation returns the cached explanation of a transaction.
// If it doesn't exist, returns ErrExplanationNotFound (404 to the client).
func (s *LLMService) GetExplanation(ctx context.Context, txID uuid.UUID) (*domain.Explanation, error) {
	r := repo.NewExplanationRepo(s.pool)
	return r.GetByTxID(ctx, txID)
}

// GetTransactionView returns the local copy of a transaction.
// Used by the chat handler to build the context.
func (s *LLMService) GetTransactionView(ctx context.Context, txID uuid.UUID) (*domain.TransactionView, error) {
	r := repo.NewTransactionsViewRepo(s.pool)
	return r.GetByID(ctx, txID)
}

// ChatStream opens a stream with the LLM scoped to a transaction.
// The caller (HTTP handler) must iterate the stream and write SSE.
func (s *LLMService) ChatStream(ctx context.Context, tx *domain.TransactionView, messages []domain.ChatMessage) (explainer.Stream, error) {
	return s.explainer.ChatStream(ctx, tx, messages)
}

// Explainer returns the explainer (the consumer needs it to generate
// the initial explanation when receiving the transaction event).
func (s *LLMService) Explainer() explainer.Explainer {
	return s.explainer
}
