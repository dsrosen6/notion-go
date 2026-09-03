package notion

import (
	"encoding/json"
	"strings"
)

// RichTextType identifies a rich text variant.
type RichTextType string

const (
	// RichTextTypeText is literal text, optionally hyperlinked.
	RichTextTypeText RichTextType = "text"
	// RichTextTypeMention is an inline reference to a user, page, date, or
	// similar, created in the UI by typing "@".
	RichTextTypeMention RichTextType = "mention"
	// RichTextTypeEquation is an inline KaTeX equation.
	RichTextTypeEquation RichTextType = "equation"
)

// RichText is a styled run of text: one of [*Text], [*Mention], [*Equation], or
// [*UnknownRichText] for a variant this package does not recognize.
//
// Consume it with a type switch, or call [RichTextList.PlainText] when only the
// unstyled text matters.
//
//	switch rt := item.(type) {
//	case *notion.Text:
//		fmt.Println(rt.Text.Content)
//	case *notion.Mention:
//		fmt.Println(rt.Mention.Type)
//	}
type RichText interface {
	// RichTextType returns the variant's discriminant.
	RichTextType() RichTextType
	// Common returns the fields every rich text run carries.
	Common() *RichTextCommon
}

// RichTextCommon holds the fields shared by every rich text variant.
type RichTextCommon struct {
	// PlainText is the run's text with no styling applied.
	PlainText string `json:"plain_text,omitempty"`
	// Href is the URL this run links to or mentions, empty when it links
	// nowhere.
	Href string `json:"href,omitempty"`
	// Annotations is the styling applied to the run.
	Annotations Annotations `json:"annotations"`
}

// Common implements part of [RichText].
func (c *RichTextCommon) Common() *RichTextCommon { return c }

// TextContent is the payload of a [Text] run.
type TextContent struct {
	// Content is the literal text.
	Content string `json:"content"`
	// Link is the inline hyperlink, nil when the run is not a link.
	Link *Link `json:"link,omitempty"`
}

// Link is an inline hyperlink.
type Link struct {
	URL string `json:"url"`
}

// Text is a run of literal text, optionally hyperlinked.
type Text struct {
	RichTextCommon
	Text TextContent `json:"text"`
}

// RichTextType implements [RichText].
func (*Text) RichTextType() RichTextType { return RichTextTypeText }

func (t *Text) UnmarshalJSON(data []byte) error {
	return unwrap(data, &t.RichTextCommon, &t.Text)
}

func (t Text) MarshalJSON() ([]byte, error) {
	return wrap(t.RichTextCommon, string(RichTextTypeText), t.Text)
}

// EquationContent is the payload of an [Equation] run.
type EquationContent struct {
	// Expression is a KaTeX-compatible string.
	Expression string `json:"expression"`
}

// Equation is an inline KaTeX equation.
type Equation struct {
	RichTextCommon
	Equation EquationContent `json:"equation"`
}

// RichTextType implements [RichText].
func (*Equation) RichTextType() RichTextType { return RichTextTypeEquation }

func (e *Equation) UnmarshalJSON(data []byte) error {
	return unwrap(data, &e.RichTextCommon, &e.Equation)
}

func (e Equation) MarshalJSON() ([]byte, error) {
	return wrap(e.RichTextCommon, string(RichTextTypeEquation), e.Equation)
}

// MentionType identifies what a [Mention] refers to.
type MentionType string

const (
	MentionTypeUser            MentionType = "user"
	MentionTypeDate            MentionType = "date"
	MentionTypePage            MentionType = "page"
	MentionTypeDatabase        MentionType = "database"
	MentionTypeLinkPreview     MentionType = "link_preview"
	MentionTypeLinkMention     MentionType = "link_mention"
	MentionTypeTemplateMention MentionType = "template_mention"
	MentionTypeCustomEmoji     MentionType = "custom_emoji"
)

// LinkPreview is the payload of a link preview mention.
type LinkPreview struct {
	URL string `json:"url"`
}

