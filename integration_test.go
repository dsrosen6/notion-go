//go:build integration

// Package notion's integration tests run against a real Notion workspace.
//
//	export NOTION_TOKEN=secret_...
//	export NOTION_TEST_PAGE_ID=...   # a page the integration can edit
//	go test -tags=integration -v ./...
//
// Alternatively, put both variables in a .env file at the module root; it is
// loaded before each test and never overrides variables already in the
// environment. See .env.example.
//
// The tests create pages under NOTION_TEST_PAGE_ID and trash them afterward.
// Notion has no hard delete for pages, so the trash accumulates; empty it
// occasionally.
package notion

import (
	"context"
	"errors"
	"maps"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

// integrationClient returns a client and the scratch page ID, skipping the test
// when the environment is not configured.
func integrationClient(t *testing.T) (*Client, string) {
	t.Helper()

	// A missing .env is the normal case in CI, so its error is ignored.
	_ = godotenv.Load()
	token := os.Getenv("NOTION_TOKEN")
	pageID := os.Getenv("NOTION_TEST_PAGE_ID")
	if token == "" || pageID == "" {
		t.Skip("set NOTION_TOKEN and NOTION_TEST_PAGE_ID to run integration tests")
	}
	return NewClient(token), ExtractID(pageID)
}

func TestIntegrationUsersMe(t *testing.T) {
	client, _ := integrationClient(t)

	user, err := client.Users.Me(context.Background())
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if user.Type != UserTypeBot {
		t.Errorf("Type = %q, want bot", user.Type)
	}
	t.Logf("authenticated as %q", user.Name)
}

func TestIntegrationPageLifecycle(t *testing.T) {
	client, parentID := integrationClient(t)
	ctx := context.Background()

	page, err := client.Pages.Create(ctx, CreatePageParams{
		Parent:     Parent{Type: ParentTypePage, PageID: parentID},
		Icon:       NewEmojiIcon("🧪"),
		Properties: PropertyValues{"title": NewTitle("notion-go integration test")},
		Children: BlockList{
			NewHeading1("Heading"),
			NewParagraph("Body text."),
			NewToDo("A task", false),
			NewDivider(),
			NewCode(`fmt.Println("hi")`, "go"),
			NewToggle("Nested", NewParagraph("Inside the toggle.")),
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		if _, err := client.Pages.Trash(context.Background(), page.ID); err != nil {
			t.Errorf("cleanup: trashing %s: %v", page.ID, err)
		}
	})
	t.Logf("created %s", page.URL)

	if !page.IsFull() {
		t.Fatal("created page is partial")
	}
	if got := page.Title(); got != "notion-go integration test" {
		t.Errorf("Title = %q", got)
	}

	// Read the content back and confirm every block type survived the round
	// trip through the real API.
	var types []BlockType
	for block, err := range client.Blocks.AllChildren(ctx, page.ID) {
		if err != nil {
			t.Fatalf("AllChildren: %v", err)
		}
		types = append(types, block.BlockType())
		if _, unknown := block.(*UnknownBlock); unknown {
			t.Errorf("block %s decoded as UnknownBlock", block.Base().ID)
		}
	}
	want := []BlockType{
		BlockTypeHeading1, BlockTypeParagraph, BlockTypeToDo,
		BlockTypeDivider, BlockTypeCode, BlockTypeToggle,
	}
	if len(types) != len(want) {
		t.Fatalf("got %v, want %v", types, want)
	}
	for i, blockType := range want {
		if types[i] != blockType {
			t.Errorf("block %d = %q, want %q", i, types[i], blockType)
		}
	}

	// Append after creation, then edit the page.
	added, err := client.Blocks.AppendChildren(ctx, page.ID, []Block{
		NewParagraph("Appended later."),
	})
	if err != nil {
		t.Fatalf("AppendChildren: %v", err)
	}
	if len(added) != 1 {
		t.Fatalf("appended %d blocks, want 1", len(added))
	}

	updated, err := client.Pages.Update(ctx, page.ID, UpdatePageParams{
		Icon: NewEmojiIcon("✅"),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Icon == nil || updated.Icon.Emoji != "✅" {
		t.Errorf("Icon = %+v, want the check emoji", updated.Icon)
	}

	// The property item endpoint returns list-valued properties one item per
	// result, in a shape distinct from the page object.
	item, err := client.Pages.RetrieveProperty(ctx, page.ID, "title", PageParams{})
	if err != nil {
		t.Fatalf("RetrieveProperty: %v", err)
	}
	title, ok := item.Combined().(*TitleProperty)
	if !ok {
		t.Fatalf("Combined() = %T, want *TitleProperty", item.Combined())
	}
	if got := title.Title.PlainText(); got != "notion-go integration test" {
		t.Errorf("property item title = %q", got)
	}

	// Updating a to-do's text without touching Checked must leave it checked.
	var todoID string
	for block, err := range client.Blocks.AllChildren(ctx, page.ID) {
		if err != nil {
			t.Fatalf("AllChildren: %v", err)
		}
		if _, ok := block.(*ToDoBlock); ok {
			todoID = block.Base().ID
		}
	}
	if _, err := client.Blocks.Update(ctx, todoID, &ToDoBlock{ToDo: ToDoContent{Checked: new(true)}}); err != nil {
		t.Fatalf("checking to-do: %v", err)
	}
	renamed, err := client.Blocks.Update(ctx, todoID, &ToDoBlock{ToDo: ToDoContent{BlockText: BlockText{RichText: NewRichText("Renamed task")}}})
	if err != nil {
		t.Fatalf("renaming to-do: %v", err)
	}
	if todo, ok := renamed.(*ToDoBlock); !ok || !todo.ToDo.IsChecked() {
		t.Errorf("to-do after text-only update = %+v, want it still checked", renamed)
	}
}

func TestIntegrationDatabaseLifecycle(t *testing.T) {
	client, parentID := integrationClient(t)
	ctx := context.Background()

	db, err := client.Databases.Create(ctx, CreateDatabaseParams{
		Parent: Parent{Type: ParentTypePage, PageID: parentID},
		Title:  NewRichText("notion-go integration db"),
		InitialDataSource: &InitialDataSource{
			Properties: PropertySchemas{
				"Name":     &TitleSchema{},
				"Priority": &NumberSchema{},
				"Done":     &CheckboxSchema{},
				"Tags":     &MultiSelectSchema{},
			},
		},
	})
	if err != nil {
		t.Fatalf("Databases.Create: %v", err)
	}
	t.Cleanup(func() {
		if _, err := client.Databases.Trash(context.Background(), db.ID); err != nil {
			t.Errorf("cleanup: trashing %s: %v", db.ID, err)
		}
	})

	if len(db.DataSources) == 0 {
		t.Fatal("the new database has no data sources")
	}
	dataSourceID := db.DataSources[0].ID

	ds, err := client.DataSources.Retrieve(ctx, dataSourceID)
	if err != nil {
		t.Fatalf("DataSources.Retrieve: %v", err)
	}
	for _, name := range []string{"Name", "Priority", "Done", "Tags"} {
		if _, ok := ds.Properties[name]; !ok {
			t.Errorf("column %q is missing from the schema", name)
		}
	}
	if _, unknown := ds.Properties["Priority"].(*UnknownSchema); unknown {
		t.Error("Priority decoded as UnknownSchema")
	}

	// Add enough rows to force pagination.
	const rowCount = 3
	for i := range rowCount {
		_, err := client.Pages.Create(ctx, CreatePageParams{
			Parent: Parent{Type: ParentTypeDataSource, DataSourceID: dataSourceID},
			Properties: PropertyValues{
				"Name":     NewTitle("Row " + string(rune('A'+i))),
				"Priority": NewNumber(float64(i)),
				"Done":     NewCheckbox(i%2 == 0),
				"Tags":     NewMultiSelect("go"),
			},
		})
		if err != nil {
			t.Fatalf("creating row %d: %v", i, err)
		}
	}

	// Query with a filter, paginating one row at a time.
	params := QueryParams{
		Filter:     ByNumber("Priority").GreaterThanOrEqualTo(1),
		Sorts:      []Sort{SortBy("Priority", Ascending)},
		PageParams: PageParams{PageSize: 1},
	}
	var titles []string
	for page, err := range client.DataSources.QueryAll(ctx, dataSourceID, params) {
		if err != nil {
			t.Fatalf("QueryAll: %v", err)
		}
		titles = append(titles, page.Title())
	}
	if len(titles) != 2 {
		t.Errorf("query returned %v, want the two rows with Priority >= 1", titles)
	}

	// Zero-value list properties must encode as [] rather than null.
	blank, err := client.Pages.Create(ctx, CreatePageParams{
		Parent: Parent{Type: ParentTypeDataSource, DataSourceID: dataSourceID},
		Properties: PropertyValues{
			"Name": NewTitle("Row blank"),
			"Tags": &MultiSelectProperty{},
		},
	})
	if err != nil {
		t.Fatalf("creating a row with empty properties: %v", err)
	}

	// Title filters must address the title column, not rich_text.
	found := 0
	for _, err := range client.DataSources.QueryAll(ctx, dataSourceID, QueryParams{
		Filter: ByTitle("Name").Contains("blank"),
	}) {
		if err != nil {
			t.Fatalf("QueryAll with title filter: %v", err)
		}
		found++
	}
	if found != 1 {
		t.Errorf("title filter matched %d rows, want 1", found)
	}

	// Column IDs come back percent-encoded; filter_properties must accept them
	// as-is and the response must then carry only that column.
	priorityID := ""
	if schema, ok := ds.Properties["Priority"].(*NumberSchema); ok {
		priorityID = schema.ID
	}
	if priorityID == "" {
		t.Fatal("Priority schema has no ID")
	}
	narrowed, err := client.Pages.Retrieve(ctx, blank.ID, WithFilterProperties(priorityID))
	if err != nil {
		t.Fatalf("Retrieve with filter_properties %q: %v", priorityID, err)
	}
	if _, ok := narrowed.Properties["Priority"]; !ok || len(narrowed.Properties) != 1 {
		t.Errorf("filter_properties returned columns %v, want only Priority", slices.Sorted(maps.Keys(narrowed.Properties)))
	}
}

func TestIntegrationErrorShape(t *testing.T) {
	client, _ := integrationClient(t)

	// A well-formed but nonexistent ID must produce a typed error.
	_, err := client.Pages.Retrieve(context.Background(), "00000000-0000-0000-0000-000000000000")
	if !IsNotFound(err) {
		t.Fatalf("err = %v, want object_not_found", err)
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.RequestID == "" {
		t.Error("RequestID is empty; the API should always send one")
	}
}

func TestIntegrationSearch(t *testing.T) {
	client, _ := integrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := client.Search(ctx, SearchParams{
		Filter:     SearchPages(),
		PageParams: PageParams{PageSize: 5},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, row := range result.Results {
		if row.Object != "page" {
			t.Errorf("result object = %q, want page", row.Object)
		}
	}
	t.Logf("search returned %d results", len(result.Results))
}
