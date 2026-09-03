package notion

import (
	"regexp"
	"strings"
)

// Patterns for pulling an ID out of the various forms Notion uses. Ported from
// extractNotionId, helpers.ts:483-572.
var (
	dashedUUIDPattern  = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	compactUUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{32}$`)
	// pathIDPattern matches the ID at the end of a page slug. It is tried
	// before the query string so that a "?v=" view ID does not win over the
	// database ID in the path.
	pathIDPattern  = regexp.MustCompile(`(?i)/[^/?#]*-([0-9a-f]{32})(?:[/?#]|$)`)
	queryIDPattern = regexp.MustCompile(`(?i)[?&](?:p|page_id|database_id)=([0-9a-f]{32})`)
	anyIDPattern   = regexp.MustCompile(`(?i)([0-9a-f]{32})`)
	blockIDPattern = regexp.MustCompile(`(?i)#(?:block-)?([0-9a-f]{32})`)
)

// formatUUID inserts the standard hyphens into a 32-character hex ID.
func formatUUID(compact string) string {
	c := strings.ToLower(compact)
	return c[0:8] + "-" + c[8:12] + "-" + c[12:16] + "-" + c[16:20] + "-" + c[20:32]
}

// ExtractID pulls a Notion ID out of a URL or returns an ID unchanged, always
// in the hyphenated lowercase form.
//
// It accepts what a user is likely to paste: a page URL, a bare ID with or
// without hyphens, or a URL with query parameters. It returns the empty string
// when it finds no ID.
//
//	notion.ExtractID("https://notion.so/My-Page-1429989fe8ac4effbc8f57f56486db54")
//	// "1429989f-e8ac-4eff-bc8f-57f56486db54"
//
// An ID in the path wins over one in the query string, so a URL carrying a
// "?v=" view ID still yields the database ID.
func ExtractID(urlOrID string) string {
	trimmed := strings.TrimSpace(urlOrID)
	if trimmed == "" {
		return ""
	}

	if dashedUUIDPattern.MatchString(trimmed) {
		return strings.ToLower(trimmed)
	}
	if compactUUIDPattern.MatchString(trimmed) {
		return formatUUID(trimmed)
	}
	for _, pattern := range []*regexp.Regexp{pathIDPattern, queryIDPattern, anyIDPattern} {
		if match := pattern.FindStringSubmatch(trimmed); match != nil {
			return formatUUID(match[1])
		}
	}
	return ""
}

// ExtractBlockID pulls a block ID from a URL fragment, the form Notion uses
// when you copy a link to a specific block. It returns the empty string when
// the URL carries no block fragment.
//
//	notion.ExtractBlockID("https://notion.so/Page-abc#block-1429989fe8ac4effbc8f57f56486db54")
//	// "1429989f-e8ac-4eff-bc8f-57f56486db54"
func ExtractBlockID(url string) string {
	if match := blockIDPattern.FindStringSubmatch(strings.TrimSpace(url)); match != nil {
		return formatUUID(match[1])
	}
	return ""
}
