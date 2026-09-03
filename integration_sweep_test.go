//go:build integration

package notion

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// This file sweeps the API surface against a live workspace: every block type,
// every schema and property type, every filter builder, comments, users, and
// search variants. See integration_test.go for setup.

// TestMain trashes the shared data source fixture after every test has run,
// since the fixture outlives any single test.
func TestMain(m *testing.M) {
	code := m.Run()
	trashSweepFixture()
	os.Exit(code)
}

const (
	sweepImageURL = "https://www.notion.so/images/favicon.ico"
	sweepPDFURL   = "https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf"
	sweepVideoURL = "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
	sweepAudioURL = "https://interactive-examples.mdn.mozilla.net/media/cc0-audio/t-rex-roar.mp3"
	sweepEmbedURL = "https://example.com/"
)

// trashOnCleanup registers a page to be trashed when the test finishes.
func trashOnCleanup(t *testing.T, client *Client, pageID string) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := client.Pages.Trash(context.Background(), pageID); err != nil {
			t.Errorf("cleanup: trashing page %s: %v", pageID, err)
		}
	})
}

// createSweepPage creates an empty page under the scratch page and trashes it
// on cleanup.
func createSweepPage(t *testing.T, client *Client, parentID, title string) *Page {
	t.Helper()
	page, err := client.Pages.Create(t.Context(), CreatePageParams{
		Parent:     Parent{Type: ParentTypePage, PageID: parentID},
		Properties: PropertyValues{"title": NewTitle(title)},
	})
	if err != nil {
		t.Fatalf("creating page %q: %v", title, err)
	}
	trashOnCleanup(t, client, page.ID)
	return page
}

// collectChildren reads every direct child of a block or page.
func collectChildren(t *testing.T, client *Client, blockID string) []Block {
	t.Helper()
	blocks, err := Collect(client.Blocks.AllChildren(t.Context(), blockID))
	if err != nil {
		t.Fatalf("AllChildren(%s): %v", blockID, err)
	}
	return blocks
}

// blockTypes returns the type of every block in order.
func blockTypes(blocks []Block) []BlockType {
	types := make([]BlockType, len(blocks))
	for i, block := range blocks {
		types[i] = block.BlockType()
	}
	return types
}

