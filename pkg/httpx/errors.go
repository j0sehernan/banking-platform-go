// Package httpx contiene helpers HTTP compartidos: validación, manejo de
// errores con formato estándar, decode genérico, y un wrapper para handlers
// que devuelven error.
//
// La idea es que cada handler quede en ~15 líneas sin código de error
// duplicado en cada uno.
package httpx

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// Error es nuestro error tipado para responses HTTP.
// Implementa error y se mapea a status codes en WriteError.
type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Status  int            `json:"-"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Message
}

// NewError construye un Error con código snake_case y mensaje user-friendly.
func NewError(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// errorEnvelope es el formato JSON estándar de error response.
// Mantenerlo en un solo lugar facilita que el front lo consuma siempre igual.
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
}

// WriteError serializa un error como JSON con el status apropiado.
// Si el error es un *Error usa su status. Si no, intenta mapear errores
// comunes (registrados con RegisterDomainError). Como último recurso, 500.
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

	// Mapeo de errores de dominio registrados
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
			Message:   "Ocurrió un error inesperado",
			RequestID: requestID,
		},
	})
}

// WriteJSON serializa cualquier valor como JSON con status code dado.
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

// ===== Registro de errores de dominio =====
//
// Cada microservicio puede registrar sus errores tipados con su mapeo
// HTTP correspondiente al inicio de main(). Así el handler solo hace
// `return err` y el wrapper se encarga del status code.

var domainErrors = map[error]*Error{}

// RegisterDomainError mapea un error de dominio a un Error HTTP.
// Llamar al inicio del programa.
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
