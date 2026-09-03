package notion

import (
	"encoding/json"
	"time"
)

// PropertyType identifies a page property's kind.
type PropertyType string

// The property types Notion supports.
const (
	PropertyTypeTitle          PropertyType = "title"
	PropertyTypeRichText       PropertyType = "rich_text"
	PropertyTypeNumber         PropertyType = "number"
	PropertyTypeSelect         PropertyType = "select"
	PropertyTypeMultiSelect    PropertyType = "multi_select"
	PropertyTypeStatus         PropertyType = "status"
	PropertyTypeDate           PropertyType = "date"
	PropertyTypePeople         PropertyType = "people"
	PropertyTypeFiles          PropertyType = "files"
	PropertyTypeCheckbox       PropertyType = "checkbox"
	PropertyTypeURL            PropertyType = "url"
	PropertyTypeEmail          PropertyType = "email"
	PropertyTypePhoneNumber    PropertyType = "phone_number"
	PropertyTypeFormula        PropertyType = "formula"
	PropertyTypeRelation       PropertyType = "relation"
	PropertyTypeRollup         PropertyType = "rollup"
	PropertyTypeCreatedBy      PropertyType = "created_by"
	PropertyTypeCreatedTime    PropertyType = "created_time"
	PropertyTypeLastEditedBy   PropertyType = "last_edited_by"
	PropertyTypeLastEditedTime PropertyType = "last_edited_time"
	PropertyTypeButton         PropertyType = "button"
	PropertyTypeUniqueID       PropertyType = "unique_id"
	PropertyTypeVerification   PropertyType = "verification"
	PropertyTypePlace          PropertyType = "place"
)

// PropertyValue is one property of a page: the value stored in a data source
// column for that row.
//
// Consume it with a type switch, or reach for the accessors on
// [PropertyValues] when you know the column's type:
//
//	switch v := page.Properties["Status"].(type) {
//	case *notion.StatusProperty:
//		fmt.Println(v.Status.Name)
//	case *notion.NumberProperty:
//		fmt.Println(*v.Number)
//	}
//
// A property type this package does not recognize decodes as
// [*UnknownProperty] rather than failing.
type PropertyValue interface {
	// PropertyType returns the property's kind.
	PropertyType() PropertyType
	// PropertyID returns Notion's opaque identifier for the column.
	PropertyID() string
}

// propertyBase is the envelope every property value carries.
type propertyBase struct {
	ID string `json:"id,omitempty"`
}

// PropertyID implements [PropertyValue].
func (p propertyBase) PropertyID() string { return p.ID }

// TitleProperty is the page's title. Every data source has exactly one.
type TitleProperty struct {
	propertyBase
	Title RichTextList `json:"title"`
}

func (*TitleProperty) PropertyType() PropertyType { return PropertyTypeTitle }
func (p *TitleProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.Title)
}
func (p TitleProperty) MarshalJSON() ([]byte, error) {
	return wrap(p.propertyBase, "title", p.Title)
}

// RichTextProperty is a formatted text column.
type RichTextProperty struct {
	propertyBase
	RichText RichTextList `json:"rich_text"`
}

func (*RichTextProperty) PropertyType() PropertyType { return PropertyTypeRichText }
func (p *RichTextProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.RichText)
}
func (p RichTextProperty) MarshalJSON() ([]byte, error) {
	return wrap(p.propertyBase, "rich_text", p.RichText)
}

// NumberProperty is a numeric column. Number is nil when the cell is empty,
// which is distinct from zero.
type NumberProperty struct {
	propertyBase
	Number *float64 `json:"number"`
}

func (*NumberProperty) PropertyType() PropertyType { return PropertyTypeNumber }
func (p *NumberProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.Number)
}
func (p NumberProperty) MarshalJSON() ([]byte, error) {
	return wrap(p.propertyBase, "number", p.Number)
}

// SelectProperty is a single-choice column. Select is nil when unset.
type SelectProperty struct {
	propertyBase
	Select *SelectOption `json:"select"`
}

func (*SelectProperty) PropertyType() PropertyType { return PropertyTypeSelect }
func (p *SelectProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.Select)
}
func (p SelectProperty) MarshalJSON() ([]byte, error) {
	return wrap(p.propertyBase, "select", p.Select)
}

// StatusProperty is a status column, a single choice grouped into to-do,
// in-progress, and complete.
type StatusProperty struct {
	propertyBase
	Status *SelectOption `json:"status"`
}

