// Package explainer abstracts the LLM behind an interface.
// We have two implementations: ClaudeExplainer (real) and MockExplainer
// (deterministic templates for tests/local without an API key).
package explainer

import (
	"context"
	"encoding/json"

	"github.com/j0sehernan/banking-platform-go/services/llm-ms/internal/domain"
)

// Explainer is the LLM abstraction.
//
// Having an interface instead of using Claude directly allows us to:
//  1. Run tests without hitting the real API
//  2. Automatically fall back to a mock when ANTHROPIC_API_KEY is not set
//  3. Swap providers (OpenAI, Gemini, etc.) without touching business code
type Explainer interface {
	// ExplainTransaction generates a short, self-contained explanation
	// of a transaction. Called by the consumer when receiving the final event.
	ExplainTransaction(ctx context.Context, tx *domain.TransactionView) (string, error)

	// ChatStream opens a streaming response for the chat scoped to a
	// transaction. The user sends messages and Claude replies bound to
	// that specific transaction.
	ChatStream(ctx context.Context, tx *domain.TransactionView, messages []domain.ChatMessage) (Stream, error)

	// Model returns the model name (used to persist with the explanation).
	Model() string
}

// Stream represents an incremental stream of text chunks from the LLM.
// Each implementation wraps it on its own mechanism (SSE for Claude,
// channel-based for mock).
type Stream interface {
	// Next returns the next text chunk. Blocks until one is available
	// or the stream ends. Returns "", io.EOF when finished.
	Next(ctx context.Context) (string, error)

	// Close releases stream resources.
	Close() error
}

// txContextJSON serializes a TransactionView into self-describing JSON
// to inject into the system prompt.
func txContextJSON(tx *domain.TransactionView) string {
	type fromTo struct {
		ID string `json:"id,omitempty"`
	}
	type ctxOut struct {
		ID            string  `json:"id"`
		Type          string  `json:"type"`
		Status        string  `json:"status"`
		Amount        string  `json:"amount"`
		Currency      string  `json:"currency"`
		FromAccount   *fromTo `json:"from_account,omitempty"`
		ToAccount     *fromTo `json:"to_account,omitempty"`
		RejectionCode string  `json:"rejection_code,omitempty"`
		RejectionMsg  string  `json:"rejection_message,omitempty"`
		CreatedAt     string  `json:"created_at"`
	}
	out := ctxOut{
		ID:            tx.ID.String(),
		Type:          tx.Type,
		Status:        tx.Status,
		Amount:        tx.Amount.String(),
		Currency:      tx.Currency,
		RejectionCode: tx.RejectionCode,
		RejectionMsg:  tx.RejectionMsg,
		CreatedAt:     tx.CreatedAt.Format("2006-01-02 15:04:05 UTC"),
	}
	if tx.FromAccountID != nil {
		out.FromAccount = &fromTo{ID: tx.FromAccountID.String()}
	}
	if tx.ToAccountID != nil {
		out.ToAccount = &fromTo{ID: tx.ToAccountID.String()}
	}
	bytes, _ := json.MarshalIndent(out, "", "  ")
	return string(bytes)
}

const baseSystemPrompt = `You are a specialized banking assistant. Your only job is to answer questions about the following specific transaction:

%s

Strict rules you must always follow:
1. Answer only about this transaction. Do not discuss other accounts, other transactions, or unrelated topics.
2. If the user asks something out of scope (other transactions, general finance, non-banking topics), decline politely and steer them back to this transaction.
3. Do not make up data that is not in the JSON. If you do not know something concrete, say so.
4. Reply in clear, brief, professional English.
5. If the transaction is REJECTED, explain the reason with empathy.
6. Do not include UUID identifiers in your replies unless the user explicitly asks for them.`
