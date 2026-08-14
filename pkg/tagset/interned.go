// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"unique"

	"github.com/twmb/murmur3"
)

// Handle is a handle to a canonical, deduplicated copy of a string, as issued by
// the standard library `unique` package. Two handles are equal if and only if the
// strings they were made from are equal, which makes handle comparison a pointer
// comparison instead of a memcmp, and a handle 8 bytes instead of a 16 byte
// string header.
//
// The runtime owns the string a handle refers to and keeps it alive for exactly
// as long as some handle still refers to it.
type Handle = unique.Handle[string]

// InternedTag is an interned tag: a handle to the canonical copy of the tag
// string, paired with the murmur3 hash that tagset uses as tag identity.
//
// Interning gives us a stable identity to memoize the hash against, so a tag is
// hashed once for as long as it is referenced anywhere, rather than once per
// metric sample that carries it. This is what lets samples reach the aggregator
// without being re-hashed: see HashingTagsAccumulator.AppendInterned.
//
// An InternedTag is 16 bytes, the same width as the string header it replaces.
//
// The zero value is a valid empty tag: Value returns "" rather than panicking.
type InternedTag struct {
	handle Handle
	hash   uint64
}

// Intern returns the InternedTag for s, hashing it in the process.
//
// This is the expensive path: it does a lookup in (and possibly an insert into)
// the runtime's canonical map on top of hashing. Interning is worth it for a
// string that will be seen again or retained; it is a poor trade for a
// high-cardinality value seen once. Hot paths should hold on to the InternedTag
// rather than re-interning per sample.
func Intern(s string) InternedTag {
	return InternedTag{
		handle: unique.Make(s),
		hash:   murmur3.StringSum64(s),
	}
}

// InternBytes returns the InternedTag for the string form of b.
//
// Note that unlike a map lookup keyed on string(b), this allocates a string for
// b. Callers on a hot path should front this with a cache keyed by the bytes.
func InternBytes(b []byte) InternedTag {
	return Intern(string(b))
}

// InternAll interns a slice of plain strings.
func InternAll(tags []string) []InternedTag {
	if tags == nil {
		return nil
	}
	out := make([]InternedTag, len(tags))
	for i, t := range tags {
		out[i] = Intern(t)
	}
	return out
}

// Value returns the canonical copy of the tag string. It does not allocate: the
// string data is owned by the runtime and kept alive by the handle.
func (t InternedTag) Value() string {
	if t.handle == (Handle{}) {
		return ""
	}
	return t.handle.Value()
}

// Handle returns the handle to the canonical copy of the tag string.
func (t InternedTag) Handle() Handle {
	return t.handle
}

// Hash returns the precomputed murmur3 hash of the tag. It matches
// murmur3.StringSum64(t.Value()).
func (t InternedTag) Hash() uint64 {
	return t.hash
}

// String implements fmt.Stringer so interned tags render as their value in logs.
func (t InternedTag) String() string {
	return t.Value()
}

// Values resolves interned tags into a plain string slice.
func Values(tags []InternedTag) []string {
	if tags == nil {
		return nil
	}
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = t.Value()
	}
	return out
}

// HandleValues resolves handles into a plain string slice. This is the boundary
// where a context's tags become ordinary strings again, on the way to a serie
// and then the serializer.
//
// The result is never nil, so that an empty tag set resolves the same way a copy
// out of a tags accumulator used to.
func HandleValues(handles []Handle) []string {
	out := make([]string, len(handles))
	for i, h := range handles {
		out[i] = h.Value()
	}
	return out
}
