package notion

import "encoding/json"

// PropertySchema describes one column of a data source: its type and the
// configuration for that type. Notion calls this the property configuration.
//
// It is the schema; [PropertyValue] is a page's value for that column. The two
// share the [PropertyType] constants.
//
// Consume it with a type switch:
//
//	switch s := ds.Properties["Status"].(type) {
//	case *notion.StatusSchema:
//		for _, opt := range s.Status.Options {
//			fmt.Println(opt.Name)
//		}
//	}
type PropertySchema interface {
	// SchemaType returns the column's type.
	SchemaType() PropertyType
	// Schema returns the fields every column carries.
	Schema() *SchemaBase
}

// SchemaBase holds the fields every column carries.
type SchemaBase struct {
	// ID is Notion's opaque identifier for the column. It survives renames, so
	// it is the stable way to address a column.
	ID string `json:"id,omitempty"`
	// Name is the column's display name, which is also its key in
	// [PropertySchemas] and [PropertyValues].
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// Schema implements [PropertySchema].
func (s *SchemaBase) Schema() *SchemaBase { return s }

// NumberConfig configures a number column.
type NumberConfig struct {
	// Format is how the number is displayed, such as "number", "percent", or
	// "dollar".
	Format string `json:"format,omitempty"`
}

// FormulaConfig configures a formula column.
type FormulaConfig struct {
	// Expression is the formula, in Notion's formula language.
	Expression string `json:"expression"`
}

// OptionsConfig configures a select or multi-select column.
type OptionsConfig struct {
	Options []SelectOption `json:"options,omitempty"`
}

// StatusGroup collects status options into one of Notion's three phases.
type StatusGroup struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Color SelectColor `json:"color"`
	// OptionIDs lists the options belonging to this group.
	OptionIDs []string `json:"option_ids"`
}

// StatusConfig configures a status column.
type StatusConfig struct {
	Options []SelectOption `json:"options,omitempty"`
	// Groups partitions the options into to-do, in-progress, and complete.
	Groups []StatusGroup `json:"groups,omitempty"`
}

// RelationConfig configures a relation column.
//
// A single-property relation is one-way. A dual-property relation keeps a
// matching column on the other data source in sync, named by SyncedPropertyName.
type RelationConfig struct {
	// DatabaseID and DataSourceID identify what this column relates to.
	DatabaseID   string `json:"database_id,omitempty"`
	DataSourceID string `json:"data_source_id,omitempty"`
	// Type is "single_property" or "dual_property".
	Type           string       `json:"type,omitempty"`
	SingleProperty *EmptyObject `json:"single_property,omitempty"`
	// DualProperty is set for a two-way relation. To create one, set it to an
	// empty DualPropertyConfig; Notion fills in the synced column.
	DualProperty *DualPropertyConfig `json:"dual_property,omitempty"`
}

// DualPropertyConfig names the synced column on the other data source of a
// two-way relation. Both fields are optional in requests, so creating a dual
// relation sends "dual_property": {} (RelationPropertyConfigurationRequest,
// common.ts:1191-1196).
type DualPropertyConfig struct {
	SyncedPropertyID   string `json:"synced_property_id,omitempty"`
	SyncedPropertyName string `json:"synced_property_name,omitempty"`
}

// RollupConfig configures a rollup column: an aggregate computed over a
// relation.
type RollupConfig struct {
	// Function is the aggregation, such as "count", "sum", or "percent_empty".
	Function string `json:"function"`
	// RelationPropertyName names the relation column to aggregate across.
	RelationPropertyName string `json:"relation_property_name,omitempty"`
	RelationPropertyID   string `json:"relation_property_id,omitempty"`
	// RollupPropertyName names the column on the related rows to aggregate.
	RollupPropertyName string `json:"rollup_property_name,omitempty"`
	RollupPropertyID   string `json:"rollup_property_id,omitempty"`
}

// UniqueIDConfig configures an auto-incrementing ID column.
type UniqueIDConfig struct {
	// Prefix is the text shown before the number, such as "TASK".
	Prefix string `json:"prefix,omitempty"`
}

// The column variants follow. Each embeds [SchemaBase] and holds its
// configuration in a field named for its type.

// TitleSchema is the title column. Every data source has exactly one, and it cannot be deleted.
type TitleSchema struct {
	SchemaBase
	Title EmptyObject `json:"title"`
}

func (*TitleSchema) SchemaType() PropertyType { return PropertyTypeTitle }
func (s *TitleSchema) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.SchemaBase, &s.Title)
}
func (s TitleSchema) MarshalJSON() ([]byte, error) {
	return wrap(s.SchemaBase, "title", s.Title)
}