// LinkMention is a rendered preview of an external link, with whatever metadata
// Notion could resolve.
type LinkMention struct {
	Href         string `json:"href"`
	Title        string `json:"title,omitempty"`
	Description  string `json:"description,omitempty"`
	LinkAuthor   string `json:"link_author,omitempty"`
	LinkProvider string `json:"link_provider,omitempty"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	IconURL      string `json:"icon_url,omitempty"`
	IframeURL    string `json:"iframe_url,omitempty"`
	Height       int    `json:"height,omitempty"`
	Padding      int    `json:"padding,omitempty"`
	PaddingTop   int    `json:"padding_top,omitempty"`
}

// TemplateMention is a placeholder that resolves when a template is
// instantiated: the current date, or the user instantiating it.
type TemplateMention struct {
	Type string `json:"type"`
	// TemplateMentionDate is "today" or "now".
	TemplateMentionDate string `json:"template_mention_date,omitempty"`
	// TemplateMentionUser is always "me".
	TemplateMentionUser string `json:"template_mention_user,omitempty"`
}

// MentionContent is what a mention points at. Read Type first: only the field
// it names is populated.
//
// Unlike the larger unions this is a flat struct. It has small payloads and no
// shared envelope, so an interface would cost a codec and buy nothing.
type MentionContent struct {
	Type MentionType `json:"type"`
	// User is set for a user mention. It may be a partial user; check
	// [User.IsFull].
	User            *User            `json:"user,omitempty"`
	Date            *Date            `json:"date,omitempty"`
	Page            *IDRef           `json:"page,omitempty"`
	Database        *IDRef           `json:"database,omitempty"`
	LinkPreview     *LinkPreview     `json:"link_preview,omitempty"`
	LinkMention     *LinkMention     `json:"link_mention,omitempty"`
	TemplateMention *TemplateMention `json:"template_mention,omitempty"`
	CustomEmoji     *CustomEmoji     `json:"custom_emoji,omitempty"`
}

// Mention is an inline reference to another object.
type Mention struct {
	RichTextCommon
	Mention MentionContent `json:"mention"`
}

// RichTextType implements [RichText].
func (*Mention) RichTextType() RichTextType { return RichTextTypeMention }

func (m *Mention) UnmarshalJSON(data []byte) error {
	return unwrap(data, &m.RichTextCommon, &m.Mention)
}

func (m Mention) MarshalJSON() ([]byte, error) {
	return wrap(m.RichTextCommon, string(RichTextTypeMention), m.Mention)
}

// UnknownRichText is a rich text variant this package does not recognize. It
// retains the raw JSON and re-encodes it unchanged, so a newer Notion feature
// survives a decode and encode round trip.
type UnknownRichText struct {
	RichTextCommon
	// Type is the unrecognized discriminant.
	Type RichTextType
	// Raw is the variant's original JSON.
	Raw json.RawMessage
}

// RichTextType implements [RichText].
func (u *UnknownRichText) RichTextType() RichTextType { return u.Type }

func (u UnknownRichText) MarshalJSON() ([]byte, error) { return u.Raw, nil }

var richTextRegistry = map[string]func() RichText{
	string(RichTextTypeText):     func() RichText { return new(Text) },
	string(RichTextTypeMention):  func() RichText { return new(Mention) },
	string(RichTextTypeEquation): func() RichText { return new(Equation) },
}

func unknownRichText(tag string, raw []byte) RichText {
	u := &UnknownRichText{Type: RichTextType(tag), Raw: raw}
	// Best effort: the common fields are present regardless of the variant.
	_ = json.Unmarshal(raw, &u.RichTextCommon)
	return u
}

// DecodeRichText decodes a single rich text value. An unrecognized variant
// yields an [*UnknownRichText] rather than an error.
func DecodeRichText(data []byte) (RichText, error) {
	return decodeUnion(data, richTextRegistry, unknownRichText)
}

// RichTextList is a sequence of rich text runs. Notion splits text at every
// styling change, so a single visible sentence is often several runs.
type RichTextList []RichText

func (l *RichTextList) UnmarshalJSON(data []byte) error {
	items, err := decodeUnionSlice(data, richTextRegistry, unknownRichText)
	if err != nil {
		return err
	}
	*l = items
	return nil
}

// MarshalJSON encodes a nil list as [] rather than null, since every request
// field holding rich text requires an array.
func (l RichTextList) MarshalJSON() ([]byte, error) {
	return json.Marshal(orEmpty([]RichText(l)))
}

// PlainText returns the list's text with all styling removed, which is usually
// what you want when reading a title or a text property.
func (l RichTextList) PlainText() string {
	var b strings.Builder
	for _, item := range l {
		b.WriteString(item.Common().PlainText)
	}
	return b.String()
}

// NewRichText returns a single unstyled run of text, for building requests.
func NewRichText(content string) RichTextList {
	return RichTextList{&Text{
		RichTextCommon: RichTextCommon{PlainText: content},
		Text:           TextContent{Content: content},
	}}
}
