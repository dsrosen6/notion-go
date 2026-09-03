package notion

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Color is a text or background color. Notion applies it to rich text
// annotations and to whole blocks.
type Color string

// The colors Notion supports. Values ending in Background color the background
// rather than the text.
const (
	ColorDefault           Color = "default"
	ColorGray              Color = "gray"
	ColorBrown             Color = "brown"
	ColorOrange            Color = "orange"
	ColorYellow            Color = "yellow"
	ColorGreen             Color = "green"
	ColorBlue              Color = "blue"
	ColorPurple            Color = "purple"
	ColorPink              Color = "pink"
	ColorRed               Color = "red"
	ColorDefaultBackground Color = "default_background"
	ColorGrayBackground    Color = "gray_background"
	ColorBrownBackground   Color = "brown_background"
	ColorOrangeBackground  Color = "orange_background"
	ColorYellowBackground  Color = "yellow_background"
	ColorGreenBackground   Color = "green_background"
	ColorBlueBackground    Color = "blue_background"
	ColorPurpleBackground  Color = "purple_background"
	ColorPinkBackground    Color = "pink_background"
	ColorRedBackground     Color = "red_background"
)

// SelectColor is the color of a select, multi-select, or status option. It is a
// narrower set than [Color]: no background variants.
type SelectColor string

// The colors a select option may use.
const (
	SelectColorDefault SelectColor = "default"
	SelectColorGray    SelectColor = "gray"
	SelectColorBrown   SelectColor = "brown"
	SelectColorOrange  SelectColor = "orange"
	SelectColorYellow  SelectColor = "yellow"
	SelectColorGreen   SelectColor = "green"
	SelectColorBlue    SelectColor = "blue"
	SelectColorPurple  SelectColor = "purple"
	SelectColorPink    SelectColor = "pink"
	SelectColorRed     SelectColor = "red"
)

// dateLayout is the date-only form Notion uses for dates without a time.
const dateLayout = "2006-01-02"

// DateTime is a point in time in a Notion date value.
//
// Notion returns either a date-only string ("2024-01-01") or a full RFC 3339
// timestamp, and the distinction is meaningful: a date-only value means the
// user did not set a time. HasTime records which form was received, and
// encoding reproduces it.
type DateTime struct {
	Time time.Time
	// HasTime reports whether the value carries a time of day. When false, only
	// the date part of Time is meaningful.
	HasTime bool
}

// NewDate returns a date-only DateTime.
func NewDate(t time.Time) DateTime { return DateTime{Time: t, HasTime: false} }

// NewDateTime returns a DateTime carrying a time of day.
func NewDateTime(t time.Time) DateTime { return DateTime{Time: t, HasTime: true} }

func (d DateTime) String() string {
	if d.HasTime {
		return d.Time.Format(time.RFC3339)
	}
	return d.Time.Format(dateLayout)
}

// IsZero reports whether d holds no time at all.
func (d DateTime) IsZero() bool { return d.Time.IsZero() }

func (d DateTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *DateTime) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if s == "" {
		*d = DateTime{}
		return nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		*d = DateTime{Time: t, HasTime: true}
		return nil
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return fmt.Errorf("notion: parsing date %q: %w", s, err)
	}
	*d = DateTime{Time: t, HasTime: false}
	return nil
}

// Date is a date or date range, as stored in a date property or a date mention.
type Date struct {
	// Start is the beginning of the range, or the whole value when End is nil.
	Start DateTime `json:"start"`
	// End is the end of the range, nil for a single date.
	End *DateTime `json:"end,omitempty"`
	// TimeZone is an IANA time zone name, empty when the value is not zoned.
	TimeZone string `json:"time_zone,omitempty"`
}

// ParentType identifies what an object hangs off.
type ParentType string

// The kinds of parent an object may have.
const (
	ParentTypeDatabase   ParentType = "database_id"
	ParentTypeDataSource ParentType = "data_source_id"
	ParentTypePage       ParentType = "page_id"
	ParentTypeBlock      ParentType = "block_id"
	ParentTypeAgent      ParentType = "agent_id"
	ParentTypeWorkspace  ParentType = "workspace"
)

