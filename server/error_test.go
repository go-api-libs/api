//go:build goexperiment.jsonv2

package server_test

import (
	"encoding/json/v2"
	"errors"
	"net/http"
	"testing"

	"github.com/go-api-libs/api/server"
	"github.com/google/uuid"
)

func TestNewError_NilPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected NewError to panic with nil underlying error")
		}
	}()
	server.NewError(http.StatusInternalServerError, nil)
}

func TestNewError(t *testing.T) {
	tests := []struct {
		code        int
		wantMessage string
	}{
		{http.StatusUnauthorized, "Authentication required"},
		{http.StatusForbidden, "You don't have permission to perform this action"},
		{http.StatusNotFound, "Resource not found"},
		{http.StatusUnprocessableEntity, "Validation error"},
		{http.StatusInternalServerError, "Internal server error"},
		{http.StatusServiceUnavailable, "Internal server error"},
		{http.StatusBadRequest, "Bad Request"},
		{http.StatusTeapot, "I'm a teapot"},
	}
	for _, tc := range tests {
		underlying := errors.New("test")
		err := server.NewError(tc.code, underlying)
		if err == nil {
			t.Fatalf("[%d] expected non-nil error", tc.code)
		}
		if err.Code != tc.code {
			t.Fatalf("[%d] Code: got %d, want %d", tc.code, err.Code, tc.code)
		}
		if err.Message != tc.wantMessage {
			t.Fatalf("[%d] Message: got %q, want %q", tc.code, err.Message, tc.wantMessage)
		}
		if err.Detail != "" {
			t.Fatalf("[%d] Detail: got %q, want empty", tc.code, err.Detail)
		}
		if err.RequestID != uuid.Nil {
			t.Fatalf("[%d] RequestID: got %v, want Nil", tc.code, err.RequestID)
		}
	}
}

