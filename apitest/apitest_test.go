package apitest_test

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/MarkRosemaker/cassette"
	"github.com/go-api-libs/api/apitest"
)

// fakeT is a testing.TB that records what fails, so that a test can check a helper fails as it should. Its Fatal ends
// the goroutine, as testing.T's does, so run the helper with run.
type fakeT struct {
	testing.TB

	mu       sync.Mutex
	errs     []string
	fatal    bool
	cleanups []func()
}

func (f *fakeT) Helper() {}

func (f *fakeT) Cleanup(fn func()) { f.cleanups = append(f.cleanups, fn) }

func (f *fakeT) Errorf(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.errs = append(f.errs, fmt.Sprintf(format, args...))
}

func (f *fakeT) Fatalf(format string, args ...any) {
	f.Errorf(format, args...)
	f.fatal = true

	runtime.Goexit()
}

func (f *fakeT) Fatal(args ...any) { f.Fatalf("%s", fmt.Sprint(args...)) }

// run runs fn with a fakeT, then its cleanups, and returns what failed.
func run(fn func(t testing.TB)) *fakeT {
	f := &fakeT{}
	done := make(chan struct{})

	go func() {
		defer close(done)

		fn(f)
	}()

	<-done

	for _, v := range slices.Backward(f.cleanups) {
		v()
	}

	return f
}

func (f *fakeT) failed(contains string) bool {
	for _, e := range f.errs {
		if strings.Contains(e, contains) {
			return true
		}
	}

	return false
}

func TestNewServer(t *testing.T) {
	srv := apitest.NewServer(t, http.StatusTeapot)

	rsp, err := get(t, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusTeapot || rsp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("got %s, %s", rsp.Status, rsp.Header.Get("Content-Type"))
	}
}

func TestBodyServer(t *testing.T) {
	u := apitest.BodyServer(t, http.StatusAccepted, "text/plain", []byte("hi"))

	rsp, err := get(t, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rsp.Body.Close()

	body, _ := io.ReadAll(rsp.Body)
	if rsp.StatusCode != http.StatusAccepted || rsp.Header.Get("Content-Type") != "text/plain" || string(body) != "hi" {
		t.Errorf("got %s, %s, %q", rsp.Status, rsp.Header.Get("Content-Type"), body)
	}
}

func TestRoundTripFunc(t *testing.T) {
	want := &http.Response{StatusCode: http.StatusOK}

	got, err := apitest.RoundTripFunc(func(*http.Request) (*http.Response, error) { return want, nil }).RoundTrip(nil)
	if err != nil || got != want {
		t.Errorf("got %v, %v", got, err)
	}
}

type item struct {
	ID string `json:"id,omitzero"`
}

func TestWithUnknownMember(t *testing.T) {
	if got := string(apitest.WithUnknownMember(t, item{ID: "a"}, json.DefaultOptionsV2())); got != `{"not_a_member_of_any_type": true, "id":"a"}` {
		t.Errorf("got %s", got)
	}

	if got := string(apitest.WithUnknownMember(t, item{}, json.DefaultOptionsV2())); got != `{"not_a_member_of_any_type": true}` {
		t.Errorf("empty: got %s", got)
	}

	if f := run(func(t testing.TB) { apitest.WithUnknownMember(t, make(chan int), json.DefaultOptionsV2()) }); !f.fatal {
		t.Error("what does not encode: no failure")
	}
}

func TestMustDecode(t *testing.T) {
	if got := apitest.MustDecode[item](t, `{"id":"a"}`, json.DefaultOptionsV2()); got.ID != "a" {
		t.Errorf("got %+v", got)
	}

	f := run(func(t testing.TB) { apitest.MustDecode[item](t, `{"id":1}`, json.DefaultOptionsV2()) })
	if !f.fatal || !f.failed(`decoding {"id":1}`) {
		t.Errorf("what does not decode: got %q", f.errs)
	}
}

// choice encodes and decodes as what it holds: a number or a string.
type choice struct {
	n *int
	s *string
}

func (c choice) MarshalJSON() ([]byte, error) {
	if c.n != nil {
		return json.Marshal(*c.n)
	}

	return json.Marshal(*c.s)
}

func (c *choice) UnmarshalJSON(b []byte) error {
	var n int
	if err := json.Unmarshal(b, &n); err == nil {
		c.n = &n
		return nil
	}

	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	c.s = &s

	return nil
}

// drifting encodes as something else each time.
type drifting struct{ N int }

var drifts int

func (drifting) MarshalJSON() ([]byte, error) {
	drifts++
	return json.Marshal(drifts)
}

func (d *drifting) UnmarshalJSON(b []byte) error { return json.Unmarshal(b, &d.N) }

func TestRoundTrip(t *testing.T) {
	n := 1
	opts := json.DefaultOptionsV2()

	apitest.RoundTrip(t, choice{n: &n}, func(c choice) bool { return c.n != nil }, opts)

	if f := run(func(t testing.TB) {
		apitest.RoundTrip(t, choice{n: &n}, func(c choice) bool { return c.s != nil }, opts)
	}); !f.failed("decoded as another variant") {
		t.Errorf("another variant: got %q", f.errs)
	}

	if f := run(func(t testing.TB) {
		apitest.RoundTrip(t, drifting{}, func(drifting) bool { return true }, opts)
	}); !f.failed("encoded 1, then 2") {
		t.Errorf("encoded otherwise: got %q", f.errs)
	}

	if f := run(func(t testing.TB) {
		apitest.RoundTrip(t, make(chan int), func(chan int) bool { return true }, opts)
	}); !f.fatal {
		t.Error("what does not encode: no failure")
	}

	if f := run(func(t testing.TB) {
		apitest.RoundTrip(t, item{ID: "a"}, func(item) bool { return true }, json.JoinOptions(opts, json.WithUnmarshalers(
			json.UnmarshalFunc(func([]byte, *item) error { return fmt.Errorf("no") }),
		)))
	}); !f.fatal || !f.failed("no") {
		t.Errorf("what does not decode: got %q", f.errs)
	}
}

// interactionsFile writes ias to a file of a test's own and returns its path.
func interactionsFile(t *testing.T, ias cassette.Interactions) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "interactions.json")
	if err := ias.WriteFile(path); err != nil {
		t.Fatal(err)
	}

	return path
}

