package notion

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRichTextFixtures(t *testing.T) {
	tests := []struct {
		fixture string
		want    RichTextType
		check   func(*testing.T, RichText)
	}{
		{
			fixture: "richtext_text.json",
			want:    RichTextTypeText,
			check: func(t *testing.T, rt RichText) {
				text := rt.(*Text)
				if text.Text.Content != "Hello " {
					t.Errorf("Content = %q, want %q", text.Text.Content, "Hello ")
				}
				if text.Text.Link != nil {
					t.Errorf("Link = %+v, want nil", text.Text.Link)
				}
				if !text.Annotations.Bold {
					t.Error("Bold = false, want true")
				}
			},
		},
		{
			fixture: "richtext_link.json",
			want:    RichTextTypeText,
			check: func(t *testing.T, rt RichText) {
				text := rt.(*Text)
				if text.Text.Link == nil || text.Text.Link.URL != "https://notion.so" {
					t.Errorf("Link = %+v, want https://notion.so", text.Text.Link)
				}
				if text.Common().Href != "https://notion.so" {
					t.Errorf("Href = %q, want https://notion.so", text.Common().Href)
				}
				if text.Annotations.Color != ColorBlue {
					t.Errorf("Color = %q, want blue", text.Annotations.Color)
				}
			},
		},
		{
			fixture: "richtext_mention_user.json",
			want:    RichTextTypeMention,
			check: func(t *testing.T, rt RichText) {
				mention := rt.(*Mention)
				if mention.Mention.Type != MentionTypeUser {
					t.Fatalf("mention type = %q, want user", mention.Mention.Type)
				}
				user := mention.Mention.User
				if user == nil || user.Name != "Ada" {
					t.Fatalf("User = %+v, want Ada", user)
				}
				if !user.IsFull() {
					t.Error("IsFull = false, want true")
				}
				if user.Person == nil || user.Person.Email != "ada@example.com" {
					t.Errorf("Person = %+v, want ada@example.com", user.Person)
				}
			},
		},
		{
			fixture: "richtext_mention_date.json",
			want:    RichTextTypeMention,
			check: func(t *testing.T, rt RichText) {
				mention := rt.(*Mention)
				date := mention.Mention.Date
				if date == nil {
					t.Fatal("Date = nil")
				}
				// A date-only value must not be reported as carrying a time.
				if date.Start.HasTime {
					t.Error("HasTime = true, want false for a date-only value")
				}
				want := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
				if !date.Start.Time.Equal(want) {
					t.Errorf("Start = %v, want %v", date.Start.Time, want)
				}
				if date.End != nil {
					t.Errorf("End = %v, want nil", date.End)
				}
			},
		},
		{
			fixture: "richtext_equation.json",
			want:    RichTextTypeEquation,
			check: func(t *testing.T, rt RichText) {
				eq := rt.(*Equation)
				if eq.Equation.Expression != `e^{i\pi} + 1 = 0` {
					t.Errorf("Expression = %q", eq.Equation.Expression)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			data := readFixture(t, tt.fixture)
			got := assertRoundTrip(t, data, DecodeRichText)
			if got.RichTextType() != tt.want {
				t.Fatalf("RichTextType = %q, want %q", got.RichTextType(), tt.want)
			}
			tt.check(t, got)
		})
	}
}

func TestRichTextListPlainText(t *testing.T) {
	// Notion splits text at every styling change, so reassembling the runs is
	// the only way to read a title.
	raw := []byte(`[
		{"type":"text","text":{"content":"Hello "},"plain_text":"Hello ","annotations":{"color":"default"}},
		{"type":"text","text":{"content":"world"},"plain_text":"world","annotations":{"bold":true,"color":"default"}}
	]`)

	var list RichTextList
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d runs, want 2", len(list))
	}
	if got := list.PlainText(); got != "Hello world" {
		t.Errorf("PlainText = %q, want %q", got, "Hello world")
	}
}

func TestRichTextListEmpty(t *testing.T) {
	var list RichTextList
	if err := json.Unmarshal([]byte(`[]`), &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := list.PlainText(); got != "" {
		t.Errorf("PlainText = %q, want empty", got)
	}
}

func TestNewRichText(t *testing.T) {
	list := NewRichText("hi")
	encoded, err := json.Marshal(list)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back RichTextList
	if err := json.Unmarshal(encoded, &back); err != nil {
		t.Fatalf("unmarshal %s: %v", encoded, err)
	}
	if got := back.PlainText(); got != "hi" {
		t.Errorf("PlainText = %q, want hi", got)
	}
}

func TestDateTimeRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		hasTime bool
	}{
		{"date only", `"2026-09-03"`, false},
		{"rfc3339 with zone", `"2026-09-03T12:30:00.000Z"`, true},
		{"rfc3339 with offset", `"2026-09-03T12:30:00-04:00"`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DateTime
			if err := json.Unmarshal([]byte(tt.json), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.HasTime != tt.hasTime {
				t.Errorf("HasTime = %v, want %v", got.HasTime, tt.hasTime)
			}

			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var back DateTime
			if err := json.Unmarshal(encoded, &back); err != nil {
				t.Fatalf("re-unmarshal %s: %v", encoded, err)
			}
			if !back.Time.Equal(got.Time) || back.HasTime != got.HasTime {
				t.Errorf("round trip gave %v (hasTime %v), want %v (hasTime %v)",
					back.Time, back.HasTime, got.Time, got.HasTime)
			}
		})
	}
}

