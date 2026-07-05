package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// Error is an error returned by the API server.
type Error struct {
	RequestID uuid.UUID `json:"request_id,omitzero"`
	Code      int       `json:"code,omitzero"`
	Message   string    `json:"message,omitzero"`
	Detail    string    `json:"detail,omitzero"`

	underlying error `json:"-"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "%d %s", e.Code, e.Message)

	if e.RequestID != uuid.Nil {
		fmt.Fprintf(b, " [%s]", e.RequestID)
	}

	if e.Detail != "" {
		fmt.Fprintf(b, ": %s", e.Detail)
	}

	fmt.Fprintf(b, " <- %v", e.underlying)

	return b.String()
}

func (e Error) GetCode() int { return e.Code }

// Unwrap allows errors.Is / errors.As to work with the underlying error.
func (e *Error) Unwrap() error { return e.underlying }

func NewError(code int, err error) *Error {
	if err == nil {
		panic("server.NewError: underlying error must not be nil")
	}

	return &Error{
		Code:       code,
		Message:    defaultMessage(code),
		underlying: err,
	}
}

// WithDetail adds extra safe detail (only for 4xx errors).
func (e *Error) WithDetail(detail string) *Error {
	if isClientError(e.Code) {
		e.Detail = detail
	}

	return e
}

// WithRequestID adds correlation ID (highly recommended).
func (e *Error) WithRequestID(id uuid.UUID) *Error {
	e.RequestID = id
	return e
}

func isClientError(code int) bool { return code >= 400 && code < 500 }

// defaultMessage returns a user-friendly message for the given HTTP status code.
// Custom strings are used where http.StatusText is either misleading (401 says
// "Unauthorized" but means "unauthenticated"), too jargon-heavy for end users
// (422 "Unprocessable Entity"), or where sentence case consistency matters more
// than saving a line of code. Everything else falls back to http.StatusText.
func defaultMessage(code int) string {
	switch code {
	case http.StatusUnauthorized:
		// "Unauthorized" is a misnomer in the HTTP spec; it means the request
		// lacks valid credentials, not that the action is forbidden.
		return "Authentication required"
	case http.StatusForbidden:
		return "You don't have permission to perform this action"
	case http.StatusNotFound:
		return "Resource not found"
	case http.StatusUnprocessableEntity:
		// "Unprocessable Entity" is HTTP jargon; "Validation error" is what
		// API clients and end users actually expect to see.
		return "Validation error"
	case http.StatusNotImplemented:
		// 501 is the one 5xx code safe to surface: it describes a missing
		// feature, not an infrastructure failure.
		return "Not implemented"
	}

	if code >= 500 {
		// Suppress specific 5xx text (e.g. "Bad Gateway", "Gateway Timeout")
		// to avoid leaking topology details about upstream dependencies.
		return "Internal server error"
	}

	return http.StatusText(code)
}
