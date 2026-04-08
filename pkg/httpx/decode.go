package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate = validator.New(validator.WithRequiredStructEnabled())

// DecodeAndValidate parsea el body JSON al tipo destino y corre la
// validación con go-playground/validator. Devuelve un *Error con
// status apropiado si algo falla.
//
// Uso típico en un handler:
//
//	var req CreateAccountRequest
//	if err := httpx.DecodeAndValidate(r, &req); err != nil {
//	    return err
//	}
func DecodeAndValidate[T any](r *http.Request, dst *T) error {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		return NewError(http.StatusBadRequest, "invalid_json",
			"El cuerpo del request no es JSON válido")
	}

	if err := validate.Struct(dst); err != nil {
		return validationError(err)
	}
	return nil
}

// Validate corre solo la validación (cuando el decode lo hizo otro).
func Validate(v any) error {
	if err := validate.Struct(v); err != nil {
		return validationError(err)
	}
	return nil
}

func validationError(err error) error {
	var ves validator.ValidationErrors
	if !errors.As(err, &ves) {
		return NewError(http.StatusUnprocessableEntity, "validation_failed", err.Error())
	}

	fields := make([]map[string]any, 0, len(ves))
	for _, fe := range ves {
		fields = append(fields, map[string]any{
			"field":   toSnake(fe.Field()),
			"rule":    fe.Tag(),
			"message": humanize(fe),
		})
	}

	return &Error{
		Status:  http.StatusUnprocessableEntity,
		Code:    "validation_failed",
		Message: "El request tiene campos inválidos",
		Details: map[string]any{"fields": fields},
	}
}

func humanize(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "es requerido"
	case "uuid":
		return "debe ser un UUID válido"
	case "email":
		return "debe ser un email válido"
	case "gt":
		return "debe ser mayor que " + fe.Param()
	case "gte":
		return "debe ser mayor o igual a " + fe.Param()
	case "min":
		return "longitud mínima: " + fe.Param()
	case "max":
		return "longitud máxima: " + fe.Param()
	case "oneof":
		return "debe ser uno de: " + fe.Param()
	case "nefield":
		return "debe ser distinto a " + fe.Param()
	case "numeric":
		return "debe ser numérico"
	default:
		return "valor inválido"
	}
}

// toSnake convierte CamelCase a snake_case (FieldName -> field_name).
func toSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		if r >= 'A' && r <= 'Z' {
			b.WriteRune(r + 32)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