// Parent identifies the container an object belongs to. Read Type first: only
// the field it names is populated.
//
// A data source parent is the exception and populates both DataSourceID and the
// DatabaseID of the database that data source belongs to.
type Parent struct {
	Type         ParentType `json:"type"`
	DatabaseID   string     `json:"database_id,omitempty"`
	DataSourceID string     `json:"data_source_id,omitempty"`
	PageID       string     `json:"page_id,omitempty"`
	BlockID      string     `json:"block_id,omitempty"`
	AgentID      string     `json:"agent_id,omitempty"`
	// Workspace is true when Type is [ParentTypeWorkspace], identifying a
	// top-level object.
	Workspace bool `json:"workspace,omitempty"`
}

// ID returns the parent's identifier, or the empty string for a workspace
// parent, which has none.
func (p Parent) ID() string {
	switch p.Type {
	case ParentTypeDatabase:
		return p.DatabaseID
	case ParentTypeDataSource:
		return p.DataSourceID
	case ParentTypePage:
		return p.PageID
	case ParentTypeBlock:
		return p.BlockID
	case ParentTypeAgent:
		return p.AgentID
	default:
		return ""
	}
}

// MarshalJSON sets Workspace when Type is [ParentTypeWorkspace], since the API
// requires "workspace": true alongside the type (common.ts:1755-1760).
func (p Parent) MarshalJSON() ([]byte, error) {
	type plain Parent
	if p.Type == ParentTypeWorkspace {
		p.Workspace = true
	}
	return json.Marshal(plain(p))
}

// FileRef is a file Notion hosts. Its URL is signed and expires, so it is
// suitable for immediate download but not for storing.
type FileRef struct {
	URL string `json:"url"`
	// ExpiryTime is when URL stops working.
	ExpiryTime *time.Time `json:"expiry_time,omitempty"`
}

// ExternalRef is a file hosted outside Notion. Its URL does not expire.
type ExternalRef struct {
	URL string `json:"url"`
}

// FileType distinguishes a Notion-hosted file from an external one.
type FileType string

const (
	// FileTypeFile is a file Notion hosts, reachable at a signed URL.
	FileTypeFile FileType = "file"
	// FileTypeExternal is a file referenced by a URL Notion does not host.
	FileTypeExternal FileType = "external"
	// FileTypeFileUpload references a completed file upload. Requests only.
	FileTypeFileUpload FileType = "file_upload"
)

// File is a file attached to a page, block, or property. Use [File.URL] rather
// than reaching into the variant fields.
type File struct {
	Type FileType `json:"type"`
	// File is set when Type is [FileTypeFile].
	File *FileRef `json:"file,omitempty"`
	// External is set when Type is [FileTypeExternal].
	External *ExternalRef `json:"external,omitempty"`
	// FileUpload is set when Type is [FileTypeFileUpload], in requests only.
	FileUpload *IDRef `json:"file_upload,omitempty"`
	// Name is set for files in a files property.
	Name string `json:"name,omitempty"`
	// Caption is set for file, image, video, pdf, and audio blocks.
	Caption RichTextList `json:"caption,omitempty"`
}

// URL returns the file's location whichever way it is hosted. For a
// Notion-hosted file the URL is signed and expires.
func (f File) URL() string {
	switch {
	case f.File != nil:
		return f.File.URL
	case f.External != nil:
		return f.External.URL
	default:
		return ""
	}
}

// NewExternalFile returns a File referencing a URL Notion does not host.
func NewExternalFile(url string) File {
	return File{Type: FileTypeExternal, External: &ExternalRef{URL: url}}
}

// NewNamedExternalFile returns an external File carrying a display name, which
// a files property requires of every entry (InternalOrExternalFileWithNameRequest,
// common.ts:735-737).
func NewNamedExternalFile(name, url string) File {
	f := NewExternalFile(url)
	f.Name = name
	return f
}

// IDRef is a bare reference to another object by ID.
type IDRef struct {
	ID string `json:"id"`
}

// CustomEmoji is a workspace-defined emoji.
type CustomEmoji struct {
	ID string `json:"id"`
	// Name and URL are set in responses; a request needs only the ID
	// (CustomEmojiRequest, common.ts).
	Name string `json:"name,omitempty"`
	URL  string `json:"url,omitempty"`
}

// Noticon is one of Notion's built-in named icons.
type Noticon struct {
	// Name is the icon's name, such as "pizza" or "home". The Notion icon
	// picker lists the valid names.
	Name string `json:"name"`
	// Color is one of gray, lightgray, brown, yellow, orange, green, blue,
	// purple, pink, or red. It is optional in requests.
	Color string `json:"color,omitempty"`
}

// IconType identifies which kind of icon an [Icon] holds.
type IconType string

