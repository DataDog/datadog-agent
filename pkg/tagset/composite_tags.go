// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package tagset

import (
	"encoding/json"
	"strings"
)

// CompositeTags stores two tag sets and provides methods to manipulate them easily
// CompositeTags is designed to be used for metric tags created by the aggregator
// (Context, Serie, SketchSeries, ...)
type CompositeTags struct {
	tags1 []string
	tags2 []string
}

// NewCompositeTags creates a new CompositeTags
func NewCompositeTags(tags1 []string, tags2 []string) CompositeTags {
	return CompositeTags{
		tags1: tags1,
		tags2: tags2,
	}
}

// CompositeTagsFromSlice creates a new CompositeTags from a slice
func CompositeTagsFromSlice(tags []string) CompositeTags {
	return NewCompositeTags(tags, nil)
}

// CombineCompositeTagsAndSlice creates a new CompositeTags from an existing CompositeTags and string slice
// Implementation note: This function may duplicate `compositeTags.tags2`.
func CombineCompositeTagsAndSlice(compositeTags CompositeTags, tags []string) CompositeTags {
	if compositeTags.tags2 == nil {
		return NewCompositeTags(compositeTags.tags1, tags)
	}
	// Copy tags in case `CombineCompositeTagsAndSlice` is called twice with the same first argument.
	// For example see TestCompositeTagsCombineCompositeTagsAndSlice.
	newTags := append(append([]string{}, compositeTags.tags2...), tags...)
	return NewCompositeTags(compositeTags.tags1, newTags)
}

// CombineWithSlice adds tags to the composite tags. Consumes the slice.
func (t *CompositeTags) CombineWithSlice(tags []string) {
	*t = CombineCompositeTagsAndSlice(*t, tags)
}

// ForEach applies `callback` to each tag
func (t CompositeTags) ForEach(callback func(tag string)) {
	for _, t := range t.tags1 {
		callback(t)
	}
	for _, t := range t.tags2 {
		callback(t)
	}
}

// ForEachErr applies `callback` to each tag while `callback`` returns nil.
// The first error is returned.
func (t CompositeTags) ForEachErr(callback func(tag string) error) error {
	for _, t := range t.tags1 {
		if err := callback(t); err != nil {
			return err
		}
	}
	for _, t := range t.tags2 {
		if err := callback(t); err != nil {
			return err
		}
	}

	return nil
}

// Find returns whether `callback` returns true for a tag
func (t CompositeTags) Find(callback func(tag string) bool) bool {
	for _, t := range t.tags1 {
		if callback(t) {
			return true
		}
	}
	for _, t := range t.tags2 {
		if callback(t) {
			return true
		}
	}

	return false
}

// Len returns the length of the tags
func (t CompositeTags) Len() int {
	return len(t.tags1) + len(t.tags2)
}

// Join performs strings.Join on tags
func (t CompositeTags) Join(separator string) string {
	if len(t.tags2) == 0 {
		return strings.Join(t.tags1, separator)
	}
	if len(t.tags1) == 0 {
		return strings.Join(t.tags2, separator)
	}
	return strings.Join(t.tags1, separator) + separator + strings.Join(t.tags2, separator)
}

// MarshalJSON serializes a Payload to JSON
func (t CompositeTags) MarshalJSON() ([]byte, error) {
	tags := append([]string{}, t.tags1...)
	return json.Marshal(append(tags, t.tags2...))
}

// UnmarshalJSON implements the json.Unmarshaler interface.
// UnmarshalJSON receiver need to be a pointer to modify `t`.
func (t *CompositeTags) UnmarshalJSON(b []byte) error {
	t.tags2 = nil
	return json.Unmarshal(b, &t.tags1)
}

// UnsafeToReadOnlySliceString creates a new slice containing all tags.
// The caller of this method must ensure that the slice is never mutate.
// Should be used only for performance reasons.
func (t CompositeTags) UnsafeToReadOnlySliceString() []string {
	if len(t.tags2) == 0 {
		return t.tags1
	}
	return append(append([]string{}, t.tags1...), t.tags2...)
}
