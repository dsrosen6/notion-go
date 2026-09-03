package notion

import (
	"context"
	"net/http"
	"time"
)

// DataSourceRef names one data source inside a database.
type DataSourceRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Database is a container for one or more data sources.
//
// Notion split databases from data sources in API version 2025-09-03: a
// database holds the shared title, icon, and location, while each data source
// holds a schema and rows. A database created in the UI has exactly one data
// source, so [Database.DataSources] usually has a single entry — which is what
// you pass to [DataSourcesService.Query].
//
// Notion returns a partial database — only Object and ID — when the integration
// lacks read access. Check [Database.IsFull].
type Database struct {
	Object string `json:"object"`
	ID     string `json:"id"`

	Title       RichTextList `json:"title,omitempty"`
	Description RichTextList `json:"description,omitempty"`
	Parent      Parent       `json:"parent,omitzero"`
	// DataSources lists the database's data sources. Query one of these rather
	// than the database itself.
	DataSources []DataSourceRef `json:"data_sources,omitempty"`

	// IsInline reports whether the database renders inside its parent page
	// rather than as its own page.
	IsInline bool `json:"is_inline,omitempty"`
	// DatabaseType names a typed database, such as "tasks" or "projects", and
	// is empty for a regular one.
	DatabaseType string `json:"database_type,omitempty"`
	InTrash      bool   `json:"in_trash,omitempty"`
	IsLocked     bool   `json:"is_locked,omitempty"`

	CreatedTime    time.Time `json:"created_time,omitzero"`
	LastEditedTime time.Time `json:"last_edited_time,omitzero"`

	Icon  *Icon  `json:"icon,omitempty"`
	Cover *File  `json:"cover,omitempty"`
	URL   string `json:"url,omitempty"`
	// PublicURL is set only when the database has been published to the web.
	PublicURL string `json:"public_url,omitempty"`
}

// IsFull reports whether the database carries more than an ID. Notion signals a
// full database by including its title, per isFullDatabase in
// helpers.ts:387-391.
func (d *Database) IsFull() bool { return d != nil && d.Title != nil }

// DatabasesService creates and edits databases. Reach it through
// [Client.Databases].
//
// To read or query rows, use [Client.DataSources] with an ID from
// [Database.DataSources].
type DatabasesService struct {
	c *Client
}

// CreateDatabaseParams describes a database to create.
type CreateDatabaseParams struct {
	// Parent is the page the database lives on. Only a page parent is
	// supported.
	Parent      Parent       `json:"parent"`
	Title       RichTextList `json:"title,omitempty"`
	Description RichTextList `json:"description,omitempty"`
	// InitialDataSource is the schema of the data source created alongside the
	// database. Without it the database gets a title column only.
	InitialDataSource *InitialDataSource `json:"initial_data_source,omitempty"`
	// IsInline renders the database inside its parent page.
	IsInline bool `json:"is_inline,omitempty"`
	// DatabaseType creates a typed database. Notion accepts "tasks",
	// "projects", and "skills".
	DatabaseType string `json:"database_type,omitempty"`
	Icon         *Icon  `json:"icon,omitempty"`
	Cover        *File  `json:"cover,omitempty"`
}

// InitialDataSource is the schema for the data source created with a database.
type InitialDataSource struct {
	Properties PropertySchemas `json:"properties,omitempty"`
}

// Create adds a new database along with its first data source.
//
//	db, err := client.Databases.Create(ctx, notion.CreateDatabaseParams{
//		Parent: notion.Parent{Type: notion.ParentTypePage, PageID: pageID},
//		Title:  notion.NewRichText("Tasks"),
//		InitialDataSource: &notion.InitialDataSource{
//			Properties: notion.PropertySchemas{
//				"Name":     &notion.TitleSchema{},
//				"Priority": &notion.NumberSchema{},
//			},
//		},
//	})
func (s *DatabasesService) Create(ctx context.Context, params CreateDatabaseParams, opts ...RequestOption) (*Database, error) {
	var out Database
	err := s.c.do(ctx, request{
		method: http.MethodPost,
		path:   "databases",
		body:   params,
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Retrieve returns a database by ID.
//
// The result describes the container, not its schema or rows. Read those from
// one of its data sources.
func (s *DatabasesService) Retrieve(ctx context.Context, databaseID string, opts ...RequestOption) (*Database, error) {
	var out Database
	err := s.c.do(ctx, request{
		method: http.MethodGet,
		path:   "databases/" + escapeID(databaseID),
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateDatabaseParams describes an edit to a database. Only the fields set are
// changed.
type UpdateDatabaseParams struct {
	Title       RichTextList `json:"title,omitempty"`
	Description RichTextList `json:"description,omitempty"`
	// Parent moves the database to another page.
	Parent   *Parent `json:"parent,omitempty"`
	IsInline *bool   `json:"is_inline,omitempty"`
	Icon     *Icon   `json:"icon,omitempty"`
	Cover    *File   `json:"cover,omitempty"`
	InTrash  *bool   `json:"in_trash,omitempty"`
	IsLocked *bool   `json:"is_locked,omitempty"`
}

// Update changes a database's title, description, location, or trashed state.
//
// It does not change the schema; use [DataSourcesService.Update] for that.
func (s *DatabasesService) Update(ctx context.Context, databaseID string, params UpdateDatabaseParams, opts ...RequestOption) (*Database, error) {
	var out Database
	err := s.c.do(ctx, request{
		method: http.MethodPatch,
		path:   "databases/" + escapeID(databaseID),
		body:   params,
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Trash moves a database to the trash.
func (s *DatabasesService) Trash(ctx context.Context, databaseID string, opts ...RequestOption) (*Database, error) {
	trash := true
	return s.Update(ctx, databaseID, UpdateDatabaseParams{InTrash: &trash}, opts...)
}
