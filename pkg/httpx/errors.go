// Package httpx contains shared HTTP helpers: validation, error handling
// with a standard format, generic decode, and a wrapper for handlers
// that return error.
//
// The goal is that each handler stays around ~15 lines without duplicating
// error-handling code in every one.
package httpx

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// Error is our typed error for HTTP responses.
// Implements error and is mapped to a status code in WriteError.
type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Status  int            `json:"-"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Message
}

// NewError builds an Error with a snake_case code and a user-friendly message.
func NewError(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// errorEnvelope is the standard JSON format for an error response.
// Keeping it in a single place lets the front consume it consistently.
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
}

// WriteError serializes an error as JSON with the appropriate status.
// If the error is a *Error it uses its status. Otherwise it tries to
// map common errors (registered via RegisterDomainError). As a last
// resort, returns 500.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	requestID := middleware.GetReqID(r.Context())

	var apiErr *Error
	if errors.As(err, &apiErr) {
		writeJSON(w, apiErr.Status, errorEnvelope{
			Error: errorBody{
				Code:      apiErr.Code,
				Message:   apiErr.Message,
				Details:   apiErr.Details,
				RequestID: requestID,
			},
		})
		return
	}

	// Mapping of registered domain errors
	if mapped := lookupDomainError(err); mapped != nil {
		writeJSON(w, mapped.Status, errorEnvelope{
			Error: errorBody{
				Code:      mapped.Code,
				Message:   mapped.Message,
				RequestID: requestID,
			},
		})
		return
	}

	// Default: 500
	slog.ErrorContext(r.Context(), "unhandled error", "err", err, "request_id", requestID)
	writeJSON(w, http.StatusInternalServerError, errorEnvelope{
		Error: errorBody{
			Code:      "internal_error",
			Message:   "An unexpected error occurred",
			RequestID: requestID,
		},
	})
}

// WriteJSON serializes any value as JSON with the given status code.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	writeJSON(w, status, payload)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("json encode error", "err", err)
	}
}

// ===== Domain error registration =====
//
// Each microservice can register its typed errors with their HTTP
// mapping at the start of main(). That way handlers just `return err`
// and the wrapper takes care of the status code.

var domainErrors = map[error]*Error{}

// RegisterDomainError maps a domain error to an HTTP Error.
// Call at program startup.
func RegisterDomainError(domainErr error, apiErr *Error) {
	domainErrors[domainErr] = apiErr
}

func lookupDomainError(err error) *Error {
	for domainErr, apiErr := range domainErrors {
		if errors.Is(err, domainErr) {
			return apiErr
		}
	}
	return nil
}