func (*StatusProperty) PropertyType() PropertyType { return PropertyTypeStatus }
func (p *StatusProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.Status)
}
func (p StatusProperty) MarshalJSON() ([]byte, error) {
	return wrap(p.propertyBase, "status", p.Status)
}

// MultiSelectProperty is a multiple-choice column.
type MultiSelectProperty struct {
	propertyBase
	MultiSelect []SelectOption `json:"multi_select"`
}

func (*MultiSelectProperty) PropertyType() PropertyType { return PropertyTypeMultiSelect }
func (p *MultiSelectProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.MultiSelect)
}
func (p MultiSelectProperty) MarshalJSON() ([]byte, error) {
	// The request requires an array (pages.ts:377-390), so nil encodes as [].
	return wrap(p.propertyBase, "multi_select", orEmpty(p.MultiSelect))
}

// DateProperty is a date or date-range column. Date is nil when unset.
type DateProperty struct {
	propertyBase
	Date *Date `json:"date"`
}

func (*DateProperty) PropertyType() PropertyType { return PropertyTypeDate }
func (p *DateProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.Date)
}
func (p DateProperty) MarshalJSON() ([]byte, error) {
	return wrap(p.propertyBase, "date", p.Date)
}

// PeopleProperty is a column referencing workspace members. The users may be
// partial; check [User.IsFull].
type PeopleProperty struct {
	propertyBase
	People []User `json:"people"`
}

func (*PeopleProperty) PropertyType() PropertyType { return PropertyTypePeople }
func (p *PeopleProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.People)
}
func (p PeopleProperty) MarshalJSON() ([]byte, error) {
	// The request requires an array (pages.ts:393-396), so nil encodes as [].
	return wrap(p.propertyBase, "people", orEmpty(p.People))
}

// FilesProperty is a column holding file attachments.
type FilesProperty struct {
	propertyBase
	Files []File `json:"files"`
}

func (*FilesProperty) PropertyType() PropertyType { return PropertyTypeFiles }
func (p *FilesProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.Files)
}
func (p FilesProperty) MarshalJSON() ([]byte, error) {
	// The request requires an array (pages.ts:402-408), so nil encodes as [].
	return wrap(p.propertyBase, "files", orEmpty(p.Files))
}

// CheckboxProperty is a boolean column.
type CheckboxProperty struct {
	propertyBase
	Checkbox bool `json:"checkbox"`
}

func (*CheckboxProperty) PropertyType() PropertyType { return PropertyTypeCheckbox }
func (p *CheckboxProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.Checkbox)
}
func (p CheckboxProperty) MarshalJSON() ([]byte, error) {
	return wrap(p.propertyBase, "checkbox", p.Checkbox)
}

// URLProperty is a URL column.
type URLProperty struct {
	propertyBase
	URL string `json:"url"`
}

func (*URLProperty) PropertyType() PropertyType { return PropertyTypeURL }
func (p *URLProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.URL)
}
func (p URLProperty) MarshalJSON() ([]byte, error) {
	return wrap(p.propertyBase, "url", p.URL)
}

// EmailProperty is an email address column.
type EmailProperty struct {
	propertyBase
	Email string `json:"email"`
}

func (*EmailProperty) PropertyType() PropertyType { return PropertyTypeEmail }
func (p *EmailProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.Email)
}
func (p EmailProperty) MarshalJSON() ([]byte, error) {
	return wrap(p.propertyBase, "email", p.Email)
}

// PhoneNumberProperty is a phone number column.
type PhoneNumberProperty struct {
	propertyBase
	PhoneNumber string `json:"phone_number"`
}

func (*PhoneNumberProperty) PropertyType() PropertyType { return PropertyTypePhoneNumber }
func (p *PhoneNumberProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.PhoneNumber)
}
func (p PhoneNumberProperty) MarshalJSON() ([]byte, error) {
	return wrap(p.propertyBase, "phone_number", p.PhoneNumber)
}

// Formula is the computed result of a formula column. Read Type first: only
// the field it names is populated.
type Formula struct {
	Type    string   `json:"type"`
	String  string   `json:"string,omitempty"`
	Number  *float64 `json:"number,omitempty"`
	Boolean *bool    `json:"boolean,omitempty"`
	Date    *Date    `json:"date,omitempty"`
}

// FormulaProperty is a computed column. It is read-only: Notion rejects writes.
type FormulaProperty struct {
	propertyBase
	Formula Formula `json:"formula"`
}

