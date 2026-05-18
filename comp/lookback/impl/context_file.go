// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lookbackimpl

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/twmb/murmur3"
)

// contextEntry holds the canonical name and sorted tags for a context key.
type contextEntry struct {
	name string
	tags []string // sorted
}

// contextStore is the storage backend for metric context mappings.
// Two implementations exist: flatContextStore (append-only binary file) and
// boltContextStore (bbolt B+tree). Both are safe for concurrent use.
type contextStore interface {
	// maybeWrite persists the context mapping if it has not been written before.
	// The bloom filter in contextFile gates this: it is only called on a new key.
	maybeWrite(key uint64, name string, tags []string) error

	// scan returns all contexts for the given metric name. If filterTags is non-nil,
	// only entries whose tags are a superset of filterTags are returned.
	scan(name string, filterTags []string) (map[uint64]contextEntry, error)

	// loadKeys iterates all stored keys and calls fn for each. Used to populate
	// the bloom filter at startup.
	loadKeys(fn func(key uint64)) error

	// close flushes and closes the underlying storage.
	close() error
}

// contextFile wraps a contextStore with a lock-free bloom filter that gates
// writes. The bloom check is O(1) and lock-free; the underlying store is only
// touched on first sight of a new key (~rare in steady state).
type contextFile struct {
	bloom *contextSet
	store contextStore
}

// newContextFile opens a boltContextStore at <dir>/contexts.db and populates
// the bloom filter from any existing entries.
func newContextFile(dir string) (*contextFile, error) {
	store, err := newBoltContextStore(filepath.Join(dir, "contexts.db"))
	if err != nil {
		return nil, err
	}
	cf := &contextFile{bloom: newContextSet(0), store: store}
	if err := store.loadKeys(func(key uint64) {
		cf.bloom.IsKnown(key) // sets bits as side-effect
	}); err != nil {
		_ = store.close()
		return nil, err
	}
	return cf, nil
}

// maybeWrite writes the context if the bloom filter reports the key as unseen.
func (cf *contextFile) maybeWrite(key uint64, name string, tags []string) error {
	if cf.bloom.IsKnown(key) {
		return nil
	}
	return cf.store.maybeWrite(key, name, tags)
}

// scan delegates to the underlying store.
func (cf *contextFile) scan(name string, filterTags []string) (map[uint64]contextEntry, error) {
	return cf.store.scan(name, filterTags)
}

// close closes the underlying store.
func (cf *contextFile) close() error {
	return cf.store.close()
}

// --- Shared tag encoding (used by both store implementations) ---
//
// Value format (tags only — key and name are stored separately):
//   tagsCount uint16
//   for each tag:
//     tagLen uint16
//     tag    []byte

func encodeTags(tags []string) []byte {
	size := 2
	for _, t := range tags {
		size += 2 + len(t)
	}
	buf := make([]byte, 0, size)
	buf = append(buf, byte(len(tags)>>8), byte(len(tags)))
	for _, t := range tags {
		buf = append(buf, byte(len(t)>>8), byte(len(t)))
		buf = append(buf, t...)
	}
	return buf
}

func decodeTags(b []byte) []string {
	if len(b) < 2 {
		return nil
	}
	count := int(b[0])<<8 | int(b[1])
	tags := make([]string, 0, count)
	pos := 2
	for range count {
		if pos+2 > len(b) {
			break
		}
		tl := int(b[pos])<<8 | int(b[pos+1])
		pos += 2
		if pos+tl > len(b) {
			break
		}
		tags = append(tags, string(b[pos:pos+tl]))
		pos += tl
	}
	return tags
}

// --- Utility functions ---

func syntheticKey(name string, sortedTags []string) uint64 {
	return murmur3.Sum64([]byte(name + "|" + strings.Join(sortedTags, ",")))
}

func sortedTagsCopy(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, len(tags))
	copy(out, tags)
	slices.Sort(out)
	return out
}

// tagsSubset reports whether every tag in requested is present in registered.
func tagsSubset(requested, registered []string) bool {
	for _, req := range requested {
		if !slices.Contains(registered, req) {
			return false
		}
	}
	return true
}
