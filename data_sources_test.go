package notion

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func decodeDataSource(data []byte) (*DataSource, error) {
	var ds DataSource
	if err := json.Unmarshal(data, &ds); err != nil {
		return nil, err
	}
	return &ds, nil
}

func TestSchemaRegistryCoverage(t *testing.T) {
	// Every property type that can be a column must be registered. A missing
	// entry decodes the column as UnknownSchema instead of its real type.
	declared := []PropertyType{
		PropertyTypeTitle, PropertyTypeRichText, PropertyTypeNumber, PropertyTypeSelect,
		PropertyTypeMultiSelect, PropertyTypeStatus, PropertyTypeDate, PropertyTypePeople,
		PropertyTypeFiles, PropertyTypeCheckbox, PropertyTypeURL, PropertyTypeEmail,
		PropertyTypePhoneNumber, PropertyTypeFormula, PropertyTypeRelation,
		PropertyTypeRollup, PropertyTypeCreatedBy, PropertyTypeCreatedTime,
		PropertyTypeLastEditedBy, PropertyTypeLastEditedTime, PropertyTypeUniqueID,
	}

	if len(declared) != len(schemaRegistry) {
		t.Errorf("%d column types declared but %d registered", len(declared), len(schemaRegistry))
	}
	for _, propertyType := range declared {
		newSchema, ok := schemaRegistry[string(propertyType)]
		if !ok {
			t.Errorf("column type %q has no registry entry", propertyType)
			continue
		}
		if got := newSchema().SchemaType(); got != propertyType {
			t.Errorf("registry[%q] builds a schema reporting %q", propertyType, got)
		}
	}
}

func TestPropertyRegistryCoverage(t *testing.T) {
	// Page property values cover the column types plus button, verification,
	// and place, which exist as values but are configured differently.
	if len(propertyRegistry) != 24 {
		t.Errorf("%d property value types registered, want 24", len(propertyRegistry))
	}
	for tag, newValue := range propertyRegistry {
		if got := string(newValue().PropertyType()); got != tag {
			t.Errorf("registry[%q] builds a value reporting %q", tag, got)
		}
	}
}

func TestDataSourceFixture(t *testing.T) {
	ds := assertRoundTrip(t, readFixture(t, "data_source.json"), decodeDataSource)

	if !ds.IsFull() {
		t.Error("IsFull = false, want true")
	}
	if got := ds.Title.PlainText(); got != "Tasks" {
		t.Errorf("Title = %q, want Tasks", got)
	}
	if ds.Parent.Type != ParentTypeDatabase || ds.Parent.DatabaseID != "db-1" {
		t.Errorf("Parent = %+v, want database db-1", ds.Parent)
	}
	// The data source's own parent is the database; the page is above that.
	if ds.DatabaseParent.Type != ParentTypePage {
		t.Errorf("DatabaseParent = %+v, want a page", ds.DatabaseParent)
	}
	if got := ds.Properties.TitleName(); got != "Name" {
		t.Errorf("TitleName = %q, want Name", got)
	}
}

func TestPropertySchemaTypes(t *testing.T) {
	ds, err := decodeDataSource(readFixture(t, "data_source.json"))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	status, ok := ds.Properties["Status"].(*StatusSchema)
	if !ok {
		t.Fatalf("Status = %T, want *StatusSchema", ds.Properties["Status"])
	}
	if len(status.Status.Options) != 3 || status.Status.Options[2].Name != "Done" {
		t.Errorf("options = %+v", status.Status.Options)
	}
	if len(status.Status.Groups) != 3 || status.Status.Groups[0].OptionIDs[0] != "o1" {
		t.Errorf("groups = %+v", status.Status.Groups)
	}

	number, ok := ds.Properties["Priority"].(*NumberSchema)
	if !ok {
		t.Fatalf("Priority = %T, want *NumberSchema", ds.Properties["Priority"])
	}
	if number.Number.Format != "number" {
		t.Errorf("format = %q", number.Number.Format)
	}

	relation, ok := ds.Properties["Blocked by"].(*RelationSchema)
	if !ok {
		t.Fatalf("Blocked by = %T, want *RelationSchema", ds.Properties["Blocked by"])
	}
	if relation.Relation.Type != "dual_property" || relation.Relation.DualProperty == nil {
		t.Fatalf("relation = %+v, want a dual property", relation.Relation)
	}
	if relation.Relation.DualProperty.SyncedPropertyName != "Blocks" {
		t.Errorf("synced name = %q, want Blocks", relation.Relation.DualProperty.SyncedPropertyName)
	}

	rollup, ok := ds.Properties["Total"].(*RollupSchema)
	if !ok {
		t.Fatalf("Total = %T, want *RollupSchema", ds.Properties["Total"])
	}
	if rollup.Rollup.Function != "sum" || rollup.Rollup.RollupPropertyName != "Priority" {
		t.Errorf("rollup = %+v", rollup.Rollup)
	}

	// The column ID survives, and it is what addresses a column across renames.
	if got := ds.Properties["Ticket"].Schema().ID; got != "uid1" {
		t.Errorf("Ticket ID = %q, want uid1", got)
	}
	if got := ds.Properties["Ticket"].Schema().Name; got != "Ticket" {
		t.Errorf("Ticket Name = %q", got)
	}
}

