package notion

import (
	"encoding/json"
	"strings"
	"time"
)

// BlockType identifies a block's kind.
type BlockType string

// The block types Notion supports.
const (
	BlockTypeParagraph        BlockType = "paragraph"
	BlockTypeHeading1         BlockType = "heading_1"
	BlockTypeHeading2         BlockType = "heading_2"
	BlockTypeHeading3         BlockType = "heading_3"
	BlockTypeHeading4         BlockType = "heading_4"
	BlockTypeBulletedListItem BlockType = "bulleted_list_item"
	BlockTypeNumberedListItem BlockType = "numbered_list_item"
	BlockTypeQuote            BlockType = "quote"
	BlockTypeToDo             BlockType = "to_do"
	BlockTypeToggle           BlockType = "toggle"
	BlockTypeTemplate         BlockType = "template"
	BlockTypeSyncedBlock      BlockType = "synced_block"
	BlockTypeChildPage        BlockType = "child_page"
	BlockTypeChildDatabase    BlockType = "child_database"
	BlockTypeEquation         BlockType = "equation"
	BlockTypeCode             BlockType = "code"
	BlockTypeCallout          BlockType = "callout"
	BlockTypeDivider          BlockType = "divider"
	BlockTypeBreadcrumb       BlockType = "breadcrumb"
	BlockTypeTableOfContents  BlockType = "table_of_contents"
	BlockTypeTab              BlockType = "tab"
	BlockTypeColumnList       BlockType = "column_list"
	BlockTypeColumn           BlockType = "column"
	BlockTypeLinkToPage       BlockType = "link_to_page"
	BlockTypeTable            BlockType = "table"
	BlockTypeTableRow         BlockType = "table_row"
	BlockTypeMeetingNotes     BlockType = "meeting_notes"
	BlockTypeTranscription    BlockType = "transcription"
	BlockTypeEmbed            BlockType = "embed"
	BlockTypeBookmark         BlockType = "bookmark"
	BlockTypeImage            BlockType = "image"
	BlockTypeVideo            BlockType = "video"
	BlockTypePDF              BlockType = "pdf"
	BlockTypeFile             BlockType = "file"
	BlockTypeAudio            BlockType = "audio"
	BlockTypeLinkPreview      BlockType = "link_preview"
	BlockTypeUnsupported      BlockType = "unsupported"
)

// Block is one piece of page content: a paragraph, a heading, an image, and so
// on. Every block type is a distinct struct implementing this interface, so
// blocks are consumed with a type switch:
//
//	for block, err := range client.Blocks.AllChildren(ctx, pageID) {
//		switch b := block.(type) {
//		case *notion.ParagraphBlock:
//			fmt.Println(b.Paragraph.RichText.PlainText())
//		case *notion.ImageBlock:
//			fmt.Println(b.Image.URL())
//		}
//	}
//
// The same types build requests. A block constructed in Go carries no envelope
// fields, so they are omitted when it is sent:
//
//	client.Blocks.AppendChildren(ctx, pageID, []notion.Block{
//		notion.NewHeading1("Overview"),
//		notion.NewParagraph("The details follow."),
//	})
//
// A block type this package does not recognize decodes as [*UnknownBlock]
// rather than failing.
type Block interface {
	// BlockType returns the block's kind.
	BlockType() BlockType
	// Base returns the envelope fields every block response carries. They are
	// zero for a block built locally for a request.
	Base() *BlockBase
}

// BlockBase holds the fields every block response carries. They are zero for a
// block constructed locally, and omitted when such a block is sent.
type BlockBase struct {
	Object string `json:"object,omitempty"`
	ID     string `json:"id,omitempty"`
	// Parent is the block's container. It is zero for a locally built block.
	Parent         Parent    `json:"parent,omitzero"`
	CreatedTime    time.Time `json:"created_time,omitzero"`
	LastEditedTime time.Time `json:"last_edited_time,omitzero"`
	// CreatedBy and LastEditedBy are always partial users carrying only an ID.
	CreatedBy    User `json:"created_by,omitzero"`
	LastEditedBy User `json:"last_edited_by,omitzero"`
	// HasChildren reports whether the block contains nested blocks. Responses
	// never inline them; read them with [BlocksService.Children].
	HasChildren bool `json:"has_children,omitempty"`
	InTrash     bool `json:"in_trash,omitempty"`
}

// Base implements [Block].
func (b *BlockBase) Base() *BlockBase { return b }

// BlockText is the payload shared by the blocks that are just styled text.
type BlockText struct {
	RichText RichTextList `json:"rich_text"`
	Color    Color        `json:"color,omitempty"`
	// Children nests blocks inside this one. It is set only on requests;
	// responses report nesting through [BlockBase.HasChildren] instead.
	Children BlockList `json:"children,omitempty"`
}