func (*FormulaProperty) PropertyType() PropertyType { return PropertyTypeFormula }
func (p *FormulaProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.Formula)
}
func (p FormulaProperty) MarshalJSON() ([]byte, error) {
	return wrap(p.propertyBase, "formula", p.Formula)
}

// RelationProperty is a column linking to rows in another data source.
//
// Notion caps the relations returned inline at 25. When HasMore is true, read
// the rest through the page property endpoint,
// [PagesService.RetrieveProperty].
type RelationProperty struct {
	propertyBase
	Relation []IDRef `json:"relation"`
	// HasMore reports that the relation has more entries than were returned.
	HasMore bool `json:"has_more,omitempty"`
}

func (*RelationProperty) PropertyType() PropertyType { return PropertyTypeRelation }
func (p *RelationProperty) UnmarshalJSON(b []byte) error {
	if err := unwrap(b, &p.propertyBase, &p.Relation); err != nil {
		return err
	}
	// has_more sits beside the payload rather than inside it.
	var envelope struct {
		HasMore bool `json:"has_more"`
	}
	if err := json.Unmarshal(b, &envelope); err != nil {
		return err
	}
	p.HasMore = envelope.HasMore
	return nil
}

func (p RelationProperty) MarshalJSON() ([]byte, error) {
	envelope := struct {
		ID      string `json:"id,omitempty"`
		HasMore bool   `json:"has_more,omitempty"`
	}{ID: p.ID, HasMore: p.HasMore}
	// The request requires an array (pages.ts:401), so nil encodes as [].
	return wrap(envelope, "relation", orEmpty(p.Relation))
}

// Rollup is the aggregate a rollup column computed across a relation.
type Rollup struct {
	// Function is the aggregation applied, such as "count" or "sum".
	Function string `json:"function"`
	// Type names the result's shape: number, date, array, or unsupported.
	Type   string          `json:"type"`
	Number *float64        `json:"number,omitempty"`
	Date   *Date           `json:"date,omitempty"`
	Array  []PropertyValue `json:"array,omitempty"`
}

