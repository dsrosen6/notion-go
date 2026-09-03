package notion

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBlockRegistryCoverage(t *testing.T) {
	// Every BlockType constant must have a registry entry. A missing one is
	// silent at runtime — the block decodes as UnknownBlock instead of its
	// real type — so it is caught here instead.
	declared := []BlockType{
		BlockTypeParagraph, BlockTypeHeading1, BlockTypeHeading2, BlockTypeHeading3,
		BlockTypeHeading4, BlockTypeBulletedListItem, BlockTypeNumberedListItem,
		BlockTypeQuote, BlockTypeToDo, BlockTypeToggle, BlockTypeTemplate,
		BlockTypeSyncedBlock, BlockTypeChildPage, BlockTypeChildDatabase,
		BlockTypeEquation, BlockTypeCode, BlockTypeCallout, BlockTypeDivider,
		BlockTypeBreadcrumb, BlockTypeTableOfContents, BlockTypeTab,
		BlockTypeColumnList, BlockTypeColumn, BlockTypeLinkToPage, BlockTypeTable,
		BlockTypeTableRow, BlockTypeMeetingNotes, BlockTypeTranscription,
		BlockTypeEmbed, BlockTypeBookmark, BlockTypeImage, BlockTypeVideo,
		BlockTypePDF, BlockTypeFile, BlockTypeAudio, BlockTypeLinkPreview,
		BlockTypeUnsupported,
	}

	if len(declared) != len(blockRegistry) {
		t.Errorf("%d block types declared but %d registered", len(declared), len(blockRegistry))
	}
	for _, blockType := range declared {
		newBlock, ok := blockRegistry[string(blockType)]
		if !ok {
			t.Errorf("block type %q has no registry entry", blockType)
			continue
		}
		// The variant must report the type it was registered under.
		if got := newBlock().BlockType(); got != blockType {
			t.Errorf("registry[%q] builds a block reporting %q", blockType, got)
		}
	}
}

