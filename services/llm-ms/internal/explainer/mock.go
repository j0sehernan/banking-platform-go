package explainer

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/j0sehernan/banking-platform-go/services/llm-ms/internal/domain"
)

// MockExplainer generates deterministic template responses.
// It is used when ANTHROPIC_API_KEY is empty so the system can run
// completely in local without credentials.
type MockExplainer struct{}

func NewMockExplainer() *MockExplainer { return &MockExplainer{} }

func (m *MockExplainer) Model() string { return "mock-explainer-v1" }

func (m *MockExplainer) ExplainTransaction(_ context.Context, tx *domain.TransactionView) (string, error) {
	switch tx.Status {
	case "COMPLETED":
		switch tx.Type {
		case "DEPOSIT":
			return fmt.Sprintf("[mock] The deposit of %s %s was completed successfully.", tx.Amount.String(), tx.Currency), nil
		case "WITHDRAW":
			return fmt.Sprintf("[mock] The withdrawal of %s %s was completed successfully.", tx.Amount.String(), tx.Currency), nil
		case "TRANSFER":
			return fmt.Sprintf("[mock] The transfer of %s %s was completed successfully.", tx.Amount.String(), tx.Currency), nil
		}
	case "REJECTED":
		reason := tx.RejectionMsg
		if reason == "" {
			reason = "a validation failed"
		}
		return fmt.Sprintf("[mock] The transaction of %s %s was rejected because %s.", tx.Amount.String(), tx.Currency, reason), nil
	}
	return fmt.Sprintf("[mock] The transaction of %s %s is in status %s.", tx.Amount.String(), tx.Currency, tx.Status), nil
}

func (m *MockExplainer) ChatStream(_ context.Context, tx *domain.TransactionView, messages []domain.ChatMessage) (Stream, error) {
	last := ""
	if len(messages) > 0 {
		last = messages[len(messages)-1].Content
	}
	response := fmt.Sprintf(
		"[mock LLM] I received your question: %q. This transaction is of type %s, amount %s %s, status %s. For real answers, set ANTHROPIC_API_KEY.",
		last, tx.Type, tx.Amount.String(), tx.Currency, tx.Status,
	)
	return newMockStream(response), nil
}

// mockStream emits the text in word-sized chunks to simulate streaming.
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
