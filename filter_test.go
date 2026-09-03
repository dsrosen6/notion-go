package notion

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// filterJSON marshals a filter and returns it as a generic map.
func filterJSON(t *testing.T, f Filter) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshal %s: %v", encoded, err)
	}
	return got
}

func TestPropertyFilterShape(t *testing.T) {
	// The API discriminates by which key is present, with the property name a
	// sibling and the condition nested under a key named for the type.
	got := filterJSON(t, ByStatus("Status").Equals("Done"))

	if got["property"] != "Status" {
		t.Errorf("property = %v, want Status", got["property"])
	}
	if got["type"] != "status" {
		t.Errorf("type = %v, want status", got["type"])
	}
	condition, ok := got["status"].(map[string]any)
	if !ok {
		t.Fatalf("status = %#v, want an object", got["status"])
	}
	if condition["equals"] != "Done" {
		t.Errorf("condition = %#v, want equals Done", condition)
	}
}

func TestTimestampFilterUsesTimestampKey(t *testing.T) {
	// Timestamp filters name a built-in field rather than a column, so they
	// carry "timestamp" instead of "property".
	got := filterJSON(t, ByCreatedTime().PastWeek())

	if _, present := got["property"]; present {
		t.Errorf("filter carries a property key: %#v", got)
	}
	if got["timestamp"] != "created_time" {
		t.Errorf("timestamp = %v, want created_time", got["timestamp"])
	}
	condition := got["created_time"].(map[string]any)
	if _, present := condition["past_week"]; !present {
		t.Errorf("condition = %#v, want past_week", condition)
	}
}

func TestExistenceFiltersUseLiteralTrue(t *testing.T) {
	// is_empty and is_not_empty must serialize as true, never omitted.
	got := filterJSON(t, ByText("Notes").IsEmpty())
	condition := got["rich_text"].(map[string]any)
	if condition["is_empty"] != true {
		t.Errorf("condition = %#v, want is_empty true", condition)
	}

	got = filterJSON(t, ByNumber("Priority").IsNotEmpty())
	condition = got["number"].(map[string]any)
	if condition["is_not_empty"] != true {
		t.Errorf("condition = %#v, want is_not_empty true", condition)
	}
}

func TestRelativeDateFiltersUseEmptyObject(t *testing.T) {
	// Relative date conditions take {} as their value, not null.
	encoded, err := json.Marshal(ByDate("Due").NextMonth())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]json.RawMessage
	json.Unmarshal(encoded, &got)

	var condition map[string]json.RawMessage
	json.Unmarshal(got["date"], &condition)
	if string(condition["next_month"]) != "{}" {
		t.Errorf("next_month = %s, want {}", condition["next_month"])
	}
}

func TestCompoundFilters(t *testing.T) {
	filter := And(
		ByStatus("Status").Equals("Done"),
		ByNumber("Priority").GreaterThan(3),
	)
	got := filterJSON(t, filter)

	clauses, ok := got["and"].([]any)
	if !ok || len(clauses) != 2 {
		t.Fatalf("and = %#v, want two clauses", got["and"])
	}
	first := clauses[0].(map[string]any)
	if first["property"] != "Status" {
		t.Errorf("first clause = %#v", first)
	}
	second := clauses[1].(map[string]any)
	condition := second["number"].(map[string]any)
	if condition["greater_than"] != float64(3) {
		t.Errorf("second clause = %#v", second)
	}
}

func TestNestedCompoundFilter(t *testing.T) {
	filter := And(
		ByCheckbox("Done").Equals(false),
		Or(
			ByStatus("Status").Equals("In progress"),
			ByStatus("Status").Equals("Blocked"),
		),
	)
	got := filterJSON(t, filter)

	clauses := got["and"].([]any)
	if len(clauses) != 2 {
		t.Fatalf("got %d clauses, want 2", len(clauses))
	}
	nested, ok := clauses[1].(map[string]any)["or"].([]any)
	if !ok || len(nested) != 2 {
		t.Errorf("nested or = %#v, want two clauses", clauses[1])
	}
}

