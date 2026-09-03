package notion

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// This file holds the codec shared by every tagged union in the API.
//
// Notion encodes a union as a "type" discriminant plus a sibling key named
// after the tag value, alongside whatever envelope fields the object carries:
//
//	{"object":"block","id":"…","type":"paragraph","paragraph":{"rich_text":[]}}
//
// Each variant is a struct embedding the envelope and holding the payload in a
// field named for the tag, so encoding and decoding are two mechanical calls to
// wrap and unwrap.

// tagOf reads the "type" discriminant. A value with no discriminant yields the
// empty string rather than an error, since some payload keys are optional.
func tagOf(data []byte) (string, error) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return "", err
	}
	return probe.Type, nil
}

// unwrap decodes a tagged union: envelope fields come from the top-level
// object, payload fields from the sibling key named by the discriminant.
// Either target may be nil to skip it.
func unwrap(data []byte, envelope, payload any) error {
	if envelope != nil {
		if err := json.Unmarshal(data, envelope); err != nil {
			return err
		}
	}
	if payload == nil {
		return nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	var tag string
	if raw, ok := fields["type"]; ok {
		if err := json.Unmarshal(raw, &tag); err != nil {
			return err
		}
	}
	// Notion omits the payload key for variants that carry no data, which
	// leaves the zero value in place.
	if raw, ok := fields[tag]; ok {
		return json.Unmarshal(raw, payload)
	}
	return nil
}

// wrap is the inverse of unwrap: it merges the envelope's fields with the
// discriminant and the payload nested under a key named for it.
func wrap(envelope any, tag string, payload any) ([]byte, error) {
	fields := map[string]json.RawMessage{}
	if envelope != nil {
		raw, err := json.Marshal(envelope)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, err
		}
	}

	payloadRaw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	tagRaw, err := json.Marshal(tag)
	if err != nil {
		return nil, err
	}
	fields["type"] = tagRaw
	fields[tag] = payloadRaw
	return json.Marshal(fields)
}

// orEmpty returns s, or an empty non-nil slice when s is nil, so a field the
// API requires to be an array encodes as [] rather than null.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// decodeUnion builds the variant registered for the value's discriminant.
//
// An unrecognized tag is not an error: it produces the union's Unknown variant,
// which retains the raw JSON. Notion adds variants continuously, and decoding a
// workspace that uses a newer one must not fail.
func decodeUnion[T any](data []byte, registry map[string]func() T, unknown func(tag string, raw []byte) T) (T, error) {
	var zero T
	tag, err := tagOf(data)
	if err != nil {
		return zero, fmt.Errorf("reading type discriminant: %w", err)
	}
	newVariant, ok := registry[tag]
	if !ok {
		return unknown(tag, bytes.Clone(data)), nil
	}
	variant := newVariant()
	if err := json.Unmarshal(data, variant); err != nil {
		return zero, fmt.Errorf("decoding %q: %w", tag, err)
	}
	return variant, nil
}

// decodeUnionSlice decodes a JSON array whose elements are all members of one
// union. Interface-typed slices cannot decode themselves, so every slice of a
// union is a named type whose UnmarshalJSON calls this.
func decodeUnionSlice[T any](data []byte, registry map[string]func() T, unknown func(tag string, raw []byte) T) ([]T, error) {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return nil, err
	}
	out := make([]T, len(raws))
	for i, raw := range raws {
		v, err := decodeUnion(raw, registry, unknown)
		if err != nil {
			return nil, fmt.Errorf("index %d: %w", i, err)
		}
		out[i] = v
	}
	return out, nil
}