func TestError_ErrorString(t *testing.T) {
	underlying := errors.New("db fail")
	id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name string
		err  *server.Error
		want string
	}{
		{
			"500 no extras",
			server.NewError(http.StatusInternalServerError, underlying),
			"500 Internal server error <- db fail",
		},
		{
			"404",
			server.NewError(http.StatusNotFound, underlying),
			"404 Resource not found <- db fail",
		},
		{
			"401",
			server.NewError(http.StatusUnauthorized, underlying),
			"401 Authentication required <- db fail",
		},
		{
			"400 with detail",
			server.NewError(http.StatusBadRequest, underlying).WithDetail("invalid field"),
			"400 Bad Request: invalid field <- db fail",
		},
		{
			"500 with request ID",
			server.NewError(http.StatusInternalServerError, underlying).WithRequestID(id),
			"500 Internal server error [550e8400-e29b-41d4-a716-446655440000] <- db fail",
		},
		{
			"422 with detail and request ID",
			server.NewError(http.StatusUnprocessableEntity, underlying).WithDetail("name required").WithRequestID(id),
			"422 Validation error [550e8400-e29b-41d4-a716-446655440000]: name required <- db fail",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestError_GetCode(t *testing.T) {
	err := server.NewError(http.StatusForbidden, errors.New("denied"))
	if got := err.GetCode(); got != http.StatusForbidden {
		t.Fatalf("GetCode: got %d, want %d", got, http.StatusForbidden)
	}
}

func TestError_Unwrap(t *testing.T) {
	underlying := errors.New("wrapped")
	err := server.NewError(http.StatusInternalServerError, underlying)

	if !errors.Is(err, underlying) {
		t.Fatal("errors.Is: did not find underlying error")
	}

	var target *server.Error
	if !errors.As(err, &target) {
		t.Fatal("errors.As: did not match *server.Error")
	}
}

func TestError_WithDetail(t *testing.T) {
	underlying := errors.New("reason")

	t.Run("4xx sets detail", func(t *testing.T) {
		err := server.NewError(http.StatusBadRequest, underlying).WithDetail("bad input")
		if err.Detail != "bad input" {
			t.Fatalf("got %q, want %q", err.Detail, "bad input")
		}
	})

	t.Run("5xx ignores detail", func(t *testing.T) {
		err := server.NewError(http.StatusInternalServerError, underlying).WithDetail("secret info")
		if err.Detail != "" {
			t.Fatalf("got %q, want empty; 5xx must not expose internal detail", err.Detail)
		}
	})

	t.Run("edge 499 sets detail", func(t *testing.T) {
		err := server.NewError(499, underlying).WithDetail("edge case")
		if err.Detail != "edge case" {
			t.Fatalf("got %q, want %q", err.Detail, "edge case")
		}
	})

	t.Run("edge 500 ignores detail", func(t *testing.T) {
		err := server.NewError(500, underlying).WithDetail("not shown")
		if err.Detail != "" {
			t.Fatalf("got %q, want empty", err.Detail)
		}
	})
}

func TestError_WithRequestID(t *testing.T) {
	id := uuid.New()
	err := server.NewError(http.StatusInternalServerError, errors.New("err")).WithRequestID(id)
	if err.RequestID != id {
		t.Fatalf("RequestID: got %v, want %v", err.RequestID, id)
	}
}

func TestError_MarshalJSON(t *testing.T) {
	underlying := errors.New("internal")
	id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name string
		err  *server.Error
		want string
	}{
		{
			"omits zero fields",
			server.NewError(http.StatusInternalServerError, underlying),
			`{"code":500,"message":"Internal server error"}`,
		},
		{
			"includes detail",
			server.NewError(http.StatusBadRequest, underlying).WithDetail("invalid email"),
			`{"code":400,"message":"Bad Request","detail":"invalid email"}`,
		},
		{
			"includes request_id",
			server.NewError(http.StatusNotFound, underlying).WithRequestID(id),
			`{"request_id":"550e8400-e29b-41d4-a716-446655440000","code":404,"message":"Resource not found"}`,
		},
		{
			"all fields",
			server.NewError(http.StatusUnprocessableEntity, underlying).
				WithDetail("email required").
				WithRequestID(id),
			`{"request_id":"550e8400-e29b-41d4-a716-446655440000","code":422,"message":"Validation error","detail":"email required"}`,
		},
		{
			"underlying not marshaled",
			server.NewError(http.StatusInternalServerError, errors.New("secret: conn refused")),
			`{"code":500,"message":"Internal server error"}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.err)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if got := string(b); got != tc.want {
				t.Fatalf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestError_UnmarshalJSON(t *testing.T) {
	id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name       string
		input      string
		wantCode   int
		wantMsg    string
		wantDetail string
		wantID     uuid.UUID
	}{
		{
			"full object",
			`{"request_id":"550e8400-e29b-41d4-a716-446655440000","code":404,"message":"Resource not found","detail":"item 42"}`,
			404, "Resource not found", "item 42", id,
		},
		{
			"partial object",
			`{"code":500,"message":"Internal server error"}`,
			500, "Internal server error", "", uuid.Nil,
		},
		{
			"empty object",
			`{}`,
			0, "", "", uuid.Nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var e server.Error
			if err := json.Unmarshal([]byte(tc.input), &e); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if e.Code != tc.wantCode {
				t.Fatalf("Code: got %d, want %d", e.Code, tc.wantCode)
			}
			if e.Message != tc.wantMsg {
				t.Fatalf("Message: got %q, want %q", e.Message, tc.wantMsg)
			}
			if e.Detail != tc.wantDetail {
				t.Fatalf("Detail: got %q, want %q", e.Detail, tc.wantDetail)
			}
			if e.RequestID != tc.wantID {
				t.Fatalf("RequestID: got %v, want %v", e.RequestID, tc.wantID)
			}
		})
	}
}

func TestError_RoundTrip(t *testing.T) {
	id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	original := server.NewError(http.StatusUnprocessableEntity, errors.New("validation")).
		WithDetail("email is required").
		WithRequestID(id)

	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got server.Error
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.Code != original.Code {
		t.Fatalf("Code: got %d, want %d", got.Code, original.Code)
	}
	if got.Message != original.Message {
		t.Fatalf("Message: got %q, want %q", got.Message, original.Message)
	}
	if got.Detail != original.Detail {
		t.Fatalf("Detail: got %q, want %q", got.Detail, original.Detail)
	}
	if got.RequestID != original.RequestID {
		t.Fatalf("RequestID: got %v, want %v", got.RequestID, original.RequestID)
	}
}
