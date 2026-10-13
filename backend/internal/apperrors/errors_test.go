package apperrors

import (
	"errors"
	"strings"
	"testing"

	"roadmap/backend/internal/domain"
)

func TestHTTPMapping(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{name: "validation", err: domain.ValidationError{Field: "title", Reason: "required"}, status: 400, message: "validation failed"},
		{name: "unauthorized", err: domain.ErrUnauthorized, status: 401, message: "authentication required"},
		{name: "forbidden", err: domain.ErrForbidden, status: 403, message: "access denied"},
		{name: "not found", err: domain.ErrNotFound, status: 404, message: "not found"},
		{name: "conflict", err: domain.ErrConflict, status: 409, message: "conflict"},
		{name: "unknown", err: errors.New("database password must not leak"), status: 500, message: "internal server error"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mapped := HTTP(test.err)
			if mapped.Status != test.status || mapped.Message != test.message {
				t.Fatalf("HTTP(%v) = %#v", test.err, mapped)
			}
			if test.name == "unknown" && strings.Contains(mapped.Message, "password") {
				t.Fatal("unknown error leaked into HTTP message")
			}
		})
	}
}

func TestMCPMappingDoesNotLeakUnknownCause(t *testing.T) {
	const secret = "database password must not leak"
	if got := MCP(errors.New(secret)); got != "operation failed" || strings.Contains(got, secret) {
		t.Fatalf("MCP unknown mapping = %q", got)
	}
	if got := MCP(domain.ValidationError{Field: "title", Reason: "required"}); got != "invalid input: title: required" {
		t.Fatalf("MCP validation mapping = %q", got)
	}
}

func TestClass(t *testing.T) {
	if got := Class(errors.New("unexpected")); got != "internal" {
		t.Fatalf("Class(unknown) = %q", got)
	}
	if got := Class(domain.ErrConflict); got != "conflict" {
		t.Fatalf("Class(conflict) = %q", got)
	}
}