func TestBlockFixtures(t *testing.T) {
	tests := []struct {
		fixture string
		want    BlockType
		check   func(*testing.T, Block)
	}{
		{
			fixture: "block_paragraph.json",
			want:    BlockTypeParagraph,
			check: func(t *testing.T, b Block) {
				p := b.(*ParagraphBlock)
				if got := p.Paragraph.RichText.PlainText(); got != "Some text" {
					t.Errorf("text = %q, want %q", got, "Some text")
				}
				if p.Paragraph.Color != ColorDefault {
					t.Errorf("Color = %q, want default", p.Paragraph.Color)
				}
				base := p.Base()
				if base.ID != "aaaaaaaa-1111-2222-3333-444444444444" {
					t.Errorf("ID = %q", base.ID)
				}
				if base.Parent.Type != ParentTypePage {
					t.Errorf("Parent.Type = %q, want page_id", base.Parent.Type)
				}
				want := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
				if !base.CreatedTime.Equal(want) {
					t.Errorf("CreatedTime = %v, want %v", base.CreatedTime, want)
				}
			},
		},
		{
			fixture: "block_todo.json",
			want:    BlockTypeToDo,
			check: func(t *testing.T, b Block) {
				todo := b.(*ToDoBlock)
				if !todo.ToDo.IsChecked() {
					t.Error("Checked = false, want true")
				}
				if got := todo.ToDo.RichText.PlainText(); got != "Ship it" {
					t.Errorf("text = %q", got)
				}
				// Responses report nesting through the flag, never inline.
				if !todo.Base().HasChildren {
					t.Error("HasChildren = false, want true")
				}
				if todo.ToDo.Children != nil {
					t.Errorf("Children = %v, want nil in a response", todo.ToDo.Children)
				}
			},
		},
		{
			fixture: "block_image.json",
			want:    BlockTypeImage,
			check: func(t *testing.T, b Block) {
				img := b.(*ImageBlock)
				if img.Image.Type != FileTypeFile {
					t.Errorf("Type = %q, want file", img.Image.Type)
				}
				if got := img.Image.URL(); got != "https://s3.example.com/signed.png" {
					t.Errorf("URL = %q", got)
				}
				if img.Image.File == nil || img.Image.File.ExpiryTime == nil {
					t.Error("expected a signed URL with an expiry")
				}
			},
		},
		{
			fixture: "block_code.json",
			want:    BlockTypeCode,
			check: func(t *testing.T, b Block) {
				code := b.(*CodeBlock)
				if code.Code.Language != "go" {
					t.Errorf("Language = %q, want go", code.Code.Language)
				}
				if got := code.Code.RichText.PlainText(); got != `fmt.Println("hi")` {
					t.Errorf("code = %q", got)
				}
			},
		},
		{
			fixture: "block_divider.json",
			want:    BlockTypeDivider,
			check: func(t *testing.T, b Block) {
				if _, ok := b.(*DividerBlock); !ok {
					t.Errorf("got %T, want *DividerBlock", b)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			data := readFixture(t, tt.fixture)
			got := assertRoundTrip(t, data, DecodeBlock)
			if got.BlockType() != tt.want {
				t.Fatalf("BlockType = %q, want %q", got.BlockType(), tt.want)
			}
			tt.check(t, got)
		})
	}
}

func TestUnknownBlockRoundTrips(t *testing.T) {
	// Notion adds block types continuously; an unrecognized one must survive.
	raw := []byte(`{"object":"block","id":"x1","has_children":false,` +
		`"type":"hologram","hologram":{"depth":3}}`)

	got, err := DecodeBlock(raw)
	if err != nil {
		t.Fatalf("DecodeBlock: %v", err)
	}
	unknown, ok := got.(*UnknownBlock)
	if !ok {
		t.Fatalf("got %T, want *UnknownBlock", got)
	}
	if unknown.Type != "hologram" {
		t.Errorf("Type = %q, want hologram", unknown.Type)
	}
	// The envelope must still be readable.
	if unknown.Base().ID != "x1" {
		t.Errorf("ID = %q, want x1", unknown.Base().ID)
	}
	encoded, err := json.Marshal(unknown)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(encoded) != string(raw) {
		t.Errorf("re-encoded %s, want the original", encoded)
	}
}

func TestBlockRequestOmitsEnvelope(t *testing.T) {
	// A locally built block must encode to exactly what the API accepts: no
	// id, no parent, no timestamps.
	encoded, err := json.Marshal(NewParagraph("hello"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"id", "object", "parent", "created_time", "last_edited_time", "created_by", "last_edited_by", "has_children", "in_trash"} {
		if _, present := fields[key]; present {
			t.Errorf("request carries %q, want it omitted: %s", key, encoded)
		}
	}
	if _, present := fields["type"]; !present {
		t.Errorf("request is missing type: %s", encoded)
	}
	if _, present := fields["paragraph"]; !present {
		t.Errorf("request is missing the payload: %s", encoded)
	}
}

func TestBlockListDecode(t *testing.T) {
	raw := []byte(`[
		{"object":"block","id":"1","type":"heading_1","heading_1":{"rich_text":[{"type":"text","text":{"content":"Title"},"plain_text":"Title","annotations":{"color":"default"}}],"color":"default"}},
		{"object":"block","id":"2","type":"divider","divider":{}},
		{"object":"block","id":"3","type":"paragraph","paragraph":{"rich_text":[{"type":"text","text":{"content":"Body"},"plain_text":"Body","annotations":{"color":"default"}}],"color":"default"}}
	]`)

	var list BlockList
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("got %d blocks, want 3", len(list))
	}
	if _, ok := list[0].(*Heading1Block); !ok {
		t.Errorf("list[0] = %T, want *Heading1Block", list[0])
	}
	if _, ok := list[1].(*DividerBlock); !ok {
		t.Errorf("list[1] = %T, want *DividerBlock", list[1])
	}
	// A divider has no text and must not contribute a blank line.
	if got := list.PlainText(); got != "Title\nBody" {
		t.Errorf("PlainText = %q, want %q", got, "Title\nBody")
	}
}

func TestValidateNesting(t *testing.T) {
	column := func(children ...Block) *ColumnBlock {
		return &ColumnBlock{Column: ColumnContent{Children: children}}
	}
	columnList := func(columns ...Block) *ColumnListBlock {
		return &ColumnListBlock{ColumnList: ColumnListContent{Children: columns}}
	}

	tests := []struct {
		name    string
		blocks  []Block
		wantErr error
	}{
		{
			name:   "flat",
			blocks: []Block{NewParagraph("a"), NewParagraph("b")},
		},
		{
			name:   "two levels",
			blocks: []Block{NewToggle("outer", NewParagraph("inner"))},
		},
		{
			name:   "three levels is the limit for ordinary blocks",
			blocks: []Block{NewToggle("a", NewToggle("b", NewParagraph("c")))},
		},
		{
			name:    "four levels is rejected",
			blocks:  []Block{NewToggle("a", NewToggle("b", NewToggle("c", NewParagraph("d"))))},
			wantErr: ErrNestingTooDeep,
		},
		{
			// ColumnListRequest -> ColumnWithChildrenRequest ->
			// BlockObjectWithSingleLevelOfChildrenRequest -> WithoutChildren.
			name:   "column list allows four levels",
			blocks: []Block{columnList(column(NewToggle("a", NewParagraph("b"))))},
		},
		{
			name:    "column list does not allow five levels",
			blocks:  []Block{columnList(column(NewToggle("a", NewToggle("b", NewParagraph("c")))))},
			wantErr: ErrNestingTooDeep,
		},
		{
			// Appending columns to an existing column_list block.
			name:   "column at the top level",
			blocks: []Block{column(NewToggle("a", NewParagraph("b")))},
		},
		{
			name:    "column list nested in a toggle is rejected",
			blocks:  []Block{NewToggle("a", columnList(column(NewParagraph("b"))))},
			wantErr: ErrInvalidNesting,
		},
		{
			name:    "column outside a column list is rejected",
			blocks:  []Block{NewToggle("a", column(NewParagraph("b")))},
			wantErr: ErrInvalidNesting,
		},
		{
			name:    "column list holding a non-column is rejected",
			blocks:  []Block{columnList(NewParagraph("a"))},
			wantErr: ErrInvalidNesting,
		},
		{
			name:   "tab items count toward depth",
			blocks: []Block{&TabBlock{Tab: TabContent{Children: BlockList{NewParagraph("a")}}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNesting(tt.blocks, 1)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("err = %v, want nil", err)
			}
		})
	}
}

func TestTabChildren(t *testing.T) {
	tab := &TabBlock{Tab: TabContent{Children: BlockList{NewParagraph("a"), NewParagraph("b")}}}
	if got := len(childrenOf(tab)); got != 2 {
		t.Errorf("childrenOf(tab) = %d blocks, want 2", got)
	}
	// A response carries "tab": {} and must still decode.
	got, err := DecodeBlock([]byte(`{"object":"block","id":"t1","type":"tab","tab":{}}`))
	if err != nil {
		t.Fatalf("DecodeBlock: %v", err)
	}
	if _, ok := got.(*TabBlock); !ok {
		t.Errorf("got %T, want *TabBlock", got)
	}
}

func TestUnsupportedBlockFixture(t *testing.T) {
	// The API names the real type under block_type (blocks.ts:767).
	got := assertRoundTrip(t, readFixture(t, "block_unsupported.json"), DecodeBlock)
	unsupported, ok := got.(*UnsupportedBlock)
	if !ok {
		t.Fatalf("got %T, want *UnsupportedBlock", got)
	}
	if unsupported.Unsupported.BlockType != "form" {
		t.Errorf("BlockType = %q, want form", unsupported.Unsupported.BlockType)
	}
}

func TestLinkToPageCommentRoundTrip(t *testing.T) {
	raw := []byte(`{"object":"block","id":"l1","type":"link_to_page","link_to_page":{"type":"comment_id","comment_id":"c1"}}`)
	got := assertRoundTrip(t, raw, DecodeBlock)
	link, ok := got.(*LinkToPageBlock)
	if !ok {
		t.Fatalf("got %T, want *LinkToPageBlock", got)
	}
	if link.LinkToPage.Type != "comment_id" || link.LinkToPage.CommentID != "c1" {
		t.Errorf("LinkToPage = %+v, want comment c1", link.LinkToPage)
	}
	encoded, _ := json.Marshal(link)
	if !strings.Contains(string(encoded), `"comment_id":"c1"`) {
		t.Errorf("encoded %s, want comment_id", encoded)
	}
}

func TestToDoCheckedIsOptional(t *testing.T) {
	// An update that only changes the text must not send checked:false, which
	// would uncheck the item (checked?: boolean, blocks.ts:997).
	textOnly := &ToDoBlock{ToDo: ToDoContent{BlockText: BlockText{RichText: NewRichText("x")}}}
	encoded, err := json.Marshal(textOnly)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(encoded), `"checked"`) {
		t.Errorf("text-only update carries checked: %s", encoded)
	}
	if textOnly.ToDo.IsChecked() {
		t.Error("IsChecked = true for an unset checkbox")
	}

	// The constructor sets it explicitly, false included.
	encoded, _ = json.Marshal(NewToDo("x", false))
	if !strings.Contains(string(encoded), `"checked":false`) {
		t.Errorf("NewToDo(false) = %s, want checked:false", encoded)
	}
	if !NewToDo("x", true).ToDo.IsChecked() {
		t.Error("NewToDo(true).IsChecked() = false")
	}
}

