package apitest_test

import (
	"encoding/json/v2"
	"testing"

	"github.com/go-api-libs/api/apitest"
)

type page struct {
	ID       string         `json:"id"`
	Created  string         `json:"created,omitzero"`
	Count    float64        `json:"count,omitzero"`
	Filter   map[string]any `json:"filter,omitzero"`
	Tags     []string       `json:"tags,omitzero"`
	Archived bool           `json:"archived,omitzero"`
}

func TestDecodedAsSent(t *testing.T) {
	opts := json.DefaultOptionsV2()

	for _, tc := range []struct {
		name string
		out  any
		body string
	}{
		{"the same", page{ID: "a"}, `{"id":"a"}`},
		{"members in another order", page{ID: "a", Count: 1}, `{"count":1,"id":"a"}`},
		{"empty members left out", page{ID: "a"}, `{"id":"a","count":0,"archived":false,"tags":[],"created":"","filter":{"x":null}}`},
		{"zero texts left out", page{ID: "a"}, `{"id":"a","d":"0000-00-00","t":"0001-01-01T00:00:00Z","u":"00000000-0000-0000-0000-000000000000"}`},
		{"a UUID without dashes", page{ID: "6344e926-0f5a-4c3e-9b1a-2d1c3b4a5f6e"}, `{"id":"6344e9260f5a4c3e9b1a2d1c3b4a5f6e"}`},
		{"a time in another zone", page{ID: "a", Created: "2026-01-02T04:04:05+01:00"}, `{"id":"a","created":"2026-01-02T03:04:05Z"}`},
		{"a time as Unix seconds", page{ID: "a", Created: "2017-01-27T03:22:30Z"}, `{"id":"a","created":1485487350}`},
		{"a time as Unix milliseconds", page{ID: "a", Created: "2017-01-27T03:22:30.827Z"}, `{"id":"a","created":1485487350827}`},
		{"a number as a string", page{ID: "1.5"}, `{"id":1.5}`},
		{"a tag the body leaves out", page{ID: "a", Filter: map[string]any{"type": "date", "date": map[string]any{"equals": "today"}}}, `{"id":"a","filter":{"date":{"equals":"today"}}}`},
		{"arrays alike", page{ID: "a", Tags: []string{"x", "y"}}, `{"id":"a","tags":["x","y"]}`},
	} {
		if f := run(func(tb testing.TB) { apitest.DecodedAsSent(tb, "Op", tc.out, []byte(tc.body), opts) }); len(f.errs) > 0 {
			t.Errorf("%s: %q", tc.name, f.errs)
		}
	}

	for _, tc := range []struct {
		name string
		out  any
		body string
		at   string
	}{
		{"another value", page{ID: "a"}, `{"id":"b"}`, `"/id"`},
		{"a member the body holds and the value lacks", page{ID: "a"}, `{"id":"a","count":2}`, `"/count"`},
		{"another kind", page{ID: "a", Tags: []string{"x"}}, `{"id":"a","tags":{"x":1}}`, `"/tags"`},
		{"another length", page{ID: "a", Tags: []string{"x"}}, `{"id":"a","tags":["x","y"]}`, `"/tags"`},
		{"another item", page{ID: "a", Tags: []string{"x", "y"}}, `{"id":"a","tags":["x","z"]}`, `"/tags/1"`},
		{"another object", page{ID: "a", Filter: map[string]any{"x": 1.0}}, `{"id":"a","filter":"x"}`, `"/filter"`},
		{"another string", page{ID: "a", Created: "x"}, `{"id":"a","created":true}`, `"/created"`},
		{"another time", page{ID: "a", Created: "2026-01-02T03:04:05Z"}, `{"id":"a","created":"2026-01-02T03:04:06Z"}`, `"/created"`},
		{"another UUID", page{ID: "6344e926-0f5a-4c3e-9b1a-2d1c3b4a5f6e"}, `{"id":"6344e926-0f5a-4c3e-9b1a-2d1c3b4a5f6f"}`, `"/id"`},
	} {
		f := run(func(tb testing.TB) { apitest.DecodedAsSent(tb, "Op", tc.out, []byte(tc.body), opts) })
		if !f.failed("Op: decoded something else than was sent, at " + tc.at) {
			t.Errorf("%s: got %q, want a failure at %s", tc.name, f.errs, tc.at)
		}
	}

	if f := run(func(tb testing.TB) { apitest.DecodedAsSent(tb, "Op", make(chan int), []byte(`{}`), opts) }); !f.failed("Op: encoding what was decoded") {
		t.Errorf("what does not encode: got %q", f.errs)
	}

	if f := run(func(tb testing.TB) { apitest.DecodedAsSent(tb, "Op", page{}, []byte(`{`), opts) }); !f.failed("Op: the recorded body") {
		t.Errorf("a recorded body that does not parse: got %q", f.errs)
	}
}
