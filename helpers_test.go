package notion

import "testing"

func TestExtractID(t *testing.T) {
	const want = "1429989f-e8ac-4eff-bc8f-57f56486db54"

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"dashed id passes through", want, want},
		{"uppercase is lowered", "1429989F-E8AC-4EFF-BC8F-57F56486DB54", want},
		{"compact id gains hyphens", "1429989fe8ac4effbc8f57f56486db54", want},
		{"page url", "https://www.notion.so/My-Page-1429989fe8ac4effbc8f57f56486db54", want},
		{"url with fragment", "https://notion.so/My-Page-1429989fe8ac4effbc8f57f56486db54#anchor", want},
		{"query parameter", "https://notion.so/workspace?p=1429989fe8ac4effbc8f57f56486db54", want},
		{
			// The path ID must beat the view ID in the query string.
			name:  "path wins over a view id",
			input: "https://notion.so/My-DB-1429989fe8ac4effbc8f57f56486db54?v=aaaaaaaabbbbccccddddeeeeffff0000",
			want:  want,
		},
		{"surrounding whitespace", "  " + want + "  ", want},
		{"no id", "https://notion.so/", ""},
		{"empty", "", ""},
		{"too short to be an id", "abc123", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractID(tt.input); got != tt.want {
				t.Errorf("ExtractID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractBlockID(t *testing.T) {
	const want = "1429989f-e8ac-4eff-bc8f-57f56486db54"

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"block prefix", "https://notion.so/Page-abc#block-1429989fe8ac4effbc8f57f56486db54", want},
		{"bare fragment", "https://notion.so/Page-abc#1429989fe8ac4effbc8f57f56486db54", want},
		{"no fragment", "https://notion.so/Page-1429989fe8ac4effbc8f57f56486db54", ""},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractBlockID(tt.input); got != tt.want {
				t.Errorf("ExtractBlockID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
