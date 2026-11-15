// Package paging yields what an API lists a page at a time, fetching each page as the one before is used up.
package paging

import "iter"

// Pages yields each page of a list, which fetch returns given its cursor, the zero cursor for the first, and next
// tells the cursor of the page after, and whether there is one. It stops when the loop breaks, and at the first
// error, which it yields.
func Pages[P any, C comparable](fetch func(cursor C) (P, error), next func(P) (C, bool)) iter.Seq2[P, error] {
	return func(yield func(P, error) bool) {
		var cursor C

		for {
			p, err := fetch(cursor)
			if err != nil {
				var zero P

				yield(zero, err)

				return
			}

			if !yield(p, nil) {
				return
			}

			c, more := next(p)
			if !more {
				return
			}

			cursor = c
		}
	}
}

// Items yields the items of each of pages, which items tells. It stops when the loop breaks, and at the first error,
// which it yields.
func Items[P, T any](pages iter.Seq2[P, error], items func(P) []T) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		for p, err := range pages {
			if err != nil {
				var zero T

				yield(zero, err)

				return
			}

			for _, item := range items(p) {
				if !yield(item, nil) {
					return
				}
			}
		}
	}
}