// ParagraphContent is the payload of a paragraph or callout block.
type ParagraphContent struct {
	BlockText
	Icon *Icon `json:"icon,omitempty"`
}

// HeadingContent is the payload of a heading block.
type HeadingContent struct {
	BlockText
	// IsToggleable makes the heading collapse its following content.
	IsToggleable bool `json:"is_toggleable,omitempty"`
}

// ToDoContent is the payload of a to-do block.
type ToDoContent struct {
	BlockText
	// Checked is nil when unset, so an update that only changes the text
	// leaves the checkbox alone: the request carries checked?: boolean
	// (common.ts:1922, blocks.ts:997). Responses always set it.
	Checked *bool `json:"checked,omitempty"`
}

// IsChecked reports whether the item is checked, false when Checked is unset.
func (c ToDoContent) IsChecked() bool { return c.Checked != nil && *c.Checked }

// NumberedListContent is the payload of a numbered list item.
type NumberedListContent struct {
	BlockText
	// ListStartIndex is the number this item counts from, when it restarts a
	// list.
	ListStartIndex int `json:"list_start_index,omitempty"`
	// ListFormat is the numbering style, such as "numbers" or "letters".
	ListFormat string `json:"list_format,omitempty"`
}

// CodeContent is the payload of a code block.
type CodeContent struct {
	RichText RichTextList `json:"rich_text"`
	Caption  RichTextList `json:"caption,omitempty"`
	// Language is the syntax highlighting language, such as "go".
	Language string `json:"language,omitempty"`
}

// URLContent is the payload of a bookmark, embed, or link preview block.
type URLContent struct {
	URL     string       `json:"url"`
	Caption RichTextList `json:"caption,omitempty"`
}

// TableContent is the payload of a table block. The rows are child table_row
// blocks.
type TableContent struct {
	// TableWidth is the number of columns. It is required when creating a
	// table and cannot be changed afterwards, so an update omits it
	// (TableRequestWithTableRowChildren, common.ts:2480-2485; the update
	// payload at blocks.ts:1040 has no table_width).
	TableWidth int `json:"table_width,omitempty"`
	// HasColumnHeader and HasRowHeader are nil when unset, so an update that
	// does not mention them leaves them unchanged.
	HasColumnHeader *bool `json:"has_column_header,omitempty"`
	HasRowHeader    *bool `json:"has_row_header,omitempty"`
	// Children holds the table's rows. It is required when creating a table
	// and set only on requests.
	Children BlockList `json:"children,omitempty"`
}

// TableRowContent is the payload of a table row: one rich text list per cell.
type TableRowContent struct {
	Cells []RichTextList `json:"cells"`
}

// MarshalJSON encodes nil Cells as [], since the request requires the array
// (ContentWithTableRowRequest, common.ts).
func (c TableRowContent) MarshalJSON() ([]byte, error) {
	type plain TableRowContent
	c.Cells = orEmpty(c.Cells)
	return json.Marshal(plain(c))
}

// ColumnContent is the payload of a column block.
type ColumnContent struct {
	// WidthRatio is this column's share of the column list's width, between 0
	// and 1. Zero means equal widths.
	WidthRatio float64   `json:"width_ratio,omitempty"`
	Children   BlockList `json:"children,omitempty"`
}

// ColumnListContent is the payload of a column list block.
type ColumnListContent struct {
	Children BlockList `json:"children,omitempty"`
}

// SyncedFrom identifies the block a synced block mirrors.
type SyncedFrom struct {
	Type    string `json:"type"`
	BlockID string `json:"block_id"`
}

// SyncedBlockContent is the payload of a synced block. SyncedFrom is nil on the
// original block and set on each copy.
type SyncedBlockContent struct {
	SyncedFrom *SyncedFrom `json:"synced_from"`
	Children   BlockList   `json:"children,omitempty"`
}

// LinkToPageContent is the payload of a link-to-page block. Read Type first:
// only the field it names is populated.
type LinkToPageContent struct {
	// Type is "page_id", "database_id", or "comment_id".
	Type       string `json:"type"`
	PageID     string `json:"page_id,omitempty"`
	DatabaseID string `json:"database_id,omitempty"`
	// CommentID links to a comment (common.ts:1824, 2111).
	CommentID string `json:"comment_id,omitempty"`
}

// TitleContent is the payload of a child page or child database block: the
// title of the object it stands for.
type TitleContent struct {
	Title string `json:"title"`
}

// ColorContent is the payload of a table of contents block.
type ColorContent struct {
	Color Color `json:"color,omitempty"`
}

