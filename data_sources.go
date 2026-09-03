package notion

import (
	"context"
	"encoding/json"
	"iter"
	"net/http"
	"time"
)

// DataSource is a set of rows sharing one schema, inside a [Database].
//
// Databases and data sources were separated in API version 2025-09-03. Queries,
// schemas, and rows belong to the data source; the title, icon, and location
// belong to the database.
//
// Notion returns a partial data source when the integration lacks read access.
// Check [DataSource.IsFull].
type DataSource struct {
	Object string `json:"object"`
	ID     string `json:"id"`

	Title       RichTextList `json:"title,omitempty"`
	Description RichTextList `json:"description,omitempty"`
	// Parent is the containing database.
	Parent Parent `json:"parent,omitzero"`
	// DatabaseParent is the page or workspace the containing database lives on.
	DatabaseParent Parent `json:"database_parent,omitzero"`
	// Properties is the schema: the data source's columns, keyed by name.
	Properties PropertySchemas `json:"properties,omitempty"`

	IsInline bool `json:"is_inline,omitempty"`
	// DatabaseType names a typed database, empty for a regular one.
	DatabaseType string `json:"database_type,omitempty"`
	InTrash      bool   `json:"in_trash,omitempty"`

	CreatedTime    time.Time `json:"created_time,omitzero"`
	LastEditedTime time.Time `json:"last_edited_time,omitzero"`
	CreatedBy      User      `json:"created_by,omitzero"`
	LastEditedBy   User      `json:"last_edited_by,omitzero"`

	Icon      *Icon  `json:"icon,omitempty"`
	Cover     *File  `json:"cover,omitempty"`
	URL       string `json:"url,omitempty"`
	PublicURL string `json:"public_url,omitempty"`
}

// IsFull reports whether the data source carries more than an ID and schema.
// Notion signals a full data source by including its title, per
// isFullDataSource in helpers.ts:378-382.
func (d *DataSource) IsFull() bool { return d != nil && d.Title != nil }

// DataSourcesService queries and edits data sources. Reach it through
// [Client.DataSources].
type DataSourcesService struct {
	c *Client
}

// Retrieve returns a data source by ID, including its schema.
func (s *DataSourcesService) Retrieve(ctx context.Context, dataSourceID string, opts ...RequestOption) (*DataSource, error) {
	var out DataSource
	err := s.c.do(ctx, request{
		method: http.MethodGet,
		path:   "data_sources/" + escapeID(dataSourceID),
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateDataSourceParams describes a data source to add to a database.
type CreateDataSourceParams struct {
	// Parent is the database to add it to; set Type to
	// [ParentTypeDatabase].
	Parent     Parent          `json:"parent"`
	Title      RichTextList    `json:"title,omitempty"`
	Properties PropertySchemas `json:"properties,omitempty"`
	Icon       *Icon           `json:"icon,omitempty"`
}

// Create adds a data source to an existing database.
func (s *DataSourcesService) Create(ctx context.Context, params CreateDataSourceParams, opts ...RequestOption) (*DataSource, error) {
	var out DataSource
	err := s.c.do(ctx, request{
		method: http.MethodPost,
		path:   "data_sources",
		body:   params,
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateDataSourceParams describes an edit to a data source.
type UpdateDataSourceParams struct {
	Title RichTextList `json:"title,omitempty"`
	// Properties changes the schema. A column mapped to nil is deleted; a
	// column absent from the map is left alone.
	Properties PropertySchemas `json:"properties,omitempty"`
	Icon       *Icon           `json:"icon,omitempty"`
	Parent     *Parent         `json:"parent,omitempty"`
	InTrash    *bool           `json:"in_trash,omitempty"`
}

// Update changes a data source's title, schema, or location.
func (s *DataSourcesService) Update(ctx context.Context, dataSourceID string, params UpdateDataSourceParams, opts ...RequestOption) (*DataSource, error) {
	var out DataSource
	err := s.c.do(ctx, request{
		method: http.MethodPatch,
		path:   "data_sources/" + escapeID(dataSourceID),
		body:   params,
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// QueryParams selects and orders the rows a query returns.
type QueryParams struct {
	// Filter selects which rows to return. A nil filter returns every row.
	Filter Filter `json:"filter,omitempty"`
	// Sorts orders the results. Earlier entries take precedence.
	Sorts []Sort `json:"sorts,omitempty"`
	// PageParams sets the cursor and page size, which this endpoint takes in
	// the request body rather than the query string.
	PageParams
}

// QueryParams deliberately has no in_trash field. The JavaScript SDK lists one
// (data-sources.ts queryDataSource bodyParams), but the live API rejects it
// with "body.in_trash should be not present" under both 2025-09-03 and
// 2026-03-11, so exposing it would only produce a validation_error.

// validate checks what can be checked before sending, so a malformed query
// fails with a clear error rather than a server-side validation_error.
func (p QueryParams) validate() error {
	return validateFilterDepth(p.Filter)
}

// QueryResult is one row returned by a query. A wiki database can return data
// sources alongside pages, so exactly one of Page and DataSource is set.
type QueryResult struct {
	// Object is "page" or "data_source".
	Object     string
	Page       *Page
	DataSource *DataSource
}

func (r *QueryResult) UnmarshalJSON(data []byte) error {
	var probe struct {
		Object string `json:"object"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	r.Object = probe.Object

	if probe.Object == "data_source" {
		var ds DataSource
		if err := json.Unmarshal(data, &ds); err != nil {
			return err
		}
		r.DataSource = &ds
		return nil
	}
	var page Page
	if err := json.Unmarshal(data, &page); err != nil {
		return err
	}
	r.Page = &page
	return nil
}

// QueryResults is a page of query results.
type QueryResults []QueryResult

// Pages returns just the pages, dropping any data sources a wiki database
// returned alongside them.
func (r QueryResults) Pages() []*Page {
	pages := make([]*Page, 0, len(r))
	for _, result := range r {
		if result.Page != nil {
			pages = append(pages, result.Page)
		}
	}
	return pages
}

// Query returns one page of the rows matching params.
//
// Most callers want [DataSourcesService.QueryAll], which handles the cursors.
func (s *DataSourcesService) Query(ctx context.Context, dataSourceID string, params QueryParams, opts ...RequestOption) (*List[QueryResult], error) {
	if err := params.validate(); err != nil {
		return nil, err
	}

	var out List[QueryResult]
	err := s.c.do(ctx, request{
		method: http.MethodPost,
		path:   "data_sources/" + escapeID(dataSourceID) + "/query",
		body:   params,
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// QueryAll iterates every row matching params, fetching pages as needed.
//
//	filter := notion.And(
//		notion.ByStatus("Status").Equals("In progress"),
//		notion.ByNumber("Priority").GreaterThanOrEqualTo(3),
//	)
//	for page, err := range client.DataSources.QueryAll(ctx, dsID, notion.QueryParams{Filter: filter}) {
//		if err != nil {
//			return err
//		}
//		fmt.Println(page.Title())
//	}
//
// Rows that are data sources rather than pages, which only wiki databases
// return, are skipped. Use [DataSourcesService.Query] directly to see them.
func (s *DataSourcesService) QueryAll(ctx context.Context, dataSourceID string, params QueryParams, opts ...RequestOption) iter.Seq2[*Page, error] {
	return paginate(ctx, params.StartCursor, func(ctx context.Context, cursor string) ([]*Page, string, error) {
		params.StartCursor = cursor
		page, err := s.Query(ctx, dataSourceID, params, opts...)
		if err != nil {
			return nil, "", err
		}
		return QueryResults(page.Results).Pages(), page.NextCursor, nil
	})
}
