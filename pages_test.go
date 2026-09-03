package notion

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func decodePage(data []byte) (*Page, error) {
	var page Page
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

func TestPageFixture(t *testing.T) {
	page := assertRoundTrip(t, readFixture(t, "page.json"), decodePage)

	if !page.IsFull() {
		t.Error("IsFull = false, want true")
	}
	if got := page.Title(); got != "Ship the SDK" {
		t.Errorf("Title = %q, want %q", got, "Ship the SDK")
	}
	if page.Parent.Type != ParentTypeDataSource || page.Parent.ID() != "ds-1" {
		t.Errorf("Parent = %+v, want data source ds-1", page.Parent)
	}
	// A data source parent carries its database ID too.
	if page.Parent.DatabaseID != "db-1" {
		t.Errorf("Parent.DatabaseID = %q, want db-1", page.Parent.DatabaseID)
	}
	if page.Icon == nil || page.Icon.Emoji != "🚀" {
		t.Errorf("Icon = %+v, want the rocket emoji", page.Icon)
	}
	if page.Cover != nil {
		t.Errorf("Cover = %+v, want nil", page.Cover)
	}
}

func TestPagePropertyAccessors(t *testing.T) {
	page, err := decodePage(readFixture(t, "page.json"))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	props := page.Properties

	if got := props.Select("Status"); got != "In progress" {
		t.Errorf("Select(Status) = %q, want In progress", got)
	}
	if got, ok := props.Number("Priority"); !ok || got != 3 {
		t.Errorf("Number(Priority) = %v, %v, want 3, true", got, ok)
	}
	if props.Checkbox("Done") {
		t.Error("Checkbox(Done) = true, want false")
	}
	if got := props.Text("Name"); got != "Ship the SDK" {
		t.Errorf("Text(Name) = %q", got)
	}

	// An empty number is distinct from zero.
	if got, ok := props.Number("Empty"); ok {
		t.Errorf("Number(Empty) = %v, %v, want ok false", got, ok)
	}
	// A missing or wrongly typed property reports absence rather than panicking.
	if _, ok := props.Number("Nonexistent"); ok {
		t.Error("Number(Nonexistent) reported ok")
	}
	if got := props.Select("Priority"); got != "" {
		t.Errorf("Select on a number property = %q, want empty", got)
	}

	due := props.Date("Due")
	if due == nil || due.Start.HasTime {
		t.Errorf("Date(Due) = %+v, want a date without a time", due)
	}
}

func TestPagePropertyTypes(t *testing.T) {
	page, err := decodePage(readFixture(t, "page.json"))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	tags, ok := page.Properties["Tags"].(*MultiSelectProperty)
	if !ok {
		t.Fatalf("Tags = %T, want *MultiSelectProperty", page.Properties["Tags"])
	}
	if len(tags.MultiSelect) != 2 || tags.MultiSelect[0].Name != "go" {
		t.Errorf("Tags = %+v", tags.MultiSelect)
	}

	owner, ok := page.Properties["Owner"].(*PeopleProperty)
	if !ok {
		t.Fatalf("Owner = %T, want *PeopleProperty", page.Properties["Owner"])
	}
	if len(owner.People) != 1 || owner.People[0].Name != "Ada" {
		t.Errorf("Owner = %+v", owner.People)
	}

	ticket, ok := page.Properties["Ticket"].(*UniqueIDProperty)
	if !ok {
		t.Fatalf("Ticket = %T, want *UniqueIDProperty", page.Properties["Ticket"])
	}
	if ticket.UniqueID.Prefix != "TASK" || ticket.UniqueID.Number == nil || *ticket.UniqueID.Number != 42 {
		t.Errorf("Ticket = %+v, want TASK-42", ticket.UniqueID)
	}

	// Property IDs survive decoding; they address a column across renames.
	if got := page.Properties["Status"].PropertyID(); got != "abc1" {
		t.Errorf("Status PropertyID = %q, want abc1", got)
	}
}

func TestPartialPage(t *testing.T) {
	// Notion returns only an ID when the integration lacks read access.
	page, err := decodePage([]byte(`{"object":"page","id":"p1"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if page.IsFull() {
		t.Error("IsFull = true, want false for a partial page")
	}
	if page.Title() != "" {
		t.Errorf("Title = %q, want empty", page.Title())
	}
	// A nil page must not panic.
	var nilPage *Page
	if nilPage.IsFull() || nilPage.Title() != "" {
		t.Error("nil page accessors misbehaved")
	}
}

func TestUnknownPropertyType(t *testing.T) {
	page, err := decodePage([]byte(`{"object":"page","id":"p1","url":"https://notion.so/p1",
		"properties":{"Weird":{"id":"w1","type":"quantum","quantum":{"spin":"up"}}}}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	unknown, ok := page.Properties["Weird"].(*UnknownProperty)
	if !ok {
		t.Fatalf("Weird = %T, want *UnknownProperty", page.Properties["Weird"])
	}
	if unknown.PropertyType() != "quantum" || unknown.PropertyID() != "w1" {
		t.Errorf("unknown = %+v", unknown)
	}
}

func TestPagesCreate(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/pages" {
			t.Errorf("got %s %s, want POST /v1/pages", r.Method, r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write(readFixture(t, "page.json"))
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	page, err := c.Pages.Create(context.Background(), CreatePageParams{
		Parent: Parent{Type: ParentTypeDataSource, DataSourceID: "ds-1"},
		Properties: PropertyValues{
			"Name":     NewTitle("Ship the SDK"),
			"Priority": NewNumber(3),
		},
		Children: BlockList{NewParagraph("Body text")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	parent := gotBody["parent"].(map[string]any)
	if parent["data_source_id"] != "ds-1" || parent["type"] != "data_source_id" {
		t.Errorf("parent = %#v", parent)
	}
	props := gotBody["properties"].(map[string]any)
	name := props["Name"].(map[string]any)
	if name["type"] != "title" {
		t.Errorf("Name property = %#v, want a title", name)
	}
	if _, present := gotBody["children"]; !present {
		t.Error("children were not sent")
	}
	if page.Title() != "Ship the SDK" {
		t.Errorf("Title = %q", page.Title())
	}
}

func TestPagesUpdateOmitsUnsetFields(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write(readFixture(t, "page.json"))
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	_, err := c.Pages.Update(context.Background(), "p1", UpdatePageParams{
		Properties: PropertyValues{"Done": NewCheckbox(true)},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Unset fields must be omitted, not sent as null, or they would clear the
	// page's icon and cover.
	for _, key := range []string{"icon", "cover", "in_trash", "is_archived", "is_locked"} {
		if _, present := gotBody[key]; present {
			t.Errorf("body carries %q, want it omitted: %#v", key, gotBody)
		}
	}
}

func TestPagesTrashAndRestore(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		io.WriteString(w, `{"object":"page","id":"p1","url":"https://notion.so/p1"}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	if _, err := c.Pages.Trash(context.Background(), "p1"); err != nil {
		t.Fatalf("Trash: %v", err)
	}
	if _, err := c.Pages.Restore(context.Background(), "p1"); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if bodies[0]["in_trash"] != true {
		t.Errorf("Trash sent in_trash = %v, want true", bodies[0]["in_trash"])
	}
	// false must be sent explicitly rather than omitted, or restoring is a
	// no-op.
	if bodies[1]["in_trash"] != false {
		t.Errorf("Restore sent in_trash = %v, want false", bodies[1]["in_trash"])
	}
}

func TestPagesRetrieveWithFilterProperties(t *testing.T) {
	var gotQuery []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()["filter_properties"]
		w.Write(readFixture(t, "page.json"))
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	if _, err := c.Pages.Retrieve(context.Background(), "p1", WithFilterProperties("title", "abc1")); err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(gotQuery) != 2 || gotQuery[0] != "title" || gotQuery[1] != "abc1" {
		t.Errorf("filter_properties = %v, want [title abc1]", gotQuery)
	}
}

func TestZeroValuePropertiesEncodeArrays(t *testing.T) {
	// Every list-valued property is a required array in requests
	// (pages.ts:377-408), so a nil slice must encode as [] rather than null.
	tests := []struct {
		name  string
		value PropertyValue
		want  string
	}{
		{"title", &TitleProperty{}, `{"title":[],"type":"title"}`},
		{"rich text", &RichTextProperty{}, `{"rich_text":[],"type":"rich_text"}`},
		{"multi select", &MultiSelectProperty{}, `{"multi_select":[],"type":"multi_select"}`},
		{"relation", &RelationProperty{}, `{"relation":[],"type":"relation"}`},
		{"people", &PeopleProperty{}, `{"people":[],"type":"people"}`},
		{"files", &FilesProperty{}, `{"files":[],"type":"files"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(encoded) != tt.want {
				t.Errorf("got  %s\nwant %s", encoded, tt.want)
			}
		})
	}
}

func TestPropertyItemTitleList(t *testing.T) {
	// Each list entry carries a single rich text run where a page property
	// carries an array (TitlePropertyItemObjectResponse, pages.ts:293-298).
	var item PropertyItem
	if err := json.Unmarshal(readFixture(t, "property_item_title_list.json"), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if item.Object != "list" || item.Value != nil {
		t.Fatalf("item = %+v, want a list with no single value", item)
	}
	if len(item.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(item.Results))
	}
	first, ok := item.Results[0].(*TitleProperty)
	if !ok {
		t.Fatalf("Results[0] = %T, want *TitleProperty", item.Results[0])
	}
	if len(first.Title) != 1 || first.Title.PlainText() != "Ship " {
		t.Errorf("Results[0].Title = %+v, want one run reading %q", first.Title, "Ship ")
	}
	if item.NextCursor != "" {
		t.Errorf("NextCursor = %q, want empty", item.NextCursor)
	}

	combined, ok := item.Combined().(*TitleProperty)
	if !ok {
		t.Fatalf("Combined() = %T, want *TitleProperty", item.Combined())
	}
	if got := combined.Title.PlainText(); got != "Ship the SDK" {
		t.Errorf("Combined().Title = %q, want %q", got, "Ship the SDK")
	}
	if combined.PropertyID() != "title" {
		t.Errorf("Combined().PropertyID = %q, want title", combined.PropertyID())
	}
}

func TestPropertyItemRelationList(t *testing.T) {
	// Each entry is a bare {id} (RelationPropertyItemObjectResponse,
	// pages.ts:247-252).
	var item PropertyItem
	if err := json.Unmarshal(readFixture(t, "property_item_relation_list.json"), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(item.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(item.Results))
	}
	first, ok := item.Results[0].(*RelationProperty)
	if !ok {
		t.Fatalf("Results[0] = %T, want *RelationProperty", item.Results[0])
	}
	if len(first.Relation) != 1 || first.Relation[0].ID != "p-1" {
		t.Errorf("Results[0].Relation = %+v, want [p-1]", first.Relation)
	}
	// The cursor is what a caller passes back to read the next page.
	if item.NextCursor != "cursor-2" {
		t.Errorf("NextCursor = %q, want cursor-2", item.NextCursor)
	}

	combined, ok := item.Combined().(*RelationProperty)
	if !ok {
		t.Fatalf("Combined() = %T, want *RelationProperty", item.Combined())
	}
	if len(combined.Relation) != 2 || combined.Relation[1].ID != "p-2" {
		t.Errorf("Combined().Relation = %+v, want [p-1 p-2]", combined.Relation)
	}
}

func TestPropertyItemSingleValue(t *testing.T) {
	var item PropertyItem
	raw := []byte(`{"object":"property_item","id":"n1","type":"number","number":3}`)
	if err := json.Unmarshal(raw, &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	number, ok := item.Value.(*NumberProperty)
	if !ok || number.Number == nil || *number.Number != 3 {
		t.Fatalf("Value = %+v, want number 3", item.Value)
	}
	if item.Combined() != item.Value {
		t.Error("Combined() on a single value should return Value")
	}
	var nilItem *PropertyItem
	if nilItem.Combined() != nil {
		t.Error("nil item Combined() != nil")
	}
}

func TestPagesRetrieveProperty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/pages/p1/properties/title" {
			t.Errorf("path = %q, want /v1/pages/p1/properties/title", r.URL.Path)
		}
		w.Write(readFixture(t, "property_item_title_list.json"))
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	item, err := c.Pages.RetrieveProperty(context.Background(), "p1", "title", PageParams{})
	if err != nil {
		t.Fatalf("RetrieveProperty: %v", err)
	}
	if got := item.Combined().(*TitleProperty).Title.PlainText(); got != "Ship the SDK" {
		t.Errorf("title = %q", got)
	}
}
