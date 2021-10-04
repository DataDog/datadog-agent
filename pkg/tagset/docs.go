// Package tagset supports creation and manipulation of sets of tags.  It
// does so in a safe and efficient fashion, supporting:
//
// - consistent hashing of tagsets to recognize commonalities
// - flexible combination of tagsets from multiple sources
// - immutability to allow re-use of tagsets
//
// The package otherwise presents a fairly abstract API that allows performance
// optimizations without changing semantics.
//
// ## Builders
//
// TagsBuilder and HashingTagsBuilder both allow building tagsets bit-by-bit, by
// appending new tags.
//
// ## HashedTags
//
// The HashedTags type represents an _immutable_ set of tags and associated hashes.
// It is the primary data structure used to represent a set of tags.
package tagset