func (r *Rollup) UnmarshalJSON(data []byte) error {
	// Alias to decode the scalar fields without recursing, then decode the
	// array separately since it holds a union.
	type alias Rollup
	var raw struct {
		alias
		Array json.RawMessage `json:"array"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = Rollup(raw.alias)
	if len(raw.Array) == 0 {
		return nil
	}
	items, err := decodeUnionSlice(raw.Array, propertyRegistry, unknownProperty)
	if err != nil {
		return err
	}
	r.Array = items
	return nil
}

// RollupProperty is an aggregate computed over a relation. It is read-only.
type RollupProperty struct {
	propertyBase
	Rollup Rollup `json:"rollup"`
}

func (*RollupProperty) PropertyType() PropertyType { return PropertyTypeRollup }
func (p *RollupProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.Rollup)
}
func (p RollupProperty) MarshalJSON() ([]byte, error) {
	return wrap(p.propertyBase, "rollup", p.Rollup)
}

// CreatedByProperty records who created the page. It is read-only.
type CreatedByProperty struct {
	propertyBase
	CreatedBy User `json:"created_by"`
}

func (*CreatedByProperty) PropertyType() PropertyType { return PropertyTypeCreatedBy }
func (p *CreatedByProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.CreatedBy)
}
func (p CreatedByProperty) MarshalJSON() ([]byte, error) {
	return wrap(p.propertyBase, "created_by", p.CreatedBy)
}

// LastEditedByProperty records who last edited the page. It is read-only.
type LastEditedByProperty struct {
	propertyBase
	LastEditedBy User `json:"last_edited_by"`
}

func (*LastEditedByProperty) PropertyType() PropertyType { return PropertyTypeLastEditedBy }
func (p *LastEditedByProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.LastEditedBy)
}
func (p LastEditedByProperty) MarshalJSON() ([]byte, error) {
	return wrap(p.propertyBase, "last_edited_by", p.LastEditedBy)
}

// CreatedTimeProperty records when the page was created. It is read-only.
type CreatedTimeProperty struct {
	propertyBase
	CreatedTime time.Time `json:"created_time"`
}

func (*CreatedTimeProperty) PropertyType() PropertyType { return PropertyTypeCreatedTime }
func (p *CreatedTimeProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.CreatedTime)
}
func (p CreatedTimeProperty) MarshalJSON() ([]byte, error) {
	return wrap(p.propertyBase, "created_time", p.CreatedTime)
}

// LastEditedTimeProperty records when the page was last edited. It is
// read-only.
type LastEditedTimeProperty struct {
	propertyBase
	LastEditedTime time.Time `json:"last_edited_time"`
}

func (*LastEditedTimeProperty) PropertyType() PropertyType { return PropertyTypeLastEditedTime }
func (p *LastEditedTimeProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.LastEditedTime)
}
func (p LastEditedTimeProperty) MarshalJSON() ([]byte, error) {
	return wrap(p.propertyBase, "last_edited_time", p.LastEditedTime)
}

// ButtonProperty is a button column. It carries no value through the API.
type ButtonProperty struct {
	propertyBase
	Button EmptyObject `json:"button"`
}

func (*ButtonProperty) PropertyType() PropertyType { return PropertyTypeButton }
func (p *ButtonProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.Button)
}
func (p ButtonProperty) MarshalJSON() ([]byte, error) {
	return wrap(p.propertyBase, "button", p.Button)
}

// UniqueID is an auto-incrementing identifier Notion assigns to each row.
type UniqueID struct {
	// Prefix is the optional text before the number, such as "TASK".
	Prefix string `json:"prefix,omitempty"`
	Number *int64 `json:"number"`
}

// UniqueIDProperty is an auto-incrementing identifier column. It is read-only.
type UniqueIDProperty struct {
	propertyBase
	UniqueID UniqueID `json:"unique_id"`
}

func (*UniqueIDProperty) PropertyType() PropertyType { return PropertyTypeUniqueID }
func (p *UniqueIDProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.UniqueID)
}
func (p UniqueIDProperty) MarshalJSON() ([]byte, error) {
	return wrap(p.propertyBase, "unique_id", p.UniqueID)
}

// Verification is the review state of a page in a wiki data source.
type Verification struct {
	// State is "verified", "expired", or "unverified".
	State      string `json:"state"`
	Date       *Date  `json:"date,omitempty"`
	VerifiedBy *User  `json:"verified_by,omitempty"`
}

// VerificationProperty is the wiki verification column. It is read-only.
type VerificationProperty struct {
	propertyBase
	Verification *Verification `json:"verification"`
}

func (*VerificationProperty) PropertyType() PropertyType { return PropertyTypeVerification }
func (p *VerificationProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.Verification)
}
func (p VerificationProperty) MarshalJSON() ([]byte, error) {
	return wrap(p.propertyBase, "verification", p.Verification)
}

// Place is a geographic location.
type Place struct {
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lon"`
	Name      string  `json:"name,omitempty"`
	Address   string  `json:"address,omitempty"`
	// AWSPlaceID and GooglePlaceID identify the location in those providers'
	// catalogs, when Notion resolved it.
	AWSPlaceID    string `json:"aws_place_id,omitempty"`
	GooglePlaceID string `json:"google_place_id,omitempty"`
}

// PlaceProperty is a location column. Place is nil when unset.
type PlaceProperty struct {
	propertyBase
	Place *Place `json:"place"`
}

func (*PlaceProperty) PropertyType() PropertyType { return PropertyTypePlace }
func (p *PlaceProperty) UnmarshalJSON(b []byte) error {
	return unwrap(b, &p.propertyBase, &p.Place)
}
func (p PlaceProperty) MarshalJSON() ([]byte, error) {
	return wrap(p.propertyBase, "place", p.Place)
}

// UnknownProperty is a property type this package does not recognize. It
// retains the raw JSON and re-encodes it unchanged.
type UnknownProperty struct {
	propertyBase
	// Type is the unrecognized discriminant.
	Type PropertyType
	// Raw is the property's original JSON.
	Raw json.RawMessage
}

func (u *UnknownProperty) PropertyType() PropertyType  { return u.Type }
func (u UnknownProperty) MarshalJSON() ([]byte, error) { return u.Raw, nil }

var propertyRegistry = map[string]func() PropertyValue{
	string(PropertyTypeTitle):          func() PropertyValue { return new(TitleProperty) },
	string(PropertyTypeRichText):       func() PropertyValue { return new(RichTextProperty) },
	string(PropertyTypeNumber):         func() PropertyValue { return new(NumberProperty) },
	string(PropertyTypeSelect):         func() PropertyValue { return new(SelectProperty) },
	string(PropertyTypeMultiSelect):    func() PropertyValue { return new(MultiSelectProperty) },
	string(PropertyTypeStatus):         func() PropertyValue { return new(StatusProperty) },
	string(PropertyTypeDate):           func() PropertyValue { return new(DateProperty) },
	string(PropertyTypePeople):         func() PropertyValue { return new(PeopleProperty) },
	string(PropertyTypeFiles):          func() PropertyValue { return new(FilesProperty) },
	string(PropertyTypeCheckbox):       func() PropertyValue { return new(CheckboxProperty) },
	string(PropertyTypeURL):            func() PropertyValue { return new(URLProperty) },
	string(PropertyTypeEmail):          func() PropertyValue { return new(EmailProperty) },
	string(PropertyTypePhoneNumber):    func() PropertyValue { return new(PhoneNumberProperty) },
	string(PropertyTypeFormula):        func() PropertyValue { return new(FormulaProperty) },
	string(PropertyTypeRelation):       func() PropertyValue { return new(RelationProperty) },
	string(PropertyTypeRollup):         func() PropertyValue { return new(RollupProperty) },
	string(PropertyTypeCreatedBy):      func() PropertyValue { return new(CreatedByProperty) },
	string(PropertyTypeCreatedTime):    func() PropertyValue { return new(CreatedTimeProperty) },
	string(PropertyTypeLastEditedBy):   func() PropertyValue { return new(LastEditedByProperty) },
	string(PropertyTypeLastEditedTime): func() PropertyValue { return new(LastEditedTimeProperty) },
	string(PropertyTypeButton):         func() PropertyValue { return new(ButtonProperty) },
	string(PropertyTypeUniqueID):       func() PropertyValue { return new(UniqueIDProperty) },
	string(PropertyTypeVerification):   func() PropertyValue { return new(VerificationProperty) },
	string(PropertyTypePlace):          func() PropertyValue { return new(PlaceProperty) },
}

func unknownProperty(tag string, raw []byte) PropertyValue {
	u := &UnknownProperty{Type: PropertyType(tag), Raw: raw}
	_ = json.Unmarshal(raw, &u.propertyBase)
	return u
}

// DecodePropertyValue decodes a single property value. An unrecognized type
// yields an [*UnknownProperty] rather than an error.
func DecodePropertyValue(data []byte) (PropertyValue, error) {
	return decodeUnion(data, propertyRegistry, unknownProperty)
}

// PropertyValues maps a data source's column names to a page's values for them.
type PropertyValues map[string]PropertyValue

func (p *PropertyValues) UnmarshalJSON(data []byte) error {
	var raws map[string]json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return err
	}
	out := make(PropertyValues, len(raws))
	for name, raw := range raws {
		value, err := DecodePropertyValue(raw)
		if err != nil {
			return err
		}
		out[name] = value
	}
	*p = out
	return nil
}