// TranscriptionContent is the payload of a meeting notes or transcription
// block.
type TranscriptionContent struct {
	Title  RichTextList `json:"title,omitempty"`
	Status string       `json:"status,omitempty"`
	// Children, CalendarEvent, and Recording carry the block's structured
	// detail, which this package does not yet model.
	Children      json.RawMessage `json:"children,omitempty"`
	CalendarEvent json.RawMessage `json:"calendar_event,omitempty"`
	Recording     json.RawMessage `json:"recording,omitempty"`
}

// UnsupportedContent names a block type the Notion API does not expose,
// such as a form or a drive embed.
type UnsupportedContent struct {
	// BlockType is the underlying block type, such as "form" or "button"
	// (blocks.ts:764-768).
	BlockType string `json:"block_type,omitempty"`
}

// TabContent is the payload of a tab block. Responses carry nothing; a
// request may nest the tab's items, which are paragraph blocks
// (TabRequestWithTabItemChildren, common.ts:2704-2706).
type TabContent struct {
	Children BlockList `json:"children,omitempty"`
}

// The block variants follow. Each embeds [BlockBase] for the envelope and
// holds its payload in a field named for its type, mirroring the JSON.

// ParagraphBlock is a run of text, the default block type.
type ParagraphBlock struct {
	BlockBase
	Paragraph ParagraphContent `json:"paragraph"`
}

func (*ParagraphBlock) BlockType() BlockType { return BlockTypeParagraph }
func (p *ParagraphBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &p.BlockBase, &p.Paragraph)
}
func (p ParagraphBlock) MarshalJSON() ([]byte, error) {
	return wrap(p.BlockBase, "paragraph", p.Paragraph)
}

// Heading1Block is a top-level heading.
type Heading1Block struct {
	BlockBase
	Heading1 HeadingContent `json:"heading_1"`
}

func (*Heading1Block) BlockType() BlockType { return BlockTypeHeading1 }
func (h *Heading1Block) UnmarshalJSON(data []byte) error {
	return unwrap(data, &h.BlockBase, &h.Heading1)
}
func (h Heading1Block) MarshalJSON() ([]byte, error) {
	return wrap(h.BlockBase, "heading_1", h.Heading1)
}

// Heading2Block is a second-level heading.
type Heading2Block struct {
	BlockBase
	Heading2 HeadingContent `json:"heading_2"`
}

func (*Heading2Block) BlockType() BlockType { return BlockTypeHeading2 }
func (h *Heading2Block) UnmarshalJSON(data []byte) error {
	return unwrap(data, &h.BlockBase, &h.Heading2)
}
func (h Heading2Block) MarshalJSON() ([]byte, error) {
	return wrap(h.BlockBase, "heading_2", h.Heading2)
}

// Heading3Block is a third-level heading.
type Heading3Block struct {
	BlockBase
	Heading3 HeadingContent `json:"heading_3"`
}

func (*Heading3Block) BlockType() BlockType { return BlockTypeHeading3 }
func (h *Heading3Block) UnmarshalJSON(data []byte) error {
	return unwrap(data, &h.BlockBase, &h.Heading3)
}
func (h Heading3Block) MarshalJSON() ([]byte, error) {
	return wrap(h.BlockBase, "heading_3", h.Heading3)
}

// Heading4Block is a fourth-level heading.
type Heading4Block struct {
	BlockBase
	Heading4 HeadingContent `json:"heading_4"`
}

func (*Heading4Block) BlockType() BlockType { return BlockTypeHeading4 }
func (h *Heading4Block) UnmarshalJSON(data []byte) error {
	return unwrap(data, &h.BlockBase, &h.Heading4)
}
func (h Heading4Block) MarshalJSON() ([]byte, error) {
	return wrap(h.BlockBase, "heading_4", h.Heading4)
}

// BulletedListItemBlock is one item of a bulleted list.
type BulletedListItemBlock struct {
	BlockBase
	BulletedListItem BlockText `json:"bulleted_list_item"`
}

func (*BulletedListItemBlock) BlockType() BlockType { return BlockTypeBulletedListItem }
func (b *BulletedListItemBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &b.BlockBase, &b.BulletedListItem)
}
func (b BulletedListItemBlock) MarshalJSON() ([]byte, error) {
	return wrap(b.BlockBase, "bulleted_list_item", b.BulletedListItem)
}

// NumberedListItemBlock is one item of a numbered list.
type NumberedListItemBlock struct {
	BlockBase
	NumberedListItem NumberedListContent `json:"numbered_list_item"`
}

func (*NumberedListItemBlock) BlockType() BlockType { return BlockTypeNumberedListItem }
func (n *NumberedListItemBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &n.BlockBase, &n.NumberedListItem)
}
func (n NumberedListItemBlock) MarshalJSON() ([]byte, error) {
	return wrap(n.BlockBase, "numbered_list_item", n.NumberedListItem)
}

