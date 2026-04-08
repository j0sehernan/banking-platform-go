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

// ClaudeExplainer implements Explainer using the official Anthropic SDK.
//
// It uses the configured model (default claude-haiku-4-5: fast, cheap,
// good enough to answer questions about a single transaction).
type ClaudeExplainer struct {
	client anthropic.Client
	model  string
}

// NewClaudeExplainer creates a real explainer with the given API key.
// The SDK also reads ANTHROPIC_API_KEY from env by default.
func NewClaudeExplainer(apiKey, model string) *ClaudeExplainer {
	if model == "" {
		model = "claude-haiku-4-5"
	}
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &ClaudeExplainer{client: client, model: model}
}

func (c *ClaudeExplainer) Model() string { return c.model }

// ExplainTransaction calls Claude (without streaming) to generate the
// initial explanation of a transaction when consuming the event.
// We don't need streaming here because the result is cached in DB
// and returned in full when the front asks for it.
func (c *ClaudeExplainer) ExplainTransaction(ctx context.Context, tx *domain.TransactionView) (string, error) {
	prompt := fmt.Sprintf(
		"Generate a short (1-2 sentences) explanation of this banking transaction, in plain English, in a natural and empathetic tone. Do NOT include UUIDs.\n\nTransaction data (JSON):\n%s",
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

// ChatStream opens a stream with Claude for the scoped chat.
// Injects the system prompt with the transaction context and the
// scope rules, and passes the message history coming from the front.
func (c *ClaudeExplainer) ChatStream(ctx context.Context, tx *domain.TransactionView, messages []domain.ChatMessage) (Stream, error) {
	system := fmt.Sprintf(baseSystemPrompt, txContextJSON(tx))

	// convert our array to the SDK's MessageParam
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

// claudeStream adapts the SDK stream to our Stream interface.
// The SDK uses an "iterator" pattern (Next()/Current()), we want
// a Next() that returns the next text chunk. We wrap it with an
// internal channel filled by a goroutine.
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
			// channel closed: stream ended or there was an error
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
