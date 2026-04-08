package explainer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/j0sehernan/banking-platform-go/services/llm-ms/internal/domain"
)

// ClaudeExplainer implementa Explainer usando el SDK oficial de Anthropic.
//
// Usa el modelo configurado (por defecto claude-haiku-4-5: rápido, barato,
// suficiente para responder preguntas sobre una sola transacción).
type ClaudeExplainer struct {
	client anthropic.Client
	model  string
}

// NewClaudeExplainer crea un explainer real con la API key dada.
// El SDK también lee ANTHROPIC_API_KEY del env por default.
func NewClaudeExplainer(apiKey, model string) *ClaudeExplainer {
	if model == "" {
		model = "claude-haiku-4-5"
	}
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &ClaudeExplainer{client: client, model: model}
}

func (c *ClaudeExplainer) Model() string { return c.model }

// ExplainTransaction llama a Claude (sin streaming) para generar la
// explicación inicial de una transacción al consumir el evento.
// Acá no necesitamos streaming porque el resultado se cachea en DB
// y se devuelve completo cuando el front lo pide.
func (c *ClaudeExplainer) ExplainTransaction(ctx context.Context, tx *domain.TransactionView) (string, error) {
	prompt := fmt.Sprintf(
		"Generá una explicación breve (1-2 oraciones) de esta transacción bancaria, en español, en lenguaje natural y empático. NO incluyas IDs ni UUIDs.\n\nDatos de la transacción (JSON):\n%s",
		txContextJSON(tx),
	)

	msg, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 300,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("claude api error: %w", err)
	}

	var text string
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			text += t.Text
		}
	}
	if text == "" {
		return "", errors.New("claude returned empty response")
	}
	return text, nil
}

// ChatStream abre un stream con Claude para el chat scoped.
// Inyecta el system prompt con el contexto de la transacción y las
// reglas de scope, y pasa el historial de mensajes que viene del front.
func (c *ClaudeExplainer) ChatStream(ctx context.Context, tx *domain.TransactionView, messages []domain.ChatMessage) (Stream, error) {
	system := fmt.Sprintf(baseSystemPrompt, txContextJSON(tx))

	// convertir nuestro array a MessageParam del SDK
	apiMessages := make([]anthropic.MessageParam, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case "user":
			apiMessages = append(apiMessages, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
		case "assistant":
			apiMessages = append(apiMessages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content)))
		}
	}
	if len(apiMessages) == 0 {
		return nil, errors.New("no messages provided")
	}

	stream := c.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 600,
		System: []anthropic.TextBlockParam{
			{Text: system},
		},
		Messages: apiMessages,
	})

	return newClaudeStream(stream), nil
}

// claudeStream adapta el stream del SDK a nuestra interface Stream.
// El SDK usa un patrón "iterator" (Next()/Current()), nosotros queremos
// un Next() que devuelva el próximo chunk de texto. Lo envolvemos
// con un canal interno que se llena en una goroutine.
type claudeStream struct {
	chunks chan string
	errCh  chan error
	cancel context.CancelFunc
	once   sync.Once
}

func newClaudeStream(sdkStream *ssestream.Stream[anthropic.MessageStreamEventUnion]) *claudeStream {
	ctx, cancel := context.WithCancel(context.Background())
	cs := &claudeStream{
		chunks: make(chan string, 32),
		errCh:  make(chan error, 1),
		cancel: cancel,
	}

	go func() {
		defer close(cs.chunks)
		defer close(cs.errCh)

		for sdkStream.Next() {
			select {
			case <-ctx.Done():
				return
			default:
			}
			event := sdkStream.Current()
			if delta, ok := event.AsAny().(anthropic.ContentBlockDeltaEvent); ok {
				if textDelta, ok := delta.Delta.AsAny().(anthropic.TextDelta); ok {
					select {
					case cs.chunks <- textDelta.Text:
					case <-ctx.Done():
						return
					}
				}
			}
		}
		if err := sdkStream.Err(); err != nil {
			cs.errCh <- err
		}
	}()

	return cs
}

func (s *claudeStream) Next(ctx context.Context) (string, error) {
	select {
	case chunk, ok := <-s.chunks:
		if !ok {
			// canal cerrado: o terminó el stream o hubo error
			select {
			case err := <-s.errCh:
				if err != nil {
					return "", err
				}
			default:
			}
			return "", io.EOF
		}
		return chunk, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s *claudeStream) Close() error {
	s.once.Do(func() { s.cancel() })
	return nil
}
