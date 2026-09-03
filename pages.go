package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Page is a Notion page: a document, or one row of a data source.
//
// Notion returns a partial page — only Object and ID — when the integration
// lacks read access to it. Check [Page.IsFull] before reading anything else.
type Page struct {
	Object string `json:"object"`
	ID     string `json:"id"`

	CreatedTime    time.Time `json:"created_time"`
	LastEditedTime time.Time `json:"last_edited_time"`
	// CreatedBy and LastEditedBy are always partial users carrying only an ID.
	CreatedBy    User `json:"created_by"`
	LastEditedBy User `json:"last_edited_by"`

	// InTrash reports whether the page is in the trash but still recoverable.
	InTrash bool `json:"in_trash"`
	// IsArchived reports whether the page has been archived.
	IsArchived bool `json:"is_archived"`
	// IsLocked reports whether editing is locked in the Notion app.
	IsLocked bool `json:"is_locked"`

	// URL is the page's address in Notion.
	URL string `json:"url"`
	// PublicURL is set only when the page has been published to the web.
	PublicURL string `json:"public_url,omitempty"`

	Parent Parent `json:"parent"`
	// Properties holds the page's column values, keyed by column name. It is
	// empty for a page that is not in a data source, apart from its title.
	Properties PropertyValues `json:"properties,omitempty"`
	Icon       *Icon          `json:"icon,omitempty"`
	Cover      *File          `json:"cover,omitempty"`
}

// IsFull reports whether the page carries more than an ID.
//
// Notion signals a full page by including its URL, per isFullPage in
// helpers.ts:369-373.
func (p *Page) IsFull() bool { return p != nil && p.URL != "" }

// Title returns the plain text of the page's title, or the empty string if the
// page is partial or has none.
func (p *Page) Title() string {
	if p == nil {
		return ""
	}
	return p.Properties.Title()
}

// PagesService creates and edits pages. Reach it through [Client.Pages].
type PagesService struct {
	c *Client
}

// CreatePageParams describes a page to create. Parent is required; everything
// else is optional.
type CreatePageParams struct {
	// Parent is the page or data source to create this page under. Set exactly
	// one of its ID fields along with the matching Type.
	Parent Parent `json:"parent"`
	// Properties are the page's column values, keyed by column name. A page
	// under a data source must set the title column; a page under another page
	// takes only a title.
	Properties PropertyValues `json:"properties,omitempty"`
	Icon       *Icon          `json:"icon,omitempty"`
	Cover      *File          `json:"cover,omitempty"`
	// Children is the page's initial content, at most 100 blocks. Append the
	// rest with [BlocksService.AppendChildren].
	Children BlockList `json:"children,omitempty"`
}

