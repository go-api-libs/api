package paging_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/go-api-libs/api/paging"
)

// list is a page of a list: its items, and the cursor of the next page if there is one.
type list struct {
	Items []string
	Next  string
	More  bool
}

// lists serves pages of letters by cursor, and counts the pages fetched.
type lists struct {
	pages   map[string]list
	fail    string // the cursor whose page fails
	fetched []string
}

func (l *lists) fetch(cursor string) (list, error) {
	l.fetched = append(l.fetched, cursor)

	if cursor == l.fail && l.fail != "" {
		return list{}, errBroken
	}

	return l.pages[cursor], nil
}

func next(l list) (string, bool) { return l.Next, l.More }

func items(l list) []string { return l.Items }

var errBroken = errors.New("broken")

func newLists() *lists {
	return &lists{pages: map[string]list{
		"":   {Items: []string{"a", "b"}, Next: "c2", More: true},
		"c2": {Items: []string{"c"}, Next: "c3", More: true},
		"c3": {Items: []string{"d"}},
	}}
}

func TestItems(t *testing.T) {
	t.Parallel()

	l := newLists()

	var got []string

	for item, err := range paging.Items(paging.Pages(l.fetch, next), items) {
		if err != nil {
			t.Fatal(err)
		}

		got = append(got, item)
	}

	if want := []string{"a", "b", "c", "d"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	if want := []string{"", "c2", "c3"}; !slices.Equal(l.fetched, want) {
		t.Errorf("fetched %q, want %q", l.fetched, want)
	}
}

// TestBreak: breaking out of the loop fetches no page after.
func TestBreak(t *testing.T) {
	t.Parallel()

	l := newLists()

	for _, err := range paging.Items(paging.Pages(l.fetch, next), items) {
		if err != nil {
			t.Fatal(err)
		}

		break
	}

	if len(l.fetched) != 1 {
		t.Errorf("fetched %q after breaking at the first item, want the first page alone", l.fetched)
	}

	l = newLists()

	for _, err := range paging.Pages(l.fetch, next) {
		if err != nil {
			t.Fatal(err)
		}

		break
	}

	if len(l.fetched) != 1 {
		t.Errorf("fetched %q after breaking at the first page, want it alone", l.fetched)
	}
}

// TestError: an error is yielded once, after the items before it, and ends the loop.
func TestError(t *testing.T) {
	t.Parallel()

	l := newLists()
	l.fail = "c3"

	var (
		got  []string
		errs []error
	)

	for item, err := range paging.Items(paging.Pages(l.fetch, next), items) {
		if err != nil {
			errs = append(errs, err)
			continue
		}

		got = append(got, item)
	}

	if want := []string{"a", "b", "c"}; !slices.Equal(got, want) {
		t.Errorf("got %v before the error, want %v", got, want)
	}

	if len(errs) != 1 || !errors.Is(errs[0], errBroken) {
		t.Errorf("got errors %v, want %v once", errs, errBroken)
	}
}
