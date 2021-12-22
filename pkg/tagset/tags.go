// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/twmb/murmur3"
)

// Tags contains a set of tags, along with a 64-bit hash considered to be
// unique to that set of tags. Tags in the tagset are unique. The order of the
// tags is undefined.
//
// Tags are always handled by pointer (`*tagset.Tags`).  Avoid using the nil
// form of this pointer; in cases where no tags are necessary, use
// `tagset.EmptyTags` instead.  This avoids many unnecessary nil checks.
//
// The constructor functions associated with this type use the default factory.
type Tags struct {
	// Tags is the central struct in this package, so instances are created
	// directly in many locations in the package. All must adhere to the
	// invariants described here.

	// tags are the tags contained in this tagset. They must be unique.
	tags []string

	// hashes are the hashes of the tags at the same index in the tags field
	hashes []uint64

	// hash is the xor of all values in the hashes field
	hash uint64
}

// calcHashes calculates the per-tag hashes and the overall hash for the
// given slice of tags.
func calcHashes(tags []string) (hashes []uint64, hash uint64) {
	hashes = make([]uint64, len(tags))
	for i, t := range tags {
		h := murmur3.StringSum64(t)
		hashes[i] = h
		hash ^= h
	}
	return
}

// String returns a human-readable form of this tagset.
func (tags *Tags) String() string {
	return strings.Join(tags.tags, ", ")
}

// Len returns the number of tags in the tagset
func (tags *Tags) Len() int {
	return len(tags.tags)
}

// MarshalJSON implements json.Marshaler to marshal the tag set as a JSON
// array.
func (tags *Tags) MarshalJSON() ([]byte, error) {
	return json.Marshal(tags.tags)
}

// Hash returns the hash of this tagset
func (tags *Tags) Hash() uint64 {
	return tags.hash
}

// Sorted returns a copy of the tags in this tagset, sorted. This is intended
// for ease of assertions in tests, and not for use in production code.
func (tags *Tags) Sorted() []string {
	clone := make([]string, len(tags.tags))
	copy(clone, tags.tags)
	sort.Strings(clone)
	return clone
}

// Contains checks whether the given tag is in this tagset.
func (tags *Tags) Contains(tag string) bool {
	for _, t := range tags.tags {
		if t == tag {
			return true
		}
	}
	return false
}

// IsSubsetOf checks whether this tagset is a subset of another tagset.
func (tags *Tags) IsSubsetOf(other *Tags) bool {
	for _, t := range tags.tags {
		if !other.Contains(t) {
			return false
		}
	}
	return true
}

// WithKey returns the tags in this tagset whih have the given key. This
// means all tags which begin with `key:`.
func (tags *Tags) WithKey(key string) []string {
	matches := []string{}
	pfx := key + ":"
	for _, t := range tags.tags {
		if strings.HasPrefix(t, pfx) {
			matches = append(matches, t)
		}
	}
	return matches
}

// FindByKey returns the first tag in this tagset that has the given key. If
// multiple tags have the given key, it is undefined which tag is returned.
func (tags *Tags) FindByKey(key string) string {
	pfx := key + ":"
	for _, t := range tags.tags {
		if strings.HasPrefix(t, pfx) {
			return t
		}
	}
	return ""
}

// ForEach calls the given function for each tag in the tagset.
func (tags *Tags) ForEach(each func(tag string)) {
	for _, t := range tags.tags {
		each(t)
	}
}

// UnsafeReadOnlySlice returns the slice of strings contained in this tagset. As the
// name suggests, the returned slice must not be modified, including being used
// as the first argument to `append`.
//
// All uses of this function should include a comment demonstrating that the usage is
// safe.
//
// This function should only be used to interface with external APIs that do
// not accept the Tags type.
func (tags *Tags) UnsafeReadOnlySlice() []string {
	return tags.tags
}