func TestIntegrationBlockTypes(t *testing.T) {
	client, parentID := integrationClient(t)
	ctx := t.Context()
	page := createSweepPage(t, client, parentID, "notion-go block sweep")

	yes := true
	children := []Block{
		NewParagraph("A paragraph."),
		NewHeading1("Heading one"),
		NewHeading2("Heading two"),
		NewHeading3("Heading three"),
		&Heading2Block{Heading2: HeadingContent{
			BlockText:    BlockText{RichText: NewRichText("Toggleable heading"), Children: BlockList{NewParagraph("Under the heading.")}},
			IsToggleable: true,
		}},
		NewBulletedListItem("A bullet"),
		NewNumberedListItem("A number"),
		NewQuote("A quote"),
		NewToDo("A task", true),
		NewToggle("A toggle", NewParagraph("Inside the toggle.")),
		NewCode("package main", "go"),
		&CalloutBlock{Callout: ParagraphContent{
			BlockText: BlockText{
				RichText: NewRichText("A callout"),
				Color:    ColorYellowBackground,
				Children: BlockList{NewParagraph("Inside the callout.")},
			},
			Icon: NewEmojiIcon("💡"),
		}},
		NewDivider(),
		&BreadcrumbBlock{},
		&TableOfContentsBlock{TableOfContents: ColorContent{Color: ColorGray}},
		&EquationBlock{Equation: EquationContent{Expression: "e = mc^2"}},
		&EmbedBlock{Embed: URLContent{URL: sweepEmbedURL}},
		NewBookmark(sweepEmbedURL),
		NewImage(sweepImageURL),
		&VideoBlock{Video: NewExternalFile(sweepVideoURL)},
		&PDFBlock{PDF: NewExternalFile(sweepPDFURL)},
		&FileBlock{File: NewExternalFile(sweepPDFURL)},
		&AudioBlock{Audio: NewExternalFile(sweepAudioURL)},
		&LinkToPageBlock{LinkToPage: LinkToPageContent{Type: "page_id", PageID: parentID}},
		&SyncedBlock{SyncedBlock: SyncedBlockContent{Children: BlockList{NewParagraph("Synced original.")}}},
		&ColumnListBlock{ColumnList: ColumnListContent{Children: BlockList{
			&ColumnBlock{Column: ColumnContent{Children: BlockList{
				&ParagraphBlock{Paragraph: ParagraphContent{BlockText: BlockText{
					RichText: NewRichText("Left column"),
					Children: BlockList{NewParagraph("Left nested")},
				}}},
			}}},
			&ColumnBlock{Column: ColumnContent{Children: BlockList{
				&ParagraphBlock{Paragraph: ParagraphContent{BlockText: BlockText{
					RichText: NewRichText("Right column"),
					Children: BlockList{NewParagraph("Right nested")},
				}}},
			}}},
		}}},
		&TableBlock{Table: TableContent{
			TableWidth:      2,
			HasColumnHeader: &yes,
			Children: BlockList{
				&TableRowBlock{TableRow: TableRowContent{Cells: []RichTextList{NewRichText("Key"), NewRichText("Value")}}},
				&TableRowBlock{TableRow: TableRowContent{Cells: []RichTextList{NewRichText("a"), NewRichText("1")}}},
			},
		}},
		// Every rich text variant a request may carry.
		&ParagraphBlock{Paragraph: ParagraphContent{BlockText: BlockText{RichText: RichTextList{
			&Text{RichTextCommon: RichTextCommon{Annotations: Annotations{Italic: true, Color: ColorRed}}, Text: TextContent{Content: "styled "}},
			&Equation{Equation: EquationContent{Expression: "x^2"}},
			&Mention{Mention: MentionContent{Type: MentionTypePage, Page: &IDRef{ID: parentID}}},
			&Mention{Mention: MentionContent{Type: MentionTypeDate, Date: &Date{Start: mustDate(alphaDate)}}},
		}}}},
	}

	created, err := client.Blocks.AppendChildren(ctx, page.ID, children)
	if err != nil {
		t.Fatalf("AppendChildren: %v", err)
	}
	if len(created) != len(children) {
		t.Fatalf("AppendChildren returned %d blocks, want %d", len(created), len(children))
	}

	got := collectChildren(t, client, page.ID)
	if len(got) != len(children) {
		t.Fatalf("AllChildren returned %d blocks, want %d: %v", len(got), len(children), blockTypes(got))
	}
	byType := map[BlockType]Block{}
	for i, want := range children {
		t.Run(string(want.BlockType()), func(t *testing.T) {
			block := got[i]
			if _, unknown := block.(*UnknownBlock); unknown {
				t.Fatalf("decoded as UnknownBlock")
			}
			if block.BlockType() != want.BlockType() {
				t.Fatalf("block %d = %q, want %q", i, block.BlockType(), want.BlockType())
			}
			if block.Base().ID == "" {
				t.Error("block has no ID")
			}
			byType[block.BlockType()] = block
		})
	}

	// Type-specific payloads survived the round trip.
	if h, ok := got[4].(*Heading2Block); !ok || !h.Heading2.IsToggleable || !h.HasChildren {
		t.Errorf("toggleable heading = %+v, want is_toggleable with children", got[4])
	}
	if c, ok := byType[BlockTypeCallout].(*CalloutBlock); !ok || c.Callout.Icon == nil || c.Callout.Icon.Emoji != "💡" || c.Callout.Color != ColorYellowBackground {
		t.Errorf("callout = %+v, want emoji icon and yellow background", byType[BlockTypeCallout])
	}
	if code, ok := byType[BlockTypeCode].(*CodeBlock); !ok || code.Code.Language != "go" {
		t.Errorf("code block language = %+v, want go", byType[BlockTypeCode])
	}
	if todo, ok := byType[BlockTypeToDo].(*ToDoBlock); !ok || !todo.ToDo.IsChecked() {
		t.Errorf("to_do = %+v, want checked", byType[BlockTypeToDo])
	}
	if eq, ok := byType[BlockTypeEquation].(*EquationBlock); !ok || eq.Equation.Expression != "e = mc^2" {
		t.Errorf("equation = %+v", byType[BlockTypeEquation])
	}
	if img, ok := byType[BlockTypeImage].(*ImageBlock); !ok || img.Image.Type != FileTypeExternal || img.Image.URL() != sweepImageURL {
		t.Errorf("image = %+v, want external %s", byType[BlockTypeImage], sweepImageURL)
	}
	if pdf, ok := byType[BlockTypePDF].(*PDFBlock); !ok || pdf.PDF.URL() != sweepPDFURL {
		t.Errorf("pdf = %+v", byType[BlockTypePDF])
	}
	if v, ok := byType[BlockTypeVideo].(*VideoBlock); !ok || v.Video.URL() == "" {
		t.Errorf("video = %+v", byType[BlockTypeVideo])
	}
	if a, ok := byType[BlockTypeAudio].(*AudioBlock); !ok || a.Audio.URL() != sweepAudioURL {
		t.Errorf("audio = %+v", byType[BlockTypeAudio])
	}
	if f, ok := byType[BlockTypeFile].(*FileBlock); !ok || f.File.URL() != sweepPDFURL {
		t.Errorf("file = %+v", byType[BlockTypeFile])
	}
	if b, ok := byType[BlockTypeBookmark].(*BookmarkBlock); !ok || b.Bookmark.URL != sweepEmbedURL {
		t.Errorf("bookmark = %+v", byType[BlockTypeBookmark])
	}
	if e, ok := byType[BlockTypeEmbed].(*EmbedBlock); !ok || e.Embed.URL != sweepEmbedURL {
		t.Errorf("embed = %+v", byType[BlockTypeEmbed])
	}
	if l, ok := byType[BlockTypeLinkToPage].(*LinkToPageBlock); !ok || l.LinkToPage.Type != "page_id" || !SameID(l.LinkToPage.PageID, parentID) {
		t.Errorf("link_to_page = %+v, want page %s", byType[BlockTypeLinkToPage], parentID)
	}
	if s, ok := byType[BlockTypeSyncedBlock].(*SyncedBlock); !ok || s.SyncedBlock.SyncedFrom != nil || !s.HasChildren {
		t.Errorf("synced_block = %+v, want an original with children", byType[BlockTypeSyncedBlock])
	}
	if toc, ok := byType[BlockTypeTableOfContents].(*TableOfContentsBlock); !ok || toc.TableOfContents.Color != ColorGray {
		t.Errorf("table_of_contents = %+v, want gray", byType[BlockTypeTableOfContents])
	}

	t.Run("rich_text_variants", func(t *testing.T) {
		mixed, ok := got[len(got)-1].(*ParagraphBlock)
		if !ok || len(mixed.Paragraph.RichText) != 4 {
			t.Fatalf("mixed paragraph = %+v, want four runs", got[len(got)-1])
		}
		runs := mixed.Paragraph.RichText
		if text, ok := runs[0].(*Text); !ok || !text.Annotations.Italic || text.Annotations.Color != ColorRed || text.PlainText != "styled " {
			t.Errorf("run 0 = %+v, want italic red text", runs[0])
		}
		if eq, ok := runs[1].(*Equation); !ok || eq.Equation.Expression != "x^2" || eq.RichTextType() != RichTextTypeEquation {
			t.Errorf("run 1 = %+v, want an equation", runs[1])
		}
		if m, ok := runs[2].(*Mention); !ok || m.Mention.Type != MentionTypePage || m.Mention.Page == nil || !SameID(m.Mention.Page.ID, parentID) || m.Href == "" {
			t.Errorf("run 2 = %+v, want a page mention with href", runs[2])
		}
		if m, ok := runs[3].(*Mention); !ok || m.Mention.Type != MentionTypeDate || m.Mention.Date == nil || m.Mention.Date.Start.String() != alphaDate {
			t.Errorf("run 3 = %+v, want a date mention of %s", runs[3], alphaDate)
		}
		for _, run := range runs {
			if _, unknown := run.(*UnknownRichText); unknown || run.Common().PlainText == "" {
				t.Errorf("run %+v is unknown or has no plain text", run)
			}
		}
	})

	// Nested children.
	t.Run("nested", func(t *testing.T) {
		nested := map[BlockType]string{
			BlockTypeToggle:      "Inside the toggle.",
			BlockTypeCallout:     "Inside the callout.",
			BlockTypeSyncedBlock: "Synced original.",
		}
		for blockType, want := range nested {
			parent := byType[blockType]
			if !parent.Base().HasChildren {
				t.Errorf("%s: has_children is false", blockType)
				continue
			}
			kids := collectChildren(t, client, parent.Base().ID)
			if len(kids) != 1 || BlockList(kids).PlainText() != want {
				t.Errorf("%s children = %v (%q), want one paragraph %q", blockType, blockTypes(kids), BlockList(kids).PlainText(), want)
			}
		}

		table, ok := byType[BlockTypeTable].(*TableBlock)
		if !ok {
			t.Fatalf("table = %T", byType[BlockTypeTable])
		}
		if table.Table.TableWidth != 2 || table.Table.HasColumnHeader == nil || !*table.Table.HasColumnHeader {
			t.Errorf("table = %+v, want width 2 with a column header", table.Table)
		}
		rows := collectChildren(t, client, table.ID)
		if len(rows) != 2 {
			t.Fatalf("table rows = %v, want 2", blockTypes(rows))
		}
		row, ok := rows[0].(*TableRowBlock)
		if !ok {
			t.Fatalf("row 0 = %T, want *TableRowBlock", rows[0])
		}
		if len(row.TableRow.Cells) != 2 || row.TableRow.Cells[0].PlainText() != "Key" || row.TableRow.Cells[1].PlainText() != "Value" {
			t.Errorf("row 0 cells = %v", row.TableRow.Cells)
		}

		columns := collectChildren(t, client, byType[BlockTypeColumnList].Base().ID)
		if len(columns) != 2 {
			t.Fatalf("column_list children = %v, want 2 columns", blockTypes(columns))
		}
		for i, want := range []string{"Left column", "Right column"} {
			column, ok := columns[i].(*ColumnBlock)
			if !ok {
				t.Fatalf("column %d = %T", i, columns[i])
			}
			inner := collectChildren(t, client, column.ID)
			if len(inner) != 1 || BlockList(inner).PlainText() != want || !inner[0].Base().HasChildren {
				t.Fatalf("column %d children = %v (%q), want %q with children", i, blockTypes(inner), BlockList(inner).PlainText(), want)
			}
			deepest := collectChildren(t, client, inner[0].Base().ID)
			if BlockList(deepest).PlainText() != strings.Replace(want, "column", "nested", 1) {
				t.Errorf("column %d fourth level = %q", i, BlockList(deepest).PlainText())
			}
		}
	})

	// Retrieve, Update, AppendChildrenAfter, Delete.
	first := got[0]
	retrieved, err := client.Blocks.Retrieve(ctx, first.Base().ID)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if retrieved.BlockType() != BlockTypeParagraph || !SameID(retrieved.Base().ID, first.Base().ID) {
		t.Errorf("Retrieve = %s %s, want paragraph %s", retrieved.BlockType(), retrieved.Base().ID, first.Base().ID)
	}
	if !SameID(retrieved.Base().Parent.PageID, page.ID) {
		t.Errorf("Retrieve parent = %+v, want page %s", retrieved.Base().Parent, page.ID)
	}

	updated, err := client.Blocks.Update(ctx, first.Base().ID, &ParagraphBlock{Paragraph: ParagraphContent{BlockText: BlockText{
		RichText: NewRichText("Revised paragraph."),
		Color:    ColorBlueBackground,
	}}})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if p, ok := updated.(*ParagraphBlock); !ok || p.Paragraph.RichText.PlainText() != "Revised paragraph." || p.Paragraph.Color != ColorBlueBackground {
		t.Errorf("Update = %+v, want revised text with blue background", updated)
	}

	inserted, err := client.Blocks.AppendChildrenAfter(ctx, page.ID, first.Base().ID, []Block{NewParagraph("Inserted second.")})
	if err != nil {
		t.Fatalf("AppendChildrenAfter: %v", err)
	}
	// With "after", Notion returns the inserted blocks followed by every
	// sibling that now comes after them, so only the head is the new block.
	if len(inserted) != len(children) || inserted[0].BlockType() != BlockTypeParagraph || BlockList(inserted[:1]).PlainText() != "Inserted second." {
		t.Fatalf("AppendChildrenAfter returned %d blocks headed by %v, want %d headed by the inserted paragraph", len(inserted), blockTypes(inserted[:min(1, len(inserted))]), len(children))
	}
	after := collectChildren(t, client, page.ID)
	if len(after) != len(children)+1 || !SameID(after[1].Base().ID, inserted[0].Base().ID) {
		t.Errorf("after insert, block 1 = %s, want the inserted block %s", after[1].Base().ID, inserted[0].Base().ID)
	}

	deleted, err := client.Blocks.Delete(ctx, inserted[0].Base().ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !deleted.Base().InTrash {
		t.Errorf("Delete returned in_trash=false: %+v", deleted.Base())
	}
	gone, err := client.Blocks.Retrieve(ctx, inserted[0].Base().ID)
	if err != nil && !IsNotFound(err) {
		t.Fatalf("Retrieve after delete: %v", err)
	}
	if err == nil && !gone.Base().InTrash {
		t.Error("deleted block is neither trashed nor gone")
	}
	final := collectChildren(t, client, page.ID)
	if len(final) != len(children) {
		t.Errorf("after delete, %d blocks remain, want %d", len(final), len(children))
	}
}

// sweepFixture is a database whose data source has every schema type and two
// rows with known values. Tests share one instance per process; TestMain
// trashes it.
type sweepFixture struct {
	client       *Client
	parentID     string
	db           *Database
	dataSourceID string
	// alpha has every writable property set; beta relates to alpha and has a
	// date range.
	alpha, beta *Page
	// bot is the integration's own user; person is a human member, nil when the
	// workspace has none visible to the integration.
	bot    *User
	person *User
	// hasStatus reports whether the API accepted the status column.
	hasStatus bool
	// usersErr is the error listing users produced, when the token may not
	// list them; personal access tokens cannot.
	usersErr error
}

var (
	sweepOnce    sync.Once
	sweepShared  *sweepFixture
	sweepErr     error
	sweepTrashMu sync.Mutex
)

const (
	alphaDate      = "2026-01-15"
	betaRangeStart = "2026-03-01"
	betaRangeEnd   = "2026-03-05"
	alphaURL       = "https://example.com/alpha"
	alphaEmail     = "alpha@example.com"
	alphaPhone     = "+15555550100"
)

// sharedSweepFixture returns the process-wide fixture, building it on first
// use.
func sharedSweepFixture(t *testing.T) *sweepFixture {
	t.Helper()
	client, parentID := integrationClient(t)
	sweepOnce.Do(func() {
		sweepShared, sweepErr = buildSweepFixture(context.Background(), client, parentID)
	})
	if sweepErr != nil {
		t.Fatalf("building the shared fixture: %v", sweepErr)
	}
	return sweepShared
}

func trashSweepFixture() {
	sweepTrashMu.Lock()
	defer sweepTrashMu.Unlock()
	if sweepShared == nil || sweepShared.db == nil {
		return
	}
	if _, err := sweepShared.client.Databases.Trash(context.Background(), sweepShared.db.ID); err != nil {
		fmt.Fprintf(os.Stderr, "cleanup: trashing fixture database %s: %v\n", sweepShared.db.ID, err)
	}
	sweepShared = nil
}

func mustDate(s string) DateTime {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		panic(err)
	}
	return NewDate(t)
}