// QuoteBlock is a block quote.
type QuoteBlock struct {
	BlockBase
	Quote BlockText `json:"quote"`
}

func (*QuoteBlock) BlockType() BlockType { return BlockTypeQuote }
func (q *QuoteBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &q.BlockBase, &q.Quote)
}
func (q QuoteBlock) MarshalJSON() ([]byte, error) {
	return wrap(q.BlockBase, "quote", q.Quote)
}

// ToDoBlock is a checklist item.
type ToDoBlock struct {
	BlockBase
	ToDo ToDoContent `json:"to_do"`
}

func (*ToDoBlock) BlockType() BlockType { return BlockTypeToDo }
func (t *ToDoBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &t.BlockBase, &t.ToDo)
}
func (t ToDoBlock) MarshalJSON() ([]byte, error) {
	return wrap(t.BlockBase, "to_do", t.ToDo)
}

// ToggleBlock is a collapsible section.
type ToggleBlock struct {
	BlockBase
	Toggle BlockText `json:"toggle"`
}

func (*ToggleBlock) BlockType() BlockType { return BlockTypeToggle }
func (t *ToggleBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &t.BlockBase, &t.Toggle)
}
func (t ToggleBlock) MarshalJSON() ([]byte, error) {
	return wrap(t.BlockBase, "toggle", t.Toggle)
}

// TemplateBlock is a template button. Notion deprecated it in March 2023; existing ones still render.
type TemplateBlock struct {
	BlockBase
	Template BlockText `json:"template"`
}

func (*TemplateBlock) BlockType() BlockType { return BlockTypeTemplate }
func (t *TemplateBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &t.BlockBase, &t.Template)
}
func (t TemplateBlock) MarshalJSON() ([]byte, error) {
	return wrap(t.BlockBase, "template", t.Template)
}

// SyncedBlock is content mirrored between locations.
type SyncedBlock struct {
	BlockBase
	SyncedBlock SyncedBlockContent `json:"synced_block"`
}

func (*SyncedBlock) BlockType() BlockType { return BlockTypeSyncedBlock }
func (s *SyncedBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.BlockBase, &s.SyncedBlock)
}
func (s SyncedBlock) MarshalJSON() ([]byte, error) {
	return wrap(s.BlockBase, "synced_block", s.SyncedBlock)
}

// ChildPageBlock is a page nested inside this one.
type ChildPageBlock struct {
	BlockBase
	ChildPage TitleContent `json:"child_page"`
}

func (*ChildPageBlock) BlockType() BlockType { return BlockTypeChildPage }
func (c *ChildPageBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &c.BlockBase, &c.ChildPage)
}
func (c ChildPageBlock) MarshalJSON() ([]byte, error) {
	return wrap(c.BlockBase, "child_page", c.ChildPage)
}

// ChildDatabaseBlock is a database nested inside this page.
type ChildDatabaseBlock struct {
	BlockBase
	ChildDatabase TitleContent `json:"child_database"`
}

func (*ChildDatabaseBlock) BlockType() BlockType { return BlockTypeChildDatabase }
func (c *ChildDatabaseBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &c.BlockBase, &c.ChildDatabase)
}
func (c ChildDatabaseBlock) MarshalJSON() ([]byte, error) {
	return wrap(c.BlockBase, "child_database", c.ChildDatabase)
}

// EquationBlock is a standalone KaTeX equation.
type EquationBlock struct {
	BlockBase
	Equation EquationContent `json:"equation"`
}

func (*EquationBlock) BlockType() BlockType { return BlockTypeEquation }
func (e *EquationBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &e.BlockBase, &e.Equation)
}
func (e EquationBlock) MarshalJSON() ([]byte, error) {
	return wrap(e.BlockBase, "equation", e.Equation)
}

// CodeBlock is a syntax-highlighted code listing.
type CodeBlock struct {
	BlockBase
	Code CodeContent `json:"code"`
}

func (*CodeBlock) BlockType() BlockType { return BlockTypeCode }
func (c *CodeBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &c.BlockBase, &c.Code)
}
func (c CodeBlock) MarshalJSON() ([]byte, error) {
	return wrap(c.BlockBase, "code", c.Code)
}

// CalloutBlock is highlighted text with an icon.
type CalloutBlock struct {
	BlockBase
	Callout ParagraphContent `json:"callout"`
}

func (*CalloutBlock) BlockType() BlockType { return BlockTypeCallout }
func (c *CalloutBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &c.BlockBase, &c.Callout)
}
func (c CalloutBlock) MarshalJSON() ([]byte, error) {
	return wrap(c.BlockBase, "callout", c.Callout)
}

