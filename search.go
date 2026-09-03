package notion

import (
	"context"
	"iter"
	"net/http"
)

// SearchParams narrows a workspace search.
type SearchParams struct {
	// Query matches against page and data source titles. An empty query
	// returns everything the integration can see.
	Query string `json:"query,omitempty"`
	// Filter restricts results to one object type, to the trash, or both.
	Filter *SearchFilter `json:"filter,omitempty"`
	// Sort orders the results, by last edit time or by relevance.
	Sort *SearchSort `json:"sort,omitempty"`
	// PageParams sets the cursor and page size, which this endpoint takes in
	// the request body rather than the query string.
	PageParams
}

// SearchFilter restricts a search to pages or to data sources, and optionally
// to objects in or out of the trash. Either part may be given alone, mirroring
// the two filter shapes in api-endpoints/search.ts:14-24.
type SearchFilter struct {
	// Property is "object" when filtering by object type.
	Property string `json:"property,omitempty"`
	// Value is "page" or "data_source".
	Value string `json:"value,omitempty"`
	// InTrash, when set, restricts results to trashed (true) or live (false)
	// objects. Nil applies no trash filter.
	InTrash *bool `json:"in_trash,omitempty"`
}

// SearchPages restricts a search to pages.
func SearchPages() *SearchFilter {
	return &SearchFilter{Property: "object", Value: "page"}
}

// SearchDataSources restricts a search to data sources.
func SearchDataSources() *SearchFilter {
	return &SearchFilter{Property: "object", Value: "data_source"}
}

// SearchInTrash restricts a search to trashed (true) or live (false) objects of
// any type. Combine with an object type via [SearchFilter.WithInTrash].
func SearchInTrash(inTrash bool) *SearchFilter {
	return &SearchFilter{InTrash: &inTrash}
}

// WithInTrash additionally restricts the filter to trashed (true) or live
// (false) objects and returns f for chaining:
//
//	notion.SearchPages().WithInTrash(true)
func (f *SearchFilter) WithInTrash(inTrash bool) *SearchFilter {
	f.InTrash = &inTrash
	return f
}

// SearchSort orders search results, either by last edit time or by relevance.
// Exactly one of the two forms is sent, mirroring the sort union in
// api-endpoints/search.ts:14-17.
type SearchSort struct {
	// Timestamp is "last_edited_time" when sorting by edit time.
	Timestamp string `json:"timestamp,omitempty"`
	// Direction accompanies Timestamp.
	Direction SortDirection `json:"direction,omitempty"`
	// Property is "relevance" when sorting by relevance.
	Property string `json:"property,omitempty"`
}

// SortByLastEdited orders search results by when each object was last edited.
func SortByLastEdited(direction SortDirection) *SearchSort {
	return &SearchSort{Timestamp: "last_edited_time", Direction: direction}
}

// SortByRelevance orders search results by how well they match the query.
func SortByRelevance() *SearchSort {
	return &SearchSort{Property: "relevance"}
}

// Search finds pages and data sources by title across the workspace.
//
// It returns only what the integration has been shared with, so an empty result
// usually means the integration lacks access rather than that nothing matched.
//
// Notion's search index is eventually consistent: a page created moments ago
// may not appear yet.
func (c *Client) Search(ctx context.Context, params SearchParams, opts ...RequestOption) (*List[QueryResult], error) {
	var out List[QueryResult]
	err := c.do(ctx, request{
		method: http.MethodPost,
		path:   "search",
		body:   params,
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SearchAll iterates every search result, fetching pages as needed.
//
//	for result, err := range client.SearchAll(ctx, notion.SearchParams{Query: "roadmap"}) {
//		if err != nil {
//			return err
//		}
//		if result.Page != nil {
//			fmt.Println(result.Page.Title())
//		}
//	}
func (c *Client) SearchAll(ctx context.Context, params SearchParams, opts ...RequestOption) iter.Seq2[QueryResult, error] {
	return paginate(ctx, params.StartCursor, func(ctx context.Context, cursor string) ([]QueryResult, string, error) {
		params.StartCursor = cursor
		page, err := c.Search(ctx, params, opts...)
		if err != nil {
			return nil, "", err
		}
		return page.Results, page.NextCursor, nil
	})
}