func buildSweepFixture(ctx context.Context, client *Client, parentID string) (*sweepFixture, error) {
	f := &sweepFixture{client: client, parentID: parentID}

	var err error
	if f.bot, err = client.Users.Me(ctx); err != nil {
		return nil, fmt.Errorf("Users.Me: %w", err)
	}
	for user, err := range client.Users.All(ctx) {
		if err != nil {
			if isRestricted(err) {
				f.usersErr = err
				break
			}
			return nil, fmt.Errorf("Users.All: %w", err)
		}
		if user.Type == UserTypePerson {
			f.person = &user
			break
		}
	}

	schema := PropertySchemas{
		"Name":        &TitleSchema{},
		"Notes":       &RichTextSchema{},
		"Scratch":     &RichTextSchema{},
		"Temp":        &CheckboxSchema{},
		"Priority":    &NumberSchema{Number: NumberConfig{Format: "dollar"}},
		"Kind":        &SelectSchema{Select: OptionsConfig{Options: []SelectOption{{Name: "Bug", Color: SelectColorRed}, {Name: "Feature", Color: SelectColorGreen}}}},
		"Tags":        &MultiSelectSchema{MultiSelect: OptionsConfig{Options: []SelectOption{{Name: "go", Color: SelectColorBlue}, {Name: "api"}}}},
		"Status":      &StatusSchema{},
		"Due":         &DateSchema{},
		"Owner":       &PeopleSchema{},
		"Attachments": &FilesSchema{},
		"Done":        &CheckboxSchema{},
		"Link":        &URLSchema{},
		"Email":       &EmailSchema{},
		"Phone":       &PhoneNumberSchema{},
		"Doubled":     &FormulaSchema{Formula: FormulaConfig{Expression: `prop("Priority") * 2`}},
		"Created":     &CreatedTimeSchema{},
		"Creator":     &CreatedBySchema{},
		"Edited":      &LastEditedTimeSchema{},
		"Editor":      &LastEditedBySchema{},
		"Ticket":      &UniqueIDSchema{UniqueID: UniqueIDConfig{Prefix: "TK"}},
	}
	create := func() (*Database, error) {
		return client.Databases.Create(ctx, CreateDatabaseParams{
			Parent:            Parent{Type: ParentTypePage, PageID: parentID},
			Title:             NewRichText("notion-go schema sweep"),
			Icon:              NewEmojiIcon("🧬"),
			InitialDataSource: &InitialDataSource{Properties: schema},
		})
	}
	f.db, err = create()
	if err != nil && IsValidationError(err) && strings.Contains(strings.ToLower(err.Error()), "status") {
		// Some workspaces reject creating status columns through the API.
		delete(schema, "Status")
		f.db, err = create()
	}
	if err != nil {
		return nil, fmt.Errorf("Databases.Create: %w", err)
	}
	_, f.hasStatus = schema["Status"]
	if len(f.db.DataSources) == 0 {
		return f, errors.New("the new database has no data sources")
	}
	f.dataSourceID = f.db.DataSources[0].ID

	// A self-relation needs the data source's ID, so it is added afterwards,
	// and the rollup over it in a second step.
	if _, err := client.DataSources.Update(ctx, f.dataSourceID, UpdateDataSourceParams{
		Properties: PropertySchemas{
			"Related": &RelationSchema{Relation: RelationConfig{
				DataSourceID:   f.dataSourceID,
				SingleProperty: &EmptyObject{},
			}},
		},
	}); err != nil {
		return f, fmt.Errorf("adding the relation column: %w", err)
	}
	if _, err := client.DataSources.Update(ctx, f.dataSourceID, UpdateDataSourceParams{
		Properties: PropertySchemas{
			"Related Count": &RollupSchema{Rollup: RollupConfig{
				Function:             "count",
				RelationPropertyName: "Related",
				RollupPropertyName:   "Name",
			}},
		},
	}); err != nil {
		return f, fmt.Errorf("adding the rollup column: %w", err)
	}

	owners := &PeopleProperty{}
	if f.person != nil {
		owners.People = []User{{ID: f.person.ID}}
	}
	alphaProps := PropertyValues{
		"Name": NewTitle("Alpha row"),
		"Notes": &RichTextProperty{RichText: RichTextList{
			&Text{RichTextCommon: RichTextCommon{Annotations: Annotations{Bold: true}}, Text: TextContent{Content: "bold "}},
			&Text{Text: TextContent{Content: "link", Link: &Link{URL: alphaURL}}},
		}},
		"Priority":    NewNumber(5),
		"Kind":        NewSelect("Bug"),
		"Tags":        NewMultiSelect("go", "api"),
		"Due":         NewDateValue(mustDate(alphaDate)),
		"Owner":       owners,
		"Attachments": &FilesProperty{Files: []File{NewNamedExternalFile("dummy.pdf", sweepPDFURL)}},
		"Done":        NewCheckbox(true),
		"Link":        &URLProperty{URL: alphaURL},
		"Email":       &EmailProperty{Email: alphaEmail},
		"Phone":       &PhoneNumberProperty{PhoneNumber: alphaPhone},
	}
	if f.hasStatus {
		alphaProps["Status"] = NewStatus("In progress")
	}
	f.alpha, err = client.Pages.Create(ctx, CreatePageParams{
		Parent:     Parent{Type: ParentTypeDataSource, DataSourceID: f.dataSourceID},
		Icon:       NewEmojiIcon("🅰️"),
		Properties: alphaProps,
	})
	if err != nil {
		return f, fmt.Errorf("creating the alpha row: %w", err)
	}
	f.beta, err = client.Pages.Create(ctx, CreatePageParams{
		Parent: Parent{Type: ParentTypeDataSource, DataSourceID: f.dataSourceID},
		Properties: PropertyValues{
			"Name":     NewTitle("Beta row"),
			"Priority": NewNumber(10),
			"Kind":     NewSelect("Feature"),
			"Tags":     NewMultiSelect("go"),
			"Due":      NewDateRange(mustDate(betaRangeStart), mustDate(betaRangeEnd)),
			"Done":     NewCheckbox(false),
			"Related":  NewRelation(f.alpha.ID),
		},
	})
	if err != nil {
		return f, fmt.Errorf("creating the beta row: %w", err)
	}
	return f, nil
}