func TestTableContentOmitsUnsetFields(t *testing.T) {
	// An update may carry only the header flags (blocks.ts:1040); table_width
	// is create-only, and an absent flag must not reset the header.
	tests := []struct {
		name  string
		block Block
		want  string
	}{
		{"empty update", &TableBlock{}, `{"table":{},"type":"table"}`},
		{
			"header flag only",
			&TableBlock{Table: TableContent{HasColumnHeader: new(true)}},
			`{"table":{"has_column_header":true},"type":"table"}`,
		},
		{
			"turning a header off is sent",
			&TableBlock{Table: TableContent{HasRowHeader: new(false)}},
			`{"table":{"has_row_header":false},"type":"table"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(tt.block)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(encoded) != tt.want {
				t.Errorf("got  %s\nwant %s", encoded, tt.want)
			}
		})
	}
}

func TestZeroValueBlocksEncodeArrays(t *testing.T) {
	// The API requires rich_text and cells to be arrays; a nil slice must not
	// become null.
	tests := []struct {
		name  string
		block Block
		want  string
	}{
		{"paragraph", &ParagraphBlock{}, `{"paragraph":{"rich_text":[]},"type":"paragraph"}`},
		{"code", &CodeBlock{}, `{"code":{"rich_text":[]},"type":"code"}`},
		{"table row", &TableRowBlock{}, `{"table_row":{"cells":[]},"type":"table_row"}`},
		{
			"table row with an empty cell",
			&TableRowBlock{TableRow: TableRowContent{Cells: []RichTextList{nil}}},
			`{"table_row":{"cells":[[]]},"type":"table_row"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(tt.block)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(encoded) != tt.want {
				t.Errorf("got  %s\nwant %s", encoded, tt.want)
			}
			if strings.Contains(string(encoded), "null") {
				t.Errorf("encoded %s carries null", encoded)
			}
		})
	}
}

func TestBlockChildrenEncode(t *testing.T) {
	toggle := NewToggle("Details", NewParagraph("Hidden"), NewDivider())
	encoded, err := json.Marshal(toggle)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"children"`) {
		t.Errorf("encoded %s, want it to carry children", encoded)
	}

	back, err := DecodeBlock(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	children := childrenOf(back)
	if len(children) != 2 {
		t.Fatalf("got %d children, want 2", len(children))
	}
	if _, ok := children[1].(*DividerBlock); !ok {
		t.Errorf("children[1] = %T, want *DividerBlock", children[1])
	}
}
