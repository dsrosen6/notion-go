package notion

import (
	"encoding/json"
	"testing"
)

func TestSearchParamsJSON(t *testing.T) {
	// Every filter and sort shape in api-endpoints/search.ts:14-24 must be
	// expressible, and nothing unset may leak into the body.
	tests := []struct {
		name   string
		params SearchParams
		want   string
	}{
		{
			name:   "empty",
			params: SearchParams{},
			want:   `{}`,
		},
		{
			name:   "pages filter",
			params: SearchParams{Filter: SearchPages()},
			want:   `{"filter":{"property":"object","value":"page"}}`,
		},
		{
			name:   "data sources in trash",
			params: SearchParams{Filter: SearchDataSources().WithInTrash(true)},
			want:   `{"filter":{"property":"object","value":"data_source","in_trash":true}}`,
		},
		{
			name:   "pages not in trash",
			params: SearchParams{Filter: SearchPages().WithInTrash(false)},
			want:   `{"filter":{"property":"object","value":"page","in_trash":false}}`,
		},
		{
			name:   "bare trash filter",
			params: SearchParams{Filter: SearchInTrash(true)},
			want:   `{"filter":{"in_trash":true}}`,
		},
		{
			name:   "sort by last edited",
			params: SearchParams{Sort: SortByLastEdited(Descending)},
			want:   `{"sort":{"timestamp":"last_edited_time","direction":"descending"}}`,
		},
		{
			name:   "sort by relevance",
			params: SearchParams{Sort: SortByRelevance()},
			want:   `{"sort":{"property":"relevance"}}`,
		},
		{
			name: "everything",
			params: SearchParams{
				Query:      "roadmap",
				Filter:     SearchPages(),
				Sort:       SortByRelevance(),
				PageParams: PageParams{StartCursor: "cur", PageSize: 10},
			},
			want: `{"query":"roadmap","filter":{"property":"object","value":"page"},"sort":{"property":"relevance"},"start_cursor":"cur","page_size":10}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.params)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("json = %s\nwant   %s", got, tt.want)
			}
		})
	}
}
