package notion

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// maxFilterDepth is how deep and/or groups may nest in a query. Notion accepts
// a group of leaves, or a group of groups of leaves, and no more.
const maxFilterDepth = 2

// ErrFilterTooDeep reports and/or groups nested deeper than Notion allows.
var ErrFilterTooDeep = errors.New("notion: filter groups nested more than two levels deep")

// Filter selects which rows a data source query returns.
//
// Build one with the property constructors and combine them with [And] and
// [Or]:
//
//	filter := notion.And(
//		notion.ByStatus("Status").Equals("Done"),
//		notion.ByNumber("Priority").GreaterThan(3),
//	)
//
// The Notion API discriminates property filters by which key is present rather
// than by a type tag, and expresses each condition as a single-key object.
// These constructors hide that; [RawFilter] is the escape hatch for anything
// they do not cover.
type Filter interface {
	json.Marshaler
	// depth returns how many levels of and/or nesting the filter uses.
	depth() int
}

// propertyFilter is the one place that knows the wire shape of a property
// filter: a property name, a type tag, and a condition nested under a key
// named for the type.
type propertyFilter struct {
	property  string
	kind      PropertyType
	condition any
	// timestamp names a built-in timestamp instead of a column, for the
	// created_time and last_edited_time filters, which use a different key.
	timestamp string
}

func (f propertyFilter) MarshalJSON() ([]byte, error) {
	fields := map[string]any{
		"type":         string(f.kind),
		string(f.kind): f.condition,
	}
	if f.timestamp != "" {
		fields["timestamp"] = f.timestamp
	} else {
		fields["property"] = f.property
	}
	return json.Marshal(fields)
}

func (propertyFilter) depth() int { return 0 }

// compoundFilter is an and/or group.
type compoundFilter struct {
	operator string
	filters  []Filter
}

func (f compoundFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{f.operator: f.filters})
}

func (f compoundFilter) depth() int {
	deepest := 0
	for _, sub := range f.filters {
		deepest = max(deepest, sub.depth())
	}
	return deepest + 1
}

// And matches rows satisfying every filter.
func And(filters ...Filter) Filter {
	return compoundFilter{operator: "and", filters: filters}
}

// Or matches rows satisfying any filter.
func Or(filters ...Filter) Filter {
	return compoundFilter{operator: "or", filters: filters}
}

// RawFilter sends a filter this package does not model, as literal JSON.
func RawFilter(raw json.RawMessage) Filter { return rawFilter(raw) }

type rawFilter json.RawMessage

func (f rawFilter) MarshalJSON() ([]byte, error) { return f, nil }
func (rawFilter) depth() int                     { return 0 }

// validateFilterDepth reports a filter nested deeper than Notion accepts.
// The API allows two levels; Go can express any depth, so this is checked
// before sending to give a clearer error than the server's validation_error.
func validateFilterDepth(f Filter) error {
	if f == nil {
		return nil
	}
	if got := f.depth(); got > maxFilterDepth {
		return fmt.Errorf("%w: found %d levels", ErrFilterTooDeep, got)
	}
	return nil
}

// TextFilter builds conditions on a title, rich text, URL, email, or phone
// number column. The five share one condition set (TextPropertyFilter,
// common.ts) but each is keyed by its own type (common.ts:1100-1123), so the
// constructor records which kind of column the filter addresses.
type TextFilter struct {
	property string
	kind     PropertyType
}

// ByText starts a condition on a rich text column.
func ByText(property string) TextFilter {
	return TextFilter{property: property, kind: PropertyTypeRichText}
}

// ByTitle starts a condition on the title column.
func ByTitle(property string) TextFilter {
	return TextFilter{property: property, kind: PropertyTypeTitle}
}

// ByURL starts a condition on a URL column.
func ByURL(property string) TextFilter {
	return TextFilter{property: property, kind: PropertyTypeURL}
}

// ByEmail starts a condition on an email column.
func ByEmail(property string) TextFilter {
	return TextFilter{property: property, kind: PropertyTypeEmail}
}

// ByPhoneNumber starts a condition on a phone number column.
func ByPhoneNumber(property string) TextFilter {
	return TextFilter{property: property, kind: PropertyTypePhoneNumber}
}

func (f TextFilter) build(condition any) Filter {
	return propertyFilter{property: f.property, kind: f.kind, condition: condition}
}

// Equals matches text exactly equal to value.
func (f TextFilter) Equals(value string) Filter {
	return f.build(map[string]any{"equals": value})
}

// DoesNotEqual matches text not exactly equal to value.
func (f TextFilter) DoesNotEqual(value string) Filter {
	return f.build(map[string]any{"does_not_equal": value})
}

