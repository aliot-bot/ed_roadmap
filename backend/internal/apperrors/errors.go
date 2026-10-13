package apperrors

import (
	"errors"
	"fmt"

	"roadmap/backend/internal/domain"
)

type HTTPMapping struct {
	Status  int
	Message string
	Details any
}

func HTTP(err error) HTTPMapping {
	var validation domain.ValidationError
	switch {
	case errors.As(err, &validation):
		return HTTPMapping{Status: 400, Message: "validation failed", Details: validation}
	case errors.Is(err, domain.ErrUnauthorized):
		return HTTPMapping{Status: 401, Message: "authentication required"}
	case errors.Is(err, domain.ErrForbidden):
		return HTTPMapping{Status: 403, Message: "access denied"}
	case errors.Is(err, domain.ErrNotFound):
		return HTTPMapping{Status: 404, Message: "not found"}
	case errors.Is(err, domain.ErrConflict):
		return HTTPMapping{Status: 409, Message: "conflict"}
	default:
		return HTTPMapping{Status: 500, Message: "internal server error"}
	}
}

func MCP(err error) string {
	if errors.Is(err, domain.ErrForbidden) || errors.Is(err, domain.ErrUnauthorized) {
		return "access denied"
	}
	if errors.Is(err, domain.ErrNotFound) {
		return "not found"
	}

	var validation domain.ValidationError
	if errors.As(err, &validation) {
		return fmt.Sprintf("invalid input: %s", validation.Error())
	}
	if errors.Is(err, domain.ErrConflict) {
		return "conflict"
	}

	return "operation failed"
}

func Class(err error) string {
	var validation domain.ValidationError
	switch {
	case errors.As(err, &validation):
		return "validation"
	case errors.Is(err, domain.ErrUnauthorized):
		return "unauthorized"
	case errors.Is(err, domain.ErrForbidden):
		return "forbidden"
	case errors.Is(err, domain.ErrNotFound):
		return "not_found"
	case errors.Is(err, domain.ErrConflict):
		return "conflict"
	default:
		return "internal"
	}
}
