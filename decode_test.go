package api_test

import (
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/go-api-libs/api"
)

type thing struct {
	Name string `json:"name"`
}

func withBody(body string) *http.Response {
	r := *rsp
	r.Body = io.NopCloser(strings.NewReader(body))

	return &r
}

func TestDecodeJSON(t *testing.T) {
	strict := json.RejectUnknownMembers(true)
	extra := `{"name":"a","new":1}`

	t.Run("decodes", func(t *testing.T) {
		got, err := api.DecodeJSON[thing](withBody(`{"name":"a"}`), strict, nil, nil)
		if err != nil || got.Name != "a" {
			t.Fatalf("got %+v, %v", got, err)
		}
	})

	t.Run("not in debug mode", func(t *testing.T) {
		got, err := api.DecodeJSON[thing](withBody(extra), strict, []byte(extra), nil)
		if decErr := (*api.DecodingError)(nil); !errors.As(err, &decErr) {
			t.Fatalf("got %v, want a decoding error", err)
		}

		if apiErr := (*api.Error)(nil); !errors.As(err, &apiErr) || apiErr.Response.StatusCode != rsp.StatusCode {
			t.Fatalf("got %v, want an error with the response", err)
		}

		if got != (thing{}) {
			t.Fatalf("got %+v, want the zero value", got)
		}
	})

	t.Run("debug mode keeps the call and decodes leniently", func(t *testing.T) {
		kept := 0
		got, err := api.DecodeJSON[thing](withBody(extra), strict, []byte(extra), func() error { kept++; return nil })
		if err != nil || got.Name != "a" || kept != 1 {
			t.Fatalf("got %+v, %v, kept %d times", got, err, kept)
		}
	})

	t.Run("keeping fails", func(t *testing.T) {
		errKeep := errors.New("disk full")
		got, err := api.DecodeJSON[thing](withBody(extra), strict, []byte(extra), func() error { return errKeep })
		if !errors.Is(err, errKeep) {
			t.Fatalf("got %v, want %v", err, errKeep)
		}

		if decErr := (*api.DecodingError)(nil); !errors.As(err, &decErr) {
			t.Fatalf("got %v, want a decoding error too", err)
		}

		if got != (thing{}) {
			t.Fatalf("got %+v, want the zero value", got)
		}
	})

	t.Run("not even leniently", func(t *testing.T) {
		got, err := api.DecodeJSON[thing](withBody(`[1]`), strict, []byte(`[1]`), func() error { return nil })
		if decErr := (*api.DecodingError)(nil); !errors.As(err, &decErr) {
			t.Fatalf("got %v, want a decoding error", err)
		}

		if got != (thing{}) {
			t.Fatalf("got %+v, want the zero value", got)
		}
	})
}