// Contains matches text containing value.
func (f TextFilter) Contains(value string) Filter {
	return f.build(map[string]any{"contains": value})
}

// DoesNotContain matches text not containing value.
func (f TextFilter) DoesNotContain(value string) Filter {
	return f.build(map[string]any{"does_not_contain": value})
}

// StartsWith matches text beginning with value.
func (f TextFilter) StartsWith(value string) Filter {
	return f.build(map[string]any{"starts_with": value})
}

// EndsWith matches text ending with value.
func (f TextFilter) EndsWith(value string) Filter {
	return f.build(map[string]any{"ends_with": value})
}

// IsEmpty matches rows where the column has no value.
func (f TextFilter) IsEmpty() Filter {
	return f.build(map[string]any{"is_empty": true})
}

// IsNotEmpty matches rows where the column has a value.
func (f TextFilter) IsNotEmpty() Filter {
	return f.build(map[string]any{"is_not_empty": true})
}

// NumberFilter builds conditions on a number column.
type NumberFilter struct{ property string }

// ByNumber starts a condition on a number column.
func ByNumber(property string) NumberFilter { return NumberFilter{property: property} }

func (f NumberFilter) build(condition any) Filter {
	return propertyFilter{property: f.property, kind: PropertyTypeNumber, condition: condition}
}

// Equals matches numbers equal to value.
func (f NumberFilter) Equals(value float64) Filter {
	return f.build(map[string]any{"equals": value})
}

// DoesNotEqual matches numbers not equal to value.
func (f NumberFilter) DoesNotEqual(value float64) Filter {
	return f.build(map[string]any{"does_not_equal": value})
}

// GreaterThan matches numbers strictly greater than value.
func (f NumberFilter) GreaterThan(value float64) Filter {
	return f.build(map[string]any{"greater_than": value})
}

// LessThan matches numbers strictly less than value.
func (f NumberFilter) LessThan(value float64) Filter {
	return f.build(map[string]any{"less_than": value})
}

// GreaterThanOrEqualTo matches numbers at least value.
func (f NumberFilter) GreaterThanOrEqualTo(value float64) Filter {
	return f.build(map[string]any{"greater_than_or_equal_to": value})
}

// LessThanOrEqualTo matches numbers at most value.
func (f NumberFilter) LessThanOrEqualTo(value float64) Filter {
	return f.build(map[string]any{"less_than_or_equal_to": value})
}

// IsEmpty matches rows where the column has no value.
func (f NumberFilter) IsEmpty() Filter {
	return f.build(map[string]any{"is_empty": true})
}

// IsNotEmpty matches rows where the column has a value.
func (f NumberFilter) IsNotEmpty() Filter {
	return f.build(map[string]any{"is_not_empty": true})
}

// CheckboxFilter builds conditions on a checkbox column.
type CheckboxFilter struct{ property string }

// ByCheckbox starts a condition on a checkbox column.
func ByCheckbox(property string) CheckboxFilter { return CheckboxFilter{property: property} }

func (f CheckboxFilter) build(condition any) Filter {
	return propertyFilter{property: f.property, kind: PropertyTypeCheckbox, condition: condition}
}

// Equals matches rows whose checkbox is value. false is sent literally, never
// omitted.
func (f CheckboxFilter) Equals(value bool) Filter {
	return f.build(map[string]any{"equals": value})
}

// DoesNotEqual matches rows whose checkbox is not value (common.ts:294).
func (f CheckboxFilter) DoesNotEqual(value bool) Filter {
	return f.build(map[string]any{"does_not_equal": value})
}

// SelectFilter builds conditions on a select or status column.
type SelectFilter struct {
	property string
	kind     PropertyType
}

// BySelect starts a condition on a single-choice column.
func BySelect(property string) SelectFilter {
	return SelectFilter{property: property, kind: PropertyTypeSelect}
}

// ByStatus starts a condition on a status column.
func ByStatus(property string) SelectFilter {
	return SelectFilter{property: property, kind: PropertyTypeStatus}
}

func (f SelectFilter) build(condition any) Filter {
	return propertyFilter{property: f.property, kind: f.kind, condition: condition}
}

// Equals matches rows whose option is named value.
func (f SelectFilter) Equals(value string) Filter {
	return f.build(map[string]any{"equals": value})
}

// DoesNotEqual matches rows whose option is not named value.
func (f SelectFilter) DoesNotEqual(value string) Filter {
	return f.build(map[string]any{"does_not_equal": value})
}

// IsEmpty matches rows with no option selected.
func (f SelectFilter) IsEmpty() Filter {
	return f.build(map[string]any{"is_empty": true})
}

