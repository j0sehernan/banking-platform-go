package explainer

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/j0sehernan/banking-platform-go/services/llm-ms/internal/domain"
)

// MockExplainer genera respuestas template deterministas.
// Se usa cuando ANTHROPIC_API_KEY está vacío, así el sistema funciona
// completo en local sin necesidad de credenciales.
type MockExplainer struct{}

func NewMockExplainer() *MockExplainer { return &MockExplainer{} }

func (m *MockExplainer) Model() string { return "mock-explainer-v1" }

func (m *MockExplainer) ExplainTransaction(_ context.Context, tx *domain.TransactionView) (string, error) {
	switch tx.Status {
	case "COMPLETED":
		switch tx.Type {
		case "DEPOSIT":
			return fmt.Sprintf("[mock] El depósito de %s %s se completó correctamente.", tx.Amount.String(), tx.Currency), nil
		case "WITHDRAW":
			return fmt.Sprintf("[mock] El retiro de %s %s se completó correctamente.", tx.Amount.String(), tx.Currency), nil
		case "TRANSFER":
			return fmt.Sprintf("[mock] La transferencia de %s %s se completó correctamente.", tx.Amount.String(), tx.Currency), nil
		}
	case "REJECTED":
		reason := tx.RejectionMsg
		if reason == "" {
			reason = "una validación falló"
		}
		return fmt.Sprintf("[mock] La transacción de %s %s fue rechazada porque %s.", tx.Amount.String(), tx.Currency, reason), nil
	}
	return fmt.Sprintf("[mock] La transacción de %s %s está en estado %s.", tx.Amount.String(), tx.Currency, tx.Status), nil
}

func (m *MockExplainer) ChatStream(_ context.Context, tx *domain.TransactionView, messages []domain.ChatMessage) (Stream, error) {
	last := ""
	if len(messages) > 0 {
		last = messages[len(messages)-1].Content
	}
	response := fmt.Sprintf(
		"[mock LLM] Recibí tu pregunta: %q. Esta transacción es de tipo %s, monto %s %s, estado %s. Para respuestas reales configurá ANTHROPIC_API_KEY.",
		last, tx.Type, tx.Amount.String(), tx.Currency, tx.Status,
	)
	return newMockStream(response), nil
}

// mockStream emite el texto en chunks de palabras para simular streaming.
type mockStream struct {
	chunks []string
	idx    int
}

func newMockStream(text string) *mockStream {
	parts := strings.Split(text, " ")
	chunks := make([]string, 0, len(parts))
	for i, w := range parts {
		if i == 0 {
			chunks = append(chunks, w)
		} else {
			chunks = append(chunks, " "+w)
		}
	}
	return &mockStream{chunks: chunks}
}

func (s *mockStream) Next(ctx context.Context) (string, error) {
	if s.idx >= len(s.chunks) {
		return "", io.EOF
	}
	c := s.chunks[s.idx]
	s.idx++
	return c, nil
}

func (s *mockStream) Close() error { return nil }