func TestIntegrationSchemaAndProperties(t *testing.T) {
	f := sharedSweepFixture(t)
	client, ctx := f.client, t.Context()

	// Databases.
	db, err := client.Databases.Retrieve(ctx, f.db.ID)
	if err != nil {
		t.Fatalf("Databases.Retrieve: %v", err)
	}
	if !db.IsFull() || db.Title.PlainText() != "notion-go schema sweep" || len(db.DataSources) == 0 {
		t.Errorf("Databases.Retrieve = %+v, want a full database with a data source", db)
	}
	if db.Icon == nil || db.Icon.Emoji != "🧬" {
		t.Errorf("database icon = %+v", db.Icon)
	}
	if !SameID(db.Parent.PageID, f.parentID) {
		t.Errorf("database parent = %+v, want page %s", db.Parent, f.parentID)
	}
	renamed, err := client.Databases.Update(ctx, f.db.ID, UpdateDatabaseParams{
		Title:       NewRichText("notion-go schema sweep (renamed)"),
		Description: NewRichText("Exercises every schema type."),
	})
	if err != nil {
		t.Fatalf("Databases.Update: %v", err)
	}
	if renamed.Title.PlainText() != "notion-go schema sweep (renamed)" || renamed.Description.PlainText() != "Exercises every schema type." {
		t.Errorf("Databases.Update title = %q, description = %q", renamed.Title.PlainText(), renamed.Description.PlainText())
	}

	// Data source schema.
	ds, err := client.DataSources.Retrieve(ctx, f.dataSourceID)
	if err != nil {
		t.Fatalf("DataSources.Retrieve: %v", err)
	}
	if !ds.IsFull() || !SameID(ds.Parent.DatabaseID, f.db.ID) || !SameID(ds.DatabaseParent.PageID, f.parentID) {
		t.Errorf("DataSources.Retrieve = parent %+v, database_parent %+v", ds.Parent, ds.DatabaseParent)
	}
	if got := ds.Properties.TitleName(); got != "Name" {
		t.Errorf("TitleName = %q, want Name", got)
	}
	wantSchema := map[string]PropertyType{
		"Name": PropertyTypeTitle, "Notes": PropertyTypeRichText, "Priority": PropertyTypeNumber,
		"Kind": PropertyTypeSelect, "Tags": PropertyTypeMultiSelect, "Due": PropertyTypeDate,
		"Owner": PropertyTypePeople, "Attachments": PropertyTypeFiles, "Done": PropertyTypeCheckbox,
		"Link": PropertyTypeURL, "Email": PropertyTypeEmail, "Phone": PropertyTypePhoneNumber,
		"Doubled": PropertyTypeFormula, "Created": PropertyTypeCreatedTime, "Creator": PropertyTypeCreatedBy,
		"Edited": PropertyTypeLastEditedTime, "Editor": PropertyTypeLastEditedBy, "Ticket": PropertyTypeUniqueID,
		"Related": PropertyTypeRelation, "Related Count": PropertyTypeRollup,
	}
	if f.hasStatus {
		wantSchema["Status"] = PropertyTypeStatus
	} else {
		t.Log("the API rejected a status column; skipping status assertions")
	}
	for name, want := range wantSchema {
		t.Run("schema/"+string(want), func(t *testing.T) {
			schema, ok := ds.Properties[name]
			if !ok {
				t.Fatalf("column %q is missing", name)
			}
			if _, unknown := schema.(*UnknownSchema); unknown {
				t.Fatalf("column %q decoded as UnknownSchema", name)
			}
			if schema.SchemaType() != want {
				t.Fatalf("column %q type = %q, want %q", name, schema.SchemaType(), want)
			}
			if schema.Schema().ID == "" || schema.Schema().Name != name {
				t.Errorf("column %q base = %+v", name, schema.Schema())
			}
		})
	}
	if s, ok := ds.Properties["Priority"].(*NumberSchema); !ok || s.Number.Format != "dollar" {
		t.Errorf("Priority schema = %+v, want dollar format", ds.Properties["Priority"])
	}
	if s, ok := ds.Properties["Kind"].(*SelectSchema); !ok || len(s.Select.Options) != 2 || s.Select.Options[0].Color != SelectColorRed || s.Select.Options[0].ID == "" {
		t.Errorf("Kind schema = %+v, want two colored options with IDs", ds.Properties["Kind"])
	}
	if s, ok := ds.Properties["Tags"].(*MultiSelectSchema); !ok || len(s.MultiSelect.Options) != 2 {
		t.Errorf("Tags schema = %+v, want two options", ds.Properties["Tags"])
	}
	if f.hasStatus {
		if s, ok := ds.Properties["Status"].(*StatusSchema); !ok || len(s.Status.Options) == 0 || len(s.Status.Groups) != 3 {
			t.Errorf("Status schema = %+v, want default options in three groups", ds.Properties["Status"])
		}
	}
	if s, ok := ds.Properties["Doubled"].(*FormulaSchema); !ok || !strings.Contains(s.Formula.Expression, "Priority") {
		t.Errorf("Doubled schema = %+v", ds.Properties["Doubled"])
	}
	if s, ok := ds.Properties["Related"].(*RelationSchema); !ok || !SameID(s.Relation.DataSourceID, f.dataSourceID) || s.Relation.Type != "single_property" || s.Relation.SingleProperty == nil {
		t.Errorf("Related schema = %+v, want a single_property self-relation", ds.Properties["Related"])
	}
	if s, ok := ds.Properties["Related Count"].(*RollupSchema); !ok || s.Rollup.Function != "count" || s.Rollup.RelationPropertyName != "Related" || s.Rollup.RelationPropertyID == "" {
		t.Errorf("Related Count schema = %+v", ds.Properties["Related Count"])
	}
	if s, ok := ds.Properties["Ticket"].(*UniqueIDSchema); !ok || s.UniqueID.Prefix != "TK" {
		t.Errorf("Ticket schema = %+v, want prefix TK", ds.Properties["Ticket"])
	}

	// Rename, add, and delete columns in one update.
	edited, err := client.DataSources.Update(ctx, f.dataSourceID, UpdateDataSourceParams{
		Title: NewRichText("sweep data source"),
		Properties: PropertySchemas{
			"Scratch": &RichTextSchema{SchemaBase: SchemaBase{Name: "Extra"}},
			"Score":   &NumberSchema{Number: NumberConfig{Format: "percent"}},
			"Temp":    nil,
		},
	})
	if err != nil {
		t.Fatalf("DataSources.Update: %v", err)
	}
	if edited.Title.PlainText() != "sweep data source" {
		t.Errorf("data source title = %q", edited.Title.PlainText())
	}
	if _, ok := edited.Properties["Extra"].(*RichTextSchema); !ok {
		t.Errorf("renamed column Extra = %T, want *RichTextSchema", edited.Properties["Extra"])
	}
	if _, ok := edited.Properties["Scratch"]; ok {
		t.Error("column Scratch still present after rename")
	}
	if s, ok := edited.Properties["Score"].(*NumberSchema); !ok || s.Number.Format != "percent" {
		t.Errorf("added column Score = %+v", edited.Properties["Score"])
	}
	if _, ok := edited.Properties["Temp"]; ok {
		t.Error("column Temp still present after deletion")
	}

	// A second data source on the same database.
	second, err := client.DataSources.Create(ctx, CreateDataSourceParams{
		Parent:     Parent{Type: ParentTypeDatabase, DatabaseID: f.db.ID},
		Title:      NewRichText("second source"),
		Properties: PropertySchemas{"Title": &TitleSchema{}, "Flag": &CheckboxSchema{}},
	})
	switch {
	case err != nil:
		t.Logf("DataSources.Create refused: %v", err)
	case !SameID(second.Parent.DatabaseID, f.db.ID):
		t.Errorf("second data source parent = %+v, want database %s", second.Parent, f.db.ID)
	default:
		if _, ok := second.Properties["Flag"].(*CheckboxSchema); !ok {
			t.Errorf("second data source Flag = %T", second.Properties["Flag"])
		}
		if _, err := client.DataSources.Update(ctx, second.ID, UpdateDataSourceParams{InTrash: new(true)}); err != nil {
			t.Errorf("trashing the second data source: %v", err)
		}
	}

	// Alpha's property values.
	alpha, err := client.Pages.Retrieve(ctx, f.alpha.ID)
	if err != nil {
		t.Fatalf("Pages.Retrieve(alpha): %v", err)
	}
	beta, err := client.Pages.Retrieve(ctx, f.beta.ID)
	if err != nil {
		t.Fatalf("Pages.Retrieve(beta): %v", err)
	}
	if !SameID(alpha.Parent.DataSourceID, f.dataSourceID) || !SameID(alpha.Parent.DatabaseID, f.db.ID) {
		t.Errorf("alpha parent = %+v", alpha.Parent)
	}
	if !SameID(alpha.CreatedBy.ID, f.bot.ID) || alpha.CreatedTime.IsZero() || alpha.Icon == nil {
		t.Errorf("alpha envelope: created_by %s, created_time %v, icon %+v", alpha.CreatedBy.ID, alpha.CreatedTime, alpha.Icon)
	}
	props := alpha.Properties
	for name, value := range props {
		if _, unknown := value.(*UnknownProperty); unknown {
			t.Errorf("property %q decoded as UnknownProperty", name)
		}
		if value.PropertyID() == "" {
			t.Errorf("property %q has no ID", name)
		}
	}
	checks := map[string]func(t *testing.T){
		"title": func(t *testing.T) {
			v, ok := props["Name"].(*TitleProperty)
			if !ok || v.Title.PlainText() != "Alpha row" || props.Title() != "Alpha row" || props.Text("Name") != "Alpha row" {
				t.Fatalf("Name = %+v", props["Name"])
			}
		},
		"rich_text": func(t *testing.T) {
			v, ok := props["Notes"].(*RichTextProperty)
			if !ok || len(v.RichText) != 2 || v.RichText.PlainText() != "bold link" {
				t.Fatalf("Notes = %+v", props["Notes"])
			}
			bold, ok := v.RichText[0].(*Text)
			if !ok || !bold.Annotations.Bold || bold.Text.Content != "bold " {
				t.Errorf("first run = %+v, want bold", v.RichText[0])
			}
			link, ok := v.RichText[1].(*Text)
			if !ok || link.Text.Link == nil || link.Text.Link.URL != alphaURL || link.Href != alphaURL {
				t.Errorf("second run = %+v, want a link to %s", v.RichText[1], alphaURL)
			}
		},
		"number": func(t *testing.T) {
			if n, ok := props.Number("Priority"); !ok || n != 5 {
				t.Fatalf("Priority = %+v", props["Priority"])
			}
		},
		"select": func(t *testing.T) {
			v, ok := props["Kind"].(*SelectProperty)
			if !ok || v.Select == nil || v.Select.Name != "Bug" || v.Select.Color != SelectColorRed || props.Select("Kind") != "Bug" {
				t.Fatalf("Kind = %+v", props["Kind"])
			}
		},
		"multi_select": func(t *testing.T) {
			v, ok := props["Tags"].(*MultiSelectProperty)
			if !ok || len(v.MultiSelect) != 2 || v.MultiSelect[0].ID == "" {
				t.Fatalf("Tags = %+v", props["Tags"])
			}
		},
		"status": func(t *testing.T) {
			if !f.hasStatus {
				t.Skip("no status column")
			}
			v, ok := props["Status"].(*StatusProperty)
			if !ok || v.Status == nil || v.Status.Name != "In progress" || props.Select("Status") != "In progress" {
				t.Fatalf("Status = %+v", props["Status"])
			}
		},
		"date": func(t *testing.T) {
			d := props.Date("Due")
			if d == nil || d.Start.String() != alphaDate || d.Start.HasTime || d.End != nil {
				t.Fatalf("alpha Due = %+v, want a single date %s", props["Due"], alphaDate)
			}
			r := beta.Properties.Date("Due")
			if r == nil || r.Start.String() != betaRangeStart || r.End == nil || r.End.String() != betaRangeEnd {
				t.Fatalf("beta Due = %+v, want a range %s..%s", beta.Properties["Due"], betaRangeStart, betaRangeEnd)
			}
		},
		"people": func(t *testing.T) {
			v, ok := props["Owner"].(*PeopleProperty)
			if !ok {
				t.Fatalf("Owner = %T", props["Owner"])
			}
			if f.person == nil {
				t.Log("no person user visible to the integration; Owner was set to an empty list")
				if len(v.People) != 0 {
					t.Errorf("Owner = %+v, want empty", v.People)
				}
				return
			}
			if len(v.People) != 1 || !SameID(v.People[0].ID, f.person.ID) {
				t.Fatalf("Owner = %+v, want %s", v.People, f.person.ID)
			}
		},
		"files": func(t *testing.T) {
			v, ok := props["Attachments"].(*FilesProperty)
			if !ok || len(v.Files) != 1 || v.Files[0].Name != "dummy.pdf" || v.Files[0].URL() != sweepPDFURL {
				t.Fatalf("Attachments = %+v", props["Attachments"])
			}
		},
		"checkbox": func(t *testing.T) {
			if !props.Checkbox("Done") {
				t.Fatalf("Done = %+v, want true", props["Done"])
			}
		},
		"url": func(t *testing.T) {
			if v, ok := props["Link"].(*URLProperty); !ok || v.URL != alphaURL {
				t.Fatalf("Link = %+v", props["Link"])
			}
		},
		"email": func(t *testing.T) {
			if v, ok := props["Email"].(*EmailProperty); !ok || v.Email != alphaEmail {
				t.Fatalf("Email = %+v", props["Email"])
			}
		},
		"phone_number": func(t *testing.T) {
			if v, ok := props["Phone"].(*PhoneNumberProperty); !ok || v.PhoneNumber != alphaPhone {
				t.Fatalf("Phone = %+v", props["Phone"])
			}
		},
		"formula": func(t *testing.T) {
			v, ok := props["Doubled"].(*FormulaProperty)
			if !ok || v.Formula.Type != "number" || v.Formula.Number == nil || *v.Formula.Number != 10 {
				t.Fatalf("Doubled = %+v, want number 10", props["Doubled"])
			}
		},
		"relation": func(t *testing.T) {
			v, ok := beta.Properties["Related"].(*RelationProperty)
			if !ok || len(v.Relation) != 1 || !SameID(v.Relation[0].ID, f.alpha.ID) || v.HasMore {
				t.Fatalf("beta Related = %+v, want alpha", beta.Properties["Related"])
			}
			if empty, ok := props["Related"].(*RelationProperty); !ok || len(empty.Relation) != 0 {
				t.Errorf("alpha Related = %+v, want empty", props["Related"])
			}
		},
		"rollup": func(t *testing.T) {
			v, ok := beta.Properties["Related Count"].(*RollupProperty)
			if !ok || v.Rollup.Function != "count" || v.Rollup.Type != "number" || v.Rollup.Number == nil || *v.Rollup.Number != 1 {
				t.Fatalf("beta Related Count = %+v, want count 1", beta.Properties["Related Count"])
			}
		},
		"created_time": func(t *testing.T) {
			v, ok := props["Created"].(*CreatedTimeProperty)
			if !ok || v.CreatedTime.IsZero() || time.Since(v.CreatedTime) > time.Hour || !v.CreatedTime.Equal(alpha.CreatedTime) {
				t.Fatalf("Created = %+v", props["Created"])
			}
		},
		"created_by": func(t *testing.T) {
			v, ok := props["Creator"].(*CreatedByProperty)
			if !ok || !SameID(v.CreatedBy.ID, f.bot.ID) {
				t.Fatalf("Creator = %+v, want bot %s", props["Creator"], f.bot.ID)
			}
		},
		"last_edited_time": func(t *testing.T) {
			v, ok := props["Edited"].(*LastEditedTimeProperty)
			if !ok || v.LastEditedTime.IsZero() || v.LastEditedTime.Before(alpha.CreatedTime) {
				t.Fatalf("Edited = %+v", props["Edited"])
			}
		},
		"last_edited_by": func(t *testing.T) {
			v, ok := props["Editor"].(*LastEditedByProperty)
			if !ok || v.LastEditedBy.ID == "" {
				t.Fatalf("Editor = %+v", props["Editor"])
			}
		},
		"unique_id": func(t *testing.T) {
			v, ok := props["Ticket"].(*UniqueIDProperty)
			if !ok || v.UniqueID.Prefix != "TK" || v.UniqueID.Number == nil || *v.UniqueID.Number < 1 {
				t.Fatalf("Ticket = %+v, want TK-n", props["Ticket"])
			}
		},
	}
	for name, check := range checks {
		t.Run("value/"+name, check)
	}

	// The property item endpoint.
	t.Run("RetrieveProperty", func(t *testing.T) {
		relation, err := client.Pages.RetrieveProperty(ctx, f.beta.ID, ds.Properties["Related"].Schema().ID, PageParams{})
		if err != nil {
			t.Fatalf("RetrieveProperty(relation): %v", err)
		}
		if v, ok := relation.Combined().(*RelationProperty); !ok || len(v.Relation) != 1 || !SameID(v.Relation[0].ID, f.alpha.ID) {
			t.Errorf("relation Combined() = %+v", relation.Combined())
		}
		if relation.Object != "list" || len(relation.PropertyItem) == 0 {
			t.Errorf("relation item = object %q, property_item %s", relation.Object, relation.PropertyItem)
		}

		notes, err := client.Pages.RetrieveProperty(ctx, f.alpha.ID, ds.Properties["Notes"].Schema().ID, PageParams{})
		if err != nil {
			t.Fatalf("RetrieveProperty(rich_text): %v", err)
		}
		if v, ok := notes.Combined().(*RichTextProperty); !ok || len(v.RichText) != 2 || v.RichText.PlainText() != "bold link" {
			t.Errorf("rich_text Combined() = %+v", notes.Combined())
		}

		title, err := client.Pages.RetrieveProperty(ctx, f.alpha.ID, "title", PageParams{PageSize: 1})
		if err != nil {
			t.Fatalf("RetrieveProperty(title): %v", err)
		}
		if v, ok := title.Combined().(*TitleProperty); !ok || v.Title.PlainText() != "Alpha row" {
			t.Errorf("title Combined() = %+v", title.Combined())
		}

		number, err := client.Pages.RetrieveProperty(ctx, f.alpha.ID, ds.Properties["Priority"].Schema().ID, PageParams{})
		if err != nil {
			t.Fatalf("RetrieveProperty(number): %v", err)
		}
		if v, ok := number.Combined().(*NumberProperty); !ok || v.Number == nil || *v.Number != 5 {
			t.Errorf("number Combined() = %+v", number.Combined())
		}
	})

	// A third row for the mutations, so alpha and beta stay as the filter test
	// expects them.
	gamma, err := client.Pages.Create(ctx, CreatePageParams{
		Parent: Parent{Type: ParentTypeDataSource, DataSourceID: f.dataSourceID},
		Properties: PropertyValues{
			"Name":    NewTitle("Gamma row"),
			"Tags":    NewMultiSelect("go", "api"),
			"Related": NewRelation(f.alpha.ID, f.beta.ID),
			"Done":    NewCheckbox(true),
		},
	})
	if err != nil {
		t.Fatalf("creating the gamma row: %v", err)
	}
	trashOnCleanup(t, client, gamma.ID)

	cleared, err := client.Pages.Update(ctx, gamma.ID, UpdatePageParams{Properties: PropertyValues{
		"Tags":    &MultiSelectProperty{},
		"Related": &RelationProperty{},
		"Done":    NewCheckbox(false),
	}})
	if err != nil {
		t.Fatalf("Pages.Update(clear): %v", err)
	}
	if v, ok := cleared.Properties["Tags"].(*MultiSelectProperty); !ok || len(v.MultiSelect) != 0 {
		t.Errorf("cleared Tags = %+v", cleared.Properties["Tags"])
	}
	if v, ok := cleared.Properties["Related"].(*RelationProperty); !ok || len(v.Relation) != 0 {
		t.Errorf("cleared Related = %+v", cleared.Properties["Related"])
	}
	if cleared.Properties.Checkbox("Done") {
		t.Error("Done is still true after clearing")
	}

	trashed, err := client.Pages.Trash(ctx, gamma.ID)
	if err != nil {
		t.Fatalf("Pages.Trash: %v", err)
	}
	if !trashed.InTrash {
		t.Error("Trash left in_trash false")
	}
	restored, err := client.Pages.Restore(ctx, gamma.ID)
	if err != nil {
		t.Fatalf("Pages.Restore: %v", err)
	}
	if restored.InTrash {
		t.Error("Restore left in_trash true")
	}

	t.Run("Move", func(t *testing.T) {
		moved, err := client.Pages.Move(ctx, gamma.ID, Parent{Type: ParentTypePage, PageID: f.parentID})
		if err != nil {
			t.Logf("moving a row out of its data source refused: %v", err)
			moved = nil
		}
		if moved != nil {
			if moved.Parent.Type != ParentTypePage || !SameID(moved.Parent.PageID, f.parentID) {
				t.Errorf("moved parent = %+v, want page %s", moved.Parent, f.parentID)
			}
			back, err := client.Pages.Move(ctx, gamma.ID, Parent{Type: ParentTypeDataSource, DataSourceID: f.dataSourceID})
			if err != nil {
				t.Fatalf("moving the row back: %v", err)
			}
			if back.Parent.Type != ParentTypeDataSource || !SameID(back.Parent.DataSourceID, f.dataSourceID) {
				t.Errorf("moved-back parent = %+v, want data source %s", back.Parent, f.dataSourceID)
			}
			return
		}
		// Rows cannot leave their data source; move a plain page between two
		// pages instead.
		mover := createSweepPage(t, client, f.parentID, "notion-go mover")
		target := createSweepPage(t, client, f.parentID, "notion-go move target")
		moved, err = client.Pages.Move(ctx, mover.ID, Parent{Type: ParentTypePage, PageID: target.ID})
		if err != nil {
			t.Fatalf("Pages.Move: %v", err)
		}
		if !SameID(moved.Parent.PageID, target.ID) {
			t.Errorf("moved parent = %+v, want %s", moved.Parent, target.ID)
		}
		back, err := client.Pages.Move(ctx, mover.ID, Parent{Type: ParentTypePage, PageID: f.parentID})
		if err != nil {
			t.Fatalf("Pages.Move back: %v", err)
		}
		if !SameID(back.Parent.PageID, f.parentID) {
			t.Errorf("moved-back parent = %+v, want %s", back.Parent, f.parentID)
		}
	})

	// Users.
	t.Run("Users", func(t *testing.T) {
		if f.bot.Bot == nil || f.bot.Bot.Owner == nil || f.bot.Bot.WorkspaceName == "" {
			t.Errorf("Me = %+v, want bot owner and workspace name", f.bot.Bot)
		}
		bot, err := client.Users.Retrieve(ctx, f.bot.ID)
		switch {
		case err == nil:
			if !bot.IsFull() || bot.Type != UserTypeBot || !SameID(bot.ID, f.bot.ID) || bot.Bot == nil {
				t.Errorf("Users.Retrieve = %+v, want the bot", bot)
			}
		case f.usersErr != nil && isRestricted(err) && f.bot.Bot != nil && f.bot.Bot.Owner != nil && f.bot.Bot.Owner.User != nil:
			// A personal access token may retrieve only the user who
			// authorized it.
			owner, err := client.Users.Retrieve(ctx, f.bot.Bot.Owner.User.ID)
			if err != nil {
				t.Fatalf("Users.Retrieve(owner): %v", err)
			}
			if !owner.IsFull() || owner.Type != UserTypePerson || !SameID(owner.ID, f.bot.Bot.Owner.User.ID) {
				t.Errorf("Users.Retrieve(owner) = %+v, want the authorizing person", owner)
			}
		default:
			t.Fatalf("Users.Retrieve: %v", err)
		}
		one, err := client.Users.List(ctx, PageParams{PageSize: 1})
		if f.usersErr != nil {
			// The token cannot list users; the error must at least be typed.
			if !isRestricted(err) {
				t.Fatalf("Users.List = %v, want restricted_resource like Users.All", err)
			}
			t.Skipf("this token cannot list users: %v", f.usersErr)
		}
		if err != nil {
			t.Fatalf("Users.List: %v", err)
		}
		if len(one.Results) != 1 || one.Object != "list" {
			t.Errorf("Users.List(1) = %d results, object %q", len(one.Results), one.Object)
		}
		all, err := Collect(client.Users.All(ctx))
		if err != nil {
			t.Fatalf("Users.All: %v", err)
		}
		found := false
		for _, user := range all {
			if !user.IsFull() {
				t.Errorf("user %s is partial", user.ID)
			}
			found = found || SameID(user.ID, f.bot.ID)
		}
		if !found {
			t.Errorf("Users.All (%d users) does not include the bot", len(all))
		}
		if len(all) < len(one.Results) {
			t.Errorf("Users.All returned %d, fewer than one page", len(all))
		}
	})
}