func TestFilterDepthValidation(t *testing.T) {
	leaf := ByCheckbox("Done").Equals(true)

	tests := []struct {
		name    string
		filter  Filter
		wantErr bool
	}{
		{"bare leaf", leaf, false},
		{"one group", And(leaf, leaf), false},
		{"two groups is the limit", And(leaf, Or(leaf, leaf)), false},
		{"three groups is rejected", And(Or(And(leaf, leaf), leaf), leaf), true},
		{"no filter", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFilterDepth(tt.filter)
			if tt.wantErr != (err != nil) {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrFilterTooDeep) {
				t.Errorf("err = %v, want ErrFilterTooDeep", err)
			}
		})
	}
}

func TestDateFilterFormatsRFC3339(t *testing.T) {
	when := time.Date(2026, 9, 15, 8, 30, 0, 0, time.UTC)
	got := filterJSON(t, ByDate("Due").OnOrAfter(when))

	condition := got["date"].(map[string]any)
	if condition["on_or_after"] != "2026-09-15T08:30:00Z" {
		t.Errorf("on_or_after = %v, want RFC 3339", condition["on_or_after"])
	}
}

func TestRawFilter(t *testing.T) {
	raw := json.RawMessage(`{"property":"Custom","people":{"contains":"u1"}}`)
	got := filterJSON(t, RawFilter(raw))

	if got["property"] != "Custom" {
		t.Errorf("property = %v, want Custom", got["property"])
	}
	// A raw filter must compose with the builders.
	combined := filterJSON(t, And(RawFilter(raw), ByCheckbox("Done").Equals(true)))
	if len(combined["and"].([]any)) != 2 {
		t.Errorf("and = %#v, want two clauses", combined["and"])
	}
}

func TestSortShapes(t *testing.T) {
	encoded, _ := json.Marshal(SortBy("Priority", Descending))
	if string(encoded) != `{"property":"Priority","direction":"descending"}` {
		t.Errorf("SortBy = %s", encoded)
	}

	encoded, _ = json.Marshal(SortByCreatedTime(Ascending))
	if string(encoded) != `{"timestamp":"created_time","direction":"ascending"}` {
		t.Errorf("SortByCreatedTime = %s", encoded)
	}
}

func TestFilterBuildersEncodeExactly(t *testing.T) {
	// Each column kind is keyed and tagged by its own type, even though title,
	// rich text, URL, email, and phone number share one condition set.
	tests := []struct {
		name   string
		filter Filter
		want   string
	}{
		{"title starts_with", ByTitle("Name").StartsWith("A"), `{"property":"Name","title":{"starts_with":"A"},"type":"title"}`},
		{"title is_empty", ByTitle("Name").IsEmpty(), `{"property":"Name","title":{"is_empty":true},"type":"title"}`},
		{"rich text does_not_equal", ByText("Notes").DoesNotEqual("x"), `{"property":"Notes","rich_text":{"does_not_equal":"x"},"type":"rich_text"}`},
		{"url contains", ByURL("Site").Contains("notion"), `{"property":"Site","type":"url","url":{"contains":"notion"}}`},
		{"email ends_with", ByEmail("Mail").EndsWith("@x.com"), `{"email":{"ends_with":"@x.com"},"property":"Mail","type":"email"}`},
		{"phone is_not_empty", ByPhoneNumber("Phone").IsNotEmpty(), `{"phone_number":{"is_not_empty":true},"property":"Phone","type":"phone_number"}`},
		{"checkbox equals false", ByCheckbox("Done").Equals(false), `{"checkbox":{"equals":false},"property":"Done","type":"checkbox"}`},
		{"checkbox does_not_equal", ByCheckbox("Done").DoesNotEqual(true), `{"checkbox":{"does_not_equal":true},"property":"Done","type":"checkbox"}`},
		{"last edited by", ByLastEditedBy("Editor").Contains("u1"), `{"last_edited_by":{"contains":"u1"},"property":"Editor","type":"last_edited_by"}`},
		{"files is_empty", ByFiles("Attachments").IsEmpty(), `{"files":{"is_empty":true},"property":"Attachments","type":"files"}`},
		{"files is_not_empty", ByFiles("Attachments").IsNotEmpty(), `{"files":{"is_not_empty":true},"property":"Attachments","type":"files"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(tt.filter)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(encoded) != tt.want {
				t.Errorf("got  %s\nwant %s", encoded, tt.want)
			}
		})
	}
}