const (
	IconTypeEmoji       IconType = "emoji"
	IconTypeFile        IconType = "file"
	IconTypeExternal    IconType = "external"
	IconTypeCustomEmoji IconType = "custom_emoji"
	IconTypeNoticon     IconType = "icon"
)

// Icon is the icon of a page, database, or callout. Read Type first: only the
// field it names is populated.
type Icon struct {
	Type        IconType     `json:"type"`
	Emoji       string       `json:"emoji,omitempty"`
	File        *FileRef     `json:"file,omitempty"`
	External    *ExternalRef `json:"external,omitempty"`
	CustomEmoji *CustomEmoji `json:"custom_emoji,omitempty"`
	// Noticon is set when Type is [IconTypeNoticon].
	Noticon *Noticon `json:"icon,omitempty"`
}

// NewEmojiIcon returns an Icon showing a single emoji character.
func NewEmojiIcon(emoji string) *Icon {
	return &Icon{Type: IconTypeEmoji, Emoji: emoji}
}

// UserType distinguishes a person from an integration.
type UserType string

const (
	// UserTypePerson is a human member of the workspace.
	UserTypePerson UserType = "person"
	// UserTypeBot is an integration.
	UserTypeBot UserType = "bot"
)

// Person holds the fields unique to a human user. The email is present only
// when the integration has access to member email addresses.
type Person struct {
	Email string `json:"email,omitempty"`
}

// Bot holds the fields unique to an integration user.
type Bot struct {
	Owner         *BotOwner `json:"owner,omitempty"`
	WorkspaceName string    `json:"workspace_name,omitempty"`
	// WorkspaceID identifies the bot's workspace (common.ts:113).
	WorkspaceID     string `json:"workspace_id,omitempty"`
	WorkspaceLimits *struct {
		MaxFileUploadSizeInBytes int64 `json:"max_file_upload_size_in_bytes,omitempty"`
	} `json:"workspace_limits,omitempty"`
}

// BotOwner identifies who installed an integration: a specific user, or the
// workspace as a whole.
type BotOwner struct {
	Type string `json:"type"`
	User *User  `json:"user,omitempty"`
	// Workspace is true when the integration belongs to the workspace rather
	// than to one user.
	Workspace bool `json:"workspace,omitempty"`
}

// User is a person or integration.
//
// Notion returns a partial user — only Object and ID — wherever the caller may
// not have access to the full record, which includes every created_by and
// last_edited_by field. Check [User.IsFull] before reading Name or Type.
type User struct {
	// Object is "user" in responses and optional in requests
	// (PartialUserObjectRequest, common.ts:1980-1985).
	Object string   `json:"object,omitempty"`
	ID     string   `json:"id"`
	Type   UserType `json:"type,omitempty"`
	Name   string   `json:"name,omitempty"`
	// AvatarURL is empty when the user has no avatar.
	AvatarURL string `json:"avatar_url,omitempty"`
	// Person is set when Type is [UserTypePerson].
	Person *Person `json:"person,omitempty"`
	// Bot is set when Type is [UserTypeBot].
	Bot *Bot `json:"bot,omitempty"`
}

// IsFull reports whether the user carries more than an ID. Notion signals a
// full user by including its type, per isFullUser in helpers.ts:413-417.
func (u User) IsFull() bool { return u.Type != "" }

// Annotations is the styling applied to a run of rich text.
type Annotations struct {
	Bold          bool  `json:"bold"`
	Italic        bool  `json:"italic"`
	Strikethrough bool  `json:"strikethrough"`
	Underline     bool  `json:"underline"`
	Code          bool  `json:"code"`
	Color         Color `json:"color,omitempty"`
}

// SelectOption is one choice in a select, multi-select, or status property.
type SelectOption struct {
	ID          string      `json:"id,omitempty"`
	Name        string      `json:"name,omitempty"`
	Color       SelectColor `json:"color,omitempty"`
	Description string      `json:"description,omitempty"`
}

// normalizeID strips dashes from a Notion UUID so IDs can be compared
// regardless of which form the caller supplied.
func normalizeID(id string) string {
	return strings.ReplaceAll(strings.ToLower(id), "-", "")
}

// SameID reports whether two Notion IDs refer to the same object, ignoring
// dashes and case. The API accepts both the dashed and undashed forms and
// returns whichever it prefers, so direct string comparison is unreliable.
func SameID(a, b string) bool {
	return normalizeID(a) == normalizeID(b)
}