func TestDataSourceQuery(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/data_sources/ds-1/query" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		io.WriteString(w, `{"object":"list","results":[
			{"object":"page","id":"p1","url":"https://notion.so/p1","properties":{}}
		],"next_cursor":null,"has_more":false}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	result, err := c.DataSources.Query(context.Background(), "ds-1", QueryParams{
		Filter:     ByStatus("Status").Equals("Done"),
		Sorts:      []Sort{SortBy("Priority", Descending)},
		PageParams: PageParams{PageSize: 50},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	// Filters, sorts, and pagination all travel in the body for this endpoint.
	filter := gotBody["filter"].(map[string]any)
	if filter["property"] != "Status" {
		t.Errorf("filter = %#v", filter)
	}
	if gotBody["page_size"] != float64(50) {
		t.Errorf("page_size = %v, want 50", gotBody["page_size"])
	}
	sorts := gotBody["sorts"].([]any)
	if len(sorts) != 1 {
		t.Errorf("sorts = %#v", sorts)
	}

	if len(result.Results) != 1 || result.Results[0].Page == nil {
		t.Fatalf("results = %+v, want one page", result.Results)
	}
}

func TestQueryRejectsDeepFilter(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	leaf := ByCheckbox("Done").Equals(true)
	c, _ := testClient(t, srv)
	_, err := c.DataSources.Query(context.Background(), "ds-1", QueryParams{
		Filter: And(Or(And(leaf, leaf), leaf), leaf),
	})

	if !errors.Is(err, ErrFilterTooDeep) {
		t.Fatalf("err = %v, want ErrFilterTooDeep", err)
	}
	if called {
		t.Error("the request was sent, want it rejected before sending")
	}
}

func TestQueryAllSkipsDataSources(t *testing.T) {
	// A wiki database returns data sources alongside pages; QueryAll yields
	// only the pages.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"object":"list","results":[
			{"object":"page","id":"p1","url":"https://notion.so/p1","properties":{}},
			{"object":"data_source","id":"ds-2","title":[]},
			{"object":"page","id":"p2","url":"https://notion.so/p2","properties":{}}
		],"next_cursor":null,"has_more":false}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	pages, err := Collect(c.DataSources.QueryAll(context.Background(), "ds-1", QueryParams{}))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}
	if pages[0].ID != "p1" || pages[1].ID != "p2" {
		t.Errorf("pages = %q, %q", pages[0].ID, pages[1].ID)
	}
}

func TestQueryResultDecodesBothKinds(t *testing.T) {
	var results QueryResults
	raw := []byte(`[
		{"object":"page","id":"p1","url":"https://notion.so/p1"},
		{"object":"data_source","id":"ds-2","title":[]}
	]`)
	if err := json.Unmarshal(raw, &results); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if results[0].Page == nil || results[0].DataSource != nil {
		t.Errorf("results[0] = %+v, want a page", results[0])
	}
	if results[1].DataSource == nil || results[1].Page != nil {
		t.Errorf("results[1] = %+v, want a data source", results[1])
	}
	if len(results.Pages()) != 1 {
		t.Errorf("Pages() = %d, want 1", len(results.Pages()))
	}
}

func TestSearch(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/search" {
			t.Errorf("path = %q, want /v1/search", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		io.WriteString(w, `{"object":"list","results":[
			{"object":"page","id":"p1","url":"https://notion.so/p1","properties":{}}
		],"next_cursor":null,"has_more":false}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	result, err := c.Search(context.Background(), SearchParams{
		Query:  "roadmap",
		Filter: SearchPages(),
		Sort:   SortByLastEdited(Descending),
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotBody["query"] != "roadmap" {
		t.Errorf("query = %v", gotBody["query"])
	}
	filter := gotBody["filter"].(map[string]any)
	if filter["property"] != "object" || filter["value"] != "page" {
		t.Errorf("filter = %#v", filter)
	}
	if len(result.Results) != 1 {
		t.Errorf("results = %+v", result.Results)
	}
}

func TestDatabaseRetrieve(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{
			"object":"database","id":"db-1",
			"title":[{"type":"text","text":{"content":"Work"},"plain_text":"Work","annotations":{"color":"default"}}],
			"parent":{"type":"page_id","page_id":"page-1"},
			"data_sources":[{"id":"ds-1","name":"Tasks"}],
			"url":"https://notion.so/db1"
		}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	db, err := c.Databases.Retrieve(context.Background(), "db-1")
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if !db.IsFull() {
		t.Error("IsFull = false, want true")
	}
	if got := db.Title.PlainText(); got != "Work" {
		t.Errorf("Title = %q", got)
	}
	// The data source ID is what queries address, not the database ID.
	if len(db.DataSources) != 1 || db.DataSources[0].ID != "ds-1" {
		t.Errorf("DataSources = %+v", db.DataSources)
	}
}

func TestPartialDatabase(t *testing.T) {
	var db Database
	if err := json.Unmarshal([]byte(`{"object":"database","id":"db-1"}`), &db); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if db.IsFull() {
		t.Error("IsFull = true, want false for a partial database")
	}
}

func TestDualRelationSchemaEncodesEmptyObject(t *testing.T) {
	// Creating a two-way relation sends "dual_property": {}; Notion names the
	// synced column itself (common.ts:1191-1196).
	schema := &RelationSchema{Relation: RelationConfig{
		DataSourceID: "ds-2",
		Type:         "dual_property",
		DualProperty: &DualPropertyConfig{},
	}}
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"relation":{"data_source_id":"ds-2","type":"dual_property","dual_property":{}},"type":"relation"}`
	if string(encoded) != want {
		t.Errorf("got  %s\nwant %s", encoded, want)
	}
}