func TestDateTimeInvalid(t *testing.T) {
	var got DateTime
	if err := json.Unmarshal([]byte(`"not a date"`), &got); err == nil {
		t.Error("expected an error for an unparseable date")
	}
}

func TestParentID(t *testing.T) {
	tests := []struct {
		parent Parent
		want   string
	}{
		{Parent{Type: ParentTypePage, PageID: "p1"}, "p1"},
		{Parent{Type: ParentTypeDatabase, DatabaseID: "d1"}, "d1"},
		{Parent{Type: ParentTypeDataSource, DataSourceID: "ds1", DatabaseID: "d1"}, "ds1"},
		{Parent{Type: ParentTypeBlock, BlockID: "b1"}, "b1"},
		{Parent{Type: ParentTypeWorkspace, Workspace: true}, ""},
	}
	for _, tt := range tests {
		if got := tt.parent.ID(); got != tt.want {
			t.Errorf("Parent{%s}.ID() = %q, want %q", tt.parent.Type, got, tt.want)
		}
	}
}

func TestSameID(t *testing.T) {
	// The API accepts and returns both forms, so comparing strings directly is
	// unreliable.
	dashed := "11111111-2222-3333-4444-555555555555"
	plain := "11111111222233334444555555555555"
	if !SameID(dashed, plain) {
		t.Error("SameID(dashed, plain) = false, want true")
	}
	if !SameID(dashed, "11111111-2222-3333-4444-555555555555") {
		t.Error("SameID with itself = false, want true")
	}
	if SameID(dashed, "99999999-2222-3333-4444-555555555555") {
		t.Error("SameID with a different ID = true, want false")
	}
}

func TestFileURL(t *testing.T) {
	hosted := File{Type: FileTypeFile, File: &FileRef{URL: "https://s3/signed"}}
	external := NewExternalFile("https://example.com/x.png")

	if got := hosted.URL(); got != "https://s3/signed" {
		t.Errorf("hosted URL = %q", got)
	}
	if got := external.URL(); got != "https://example.com/x.png" {
		t.Errorf("external URL = %q", got)
	}
	if got := (File{}).URL(); got != "" {
		t.Errorf("empty File URL = %q, want empty", got)
	}
}

func TestRichTextListNilEncodesAsArray(t *testing.T) {
	var list RichTextList
	encoded, err := json.Marshal(list)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(encoded) != "[]" {
		t.Errorf("nil list = %s, want []", encoded)
	}
	// Decoding is unaffected.
	var back RichTextList
	if err := json.Unmarshal(encoded, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back) != 0 {
		t.Errorf("got %d runs, want 0", len(back))
	}
}

func TestRequestOnlyFieldsOmitEmptyStrings(t *testing.T) {
	// A request may carry just an ID or name; the fields the API fills in on
	// the way back must not be sent as empty strings.
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"partial user", User{ID: "u1"}, `{"id":"u1"}`},
		{"noticon without color", Noticon{Name: "pizza"}, `{"name":"pizza"}`},
		{"custom emoji by id", CustomEmoji{ID: "e1"}, `{"id":"e1"}`},
		{"workspace parent", Parent{Type: ParentTypeWorkspace}, `{"type":"workspace","workspace":true}`},
		{"page parent", Parent{Type: ParentTypePage, PageID: "p1"}, `{"type":"page_id","page_id":"p1"}`},
		{
			"named external file",
			NewNamedExternalFile("a.png", "https://example.com/a.png"),
			`{"type":"external","external":{"url":"https://example.com/a.png"},"name":"a.png"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(encoded) != tt.want {
				t.Errorf("got  %s\nwant %s", encoded, tt.want)
			}
		})
	}
}

func TestBotWorkspaceID(t *testing.T) {
	raw := []byte(`{"object":"user","id":"b1","type":"bot","name":"Robot",` +
		`"bot":{"owner":{"type":"workspace","workspace":true},"workspace_name":"Acme","workspace_id":"ws-1"}}`)
	decode := func(data []byte) (User, error) {
		var u User
		err := json.Unmarshal(data, &u)
		return u, err
	}
	user := assertRoundTrip(t, raw, decode)
	if user.Bot == nil || user.Bot.WorkspaceID != "ws-1" {
		t.Errorf("Bot = %+v, want workspace_id ws-1", user.Bot)
	}
}