// RichTextSchema is a formatted text column.
type RichTextSchema struct {
	SchemaBase
	RichText EmptyObject `json:"rich_text"`
}

func (*RichTextSchema) SchemaType() PropertyType { return PropertyTypeRichText }
func (s *RichTextSchema) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.SchemaBase, &s.RichText)
}
func (s RichTextSchema) MarshalJSON() ([]byte, error) {
	return wrap(s.SchemaBase, "rich_text", s.RichText)
}

// NumberSchema is a numeric column.
type NumberSchema struct {
	SchemaBase
	Number NumberConfig `json:"number"`
}

func (*NumberSchema) SchemaType() PropertyType { return PropertyTypeNumber }
func (s *NumberSchema) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.SchemaBase, &s.Number)
}
func (s NumberSchema) MarshalJSON() ([]byte, error) {
	return wrap(s.SchemaBase, "number", s.Number)
}

// SelectSchema is a single-choice column.
type SelectSchema struct {
	SchemaBase
	Select OptionsConfig `json:"select"`
}

func (*SelectSchema) SchemaType() PropertyType { return PropertyTypeSelect }
func (s *SelectSchema) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.SchemaBase, &s.Select)
}
func (s SelectSchema) MarshalJSON() ([]byte, error) {
	return wrap(s.SchemaBase, "select", s.Select)
}

// MultiSelectSchema is a multiple-choice column.
type MultiSelectSchema struct {
	SchemaBase
	MultiSelect OptionsConfig `json:"multi_select"`
}

func (*MultiSelectSchema) SchemaType() PropertyType { return PropertyTypeMultiSelect }
func (s *MultiSelectSchema) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.SchemaBase, &s.MultiSelect)
}
func (s MultiSelectSchema) MarshalJSON() ([]byte, error) {
	return wrap(s.SchemaBase, "multi_select", s.MultiSelect)
}

// StatusSchema is a status column, whose options are grouped into phases.
type StatusSchema struct {
	SchemaBase
	Status StatusConfig `json:"status"`
}

func (*StatusSchema) SchemaType() PropertyType { return PropertyTypeStatus }
func (s *StatusSchema) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.SchemaBase, &s.Status)
}
func (s StatusSchema) MarshalJSON() ([]byte, error) {
	return wrap(s.SchemaBase, "status", s.Status)
}

// DateSchema is a date or date-range column.
type DateSchema struct {
	SchemaBase
	Date EmptyObject `json:"date"`
}

func (*DateSchema) SchemaType() PropertyType { return PropertyTypeDate }
func (s *DateSchema) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.SchemaBase, &s.Date)
}
func (s DateSchema) MarshalJSON() ([]byte, error) {
	return wrap(s.SchemaBase, "date", s.Date)
}

// PeopleSchema is a column referencing workspace members.
type PeopleSchema struct {
	SchemaBase
	People EmptyObject `json:"people"`
}

func (*PeopleSchema) SchemaType() PropertyType { return PropertyTypePeople }
func (s *PeopleSchema) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.SchemaBase, &s.People)
}
func (s PeopleSchema) MarshalJSON() ([]byte, error) {
	return wrap(s.SchemaBase, "people", s.People)
}

// FilesSchema is a file attachment column.
type FilesSchema struct {
	SchemaBase
	Files EmptyObject `json:"files"`
}

func (*FilesSchema) SchemaType() PropertyType { return PropertyTypeFiles }
func (s *FilesSchema) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.SchemaBase, &s.Files)
}
func (s FilesSchema) MarshalJSON() ([]byte, error) {
	return wrap(s.SchemaBase, "files", s.Files)
}

// CheckboxSchema is a boolean column.
type CheckboxSchema struct {
	SchemaBase
	Checkbox EmptyObject `json:"checkbox"`
}

func (*CheckboxSchema) SchemaType() PropertyType { return PropertyTypeCheckbox }
func (s *CheckboxSchema) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.SchemaBase, &s.Checkbox)
}
func (s CheckboxSchema) MarshalJSON() ([]byte, error) {
	return wrap(s.SchemaBase, "checkbox", s.Checkbox)
}

// URLSchema is a URL column.
type URLSchema struct {
	SchemaBase
	URL EmptyObject `json:"url"`
}

func (*URLSchema) SchemaType() PropertyType { return PropertyTypeURL }
func (s *URLSchema) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.SchemaBase, &s.URL)
}
func (s URLSchema) MarshalJSON() ([]byte, error) {
	return wrap(s.SchemaBase, "url", s.URL)
}

