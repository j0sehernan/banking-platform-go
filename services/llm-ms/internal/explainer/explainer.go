// Package explainer abstrae el LLM detrás de una interface.
// Tenemos dos implementaciones: ClaudeExplainer (real) y MockExplainer
// (templates deterministas para tests/local sin API key).
package explainer

import (
	"context"
	"encoding/json"

	"github.com/j0sehernan/banking-platform-go/services/llm-ms/internal/domain"
)

// Explainer es la abstracción del LLM.
//
// Tener una interface en vez de usar Claude directamente nos permite:
//  1. Tests sin tocar la API real
//  2. Fallback automático a mock si no hay ANTHROPIC_API_KEY
//  3. Cambiar de proveedor (OpenAI, Gemini, etc.) sin tocar el código de negocio
type Explainer interface {
	// ExplainTransaction genera una explicación corta y autocontenida
	// de una transacción. La llama el consumer al recibir el evento final.
	ExplainTransaction(ctx context.Context, tx *domain.TransactionView) (string, error)

	// ChatStream abre un stream de respuesta para el chat scoped a una
	// transacción. El usuario manda mensajes y Claude responde acotado
	// a esa transacción específica.
	ChatStream(ctx context.Context, tx *domain.TransactionView, messages []domain.ChatMessage) (Stream, error)

	// Model devuelve el nombre del modelo (para guardar en la DB).
	Model() string
}

// Stream representa un stream incremental de chunks de texto del LLM.
// Cada implementación lo envuelve sobre su propio mecanismo (SSE para Claude,
// canal con texto para mock).
type Stream interface {
	// Next devuelve el próximo chunk de texto. Bloquea hasta que haya uno
	// o el stream termine. Devuelve "", io.EOF cuando termina.
	Next(ctx context.Context) (string, error)

	// Close libera recursos del stream.
	Close() error
}

// txContextJSON serializa una TransactionView a JSON autodescriptivo
// para inyectar en el system prompt.
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

const baseSystemPrompt = `Eres un asistente bancario especializado. Tu única función es responder preguntas sobre la siguiente transacción específica:

%s

Reglas estrictas que debes seguir SIEMPRE:
1. Responde únicamente sobre esta transacción. No discutas otras cuentas, otras transacciones ni temas no relacionados.
2. Si el usuario pregunta algo fuera de scope (otra transacción, finanzas en general, temas no bancarios), declina amablemente y redirígelo a esta transacción.
3. No inventes datos que no estén en el JSON. Si no sabes algo concreto, dilo.
4. Responde en español, de forma clara, breve y profesional.
5. Si la transacción está rechazada (REJECTED), explica la razón con empatía.
6. No incluyas IDs de UUIDs en tus respuestas a menos que el usuario los pida explícitamente.`