// IsNotEmpty matches rows with an option selected.
func (f SelectFilter) IsNotEmpty() Filter {
	return f.build(map[string]any{"is_not_empty": true})
}

// MultiSelectFilter builds conditions on a multi-select column.
type MultiSelectFilter struct{ property string }

// ByMultiSelect starts a condition on a multiple-choice column.
func ByMultiSelect(property string) MultiSelectFilter {
	return MultiSelectFilter{property: property}
}

func (f MultiSelectFilter) build(condition any) Filter {
	return propertyFilter{property: f.property, kind: PropertyTypeMultiSelect, condition: condition}
}

// Contains matches rows whose selection includes the option named value.
func (f MultiSelectFilter) Contains(value string) Filter {
	return f.build(map[string]any{"contains": value})
}

// DoesNotContain matches rows whose selection excludes the option named value.
func (f MultiSelectFilter) DoesNotContain(value string) Filter {
	return f.build(map[string]any{"does_not_contain": value})
}

// IsEmpty matches rows with nothing selected.
func (f MultiSelectFilter) IsEmpty() Filter {
	return f.build(map[string]any{"is_empty": true})
}

// IsNotEmpty matches rows with something selected.
func (f MultiSelectFilter) IsNotEmpty() Filter {
	return f.build(map[string]any{"is_not_empty": true})
}

// DateFilter builds conditions on a date column or a built-in timestamp.
type DateFilter struct {
	property  string
	kind      PropertyType
	timestamp string
}

// ByDate starts a condition on a date column.
func ByDate(property string) DateFilter {
	return DateFilter{property: property, kind: PropertyTypeDate}
}

// ByCreatedTime starts a condition on each page's creation time, which every data
// source has whether or not it exposes a column for it.
func ByCreatedTime() DateFilter {
	return DateFilter{kind: PropertyTypeCreatedTime, timestamp: "created_time"}
}

// ByLastEditedTime starts a condition on each page's last edit time.
func ByLastEditedTime() DateFilter {
	return DateFilter{kind: PropertyTypeLastEditedTime, timestamp: "last_edited_time"}
}

func (f DateFilter) build(condition any) Filter {
	return propertyFilter{
		property:  f.property,
		kind:      f.kind,
		condition: condition,
		timestamp: f.timestamp,
	}
}

func formatFilterTime(t time.Time) string { return t.Format(time.RFC3339) }

// Equals matches the given date.
func (f DateFilter) Equals(t time.Time) Filter {
	return f.build(map[string]any{"equals": formatFilterTime(t)})
}

// Before matches dates strictly before t.
func (f DateFilter) Before(t time.Time) Filter {
	return f.build(map[string]any{"before": formatFilterTime(t)})
}

// After matches dates strictly after t.
func (f DateFilter) After(t time.Time) Filter {
	return f.build(map[string]any{"after": formatFilterTime(t)})
}

// OnOrBefore matches dates at or before t.
func (f DateFilter) OnOrBefore(t time.Time) Filter {
	return f.build(map[string]any{"on_or_before": formatFilterTime(t)})
}

// OnOrAfter matches dates at or after t.
func (f DateFilter) OnOrAfter(t time.Time) Filter {
	return f.build(map[string]any{"on_or_after": formatFilterTime(t)})
}

// PastWeek matches dates in the past week, relative to the server's clock.
func (f DateFilter) PastWeek() Filter {
	return f.build(map[string]any{"past_week": EmptyObject{}})
}

// PastMonth matches dates in the past month.
func (f DateFilter) PastMonth() Filter {
	return f.build(map[string]any{"past_month": EmptyObject{}})
}

// PastYear matches dates in the past year.
func (f DateFilter) PastYear() Filter {
	return f.build(map[string]any{"past_year": EmptyObject{}})
}

// NextWeek matches dates in the coming week.
func (f DateFilter) NextWeek() Filter {
	return f.build(map[string]any{"next_week": EmptyObject{}})
}

// NextMonth matches dates in the coming month.
func (f DateFilter) NextMonth() Filter {
	return f.build(map[string]any{"next_month": EmptyObject{}})
}

// NextYear matches dates in the coming year.
func (f DateFilter) NextYear() Filter {
	return f.build(map[string]any{"next_year": EmptyObject{}})
}

// ThisWeek matches dates in the current week.
func (f DateFilter) ThisWeek() Filter {
	return f.build(map[string]any{"this_week": EmptyObject{}})
}

// IsEmpty matches rows with no date set.
func (f DateFilter) IsEmpty() Filter {
	return f.build(map[string]any{"is_empty": true})
}

