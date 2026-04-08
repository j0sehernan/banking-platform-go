package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate = validator.New(validator.WithRequiredStructEnabled())

// DecodeAndValidate parses the JSON body into the destination type and
// runs validation with go-playground/validator. Returns a *Error with
// an appropriate status if anything fails.
//
// Typical usage in a handler:
//
//	var req CreateAccountRequest
//	if err := httpx.DecodeAndValidate(r, &req); err != nil {
//	    return err
//	}
func DecodeAndValidate[T any](r *http.Request, dst *T) error {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		return NewError(http.StatusBadRequest, "invalid_json",
			"Request body is not valid JSON")
	}

	if err := validate.Struct(dst); err != nil {
		return validationError(err)
	}
	return nil
}

// Validate runs only the validation (when decode was done elsewhere).
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
		Message: "The request has invalid fields",
		Details: map[string]any{"fields": fields},
	}
}

func humanize(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "uuid":
		return "must be a valid UUID"
	case "email":
		return "must be a valid email"
	case "gt":
		return "must be greater than " + fe.Param()
	case "gte":
		return "must be greater than or equal to " + fe.Param()
	case "min":
		return "minimum length: " + fe.Param()
	case "max":
		return "maximum length: " + fe.Param()
	case "oneof":
		return "must be one of: " + fe.Param()
	case "nefield":
		return "must be different from " + fe.Param()
	case "numeric":
		return "must be numeric"
	default:
		return "invalid value"
	}
}

// toSnake converts CamelCase to snake_case (FieldName -> field_name).
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