func recorded(t *testing.T, method, rawURL, reqBody, rspBody string, header http.Header) *cassette.Interaction {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), method, rawURL, strings.NewReader(reqBody))
	if err != nil {
		t.Fatal(err)
	}

	req.Header = header
	if reqBody == "" {
		req.Body = nil
	}

	r, err := cassette.NewRequest(req)
	if err != nil {
		t.Fatal(err)
	}

	return &cassette.Interaction{Request: r, Response: cassette.Response{
		StatusCode: http.StatusOK, Headers: http.Header{"Content-Type": {"application/json"}}, Body: cassette.Body(rspBody),
	}}
}

func call(t testing.TB, rt http.RoundTripper, ctx context.Context, method, rawURL, body string, header http.Header) (*http.Response, error) {
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, r)
	if err != nil {
		t.Fatal(err)
	}

	req.Header = header

	return (&http.Client{Transport: rt}).Do(req)
}

func TestReplay(t *testing.T) {
	json1 := http.Header{"Content-Type": {"application/json"}}

	t.Run("answers as recorded, a body with a tag the recording leaves out too", func(t *testing.T) {
		path := interactionsFile(t, cassette.Interactions{
			recorded(t, http.MethodGet, "https://api.example.com/a", "", `{"id":"a"}`, http.Header{}),
			recorded(t, http.MethodPost, "https://api.example.com/q", `{"filter":{"date":{"equals":"today"}}}`, `{}`, json1),
		})

		f := run(func(tb testing.TB) {
			rt := &apitest.LastBody{Next: apitest.Replay(tb, path)}

			rsp, err := call(tb, rt, t.Context(), http.MethodGet, "https://api.example.com/a", "", http.Header{"User-Agent": {"some tool"}})
			if err != nil {
				tb.Fatal(err)
			}

			rsp.Body.Close()

			if v := jsontext.Value(rt.Body); v.Compact() != nil || string(v) != `{"id":"a"}` {
				tb.Errorf("kept %s", rt.Body)
			}

			rsp, err = call(tb, rt, t.Context(), http.MethodPost, "https://api.example.com/q", `{"filter":{"type":"date","date":{"equals":"today"}}}`, json1)
			if err != nil {
				tb.Fatal(err)
			}

			rsp.Body.Close()
		})
		if len(f.errs) > 0 {
			t.Errorf("got %q", f.errs)
		}
	})

	t.Run("a body otherwise not the one recorded", func(t *testing.T) {
		path := interactionsFile(t, cassette.Interactions{
			recorded(t, http.MethodPost, "https://api.example.com/q", `{"a":1}`, `{}`, json1),
		})

		f := run(func(tb testing.TB) {
			if _, err := call(tb, apitest.Replay(tb, path), t.Context(), http.MethodPost, "https://api.example.com/q", `{"a":2}`, json1); err == nil {
				tb.Errorf("no error")
			}
		})
		if len(f.errs) != 1 || !f.failed("never replayed") {
			t.Errorf("got %q, want only the recording never replayed", f.errs)
		}
	})

	t.Run("credentials the recording lacks, only where they are optional", func(t *testing.T) {
		path := interactionsFile(t, cassette.Interactions{
			recorded(t, http.MethodGet, "https://api.example.com/a", "", `{}`, http.Header{}),
			recorded(t, http.MethodGet, "https://api.example.com/a", "", `{}`, http.Header{}),
		})
		auth := http.Header{"Authorization": {"Bearer secret"}}

		f := run(func(tb testing.TB) {
			rt := apitest.Replay(tb, path)

			rsp, err := call(tb, rt, apitest.OptionalAuth(t.Context()), http.MethodGet, "https://api.example.com/a", "", auth)
			if err != nil {
				tb.Fatal(err)
			}

			rsp.Body.Close()

			if _, err := call(tb, rt, t.Context(), http.MethodGet, "https://api.example.com/a", "", auth); err == nil {
				tb.Errorf("credentials not optional: no error")
			}
		})
		if len(f.errs) != 1 || !f.failed("never replayed") {
			t.Errorf("got %q", f.errs)
		}
	})

	t.Run("no file", func(t *testing.T) {
		if f := run(func(tb testing.TB) { apitest.Replay(tb, filepath.Join(t.TempDir(), "none.json")) }); !f.fatal {
			t.Error("no failure")
		}
	})
}

func TestLastBody(t *testing.T) {
	failing := apitest.RoundTripFunc(func(*http.Request) (*http.Response, error) { return nil, os.ErrClosed })
	if _, err := (&apitest.LastBody{Next: failing}).RoundTrip(&http.Request{}); err != os.ErrClosed {
		t.Errorf("got %v", err)
	}

	broken := apitest.RoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{Body: io.NopCloser(errReader{})}, nil
	})
	if _, err := (&apitest.LastBody{Next: broken}).RoundTrip(&http.Request{}); err == nil {
		t.Error("a body that does not read: no error")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, os.ErrClosed }

func get(t *testing.T, rawURL string) (*http.Response, error) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatal(err)
	}

	return http.DefaultClient.Do(req)
}
