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

// defaultMessage returns a user-friendly message.
func defaultMessage(code int) string {
	switch code {
	case http.StatusUnauthorized:
		return "Authentication required"
	case http.StatusForbidden:
		return "You don't have permission to perform this action"
	case http.StatusNotFound:
		return "Resource not found"
	case http.StatusUnprocessableEntity:
		return "Validation error"
	}

	if code >= 500 {
		return "Internal server error"
	}

	return http.StatusText(code) // fallback
}