// DividerBlock is a horizontal rule.
type DividerBlock struct {
	BlockBase
	Divider EmptyObject `json:"divider"`
}

func (*DividerBlock) BlockType() BlockType { return BlockTypeDivider }
func (d *DividerBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &d.BlockBase, &d.Divider)
}
func (d DividerBlock) MarshalJSON() ([]byte, error) {
	return wrap(d.BlockBase, "divider", d.Divider)
}

// BreadcrumbBlock is a trail of the page's ancestors.
type BreadcrumbBlock struct {
	BlockBase
	Breadcrumb EmptyObject `json:"breadcrumb"`
}

func (*BreadcrumbBlock) BlockType() BlockType { return BlockTypeBreadcrumb }
func (b *BreadcrumbBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &b.BlockBase, &b.Breadcrumb)
}
func (b BreadcrumbBlock) MarshalJSON() ([]byte, error) {
	return wrap(b.BlockBase, "breadcrumb", b.Breadcrumb)
}

// TableOfContentsBlock is an index built from the page's headings.
type TableOfContentsBlock struct {
	BlockBase
	TableOfContents ColorContent `json:"table_of_contents"`
}

func (*TableOfContentsBlock) BlockType() BlockType { return BlockTypeTableOfContents }
func (t *TableOfContentsBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &t.BlockBase, &t.TableOfContents)
}
func (t TableOfContentsBlock) MarshalJSON() ([]byte, error) {
	return wrap(t.BlockBase, "table_of_contents", t.TableOfContents)
}

// TabBlock is one tab of a tabbed section.
type TabBlock struct {
	BlockBase
	Tab TabContent `json:"tab"`
}

func (*TabBlock) BlockType() BlockType { return BlockTypeTab }
func (t *TabBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &t.BlockBase, &t.Tab)
}
func (t TabBlock) MarshalJSON() ([]byte, error) {
	return wrap(t.BlockBase, "tab", t.Tab)
}

// ColumnListBlock is a row of columns. Its children are all column blocks.
type ColumnListBlock struct {
	BlockBase
	ColumnList ColumnListContent `json:"column_list"`
}

func (*ColumnListBlock) BlockType() BlockType { return BlockTypeColumnList }
func (c *ColumnListBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &c.BlockBase, &c.ColumnList)
}
func (c ColumnListBlock) MarshalJSON() ([]byte, error) {
	return wrap(c.BlockBase, "column_list", c.ColumnList)
}

// ColumnBlock is one column within a column list.
type ColumnBlock struct {
	BlockBase
	Column ColumnContent `json:"column"`
}

func (*ColumnBlock) BlockType() BlockType { return BlockTypeColumn }
func (c *ColumnBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &c.BlockBase, &c.Column)
}
func (c ColumnBlock) MarshalJSON() ([]byte, error) {
	return wrap(c.BlockBase, "column", c.Column)
}

// LinkToPageBlock is a link rendered as a page reference.
type LinkToPageBlock struct {
	BlockBase
	LinkToPage LinkToPageContent `json:"link_to_page"`
}

func (*LinkToPageBlock) BlockType() BlockType { return BlockTypeLinkToPage }
func (l *LinkToPageBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &l.BlockBase, &l.LinkToPage)
}
func (l LinkToPageBlock) MarshalJSON() ([]byte, error) {
	return wrap(l.BlockBase, "link_to_page", l.LinkToPage)
}

// TableBlock is a simple table. Its children are the rows.
type TableBlock struct {
	BlockBase
	Table TableContent `json:"table"`
}

func (*TableBlock) BlockType() BlockType { return BlockTypeTable }
func (t *TableBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &t.BlockBase, &t.Table)
}
func (t TableBlock) MarshalJSON() ([]byte, error) {
	return wrap(t.BlockBase, "table", t.Table)
}

// TableRowBlock is one row of a table.
type TableRowBlock struct {
	BlockBase
	TableRow TableRowContent `json:"table_row"`
}

func (*TableRowBlock) BlockType() BlockType { return BlockTypeTableRow }
func (t *TableRowBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &t.BlockBase, &t.TableRow)
}
func (t TableRowBlock) MarshalJSON() ([]byte, error) {
	return wrap(t.BlockBase, "table_row", t.TableRow)
}

// MeetingNotesBlock is AI meeting notes with their transcript.
type MeetingNotesBlock struct {
	BlockBase
	MeetingNotes TranscriptionContent `json:"meeting_notes"`
}

func (*MeetingNotesBlock) BlockType() BlockType { return BlockTypeMeetingNotes }
func (m *MeetingNotesBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &m.BlockBase, &m.MeetingNotes)
}
func (m MeetingNotesBlock) MarshalJSON() ([]byte, error) {
	return wrap(m.BlockBase, "meeting_notes", m.MeetingNotes)
}