// filterCase is one query to run against the fixture. want is the expected
// row count, or -1 when the count depends on data the test does not control.
type filterCase struct {
	name   string
	filter Filter
	want   int
}

func TestIntegrationFilters(t *testing.T) {
	f := sharedSweepFixture(t)
	client, ctx := f.client, t.Context()

	count := func(t *testing.T, params QueryParams) []*Page {
		t.Helper()
		pages, err := Collect(client.DataSources.QueryAll(ctx, f.dataSourceID, params))
		if err != nil {
			t.Fatalf("QueryAll: %v", err)
		}
		return pages
	}

	day := func(s string) time.Time { return mustDate(s).Time }
	hourAgo := time.Now().Add(-time.Hour)
	statusIsEmpty := -1
	cases := []filterCase{
		{"title/equals", ByTitle("Name").Equals("Alpha row"), 1},
		{"title/does_not_equal", ByTitle("Name").DoesNotEqual("Alpha row"), 1},
		{"title/contains", ByTitle("Name").Contains("row"), 2},
		{"title/does_not_contain", ByTitle("Name").DoesNotContain("Alpha"), 1},
		{"title/starts_with", ByTitle("Name").StartsWith("Alpha"), 1},
		{"title/ends_with", ByTitle("Name").EndsWith("row"), 2},
		{"title/is_empty", ByTitle("Name").IsEmpty(), 0},
		{"title/is_not_empty", ByTitle("Name").IsNotEmpty(), 2},
		{"rich_text/equals", ByText("Notes").Equals("bold link"), 1},
		{"rich_text/does_not_equal", ByText("Notes").DoesNotEqual("bold link"), 1},
		{"rich_text/contains", ByText("Notes").Contains("bold"), 1},
		{"rich_text/does_not_contain", ByText("Notes").DoesNotContain("bold"), 1},
		{"rich_text/starts_with", ByText("Notes").StartsWith("bold"), 1},
		{"rich_text/ends_with", ByText("Notes").EndsWith("link"), 1},
		{"rich_text/is_empty", ByText("Notes").IsEmpty(), 1},
		{"rich_text/is_not_empty", ByText("Notes").IsNotEmpty(), 1},
		{"url/equals", ByURL("Link").Equals(alphaURL), 1},
		{"url/does_not_equal", ByURL("Link").DoesNotEqual(alphaURL), 1},
		{"url/contains", ByURL("Link").Contains("example.com"), 1},
		{"url/does_not_contain", ByURL("Link").DoesNotContain("example.com"), 1},
		{"url/starts_with", ByURL("Link").StartsWith("https://"), 1},
		{"url/ends_with", ByURL("Link").EndsWith("/alpha"), 1},
		{"url/is_empty", ByURL("Link").IsEmpty(), 1},
		{"url/is_not_empty", ByURL("Link").IsNotEmpty(), 1},
		{"email/equals", ByEmail("Email").Equals(alphaEmail), 1},
		{"email/does_not_equal", ByEmail("Email").DoesNotEqual(alphaEmail), 1},
		{"email/contains", ByEmail("Email").Contains("@example"), 1},
		{"email/does_not_contain", ByEmail("Email").DoesNotContain("@example"), 1},
		{"email/starts_with", ByEmail("Email").StartsWith("alpha"), 1},
		{"email/ends_with", ByEmail("Email").EndsWith(".com"), 1},
		{"email/is_empty", ByEmail("Email").IsEmpty(), 1},
		{"email/is_not_empty", ByEmail("Email").IsNotEmpty(), 1},
		{"phone_number/equals", ByPhoneNumber("Phone").Equals(alphaPhone), 1},
		{"phone_number/does_not_equal", ByPhoneNumber("Phone").DoesNotEqual(alphaPhone), 1},
		{"phone_number/contains", ByPhoneNumber("Phone").Contains("555"), 1},
		{"phone_number/does_not_contain", ByPhoneNumber("Phone").DoesNotContain("555"), 1},
		{"phone_number/starts_with", ByPhoneNumber("Phone").StartsWith("+1"), 1},
		{"phone_number/ends_with", ByPhoneNumber("Phone").EndsWith("0100"), 1},
		{"phone_number/is_empty", ByPhoneNumber("Phone").IsEmpty(), 1},
		{"phone_number/is_not_empty", ByPhoneNumber("Phone").IsNotEmpty(), 1},
		{"number/equals", ByNumber("Priority").Equals(5), 1},
		{"number/does_not_equal", ByNumber("Priority").DoesNotEqual(5), 1},
		{"number/greater_than", ByNumber("Priority").GreaterThan(5), 1},
		{"number/less_than", ByNumber("Priority").LessThan(10), 1},
		{"number/greater_than_or_equal_to", ByNumber("Priority").GreaterThanOrEqualTo(5), 2},
		{"number/less_than_or_equal_to", ByNumber("Priority").LessThanOrEqualTo(5), 1},
		{"number/is_empty", ByNumber("Priority").IsEmpty(), 0},
		{"number/is_not_empty", ByNumber("Priority").IsNotEmpty(), 2},
		{"checkbox/equals", ByCheckbox("Done").Equals(true), 1},
		{"checkbox/equals_false", ByCheckbox("Done").Equals(false), 1},
		{"checkbox/does_not_equal", ByCheckbox("Done").DoesNotEqual(true), 1},
		{"select/equals", BySelect("Kind").Equals("Bug"), 1},
		{"select/does_not_equal", BySelect("Kind").DoesNotEqual("Bug"), 1},
		{"select/is_empty", BySelect("Kind").IsEmpty(), 0},
		{"select/is_not_empty", BySelect("Kind").IsNotEmpty(), 2},
		{"multi_select/contains", ByMultiSelect("Tags").Contains("api"), 1},
		{"multi_select/does_not_contain", ByMultiSelect("Tags").DoesNotContain("api"), 1},
		{"multi_select/is_empty", ByMultiSelect("Tags").IsEmpty(), 0},
		{"multi_select/is_not_empty", ByMultiSelect("Tags").IsNotEmpty(), 2},
		{"date/equals", ByDate("Due").Equals(day(alphaDate)), 1},
		{"date/before", ByDate("Due").Before(day("2026-02-01")), 1},
		{"date/after", ByDate("Due").After(day("2026-02-01")), 1},
		{"date/on_or_before", ByDate("Due").OnOrBefore(day(alphaDate)), 1},
		{"date/on_or_after", ByDate("Due").OnOrAfter(day(betaRangeStart)), 1},
		{"date/past_week", ByDate("Due").PastWeek(), -1},
		{"date/past_month", ByDate("Due").PastMonth(), -1},
		{"date/past_year", ByDate("Due").PastYear(), -1},
		{"date/next_week", ByDate("Due").NextWeek(), -1},
		{"date/next_month", ByDate("Due").NextMonth(), -1},
		{"date/next_year", ByDate("Due").NextYear(), -1},
		{"date/this_week", ByDate("Due").ThisWeek(), -1},
		{"date/is_empty", ByDate("Due").IsEmpty(), 0},
		{"date/is_not_empty", ByDate("Due").IsNotEmpty(), 2},
		{"created_time/on_or_after", ByCreatedTime().OnOrAfter(hourAgo), 2},
		{"created_time/before", ByCreatedTime().Before(hourAgo), 0},
		{"created_time/after", ByCreatedTime().After(hourAgo), 2},
		{"created_time/past_week", ByCreatedTime().PastWeek(), 2},
		{"created_time/is_not_empty", ByCreatedTime().IsNotEmpty(), 2},
		{"last_edited_time/on_or_after", ByLastEditedTime().OnOrAfter(hourAgo), 2},
		{"last_edited_time/on_or_before", ByLastEditedTime().OnOrBefore(hourAgo), 0},
		{"last_edited_time/past_month", ByLastEditedTime().PastMonth(), 2},
		{"last_edited_time/equals", ByLastEditedTime().Equals(hourAgo), 0},
		{"created_by/contains", ByCreatedBy("Creator").Contains(f.bot.ID), 2},
		{"created_by/does_not_contain", ByCreatedBy("Creator").DoesNotContain(f.bot.ID), 0},
		{"created_by/is_empty", ByCreatedBy("Creator").IsEmpty(), 0},
		{"created_by/is_not_empty", ByCreatedBy("Creator").IsNotEmpty(), 2},
		{"last_edited_by/contains", ByLastEditedBy("Editor").Contains(f.bot.ID), 2},
		{"last_edited_by/does_not_contain", ByLastEditedBy("Editor").DoesNotContain(f.bot.ID), 0},
		{"last_edited_by/is_not_empty", ByLastEditedBy("Editor").IsNotEmpty(), 2},
		{"files/is_empty", ByFiles("Attachments").IsEmpty(), 1},
		{"files/is_not_empty", ByFiles("Attachments").IsNotEmpty(), 1},
		{"relation/contains", ByRelation("Related").Contains(f.alpha.ID), 1},
		{"relation/does_not_contain", ByRelation("Related").DoesNotContain(f.alpha.ID), 1},
		{"relation/is_empty", ByRelation("Related").IsEmpty(), 1},
		{"relation/is_not_empty", ByRelation("Related").IsNotEmpty(), 1},
		{"compound/and_of_or", And(
			Or(ByTitle("Name").Equals("Alpha row"), ByTitle("Name").Equals("Beta row")),
			ByNumber("Priority").GreaterThanOrEqualTo(5),
		), 2},
		{"compound/or_of_and", Or(
			And(BySelect("Kind").Equals("Bug"), ByCheckbox("Done").Equals(true)),
			And(BySelect("Kind").Equals("Feature"), ByCheckbox("Done").Equals(true)),
		), 1},
		{"compound/and_mixed", And(
			ByRelation("Related").IsNotEmpty(),
			Or(ByDate("Due").IsEmpty(), ByMultiSelect("Tags").Contains("go")),
		), 1},
	}
	if f.person != nil {
		cases = append(cases,
			filterCase{"people/contains", ByPeople("Owner").Contains(f.person.ID), 1},
			filterCase{"people/does_not_contain", ByPeople("Owner").DoesNotContain(f.person.ID), 1},
			filterCase{"people/is_empty", ByPeople("Owner").IsEmpty(), 1},
			filterCase{"people/is_not_empty", ByPeople("Owner").IsNotEmpty(), 1},
		)
	} else {
		cases = append(cases,
			filterCase{"people/contains", ByPeople("Owner").Contains(f.bot.ID), 0},
			filterCase{"people/does_not_contain", ByPeople("Owner").DoesNotContain(f.bot.ID), 2},
			filterCase{"people/is_empty", ByPeople("Owner").IsEmpty(), 2},
			filterCase{"people/is_not_empty", ByPeople("Owner").IsNotEmpty(), 0},
		)
	}
	if f.hasStatus {
		cases = append(cases,
			filterCase{"status/equals", ByStatus("Status").Equals("In progress"), 1},
			filterCase{"status/does_not_equal", ByStatus("Status").DoesNotEqual("In progress"), 1},
			filterCase{"status/is_empty", ByStatus("Status").IsEmpty(), statusIsEmpty},
			filterCase{"status/is_not_empty", ByStatus("Status").IsNotEmpty(), -1},
		)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pages := count(t, QueryParams{Filter: tc.filter})
			if tc.want >= 0 && len(pages) != tc.want {
				var titles []string
				for _, p := range pages {
					titles = append(titles, p.Title())
				}
				t.Errorf("matched %d rows %v, want %d", len(pages), titles, tc.want)
			}
		})
	}

	// A filter nested too deep is rejected before it is sent.
	_, err := client.DataSources.Query(ctx, f.dataSourceID, QueryParams{
		Filter: And(Or(And(ByCheckbox("Done").Equals(true)))),
	})
	if !errors.Is(err, ErrFilterTooDeep) {
		t.Errorf("three-level filter error = %v, want ErrFilterTooDeep", err)
	}

	t.Run("sorts", func(t *testing.T) {
		byPriority := count(t, QueryParams{Sorts: []Sort{SortBy("Priority", Descending)}})
		if len(byPriority) != 2 || byPriority[0].Title() != "Beta row" {
			t.Errorf("sorted by Priority descending: %v", pageTitles(byPriority))
		}
		byCreated := count(t, QueryParams{Sorts: []Sort{SortByCreatedTime(Ascending)}})
		if len(byCreated) != 2 || byCreated[0].Title() != "Alpha row" {
			t.Errorf("sorted by created_time ascending: %v", pageTitles(byCreated))
		}
		byEdited := count(t, QueryParams{
			Sorts:      []Sort{SortByLastEditedTime(Descending), SortBy("Name", Ascending)},
			PageParams: PageParams{PageSize: 1},
		})
		if len(byEdited) != 2 {
			t.Errorf("sorted by last_edited_time, paged by one: %v", pageTitles(byEdited))
		}
		single, err := client.DataSources.Query(ctx, f.dataSourceID, QueryParams{
			Sorts:      []Sort{SortBy("Priority", Ascending)},
			PageParams: PageParams{PageSize: 1},
		})
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		if len(single.Results) != 1 || single.NextCursor == "" || !single.HasMore || single.Results[0].Object != "page" {
			t.Errorf("Query page 1 = %d results, next_cursor %q, has_more %v", len(single.Results), single.NextCursor, single.HasMore)
		}
		if pages := QueryResults(single.Results).Pages(); len(pages) != 1 || pages[0].Title() != "Alpha row" {
			t.Errorf("Query page 1 = %v", pageTitles(pages))
		}
	})

}