// EmailSchema is an email address column.
type EmailSchema struct {
	SchemaBase
	Email EmptyObject `json:"email"`
}

func (*EmailSchema) SchemaType() PropertyType { return PropertyTypeEmail }
func (s *EmailSchema) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.SchemaBase, &s.Email)
}
func (s EmailSchema) MarshalJSON() ([]byte, error) {
	return wrap(s.SchemaBase, "email", s.Email)
}

// PhoneNumberSchema is a phone number column.
type PhoneNumberSchema struct {
	SchemaBase
	PhoneNumber EmptyObject `json:"phone_number"`
}

func (*PhoneNumberSchema) SchemaType() PropertyType { return PropertyTypePhoneNumber }
func (s *PhoneNumberSchema) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.SchemaBase, &s.PhoneNumber)
}
func (s PhoneNumberSchema) MarshalJSON() ([]byte, error) {
	return wrap(s.SchemaBase, "phone_number", s.PhoneNumber)
}

// FormulaSchema is a computed column.
type FormulaSchema struct {
	SchemaBase
	Formula FormulaConfig `json:"formula"`
}

func (*FormulaSchema) SchemaType() PropertyType { return PropertyTypeFormula }
func (s *FormulaSchema) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.SchemaBase, &s.Formula)
}
func (s FormulaSchema) MarshalJSON() ([]byte, error) {
	return wrap(s.SchemaBase, "formula", s.Formula)
}

// RelationSchema is a column linking to another data source.
type RelationSchema struct {
	SchemaBase
	Relation RelationConfig `json:"relation"`
}

func (*RelationSchema) SchemaType() PropertyType { return PropertyTypeRelation }
func (s *RelationSchema) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.SchemaBase, &s.Relation)
}
func (s RelationSchema) MarshalJSON() ([]byte, error) {
	return wrap(s.SchemaBase, "relation", s.Relation)
}

// RollupSchema is a column aggregating values across a relation.
type RollupSchema struct {
	SchemaBase
	Rollup RollupConfig `json:"rollup"`
}

func (*RollupSchema) SchemaType() PropertyType { return PropertyTypeRollup }
func (s *RollupSchema) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.SchemaBase, &s.Rollup)
}
func (s RollupSchema) MarshalJSON() ([]byte, error) {
	return wrap(s.SchemaBase, "rollup", s.Rollup)
}

// CreatedBySchema is a read-only column holding each page's author.
type CreatedBySchema struct {
	SchemaBase
	CreatedBy EmptyObject `json:"created_by"`
}

func (*CreatedBySchema) SchemaType() PropertyType { return PropertyTypeCreatedBy }
func (s *CreatedBySchema) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.SchemaBase, &s.CreatedBy)
}
func (s CreatedBySchema) MarshalJSON() ([]byte, error) {
	return wrap(s.SchemaBase, "created_by", s.CreatedBy)
}

// CreatedTimeSchema is a read-only column holding each page's creation time.
type CreatedTimeSchema struct {
	SchemaBase
	CreatedTime EmptyObject `json:"created_time"`
}

func (*CreatedTimeSchema) SchemaType() PropertyType { return PropertyTypeCreatedTime }
func (s *CreatedTimeSchema) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.SchemaBase, &s.CreatedTime)
}
func (s CreatedTimeSchema) MarshalJSON() ([]byte, error) {
	return wrap(s.SchemaBase, "created_time", s.CreatedTime)
}

// LastEditedBySchema is a read-only column holding each page's last editor.
type LastEditedBySchema struct {
	SchemaBase
	LastEditedBy EmptyObject `json:"last_edited_by"`
}

func (*LastEditedBySchema) SchemaType() PropertyType { return PropertyTypeLastEditedBy }
func (s *LastEditedBySchema) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.SchemaBase, &s.LastEditedBy)
}
func (s LastEditedBySchema) MarshalJSON() ([]byte, error) {
	return wrap(s.SchemaBase, "last_edited_by", s.LastEditedBy)
}

// LastEditedTimeSchema is a read-only column holding each page's last edit time.
type LastEditedTimeSchema struct {
	SchemaBase
	LastEditedTime EmptyObject `json:"last_edited_time"`
}

func (*LastEditedTimeSchema) SchemaType() PropertyType { return PropertyTypeLastEditedTime }
func (s *LastEditedTimeSchema) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.SchemaBase, &s.LastEditedTime)
}
func (s LastEditedTimeSchema) MarshalJSON() ([]byte, error) {
	return wrap(s.SchemaBase, "last_edited_time", s.LastEditedTime)
}

