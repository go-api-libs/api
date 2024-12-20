package api_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/go-api-libs/api"
)

var rsp = &http.Response{
	Status:     fmt.Sprintf("%d %s", http.StatusTeapot, http.StatusText(http.StatusTeapot)),
	StatusCode: http.StatusTeapot,
	Header: http.Header{
		"Content-Type": []string{"application/json"},
	},
	Request: &http.Request{
		URL: &url.URL{Scheme: "https", Host: "www.example.com", Path: "/foo"},
	},
}

func TestError(t *testing.T) {
	underlying := errors.New("test error")

	err := api.WrapDecodingError(rsp, underlying)
	if err == nil {
		t.Fatal("expected error")
	} else if want := `418 I'm a teapot: decoding response body: test error`; err.Error() != want {
		t.Fatalf("expected error to be %s, got: %s", want, err)
	} else if !errors.Is(err, underlying) {
		t.Fatalf("expected error to be %v, got %v", underlying, err)
	}

	t.Run("DecodingError", func(t *testing.T) {
		decErr := &api.DecodingError{}

		if !errors.As(err, &decErr) {
			t.Fatalf("expected error to be %T, got %T", decErr, err)
		} else if decErr.Err != underlying {
			t.Fatalf("expected error to be %v, got %v", underlying, decErr.Err)
		}
	})

	t.Run("api.Error", func(t *testing.T) {
		apiErr := &api.Error{}
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected error to be %T, got %T", apiErr, err)
		} else if apiErr.Response != rsp {
			t.Fatalf("expected response to be %v, got %v", rsp, apiErr.Response)
		} else if code := apiErr.StatusCode(); code != http.StatusTeapot {
			t.Fatalf("expected status code to be %d, got %d", http.StatusTeapot, code)
		} else if ct := apiErr.ContentType(); ct != "application/json" {
			t.Fatalf("expected content type to be application/json, got %s", ct)
		}
	})

	t.Run("api.NewErrCustom", func(t *testing.T) {
		apiErr := &api.Error{}
		if err := api.NewErrCustom(rsp, underlying); err == nil {
			t.Fatal("expected error")
		} else if want := `418 I'm a teapot: test error`; err.Error() != want {
			t.Fatalf("expected error to be %s, got: %s", want, err)
		} else if !errors.Is(err, underlying) {
			t.Fatalf("expected error to be %v, got %v", underlying, err)
		} else if !errors.As(err, &apiErr) {
			t.Fatalf("expected error to be api.Error, got %v", err)
		} else if !apiErr.IsCustom {
			t.Fatalf("expected error to be custom, got %v", apiErr.IsCustom)
		}
	})
}

func TestErrUnknownStatusCode(t *testing.T) {
	err := api.NewErrUnknownStatusCode(rsp)
	if err == nil {
		t.Fatal("expected error")
	} else if want := `418 I'm a teapot: unknown status code`; err.Error() != want {
		t.Fatalf("expected error to be %s, got: %s", want, err)
	} else if !errors.Is(err, api.ErrUnknownStatusCode) {
		t.Fatalf("expected error to be %v, got %v", api.ErrUnknownStatusCode, err)
	}
}

func TestErrStatusCode(t *testing.T) {
	err := api.NewErrStatusCode(rsp)
	if err == nil {
		t.Fatal("expected error")
	} else if want := `got 418 I'm a teapot calling https://www.example.com/foo`; err.Error() != want {
		t.Fatalf("expected error to be %s, got: %s", want, err)
	} else if !errors.Is(err, api.ErrStatusCode) {
		t.Fatalf("expected error to be %v, got %v", api.ErrStatusCode, err)
	}
}

func TestErrUnknownContentType(t *testing.T) {
	err := api.NewErrUnknownContentType(rsp)
	if err == nil {
		t.Fatal("expected error")
	} else if want := `418 I'm a teapot: unknown content type "application/json"`; err.Error() != want {
		t.Fatalf("expected error to be %s, got: %s", want, err)
	} else if !errors.Is(err, api.ErrUnknownContentType) {
		t.Fatalf("expected error to be %v, got %v", api.ErrUnknownContentType, err)
	}
}