// isRestricted reports a restricted_resource API error.
func isRestricted(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Code == CodeRestrictedResource
}

func pageTitles(pages []*Page) []string {
	titles := make([]string, len(pages))
	for i, p := range pages {
		titles[i] = p.Title()
	}
	return titles
}

func TestIntegrationComments(t *testing.T) {
	client, parentID := integrationClient(t)
	ctx := t.Context()
	page := createSweepPage(t, client, parentID, "notion-go comment sweep")

	first, err := client.Comments.Create(ctx, CreateCommentParams{
		Parent:   &Parent{Type: ParentTypePage, PageID: page.ID},
		RichText: NewRichText("First comment."),
	})
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && (apiErr.Code == CodeRestrictedResource || apiErr.Code == CodeUnauthorized || strings.Contains(strings.ToLower(apiErr.Message), "comment")) {
			t.Skipf("comments are disabled for this integration (%v); enable \"Read comments\" and \"Insert comments\" under the integration's Capabilities", err)
		}
		t.Fatalf("Comments.Create: %v", err)
	}
	if !first.IsFull() || first.Object != "comment" || first.DiscussionID == "" || first.Text() != "First comment." {
		t.Fatalf("Comments.Create = %+v", first)
	}
	if first.Parent.Type != ParentTypePage || !SameID(first.Parent.PageID, page.ID) {
		t.Errorf("comment parent = %+v, want page %s", first.Parent, page.ID)
	}
	if first.DisplayName == nil || first.DisplayName.Type == "" {
		t.Errorf("comment display_name = %+v", first.DisplayName)
	}

	reply, err := client.Comments.Create(ctx, CreateCommentParams{
		DiscussionID: first.DiscussionID,
		RichText:     NewRichText("A reply."),
	})
	if err != nil {
		t.Fatalf("Comments.Create(reply): %v", err)
	}
	if reply.DiscussionID != first.DiscussionID || reply.ID == first.ID {
		t.Errorf("reply = discussion %s id %s, want discussion %s", reply.DiscussionID, reply.ID, first.DiscussionID)
	}

	listed, err := client.Comments.List(ctx, page.ID, PageParams{PageSize: 1})
	if err != nil {
		t.Fatalf("Comments.List: %v", err)
	}
	if len(listed.Results) != 1 || listed.NextCursor == "" {
		t.Errorf("Comments.List(1) = %d results, next_cursor %q, want 1 with more", len(listed.Results), listed.NextCursor)
	}
	all, err := Collect(client.Comments.All(ctx, page.ID))
	if err != nil {
		t.Fatalf("Comments.All: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("Comments.All = %d comments, want 2", len(all))
	}
	for _, c := range all {
		if c.DiscussionID != first.DiscussionID || !c.IsFull() {
			t.Errorf("listed comment = %+v", c)
		}
	}

	got, err := client.Comments.Retrieve(ctx, first.ID)
	if err != nil {
		t.Fatalf("Comments.Retrieve: %v", err)
	}
	if !SameID(got.ID, first.ID) || got.Text() != "First comment." || got.CreatedTime.IsZero() || got.CreatedBy.ID == "" {
		t.Errorf("Comments.Retrieve = %+v", got)
	}

	updated, err := client.Comments.Update(ctx, first.ID, NewRichText("First comment, edited."))
	if err != nil {
		t.Fatalf("Comments.Update: %v", err)
	}
	if updated.Text() != "First comment, edited." {
		t.Errorf("Comments.Update text = %q", updated.Text())
	}

	for _, id := range []string{reply.ID, first.ID} {
		if err := client.Comments.Delete(ctx, id); err != nil {
			t.Fatalf("Comments.Delete(%s): %v", id, err)
		}
	}
	remaining, err := Collect(client.Comments.All(ctx, page.ID))
	if err != nil {
		t.Fatalf("Comments.All after delete: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("%d comments remain after delete", len(remaining))
	}
}

func TestIntegrationSearchVariants(t *testing.T) {
	client, _ := integrationClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	checkResult := func(t *testing.T, r QueryResult) {
		t.Helper()
		switch r.Object {
		case "page":
			if r.Page == nil || r.DataSource != nil || !r.Page.IsFull() {
				t.Errorf("page result = %+v", r)
			}
		case "data_source":
			if r.DataSource == nil || r.Page != nil || r.DataSource.ID == "" {
				t.Errorf("data_source result = %+v", r)
			}
		default:
			t.Errorf("result object = %q", r.Object)
		}
	}

	t.Run("SearchAll", func(t *testing.T) {
		n := 0
		for r, err := range client.SearchAll(ctx, SearchParams{Query: "notion-go", PageParams: PageParams{PageSize: 3}}) {
			if err != nil {
				t.Fatalf("SearchAll: %v", err)
			}
			checkResult(t, r)
			n++
		}
		t.Logf("SearchAll matched %d objects", n)
	})

	t.Run("SearchDataSources", func(t *testing.T) {
		res, err := client.Search(ctx, SearchParams{Filter: SearchDataSources(), PageParams: PageParams{PageSize: 10}})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if res.Object != "list" {
			t.Errorf("object = %q", res.Object)
		}
		for _, r := range res.Results {
			if r.Object != "data_source" {
				t.Errorf("result object = %q, want data_source", r.Object)
			}
			checkResult(t, r)
		}
	})

	t.Run("SearchPages", func(t *testing.T) {
		res, err := client.Search(ctx, SearchParams{Filter: SearchPages(), Sort: SortByRelevance(), Query: "notion-go", PageParams: PageParams{PageSize: 10}})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		for _, r := range res.Results {
			if r.Object != "page" || r.Page.InTrash {
				t.Errorf("result = %s in_trash=%v, want a live page", r.Object, r.Page != nil && r.Page.InTrash)
			}
			checkResult(t, r)
		}
	})

	t.Run("SearchInTrash", func(t *testing.T) {
		res, err := client.Search(ctx, SearchParams{Filter: SearchInTrash(true), PageParams: PageParams{PageSize: 10}})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		for _, r := range res.Results {
			checkResult(t, r)
			inTrash := (r.Page != nil && r.Page.InTrash) || (r.DataSource != nil && r.DataSource.InTrash)
			if !inTrash {
				t.Errorf("result %s is not in the trash", r.Object)
			}
		}
		t.Logf("in_trash search returned %d objects", len(res.Results))

		pagesOnly, err := client.Search(ctx, SearchParams{Filter: SearchPages().WithInTrash(true), PageParams: PageParams{PageSize: 5}})
		if err != nil {
			t.Fatalf("Search(pages in trash): %v", err)
		}
		for _, r := range pagesOnly.Results {
			if r.Object != "page" || r.Page == nil || !r.Page.InTrash {
				t.Errorf("pages-in-trash result = %+v", r)
			}
		}
	})

	t.Run("SortByLastEdited", func(t *testing.T) {
		editedAt := func(r QueryResult) time.Time {
			if r.Page != nil {
				return r.Page.LastEditedTime
			}
			return r.DataSource.LastEditedTime
		}
		for _, direction := range []SortDirection{Descending, Ascending} {
			res, err := client.Search(ctx, SearchParams{Sort: SortByLastEdited(direction), PageParams: PageParams{PageSize: 10}})
			if err != nil {
				t.Fatalf("Search(%s): %v", direction, err)
			}
			for i := 1; i < len(res.Results); i++ {
				checkResult(t, res.Results[i])
				prev, cur := editedAt(res.Results[i-1]), editedAt(res.Results[i])
				if direction == Descending && cur.After(prev) || direction == Ascending && cur.Before(prev) {
					t.Errorf("%s: result %d edited %v, result %d edited %v", direction, i-1, prev, i, cur)
				}
			}
		}
	})
}
