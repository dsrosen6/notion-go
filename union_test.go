package notion

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readFixture returns the contents of a file under testdata.
func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return data
}

// assertRoundTrip checks that a decoded value survives encoding and decoding
// unchanged, and that encoding did not silently drop a field.
//
// It compares encoded forms rather than Go values, because an absent JSON array
// decodes to a nil slice and an empty one to an empty slice, a difference that
// is not a bug. To catch the difference that is — a field this package fails to
// model, and therefore drops — it also checks that every key the fixture set to
// a meaningful value survives the encode.
func assertRoundTrip[T any](t *testing.T, data []byte, decode func([]byte) (T, error)) T {
	t.Helper()

	first, err := decode(data)
	if err != nil {
		t.Fatalf("first decode: %v", err)
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	second, err := decode(encoded)
	if err != nil {
		t.Fatalf("second decode of %s: %v", encoded, err)
	}
	reencoded, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	if string(encoded) != string(reencoded) {
		t.Errorf("round trip is not stable\n first: %s\nsecond: %s", encoded, reencoded)
	}

	assertNoFieldsDropped(t, data, encoded)
	return first
}

// deprecatedKeys are fields the API still sends that this package deliberately
// does not model, so dropping them is correct rather than a bug.
var deprecatedKeys = map[string]bool{
	// Superseded by in_trash in API version 2026-03-11.
	"archived": true,
}

// isEmptyJSON reports whether a value carries no information, and so is
// legitimately omitted when re-encoded.
func isEmptyJSON(value json.RawMessage) bool {
	switch strings.TrimSpace(string(value)) {
	case "null", "false", "0", `""`, "[]", "{}":
		return true
	default:
		return false
	}
}

// assertNoFieldsDropped reports any field present in the original with a
// meaningful value but missing after an encode. It walks the whole tree, since
// most of what this package models is nested several levels down.
func assertNoFieldsDropped(t *testing.T, original, encoded []byte) {
	t.Helper()
	compareJSON(t, original, encoded, "")
}

// compareJSON walks two JSON values in parallel, reporting keys that the
// encoded form lost. path names the position for the failure message.
func compareJSON(t *testing.T, original, encoded json.RawMessage, path string) {
	t.Helper()

	var before map[string]json.RawMessage
	if err := json.Unmarshal(original, &before); err == nil {
		var after map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &after); err != nil {
			t.Errorf("%s: was an object, encoded as %s", displayPath(path), encoded)
			return
		}
		for key, value := range before {
			if deprecatedKeys[key] || isEmptyJSON(value) {
				continue
			}
			got, ok := after[key]
			if !ok {
				t.Errorf("%s was dropped\n before: %s", displayPath(path+"."+key), value)
				continue
			}
			compareJSON(t, value, got, path+"."+key)
		}
		return
	}

	var beforeItems []json.RawMessage
	if err := json.Unmarshal(original, &beforeItems); err == nil {
		var afterItems []json.RawMessage
		if err := json.Unmarshal(encoded, &afterItems); err != nil {
			t.Errorf("%s: was an array, encoded as %s", displayPath(path), encoded)
			return
		}
		if len(beforeItems) != len(afterItems) {
			t.Errorf("%s: had %d items, encoded with %d", displayPath(path), len(beforeItems), len(afterItems))
			return
		}
		for i := range beforeItems {
			compareJSON(t, beforeItems[i], afterItems[i], fmt.Sprintf("%s[%d]", path, i))
		}
	}
	// Scalars need no comparison: reaching one means the key survived, and
	// value changes are covered by the fixture assertions.
}

func displayPath(path string) string {
	if path == "" {
		return "the root object"
	}
	return "field " + strings.TrimPrefix(path, ".")
}

func TestWrapUnwrap(t *testing.T) {
	type envelope struct {
		Object string `json:"object"`
		ID     string `json:"id"`
	}
	type payload struct {
		Content string `json:"content"`
	}

	encoded, err := wrap(envelope{Object: "block", ID: "b1"}, "paragraph", payload{Content: "hi"})
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}

	want := `{"id":"b1","object":"block","paragraph":{"content":"hi"},"type":"paragraph"}`
	if string(encoded) != want {
		t.Errorf("wrap produced %s, want %s", encoded, want)
	}

	var gotEnvelope envelope
	var gotPayload payload
	if err := unwrap(encoded, &gotEnvelope, &gotPayload); err != nil {
		t.Fatalf("unwrap: %v", err)
	}
	if gotEnvelope.ID != "b1" || gotEnvelope.Object != "block" {
		t.Errorf("envelope = %+v, want {block b1}", gotEnvelope)
	}
	if gotPayload.Content != "hi" {
		t.Errorf("payload = %+v, want {hi}", gotPayload)
	}
}

func TestUnwrapMissingPayloadKey(t *testing.T) {
	// Notion omits the payload key for variants carrying no data, which must
	// leave the zero value rather than fail.
	type payload struct {
		Content string `json:"content"`
	}
	var got payload
	if err := unwrap([]byte(`{"type":"divider"}`), nil, &got); err != nil {
		t.Fatalf("unwrap: %v", err)
	}
	if got.Content != "" {
		t.Errorf("payload = %+v, want the zero value", got)
	}
}

func TestDecodeUnionUnknownTag(t *testing.T) {
	// An unrecognized variant must decode rather than fail, so a workspace
	// using a newer Notion feature stays readable.
	raw := []byte(`{"type":"quantum_block","quantum_block":{"spin":"up"},"plain_text":"?"}`)
	got, err := DecodeRichText(raw)
	if err != nil {
		t.Fatalf("DecodeRichText: %v", err)
	}
	unknown, ok := got.(*UnknownRichText)
	if !ok {
		t.Fatalf("got %T, want *UnknownRichText", got)
	}
	if unknown.Type != "quantum_block" {
		t.Errorf("Type = %q, want quantum_block", unknown.Type)
	}
	if unknown.Common().PlainText != "?" {
		t.Errorf("PlainText = %q, want ?", unknown.Common().PlainText)
	}

	// The raw JSON must survive re-encoding untouched.
	encoded, err := json.Marshal(unknown)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(encoded) != string(raw) {
		t.Errorf("re-encoded %s, want the original %s", encoded, raw)
	}
}