// TranscriptionBlock is a recording transcript.
type TranscriptionBlock struct {
	BlockBase
	Transcription TranscriptionContent `json:"transcription"`
}

func (*TranscriptionBlock) BlockType() BlockType { return BlockTypeTranscription }
func (t *TranscriptionBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &t.BlockBase, &t.Transcription)
}
func (t TranscriptionBlock) MarshalJSON() ([]byte, error) {
	return wrap(t.BlockBase, "transcription", t.Transcription)
}

// EmbedBlock is third-party content rendered inline.
type EmbedBlock struct {
	BlockBase
	Embed URLContent `json:"embed"`
}

func (*EmbedBlock) BlockType() BlockType { return BlockTypeEmbed }
func (e *EmbedBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &e.BlockBase, &e.Embed)
}
func (e EmbedBlock) MarshalJSON() ([]byte, error) {
	return wrap(e.BlockBase, "embed", e.Embed)
}

// BookmarkBlock is a link rendered as a visual card.
type BookmarkBlock struct {
	BlockBase
	Bookmark URLContent `json:"bookmark"`
}

func (*BookmarkBlock) BlockType() BlockType { return BlockTypeBookmark }
func (b *BookmarkBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &b.BlockBase, &b.Bookmark)
}
func (b BookmarkBlock) MarshalJSON() ([]byte, error) {
	return wrap(b.BlockBase, "bookmark", b.Bookmark)
}

// ImageBlock is an image.
type ImageBlock struct {
	BlockBase
	Image File `json:"image"`
}

func (*ImageBlock) BlockType() BlockType { return BlockTypeImage }
func (i *ImageBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &i.BlockBase, &i.Image)
}
func (i ImageBlock) MarshalJSON() ([]byte, error) {
	return wrap(i.BlockBase, "image", i.Image)
}

// VideoBlock is a video.
type VideoBlock struct {
	BlockBase
	Video File `json:"video"`
}

func (*VideoBlock) BlockType() BlockType { return BlockTypeVideo }
func (v *VideoBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &v.BlockBase, &v.Video)
}
func (v VideoBlock) MarshalJSON() ([]byte, error) {
	return wrap(v.BlockBase, "video", v.Video)
}

// PDFBlock is an embedded PDF.
type PDFBlock struct {
	BlockBase
	PDF File `json:"pdf"`
}

func (*PDFBlock) BlockType() BlockType { return BlockTypePDF }
func (p *PDFBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &p.BlockBase, &p.PDF)
}
func (p PDFBlock) MarshalJSON() ([]byte, error) {
	return wrap(p.BlockBase, "pdf", p.PDF)
}

// FileBlock is an attached file.
type FileBlock struct {
	BlockBase
	File File `json:"file"`
}

func (*FileBlock) BlockType() BlockType { return BlockTypeFile }
func (f *FileBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &f.BlockBase, &f.File)
}
func (f FileBlock) MarshalJSON() ([]byte, error) {
	return wrap(f.BlockBase, "file", f.File)
}

// AudioBlock is an audio file.
type AudioBlock struct {
	BlockBase
	Audio File `json:"audio"`
}

func (*AudioBlock) BlockType() BlockType { return BlockTypeAudio }
func (a *AudioBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &a.BlockBase, &a.Audio)
}
func (a AudioBlock) MarshalJSON() ([]byte, error) {
	return wrap(a.BlockBase, "audio", a.Audio)
}

// LinkPreviewBlock is a rich preview of a link. Notion creates these itself; they cannot be created through the API.
type LinkPreviewBlock struct {
	BlockBase
	LinkPreview URLContent `json:"link_preview"`
}

func (*LinkPreviewBlock) BlockType() BlockType { return BlockTypeLinkPreview }
func (l *LinkPreviewBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &l.BlockBase, &l.LinkPreview)
}
func (l LinkPreviewBlock) MarshalJSON() ([]byte, error) {
	return wrap(l.BlockBase, "link_preview", l.LinkPreview)
}

// UnsupportedBlock is a block the API does not expose, such as a form or a drive embed.
type UnsupportedBlock struct {
	BlockBase
	Unsupported UnsupportedContent `json:"unsupported"`
}

func (*UnsupportedBlock) BlockType() BlockType { return BlockTypeUnsupported }
func (u *UnsupportedBlock) UnmarshalJSON(data []byte) error {
	return unwrap(data, &u.BlockBase, &u.Unsupported)
}
func (u UnsupportedBlock) MarshalJSON() ([]byte, error) {
	return wrap(u.BlockBase, "unsupported", u.Unsupported)
}