// Create adds a new page.
//
//	page, err := client.Pages.Create(ctx, notion.CreatePageParams{
//		Parent: notion.Parent{Type: notion.ParentTypeDataSource, DataSourceID: dsID},
//		Properties: notion.PropertyValues{
//			"Name": notion.NewTitle("Write the docs"),
//		},
//	})
func (s *PagesService) Create(ctx context.Context, params CreatePageParams, opts ...RequestOption) (*Page, error) {
	var out Page
	err := s.c.do(ctx, request{
		method: http.MethodPost,
		path:   "pages",
		body:   params,
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Retrieve returns a page by ID.
//
// It does not return the page's content. Read that with
// [BlocksService.Children] or [BlocksService.AllChildren].
//
// Pass [WithFilterProperties] to return only specific columns, which is worth
// doing for wide data sources.
func (s *PagesService) Retrieve(ctx context.Context, pageID string, opts ...RequestOption) (*Page, error) {
	var out Page
	err := s.c.do(ctx, request{
		method: http.MethodGet,
		path:   "pages/" + escapeID(pageID),
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdatePageParams describes an edit to a page. Only the fields set are
// changed.
type UpdatePageParams struct {
	// Properties holds the column values to change, keyed by column name.
	// Columns left out keep their current values.
	Properties PropertyValues `json:"properties,omitempty"`
	Icon       *Icon          `json:"icon,omitempty"`
	Cover      *File          `json:"cover,omitempty"`
	// InTrash moves the page to or from the trash.
	InTrash *bool `json:"in_trash,omitempty"`
	// IsArchived archives or unarchives the page.
	IsArchived *bool `json:"is_archived,omitempty"`
	// IsLocked locks or unlocks the page against edits in the Notion app.
	IsLocked *bool `json:"is_locked,omitempty"`
}

// Update changes a page's properties, icon, cover, or archival state.
//
// It cannot change page content; use the block endpoints for that.
func (s *PagesService) Update(ctx context.Context, pageID string, params UpdatePageParams, opts ...RequestOption) (*Page, error) {
	var out Page
	err := s.c.do(ctx, request{
		method: http.MethodPatch,
		path:   "pages/" + escapeID(pageID),
		body:   params,
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Trash moves a page to the trash. Notion has no hard delete for pages; the
// page remains recoverable until a workspace admin empties the trash.
func (s *PagesService) Trash(ctx context.Context, pageID string, opts ...RequestOption) (*Page, error) {
	trash := true
	return s.Update(ctx, pageID, UpdatePageParams{InTrash: &trash}, opts...)
}

// Restore recovers a page from the trash.
func (s *PagesService) Restore(ctx context.Context, pageID string, opts ...RequestOption) (*Page, error) {
	trash := false
	return s.Update(ctx, pageID, UpdatePageParams{InTrash: &trash}, opts...)
}

// Move relocates a page under a different parent.
func (s *PagesService) Move(ctx context.Context, pageID string, parent Parent, opts ...RequestOption) (*Page, error) {
	body := struct {
		Parent Parent `json:"parent"`
	}{Parent: parent}

	var out Page
	err := s.c.do(ctx, request{
		method: http.MethodPost,
		path:   "pages/" + escapeID(pageID) + "/move",
		body:   body,
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// RetrieveProperty returns one property of a page by its property ID.
//
// Use it for the properties [PagesService.Retrieve] truncates: a relation with
// more than 25 entries, or a rollup over one. For every other property type,
// reading the page is simpler and cheaper.
//
// Properties whose value is a list — title, rich text, people, relation,
// rollup — come back paginated, so the result holds one page of entries in
// Results rather than a single value. This method fetches exactly the page
// params addresses and does not follow NextCursor: to read every entry, call
// it again with PageParams{StartCursor: item.NextCursor} until NextCursor is
// empty. [PropertyItem.Combined] folds one page's entries into a single
// [PropertyValue].
func (s *PagesService) RetrieveProperty(ctx context.Context, pageID, propertyID string, params PageParams, opts ...RequestOption) (*PropertyItem, error) {
	var out PropertyItem
	err := s.c.do(ctx, request{
		method: http.MethodGet,
		path:   "pages/" + escapeID(pageID) + "/properties/" + escapeID(propertyID),
		query:  params.query(),
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// PropertyItem is the result of [PagesService.RetrieveProperty].
//
// A single-valued property returns Value. A list-valued one returns a page of
// Results, with NextCursor set when more remain.
type PropertyItem struct {
	Object string `json:"object"`
	// Value holds the property when it has a single value. It is nil for
	// list-valued properties.
	Value PropertyValue
	// Results holds one page of entries for a list-valued property.
	Results []PropertyValue
	// NextCursor addresses the next page of Results, empty when there are no
	// more.
	NextCursor string
	// PropertyItem describes the underlying column for a list-valued property.
	PropertyItem json.RawMessage
}

func (p *PropertyItem) UnmarshalJSON(data []byte) error {
	var envelope struct {
		Object       string            `json:"object"`
		Results      []json.RawMessage `json:"results"`
		NextCursor   string            `json:"next_cursor"`
		PropertyItem json.RawMessage   `json:"property_item"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	p.Object = envelope.Object
	p.NextCursor = envelope.NextCursor
	p.PropertyItem = envelope.PropertyItem

	// A list response carries "results"; a single-valued one is itself a
	// property value.
	if envelope.Object == "list" {
		p.Results = make([]PropertyValue, len(envelope.Results))
		for i, raw := range envelope.Results {
			value, err := decodePropertyItemResult(raw)
			if err != nil {
				return fmt.Errorf("notion: decoding property item result %d: %w", i, err)
			}
			p.Results[i] = value
		}
		return nil
	}

	value, err := DecodePropertyValue(data)
	if err != nil {
		return err
	}
	p.Value = value
	return nil
}

// singleItemTypes are the property types whose list entries carry one element
// where the page property carries an array: title and rich_text hold a single
// rich text run, people a single user, and relation a single {id}
// (pages.ts:136-141, 247-259, 293-298).
var singleItemTypes = map[string]bool{
	string(PropertyTypeTitle):    true,
	string(PropertyTypeRichText): true,
	string(PropertyTypePeople):   true,
	string(PropertyTypeRelation): true,
}

// decodePropertyItemResult decodes one entry of a property item list. Each
// entry has the shape of a page property except that, for the list-valued
// types, the payload is a single element rather than an array. Wrapping it in
// a one-element array lets it decode as the same [PropertyValue] type a page
// carries, so callers see a [*TitleProperty] holding one run, and so on.
func decodePropertyItemResult(raw json.RawMessage) (PropertyValue, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	var tag string
	if typeRaw, ok := fields["type"]; ok {
		if err := json.Unmarshal(typeRaw, &tag); err != nil {
			return nil, err
		}
	}
	payload, ok := fields[tag]
	if !ok || !singleItemTypes[tag] || bytes.HasPrefix(bytes.TrimSpace(payload), []byte("[")) {
		return DecodePropertyValue(raw)
	}
	wrapped := make(json.RawMessage, 0, len(payload)+2)
	wrapped = append(wrapped, '[')
	wrapped = append(wrapped, payload...)
	wrapped = append(wrapped, ']')
	fields[tag] = wrapped
	data, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	return DecodePropertyValue(data)
}

// Combined returns the property as a single value.
//
// For a single-valued response it returns Value. For a list response it merges
// this page's Results into one [*TitleProperty], [*RichTextProperty],
// [*PeopleProperty], or [*RelationProperty] whose slice holds every entry in
// order. A rollup list, whose entries are values of the related rows rather
// than of the rollup itself, and an empty page yield nil; read Results
// directly for those.
//
// [PagesService.RetrieveProperty] fetches one page at a time and does not
// follow NextCursor, so Combined covers only that page. To read a property
// with more entries than one page holds, call RetrieveProperty again with
// PageParams{StartCursor: item.NextCursor} until NextCursor is empty and
// combine each page.
func (p *PropertyItem) Combined() PropertyValue {
	if p == nil {
		return nil
	}
	if p.Object != "list" {
		return p.Value
	}
	if len(p.Results) == 0 {
		return nil
	}
	switch first := p.Results[0].(type) {
	case *TitleProperty:
		merged := &TitleProperty{propertyBase: first.propertyBase}
		for _, result := range p.Results {
			if item, ok := result.(*TitleProperty); ok {
				merged.Title = append(merged.Title, item.Title...)
			}
		}
		return merged
	case *RichTextProperty:
		merged := &RichTextProperty{propertyBase: first.propertyBase}
		for _, result := range p.Results {
			if item, ok := result.(*RichTextProperty); ok {
				merged.RichText = append(merged.RichText, item.RichText...)
			}
		}
		return merged
	case *PeopleProperty:
		merged := &PeopleProperty{propertyBase: first.propertyBase}
		for _, result := range p.Results {
			if item, ok := result.(*PeopleProperty); ok {
				merged.People = append(merged.People, item.People...)
			}
		}
		return merged
	case *RelationProperty:
		merged := &RelationProperty{propertyBase: first.propertyBase}
		for _, result := range p.Results {
			if item, ok := result.(*RelationProperty); ok {
				merged.Relation = append(merged.Relation, item.Relation...)
			}
		}
		return merged
	default:
		return nil
	}
}

// NewTitle returns a title property holding the given text, for building
// create and update requests.
func NewTitle(text string) *TitleProperty {
	return &TitleProperty{Title: NewRichText(text)}
}

// NewText returns a rich text property holding the given text.
func NewText(text string) *RichTextProperty {
	return &RichTextProperty{RichText: NewRichText(text)}
}

// NewNumber returns a number property holding n.
func NewNumber(n float64) *NumberProperty {
	return &NumberProperty{Number: &n}
}

// NewCheckbox returns a checkbox property.
func NewCheckbox(checked bool) *CheckboxProperty {
	return &CheckboxProperty{Checkbox: checked}
}

// NewSelect returns a select property choosing the option with the given name.
func NewSelect(name string) *SelectProperty {
	return &SelectProperty{Select: &SelectOption{Name: name}}
}

// NewStatus returns a status property set to the option with the given name.
func NewStatus(name string) *StatusProperty {
	return &StatusProperty{Status: &SelectOption{Name: name}}
}

// NewMultiSelect returns a multi-select property choosing the named options.
func NewMultiSelect(names ...string) *MultiSelectProperty {
	options := make([]SelectOption, len(names))
	for i, name := range names {
		options[i] = SelectOption{Name: name}
	}
	return &MultiSelectProperty{MultiSelect: options}
}

// NewDateValue returns a date property covering a single date or time.
func NewDateValue(d DateTime) *DateProperty {
	return &DateProperty{Date: &Date{Start: d}}
}

// NewDateRange returns a date property covering a range.
func NewDateRange(start, end DateTime) *DateProperty {
	return &DateProperty{Date: &Date{Start: start, End: &end}}
}

// NewRelation returns a relation property linking to the given page IDs.
func NewRelation(pageIDs ...string) *RelationProperty {
	refs := make([]IDRef, len(pageIDs))
	for i, id := range pageIDs {
		refs[i] = IDRef{ID: id}
	}
	return &RelationProperty{Relation: refs}
}
