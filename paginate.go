package notion

import (
	"context"
	"iter"
	"strconv"
)

// pageSizeMax is the largest page_size the API accepts on a list endpoint.
const pageSizeMax = 100

// List is one page of results from a paginated endpoint.
//
// Follow NextCursor to read the next page, or use the endpoint's All method to
// iterate every result without managing cursors yourself.
type List[T any] struct {
	Object string `json:"object"`
	// Results holds this page's items.
	Results []T `json:"results"`
	// NextCursor addresses the next page, empty when this is the last one.
	NextCursor string `json:"next_cursor"`
	// HasMore reports whether more pages follow. Prefer NextCursor: the two
	// have been observed to disagree, and the cursor is authoritative.
	HasMore bool `json:"has_more"`
	// RequestStatus is set by the data source query and search endpoints when
	// the result set was truncated.
	RequestStatus *RequestStatus `json:"request_status,omitempty"`
}

// RequestStatus reports whether a query returned everything it matched.
type RequestStatus struct {
	// Type is "complete" or "incomplete".
	Type string `json:"type"`
	// IncompleteReason explains a truncated result, currently only
	// "query_result_limit_reached".
	IncompleteReason string `json:"incomplete_reason,omitempty"`
}

// fetchPage retrieves one page of results starting at cursor and returns the
// results along with the cursor for the next page. An empty next cursor ends
// the iteration.
type fetchPage[T any] func(ctx context.Context, cursor string) (results []T, next string, err error)

// paginate turns a paged endpoint into an iterator over individual items.
//
// Iteration ends when the API stops returning a cursor. The has_more flag is
// deliberately ignored, matching iteratePaginatedAPI in helpers.ts:60-73: the
// cursor alone decides, and the two have been observed to disagree.
//
// The first error ends iteration after being yielded, so a caller that checks
// err on each step never loops forever.
func paginate[T any](ctx context.Context, start string, fetch fetchPage[T]) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		cursor := start
		for {
			results, next, err := fetch(ctx, cursor)
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}
			for _, item := range results {
				if !yield(item, nil) {
					return
				}
			}
			if next == "" {
				return
			}
			cursor = next
		}
	}
}

// Collect drains an iterator into a slice, stopping at the first error.
//
// It fetches every remaining page, so prefer ranging over the iterator when a
// large result set might be cut short:
//
//	users, err := notion.Collect(client.Users.All(ctx))
func Collect[T any](seq iter.Seq2[T, error]) ([]T, error) {
	var out []T
	for item, err := range seq {
		if err != nil {
			return out, err
		}
		out = append(out, item)
	}
	return out, nil
}

// PageParams holds the pagination arguments common to every list endpoint.
type PageParams struct {
	// StartCursor resumes from a previous response's next cursor. Empty starts
	// at the beginning.
	StartCursor string `json:"start_cursor,omitempty"`
	// PageSize is the number of items per page, capped at 100 by the API. Zero
	// uses the server default.
	PageSize int `json:"page_size,omitempty"`
}

// query renders the pagination arguments as query-string parameters, for the
// GET-style list endpoints that take them there rather than in a body.
func (p PageParams) query() map[string][]string {
	q := map[string][]string{}
	if p.StartCursor != "" {
		q["start_cursor"] = []string{p.StartCursor}
	}
	if p.PageSize > 0 {
		q["page_size"] = []string{strconv.Itoa(p.PageSize)}
	}
	return q
}
