package notion

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// pagedFetch returns a fetch function serving the given pages in order, and a
// pointer to the number of calls it received.
func pagedFetch(pages [][]string, cursors []string) (fetchPage[string], *int) {
	var calls int
	return func(ctx context.Context, cursor string) ([]string, string, error) {
		i := calls
		calls++
		if i >= len(pages) {
			return nil, "", fmt.Errorf("fetched past the last page")
		}
		return pages[i], cursors[i], nil
	}, &calls
}

func TestPaginateAcrossPages(t *testing.T) {
	fetch, calls := pagedFetch(
		[][]string{{"a", "b"}, {"c"}, {"d", "e"}},
		[]string{"cur1", "cur2", ""},
	)

	got, err := Collect(paginate(context.Background(), "", fetch))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	want := []string{"a", "b", "c", "d", "e"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("items = %v, want %v", got, want)
	}
	if *calls != 3 {
		t.Errorf("fetched %d pages, want 3", *calls)
	}
}

func TestPaginateIgnoresHasMore(t *testing.T) {
	// The cursor alone ends iteration. A page carrying results but no cursor is
	// the last one, whatever has_more claimed.
	fetch, calls := pagedFetch([][]string{{"only"}}, []string{""})

	got, err := Collect(paginate(context.Background(), "", fetch))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(got) != 1 || *calls != 1 {
		t.Errorf("got %v across %d fetches, want [only] across 1", got, *calls)
	}
}

func TestPaginateResumesFromCursor(t *testing.T) {
	var seen []string
	fetch := func(ctx context.Context, cursor string) ([]string, string, error) {
		seen = append(seen, cursor)
		if len(seen) == 1 {
			return []string{"x"}, "next", nil
		}
		return []string{"y"}, "", nil
	}

	if _, err := Collect(paginate(context.Background(), "resume-here", fetch)); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if seen[0] != "resume-here" {
		t.Errorf("first cursor = %q, want resume-here", seen[0])
	}
	if seen[1] != "next" {
		t.Errorf("second cursor = %q, want next", seen[1])
	}
}

func TestPaginateStopsOnError(t *testing.T) {
	wantErr := errors.New("boom")
	var calls int
	fetch := func(ctx context.Context, cursor string) ([]string, string, error) {
		calls++
		if calls == 2 {
			return nil, "", wantErr
		}
		return []string{"a"}, "next", nil
	}

	got, err := Collect(paginate(context.Background(), "", fetch))
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if len(got) != 1 {
		t.Errorf("items = %v, want the one from before the error", got)
	}
	// The error must end iteration rather than being retried forever.
	if calls != 2 {
		t.Errorf("fetched %d times, want 2", calls)
	}
}

func TestPaginateEarlyBreak(t *testing.T) {
	var calls int
	fetch := func(ctx context.Context, cursor string) ([]string, string, error) {
		calls++
		return []string{"a", "b"}, "always-more", nil
	}

	var got []string
	for item, err := range paginate(context.Background(), "", fetch) {
		if err != nil {
			t.Fatalf("iteration: %v", err)
		}
		got = append(got, item)
		if len(got) == 3 {
			break
		}
	}

	if len(got) != 3 {
		t.Errorf("got %d items, want 3", len(got))
	}
	// Breaking mid-page must stop the fetching too.
	if calls != 2 {
		t.Errorf("fetched %d pages, want 2", calls)
	}
}

func TestPageParamsQuery(t *testing.T) {
	tests := []struct {
		name   string
		params PageParams
		want   map[string]string
	}{
		{"empty sends nothing", PageParams{}, map[string]string{}},
		{"cursor only", PageParams{StartCursor: "c1"}, map[string]string{"start_cursor": "c1"}},
		{"size only", PageParams{PageSize: 25}, map[string]string{"page_size": "25"}},
		{"both", PageParams{StartCursor: "c1", PageSize: 100}, map[string]string{"start_cursor": "c1", "page_size": "100"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.params.query()
			if len(got) != len(tt.want) {
				t.Fatalf("query = %v, want %v", got, tt.want)
			}
			for key, want := range tt.want {
				if len(got[key]) != 1 || got[key][0] != want {
					t.Errorf("%s = %v, want %q", key, got[key], want)
				}
			}
		})
	}
}