// Title returns the plain text of the page's title property, whatever the
// column is named, or the empty string if the page has none.
//
// Every data source has exactly one title column, but its name varies, so
// looking it up by type is more reliable than guessing "Name".
func (p PropertyValues) Title() string {
	for _, value := range p {
		if title, ok := value.(*TitleProperty); ok {
			return title.Title.PlainText()
		}
	}
	return ""
}

// Text returns the plain text of the named title or rich text property, or the
// empty string if it is absent or another type.
func (p PropertyValues) Text(name string) string {
	switch value := p[name].(type) {
	case *TitleProperty:
		return value.Title.PlainText()
	case *RichTextProperty:
		return value.RichText.PlainText()
	default:
		return ""
	}
}

// Number returns the named number property's value. ok is false when the
// property is absent, another type, or empty.
func (p PropertyValues) Number(name string) (value float64, ok bool) {
	num, isNumber := p[name].(*NumberProperty)
	if !isNumber || num.Number == nil {
		return 0, false
	}
	return *num.Number, true
}

// Checkbox returns the named checkbox property's value, false if it is absent
// or another type.
func (p PropertyValues) Checkbox(name string) bool {
	box, ok := p[name].(*CheckboxProperty)
	return ok && box.Checkbox
}

// Select returns the name of the named select or status property's chosen
// option, or the empty string when unset.
func (p PropertyValues) Select(name string) string {
	switch value := p[name].(type) {
	case *SelectProperty:
		if value.Select != nil {
			return value.Select.Name
		}
	case *StatusProperty:
		if value.Status != nil {
			return value.Status.Name
		}
	}
	return ""
}

// Date returns the named date property's value, nil when absent, another type,
// or empty.
func (p PropertyValues) Date(name string) *Date {
	value, ok := p[name].(*DateProperty)
	if !ok {
		return nil
	}
	return value.Date
}
