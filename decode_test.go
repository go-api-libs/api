package api_test

import (
	"encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/MarkRosemaker/cassette"
	"github.com/go-api-libs/api"
)

type thing struct {
	Name string `json:"name"`
}

func TestDecodeLenient(t *testing.T) {
	strict := json.RejectUnknownMembers(true)
	errStrict := errors.New("unknown member")
	recorded := func(body string) *cassette.Interaction {
		return &cassette.Interaction{Response: cassette.Response{StatusCode: rsp.StatusCode, Body: cassette.Body(body)}}
	}

	t.Run("not in debug mode", func(t *testing.T) {
		out := thing{Name: "partly decoded"}

		err := api.DecodeLenient(rsp, errStrict, nil, filepath.Join(t.TempDir(), "x.json"), &out, strict)
		if !errors.Is(err, errStrict) {
			t.Fatalf("got %v, want %v", err, errStrict)
		}

		if decErr := (*api.DecodingError)(nil); !errors.As(err, &decErr) {
			t.Fatalf("got %v, want a decoding error", err)
		}

		if apiErr := (*api.Error)(nil); !errors.As(err, &apiErr) || apiErr.Response != rsp {
			t.Fatalf("got %v, want an error with the response", err)
		}
	})

	t.Run("keeps the call and decodes leniently", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "interactions.json")
		out := thing{Name: "partly decoded"}

		if err := api.DecodeLenient(rsp, errStrict, recorded(`{"name":"a","new":1}`), file, &out, strict); err != nil {
			t.Fatal(err)
		}

		if out.Name != "a" {
			t.Fatalf("got %+v", out)
		}

		kept, err := cassette.InteractionsReadFile(file)
		if err != nil || len(kept) != 1 {
			t.Fatalf("kept %d interactions, %v", len(kept), err)
		}
	})

	t.Run("starts afresh", func(t *testing.T) {
		out := thing{Name: "partly decoded"}

		if err := api.DecodeLenient(rsp, errStrict, recorded(`{}`), filepath.Join(t.TempDir(), "x.json"), &out, strict); err != nil {
			t.Fatal(err)
		}

		if out != (thing{}) {
			t.Fatalf("got %+v, want what the recorded body holds alone", out)
		}
	})

	t.Run("keeping fails", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(dir, nil, 0o600); err != nil {
			t.Fatal(err)
		}

		var out thing

		err := api.DecodeLenient(rsp, errStrict, recorded(`{"name":"a"}`), filepath.Join(dir, "x.json"), &out, strict)
		if !errors.Is(err, errStrict) {
			t.Fatalf("got %v, want %v too", err, errStrict)
		}

		if joined, ok := err.(interface{ Unwrap() []error }); !ok || len(joined.Unwrap()) != 2 {
			t.Fatalf("got %v, want the error keeping the call joined to it", err)
		}
	})

	t.Run("not even leniently", func(t *testing.T) {
		var out thing

		err := api.DecodeLenient(rsp, errStrict, recorded(`[1]`), filepath.Join(t.TempDir(), "x.json"), &out, strict)
		if decErr := (*api.DecodingError)(nil); !errors.As(err, &decErr) {
			t.Fatalf("got %v, want a decoding error", err)
		}
	})
}