// UnknownBlock is a block type this package does not recognize. It retains the
// raw JSON and re-encodes it unchanged, so a page using a newer Notion feature
// survives a decode and encode round trip.
type UnknownBlock struct {
	BlockBase
	// Type is the unrecognized discriminant.
	Type BlockType
	// Raw is the block's original JSON.
	Raw json.RawMessage
}

func (u *UnknownBlock) BlockType() BlockType        { return u.Type }
func (u UnknownBlock) MarshalJSON() ([]byte, error) { return u.Raw, nil }

// blockRegistry maps each discriminant to its variant. A missing entry means
// the block decodes as [*UnknownBlock], so keep it in step with the BlockType
// constants; TestBlockRegistryCoverage enforces that.
var blockRegistry = map[string]func() Block{
	string(BlockTypeParagraph):        func() Block { return new(ParagraphBlock) },
	string(BlockTypeHeading1):         func() Block { return new(Heading1Block) },
	string(BlockTypeHeading2):         func() Block { return new(Heading2Block) },
	string(BlockTypeHeading3):         func() Block { return new(Heading3Block) },
	string(BlockTypeHeading4):         func() Block { return new(Heading4Block) },
	string(BlockTypeBulletedListItem): func() Block { return new(BulletedListItemBlock) },
	string(BlockTypeNumberedListItem): func() Block { return new(NumberedListItemBlock) },
	string(BlockTypeQuote):            func() Block { return new(QuoteBlock) },
	string(BlockTypeToDo):             func() Block { return new(ToDoBlock) },
	string(BlockTypeToggle):           func() Block { return new(ToggleBlock) },
	string(BlockTypeTemplate):         func() Block { return new(TemplateBlock) },
	string(BlockTypeSyncedBlock):      func() Block { return new(SyncedBlock) },
	string(BlockTypeChildPage):        func() Block { return new(ChildPageBlock) },
	string(BlockTypeChildDatabase):    func() Block { return new(ChildDatabaseBlock) },
	string(BlockTypeEquation):         func() Block { return new(EquationBlock) },
	string(BlockTypeCode):             func() Block { return new(CodeBlock) },
	string(BlockTypeCallout):          func() Block { return new(CalloutBlock) },
	string(BlockTypeDivider):          func() Block { return new(DividerBlock) },
	string(BlockTypeBreadcrumb):       func() Block { return new(BreadcrumbBlock) },
	string(BlockTypeTableOfContents):  func() Block { return new(TableOfContentsBlock) },
	string(BlockTypeTab):              func() Block { return new(TabBlock) },
	string(BlockTypeColumnList):       func() Block { return new(ColumnListBlock) },
	string(BlockTypeColumn):           func() Block { return new(ColumnBlock) },
	string(BlockTypeLinkToPage):       func() Block { return new(LinkToPageBlock) },
	string(BlockTypeTable):            func() Block { return new(TableBlock) },
	string(BlockTypeTableRow):         func() Block { return new(TableRowBlock) },
	string(BlockTypeMeetingNotes):     func() Block { return new(MeetingNotesBlock) },
	string(BlockTypeTranscription):    func() Block { return new(TranscriptionBlock) },
	string(BlockTypeEmbed):            func() Block { return new(EmbedBlock) },
	string(BlockTypeBookmark):         func() Block { return new(BookmarkBlock) },
	string(BlockTypeImage):            func() Block { return new(ImageBlock) },
	string(BlockTypeVideo):            func() Block { return new(VideoBlock) },
	string(BlockTypePDF):              func() Block { return new(PDFBlock) },
	string(BlockTypeFile):             func() Block { return new(FileBlock) },
	string(BlockTypeAudio):            func() Block { return new(AudioBlock) },
	string(BlockTypeLinkPreview):      func() Block { return new(LinkPreviewBlock) },
	string(BlockTypeUnsupported):      func() Block { return new(UnsupportedBlock) },
}

func unknownBlock(tag string, raw []byte) Block {
	u := &UnknownBlock{Type: BlockType(tag), Raw: raw}
	// Best effort: the envelope is present whatever the variant.
	_ = json.Unmarshal(raw, &u.BlockBase)
	return u
}

// DecodeBlock decodes a single block. An unrecognized type yields an
// [*UnknownBlock] rather than an error.
func DecodeBlock(data []byte) (Block, error) {
	return decodeUnion(data, blockRegistry, unknownBlock)
}

// BlockList is a sequence of blocks. Interface-typed slices cannot decode
// themselves, so every list of blocks uses this named type.
type BlockList []Block

func (l *BlockList) UnmarshalJSON(data []byte) error {
	items, err := decodeUnionSlice(data, blockRegistry, unknownBlock)
	if err != nil {
		return err
	}
	*l = items
	return nil
}

