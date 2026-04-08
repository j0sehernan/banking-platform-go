package http

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/j0sehernan/banking-platform-go/pkg/httpx"
	"github.com/j0sehernan/banking-platform-go/services/llm-ms/internal/service"
)

type Handler struct {
	svc *service.LLMService
}

func NewHandler(svc *service.LLMService) *Handler {
	return &Handler{svc: svc}
}

// GetExplanation GET /transactions/{id}/explanation
// Returns the cached explanation generated when consuming the event.
func (h *Handler) GetExplanation(w http.ResponseWriter, r *http.Request) error {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return httpx.NewError(http.StatusBadRequest, "invalid_id", "The id is not a valid UUID")
	}

	exp, err := h.svc.GetExplanation(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, ExplanationResponse{
		TxID:        exp.TxID.String(),
		Explanation: exp.Text,
		Model:       exp.Model,
		GeneratedAt: exp.GeneratedAt,
	})
	return nil
}

// Chat POST /chat
//
// Endpoint with SSE streaming. The front sends the context (tx_id) and
// the message history; the backend reads the transaction from its local
// materialized view, builds the system prompt with context grounding,
// and proxies Claude's stream to the client via SSE.
//
// This function does NOT use httpx.Wrap because it writes headers
// manually (Content-Type: text/event-stream) before any WriteJSON.
func (h *Handler) Chat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := httpx.DecodeAndValidate(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	txID, _ := uuid.Parse(req.TxID) // already validated by validator

	view, err := h.svc.GetTransactionView(r.Context(), txID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.WriteError(w, r, errors.New("streaming not supported by server"))
		return
	}

	// SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disables buffering on proxies
	w.WriteHeader(http.StatusOK)

	stream, err := h.svc.ChatStream(r.Context(), view, req.Messages)
	if err != nil {
		writeSSEEvent(w, "error", fmt.Sprintf(`{"message":%q}`, err.Error()))
		flusher.Flush()
		return
	}
	defer stream.Close()

	for {
		chunk, err := stream.Next(r.Context())
		if errors.Is(err, io.EOF) {
			writeSSEEvent(w, "done", `{}`)
			flusher.Flush()
			return
		}
		if err != nil {
			writeSSEEvent(w, "error", fmt.Sprintf(`{"message":%q}`, err.Error()))
			flusher.Flush()
			return
		}
		writeSSEData(w, chunk)
		flusher.Flush()
	}
}

// writeSSEData writes a default SSE event with `data: ...`.
// Simple JSON escaping is enough for plain text.
func writeSSEData(w http.ResponseWriter, text string) {
	// Escape the text to JSON so quotes and newlines don't break SSE format
	escaped := jsonEscapeString(text)
	fmt.Fprintf(w, "data: {\"text\":%s}\n\n", escaped)
}

// writeSSEEvent writes an SSE event with a custom name (event: name).
func writeSSEEvent(w http.ResponseWriter, name, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data)
}

// jsonEscapeString turns a string into its JSON representation with quotes.
func jsonEscapeString(s string) string {
	var b []byte
	b = append(b, '"')
	for _, r := range s {
		switch r {
		case '"':
			b = append(b, '\\', '"')
		case '\\':
			b = append(b, '\\', '\\')
		case '\n':
			b = append(b, '\\', 'n')
		case '\r':
			b = append(b, '\\', 'r')
		case '\t':
			b = append(b, '\\', 't')
		default:
			if r < 0x20 {
				b = append(b, fmt.Sprintf("\\u%04x", r)...)
			} else {
				b = append(b, []byte(string(r))...)
			}
		}
	}
	b = append(b, '"')
	return string(b)
}