// IsNotEmpty matches rows with a date set.
func (f DateFilter) IsNotEmpty() Filter {
	return f.build(map[string]any{"is_not_empty": true})
}

// PeopleFilter builds conditions on a people, created-by, or last-edited-by
// column.
type PeopleFilter struct {
	property string
	kind     PropertyType
}

// ByPeople starts a condition on a people column.
func ByPeople(property string) PeopleFilter {
	return PeopleFilter{property: property, kind: PropertyTypePeople}
}

// ByCreatedBy starts a condition on each page's author.
func ByCreatedBy(property string) PeopleFilter {
	return PeopleFilter{property: property, kind: PropertyTypeCreatedBy}
}

// ByLastEditedBy starts a condition on each page's last editor
// (common.ts:1130-1134).
func ByLastEditedBy(property string) PeopleFilter {
	return PeopleFilter{property: property, kind: PropertyTypeLastEditedBy}
}

func (f PeopleFilter) build(condition any) Filter {
	return propertyFilter{property: f.property, kind: f.kind, condition: condition}
}

// Contains matches rows listing the given user ID.
func (f PeopleFilter) Contains(userID string) Filter {
	return f.build(map[string]any{"contains": userID})
}

// DoesNotContain matches rows not listing the given user ID.
func (f PeopleFilter) DoesNotContain(userID string) Filter {
	return f.build(map[string]any{"does_not_contain": userID})
}

// IsEmpty matches rows with nobody assigned.
func (f PeopleFilter) IsEmpty() Filter {
	return f.build(map[string]any{"is_empty": true})
}

// IsNotEmpty matches rows with somebody assigned.
func (f PeopleFilter) IsNotEmpty() Filter {
	return f.build(map[string]any{"is_not_empty": true})
}

// FilesFilter builds conditions on a files column, which supports only
// existence checks (common.ts:1118).
type FilesFilter struct{ property string }

// ByFiles starts a condition on a files column.
func ByFiles(property string) FilesFilter { return FilesFilter{property: property} }

func (f FilesFilter) build(condition any) Filter {
	return propertyFilter{property: f.property, kind: PropertyTypeFiles, condition: condition}
}

// IsEmpty matches rows with no files attached.
func (f FilesFilter) IsEmpty() Filter {
	return f.build(map[string]any{"is_empty": true})
}

// IsNotEmpty matches rows with at least one file attached.
func (f FilesFilter) IsNotEmpty() Filter {
	return f.build(map[string]any{"is_not_empty": true})
}

// RelationFilter builds conditions on a relation column.
type RelationFilter struct{ property string }

// ByRelation starts a condition on a relation column.
func ByRelation(property string) RelationFilter { return RelationFilter{property: property} }

func (f RelationFilter) build(condition any) Filter {
	return propertyFilter{property: f.property, kind: PropertyTypeRelation, condition: condition}
}

// Contains matches rows relating to the given page ID.
func (f RelationFilter) Contains(pageID string) Filter {
	return f.build(map[string]any{"contains": pageID})
}

// DoesNotContain matches rows not relating to the given page ID.
func (f RelationFilter) DoesNotContain(pageID string) Filter {
	return f.build(map[string]any{"does_not_contain": pageID})
}

// IsEmpty matches rows with no relations.
func (f RelationFilter) IsEmpty() Filter {
	return f.build(map[string]any{"is_empty": true})
}

// IsNotEmpty matches rows with at least one relation.
func (f RelationFilter) IsNotEmpty() Filter {
	return f.build(map[string]any{"is_not_empty": true})
}

// SortDirection is the order a sort applies.
type SortDirection string

const (
	Ascending  SortDirection = "ascending"
	Descending SortDirection = "descending"
)

// Sort orders query results. Set exactly one of Property or Timestamp.
type Sort struct {
	// Property is the column to sort by.
	Property string `json:"property,omitempty"`
	// Timestamp sorts by a built-in field: "created_time" or
	// "last_edited_time".
	Timestamp string        `json:"timestamp,omitempty"`
	Direction SortDirection `json:"direction"`
}

// SortBy orders results by a column.
func SortBy(property string, direction SortDirection) Sort {
	return Sort{Property: property, Direction: direction}
}

// SortByCreatedTime orders results by when each page was created.
func SortByCreatedTime(direction SortDirection) Sort {
	return Sort{Timestamp: "created_time", Direction: direction}
}

// SortByLastEditedTime orders results by when each page was last edited.
func SortByLastEditedTime(direction SortDirection) Sort {
	return Sort{Timestamp: "last_edited_time", Direction: direction}
}
