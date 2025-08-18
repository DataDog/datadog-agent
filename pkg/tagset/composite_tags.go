// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package tagset

import (
	"encoding/json"
	"slices"
	"strings"
	"unique"
)

// CompositeTags stores read-only views of two tag sets and provides methods to iterate them easily.
//
// CompositeTags is designed to be used for metric tags created by the aggregator (Context, Serie,
// SketchSeries, ...).
type CompositeTags struct {
	// Methods should never modify these slices without copying first.
	tags1 []unique.Handle[string]
	tags2 []unique.Handle[string]
	tags3 []string
}

// NewCompositeTags creates a new CompositeTags with the given slices.
//
// Returned value may reference the argument slices directly (or not). Callers should avoid
// modifying the slices after calling this function.
func NewCompositeTags(tags1 []unique.Handle[string], tags2 []unique.Handle[string]) CompositeTags {
	return CompositeTags{
		tags1: tags1,
		tags2: tags2,
	}
}

// CompositeTagsFromSlice creates a new CompositeTags from a slice
func CompositeTagsFromSlice(tags []string) CompositeTags {
	return CompositeTags{nil, nil, tags}
}

// CombineCompositeTagsAndSlice creates a new CompositeTags from an existing CompositeTags and string slice.
//
// Returned value may reference the argument slices directly (or not). Callers should avoid
// modifying the slices after calling this function. Slices contained in compositeTags are not
// modified, but may be copied. Prefer constructing a complete value in one go with NewCompositeTags
// instead.
func CombineCompositeTagsAndSlice(compositeTags CompositeTags, tags []string) CompositeTags {
	if compositeTags.tags3 == nil {
		return CompositeTags{compositeTags.tags1, compositeTags.tags2, tags}
	}
	// Copy tags in case `CombineCompositeTagsAndSlice` is called twice with the same first argument.
	// For example see TestCompositeTagsCombineCompositeTagsAndSlice.
	newTags := slices.Concat(compositeTags.tags3, tags)
	return CompositeTags{compositeTags.tags1, compositeTags.tags2, newTags}
}

// CombineWithSlice adds tags to the composite tags. Consumes the slice.
//
// Returned value may reference the argument tags slice directly (or not). Callers should avoid
// modifying the slices after calling this function. Slices contained in t are not modified, but may
// be copied. Prefer constructing a complete value in one go with NewCompositeTags instead.
func (t *CompositeTags) CombineWithSlice(tags []string) {
	*t = CombineCompositeTagsAndSlice(*t, tags)
}

// ForEach applies `callback` to each tag
func (t CompositeTags) ForEach(callback func(tag string)) {
	for _, t := range t.tags1 {
		callback(t.Value())
	}
	for _, t := range t.tags2 {
		callback(t.Value())
	}
	for _, t := range t.tags3 {
		callback(t)
	}
}

// ForEachErr applies `callback` to each tag while `callback“ returns nil.
// The first error is returned.
func (t CompositeTags) ForEachErr(callback func(tag string) error) error {
	for _, t := range t.tags1 {
		if err := callback(t.Value()); err != nil {
			return err
		}
	}
	for _, t := range t.tags2 {
		if err := callback(t.Value()); err != nil {
			return err
		}
	}
	for _, t := range t.tags3 {
		if err := callback(t); err != nil {
			return err
		}
	}

	return nil
}

// Find returns whether `callback` returns true for a tag
func (t CompositeTags) Find(callback func(tag string) bool) bool {
	cb := func(t unique.Handle[string]) bool {
		return callback(t.Value())
	}
	if slices.ContainsFunc(t.tags1, cb) {
		return true
	}
	if slices.ContainsFunc(t.tags2, cb) {
		return true
	}
	return slices.ContainsFunc(t.tags3, callback)
}

// Len returns the length of the tags
func (t CompositeTags) Len() int {
	return len(t.tags1) + len(t.tags2) + len(t.tags3)
}

// ToSlice returns a new slice representing all strings in this instance
func (t CompositeTags) ToSlice() []string {
	tags := []string{}
	t.ForEach(func(s string) { tags = append(tags, s) })
	return tags
}

// MarshalJSON serializes a Payload to JSON
func (t CompositeTags) MarshalJSON() ([]byte, error) {
	tags := t.ToSlice()
	return json.Marshal(tags)
}

// Join performs strings.Join on tags
func (t CompositeTags) Join(separator string) string {
	b := strings.Builder{}
	t.ForEach(func(s string) {
		if b.Len() > 0 {
			b.WriteString(separator)
		}
		b.WriteString(s)
	})
	return b.String()
}

// UnmarshalJSON implements the json.Unmarshaler interface.
// UnmarshalJSON receiver need to be a pointer to modify `t`.
func (t *CompositeTags) UnmarshalJSON(b []byte) error {
	var tags []string
	if err := json.Unmarshal(b, &tags); err != nil {
		return err
	}
	// Store as non-interned strings in tags3 field
	*t = CompositeTags{nil, nil, tags}
	return nil
}