// PlainText returns the text of every block in the list that has any,
// separated by newlines. Nested children are not included.
func (l BlockList) PlainText() string {
	var lines []string
	for _, block := range l {
		if text := blockText(block); text != "" {
			lines = append(lines, text)
		}
	}
	return strings.Join(lines, "\n")
}

// blockText returns a block's rich text, or the empty string for block types
// that hold none.
func blockText(block Block) string {
	switch b := block.(type) {
	case *ParagraphBlock:
		return b.Paragraph.RichText.PlainText()
	case *Heading1Block:
		return b.Heading1.RichText.PlainText()
	case *Heading2Block:
		return b.Heading2.RichText.PlainText()
	case *Heading3Block:
		return b.Heading3.RichText.PlainText()
	case *Heading4Block:
		return b.Heading4.RichText.PlainText()
	case *BulletedListItemBlock:
		return b.BulletedListItem.RichText.PlainText()
	case *NumberedListItemBlock:
		return b.NumberedListItem.RichText.PlainText()
	case *QuoteBlock:
		return b.Quote.RichText.PlainText()
	case *ToDoBlock:
		return b.ToDo.RichText.PlainText()
	case *ToggleBlock:
		return b.Toggle.RichText.PlainText()
	case *CalloutBlock:
		return b.Callout.RichText.PlainText()
	case *CodeBlock:
		return b.Code.RichText.PlainText()
	default:
		return ""
	}
}

// Block constructors, for building requests. Each returns a block with no
// envelope fields, so it encodes to exactly what the API expects to receive.

// NewParagraph returns a paragraph holding the given text.
func NewParagraph(text string) *ParagraphBlock {
	return &ParagraphBlock{Paragraph: ParagraphContent{BlockText: BlockText{RichText: NewRichText(text)}}}
}

// NewHeading1 returns a top-level heading.
func NewHeading1(text string) *Heading1Block {
	return &Heading1Block{Heading1: HeadingContent{BlockText: BlockText{RichText: NewRichText(text)}}}
}

// NewHeading2 returns a second-level heading.
func NewHeading2(text string) *Heading2Block {
	return &Heading2Block{Heading2: HeadingContent{BlockText: BlockText{RichText: NewRichText(text)}}}
}

// NewHeading3 returns a third-level heading.
func NewHeading3(text string) *Heading3Block {
	return &Heading3Block{Heading3: HeadingContent{BlockText: BlockText{RichText: NewRichText(text)}}}
}

// NewBulletedListItem returns one item of a bulleted list.
func NewBulletedListItem(text string) *BulletedListItemBlock {
	return &BulletedListItemBlock{BulletedListItem: BlockText{RichText: NewRichText(text)}}
}

// NewNumberedListItem returns one item of a numbered list.
func NewNumberedListItem(text string) *NumberedListItemBlock {
	return &NumberedListItemBlock{NumberedListItem: NumberedListContent{BlockText: BlockText{RichText: NewRichText(text)}}}
}

// NewToDo returns a checklist item.
func NewToDo(text string, checked bool) *ToDoBlock {
	return &ToDoBlock{ToDo: ToDoContent{BlockText: BlockText{RichText: NewRichText(text)}, Checked: new(checked)}}
}

// NewQuote returns a block quote.
func NewQuote(text string) *QuoteBlock {
	return &QuoteBlock{Quote: BlockText{RichText: NewRichText(text)}}
}

// NewCallout returns highlighted text with an icon. A nil icon uses Notion's
// default.
func NewCallout(text string, icon *Icon) *CalloutBlock {
	return &CalloutBlock{Callout: ParagraphContent{
		BlockText: BlockText{RichText: NewRichText(text)},
		Icon:      icon,
	}}
}

// NewCode returns a code listing. The language is a Notion language name such
// as "go" or "typescript"; an unrecognized one is rejected by the API.
func NewCode(code, language string) *CodeBlock {
	return &CodeBlock{Code: CodeContent{RichText: NewRichText(code), Language: language}}
}

// NewDivider returns a horizontal rule.
func NewDivider() *DividerBlock { return &DividerBlock{} }

// NewBookmark returns a link rendered as a visual card.
func NewBookmark(url string) *BookmarkBlock {
	return &BookmarkBlock{Bookmark: URLContent{URL: url}}
}

// NewImage returns an image block referencing an external URL. Notion fetches
// the image, so the URL must be publicly reachable.
func NewImage(url string) *ImageBlock {
	return &ImageBlock{Image: NewExternalFile(url)}
}

// NewToggle returns a collapsible section containing the given blocks.
func NewToggle(text string, children ...Block) *ToggleBlock {
	return &ToggleBlock{Toggle: BlockText{RichText: NewRichText(text), Children: children}}
}
