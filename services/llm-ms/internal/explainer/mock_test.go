package explainer

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/j0sehernan/banking-platform-go/services/llm-ms/internal/domain"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestMockExplainer_ExplainTransaction(t *testing.T) {
	m := NewMockExplainer()
	tx := &domain.TransactionView{
		ID:       uuid.New(),
		Type:     "TRANSFER",
		Status:   "REJECTED",
		Amount:   decimal.RequireFromString("500.00"),
		Currency: "USD",
		RejectionMsg: "no había saldo suficiente",
	}

	out, err := m.ExplainTransaction(context.Background(), tx)
	assert.NoError(t, err)
	assert.Contains(t, out, "rechazada")
	assert.Contains(t, out, "500")
	assert.Contains(t, out, "USD")
}

func TestMockExplainer_ChatStream(t *testing.T) {
	m := NewMockExplainer()
	tx := &domain.TransactionView{
		Type:     "DEPOSIT",
		Status:   "COMPLETED",
		Amount:   decimal.RequireFromString("100"),
		Currency: "USD",
	}
	messages := []domain.ChatMessage{
		{Role: "user", Content: "qué pasó?"},
	}

	stream, err := m.ChatStream(context.Background(), tx, messages)
	assert.NoError(t, err)
	defer stream.Close()

	var collected string
	for {
		chunk, err := stream.Next(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		assert.NoError(t, err)
		collected += chunk
	}

	assert.Contains(t, collected, "mock")
	assert.Contains(t, collected, "qué pasó?")
}