// UniqueIDSchema is a read-only column holding an auto-incrementing identifier.
type UniqueIDSchema struct {
	SchemaBase
	UniqueID UniqueIDConfig `json:"unique_id"`
}

func (*UniqueIDSchema) SchemaType() PropertyType { return PropertyTypeUniqueID }
func (s *UniqueIDSchema) UnmarshalJSON(data []byte) error {
	return unwrap(data, &s.SchemaBase, &s.UniqueID)
}
func (s UniqueIDSchema) MarshalJSON() ([]byte, error) {
	return wrap(s.SchemaBase, "unique_id", s.UniqueID)
}

// UnknownSchema is a column type this package does not recognize. It retains
// the raw JSON and re-encodes it unchanged.
type UnknownSchema struct {
	SchemaBase
	// Type is the unrecognized discriminant.
	Type PropertyType
	// Raw is the column's original JSON.
	Raw json.RawMessage
}

func (u *UnknownSchema) SchemaType() PropertyType    { return u.Type }
func (u UnknownSchema) MarshalJSON() ([]byte, error) { return u.Raw, nil }

// schemaRegistry maps each discriminant to its variant. TestSchemaRegistryCoverage
// keeps it in step with the PropertyType constants.
var schemaRegistry = map[string]func() PropertySchema{
	string(PropertyTypeTitle):          func() PropertySchema { return new(TitleSchema) },
	string(PropertyTypeRichText):       func() PropertySchema { return new(RichTextSchema) },
	string(PropertyTypeNumber):         func() PropertySchema { return new(NumberSchema) },
	string(PropertyTypeSelect):         func() PropertySchema { return new(SelectSchema) },
	string(PropertyTypeMultiSelect):    func() PropertySchema { return new(MultiSelectSchema) },
	string(PropertyTypeStatus):         func() PropertySchema { return new(StatusSchema) },
	string(PropertyTypeDate):           func() PropertySchema { return new(DateSchema) },
	string(PropertyTypePeople):         func() PropertySchema { return new(PeopleSchema) },
	string(PropertyTypeFiles):          func() PropertySchema { return new(FilesSchema) },
	string(PropertyTypeCheckbox):       func() PropertySchema { return new(CheckboxSchema) },
	string(PropertyTypeURL):            func() PropertySchema { return new(URLSchema) },
	string(PropertyTypeEmail):          func() PropertySchema { return new(EmailSchema) },
	string(PropertyTypePhoneNumber):    func() PropertySchema { return new(PhoneNumberSchema) },
	string(PropertyTypeFormula):        func() PropertySchema { return new(FormulaSchema) },
	string(PropertyTypeRelation):       func() PropertySchema { return new(RelationSchema) },
	string(PropertyTypeRollup):         func() PropertySchema { return new(RollupSchema) },
	string(PropertyTypeCreatedBy):      func() PropertySchema { return new(CreatedBySchema) },
	string(PropertyTypeCreatedTime):    func() PropertySchema { return new(CreatedTimeSchema) },
	string(PropertyTypeLastEditedBy):   func() PropertySchema { return new(LastEditedBySchema) },
	string(PropertyTypeLastEditedTime): func() PropertySchema { return new(LastEditedTimeSchema) },
	string(PropertyTypeUniqueID):       func() PropertySchema { return new(UniqueIDSchema) },
}

func unknownSchema(tag string, raw []byte) PropertySchema {
	u := &UnknownSchema{Type: PropertyType(tag), Raw: raw}
	_ = json.Unmarshal(raw, &u.SchemaBase)
	return u
}

// DecodePropertySchema decodes a single column definition. An unrecognized type
// yields an [*UnknownSchema] rather than an error.
func DecodePropertySchema(data []byte) (PropertySchema, error) {
	return decodeUnion(data, schemaRegistry, unknownSchema)
}

// PropertySchemas maps a data source's column names to their definitions.
type PropertySchemas map[string]PropertySchema

func (p *PropertySchemas) UnmarshalJSON(data []byte) error {
	var raws map[string]json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return err
	}
	out := make(PropertySchemas, len(raws))
	for name, raw := range raws {
		schema, err := DecodePropertySchema(raw)
		if err != nil {
			return err
		}
		out[name] = schema
	}
	*p = out
	return nil
}

// TitleName returns the name of the data source's title column, or the empty
// string if the schema has none.
//
// Every data source has exactly one title column, but its name varies by
// workspace, so looking it up by type is more reliable than guessing "Name".
func (p PropertySchemas) TitleName() string {
	for name, schema := range p {
		if schema.SchemaType() == PropertyTypeTitle {
			return name
		}
	}
	return ""
}
