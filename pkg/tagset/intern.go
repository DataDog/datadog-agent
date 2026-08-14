// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"time"

	"github.com/twmb/murmur3"
)

// Tag is one interned tag: a single copy of the tag string, plus the murmur3
// hash tagset uses as tag identity, computed once when the tag is first seen.
//
// Interning gives us somewhere to memoize the hash, so a tag is hashed once for
// as long as it keeps arriving rather than once per metric sample carrying it.
//
// Tags are referred to by pointer (see InternedTag), so a tag costs 8 bytes in a
// sample or a context rather than the 16 bytes of a string header, and copying a
// tag around never copies its bytes.
type Tag struct {
	value string
	hash  uint64

	// lastSeen is the Table epoch in which this tag last arrived. It is written
	// only by the goroutine that owns the Table, and is what makes the table
	// self-sizing: see Table.
	lastSeen uint32
}

// InternedTag is a reference to an interned tag. The nil value is a valid empty
// tag: Value returns "".
type InternedTag = *Tag

// Intern returns a standalone interned tag for s, not owned by any Table.
//
// Use this for the handful of tags that come from configuration rather than off
// the wire (static tags, infra mode tags, tests): they are interned once at
// startup and live forever, so they neither need nor benefit from a table.
//
// Nothing depends on two equal tags being the same *Tag — tag identity is
// (hash, value), never pointer equality — so a standalone tag mixes freely with
// table-owned ones.
func Intern(s string) InternedTag {
	return &Tag{value: s, hash: murmur3.StringSum64(s)}
}

// InternAll returns standalone interned tags for a slice of strings.
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

// Value returns the interned tag string. It never allocates: every holder of
// this tag shares the one copy.
func (t *Tag) Value() string {
	if t == nil {
		return ""
	}
	return t.value
}

// Hash returns the memoized murmur3 hash of the tag. It matches
// murmur3.StringSum64(t.Value()).
func (t *Tag) Hash() uint64 {
	if t == nil {
		return murmur3.StringSum64("")
	}
	return t.hash
}

// touch records that the tag arrived in the given epoch.
//
// The store is guarded by a comparison because a hot tag arrives many times per
// epoch and writing every time dirties one cache line per tag per sample. The
// tag has just been read, so the compare is free; skipping the write is not.
func (t *Tag) touch(epoch uint32) {
	if t.lastSeen != epoch {
		t.lastSeen = epoch
	}
}

// String implements fmt.Stringer so interned tags render as their value in logs.
func (t *Tag) String() string {
	return t.Value()
}

// Values resolves interned tags into a plain string slice. This is the boundary
// where tags become ordinary strings again, on the way to the serializer.
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

// AppendValues appends the resolved tag strings to dst.
func AppendValues(dst []string, tags []InternedTag) []string {
	for _, t := range tags {
		dst = append(dst, t.Value())
	}
	return dst
}

const (
	// tableEpochInterval is how long an epoch lasts. Eviction granularity is one
	// epoch, so this trades promptness against how often we walk the table.
	tableEpochInterval = 10 * time.Second

	// tableEpochsRetained is how many epochs a tag may go unseen before it is
	// evicted. With the interval above, a tag that stops arriving is dropped
	// after roughly 30 seconds.
	tableEpochsRetained = 3

	// tableCheckInterval is how many lookups to serve between clock reads. The
	// clock is not read per lookup: at dogstatsd rates that would show up in a
	// profile, and epoch boundaries do not need to be precise.
	tableCheckInterval = 8192
)

// Table interns tags read off the wire, so that a tag seen many times is stored
// once and hashed once.
//
// The table is self-sizing rather than capped at a configured entry count: each
// tag records the epoch it was last seen in, and a tag that has not arrived for
// tableEpochsRetained epochs is evicted. Nothing has to be released by hand, and
// no tuning knob decides how many distinct tags a workload is allowed.
//
// Dropping a tag that some in-flight sample or aggregator context still refers
// to is harmless: they hold the *Tag directly, so it stays alive and valid for
// as long as they need it. The only consequence is that if the tag comes back
// later we intern a second copy, which is the same thing the old size-capped
// interner did on reset — except that here it can only happen to a tag that went
// quiet, never to a hot one.
//
// A Table is not safe for concurrent use. Each dogstatsd worker owns one.
type Table struct {
	tags map[string]*Tag

	epoch          uint32
	lastEpochStart time.Time
	opsUntilCheck  int

	// onEvict, if set, is called with the number of tags dropped by a sweep.
	onEvict func(evicted int)
}

// NewTable returns an empty Table. sizeHint pre-allocates room for that many
// tags; it is only a hint, the table grows and shrinks with the workload.
func NewTable(sizeHint int) *Table {
	return &Table{
		tags:           make(map[string]*Tag, sizeHint),
		epoch:          1,
		lastEpochStart: time.Now(),
		opsUntilCheck:  tableCheckInterval,
	}
}

// SetEvictionCallback registers a callback invoked after each sweep that evicted
// at least one tag.
func (t *Table) SetEvictionCallback(onEvict func(evicted int)) {
	t.onEvict = onEvict
}

// LoadOrStore returns the interned tag for key, interning it if this is the
// first time the table has seen it. found reports whether it was already known.
func (t *Table) LoadOrStore(key []byte) (InternedTag, bool) {
	// The map lookup with string(key) does not allocate a string: the compiler
	// recognizes the pattern and looks up the bytes directly.
	// See https://github.com/golang/go/commit/f5f5a8b6209f84961687d993b93ea0d397f5d5bf
	if tag, ok := t.tags[string(key)]; ok {
		tag.touch(t.epoch)
		t.tick()
		return tag, true
	}

	return t.store(string(key)), false
}

// LoadOrStoreString is LoadOrStore for a key the caller already holds as a string.
func (t *Table) LoadOrStoreString(key string) (InternedTag, bool) {
	if tag, ok := t.tags[key]; ok {
		tag.touch(t.epoch)
		t.tick()
		return tag, true
	}

	return t.store(key), false
}

func (t *Table) store(key string) InternedTag {
	tag := &Tag{
		value:    key,
		hash:     murmur3.StringSum64(key),
		lastSeen: t.epoch,
	}
	t.tags[key] = tag
	t.tick()
	return tag
}

// Len returns the number of tags currently interned.
func (t *Table) Len() int {
	return len(t.tags)
}

// tick advances the epoch and sweeps once the epoch interval has elapsed. The
// clock is only read every tableCheckInterval operations.
func (t *Table) tick() {
	t.opsUntilCheck--
	if t.opsUntilCheck > 0 {
		return
	}
	t.opsUntilCheck = tableCheckInterval

	now := time.Now()
	if now.Sub(t.lastEpochStart) < tableEpochInterval {
		return
	}
	t.lastEpochStart = now
	t.advanceEpoch()
}

// advanceEpoch starts a new epoch and sweeps the tags that have now gone unseen
// for long enough.
func (t *Table) advanceEpoch() {
	t.epoch++
	t.sweep()
}

// sweep drops tags that have not been seen for tableEpochsRetained epochs.
func (t *Table) sweep() {
	if t.epoch <= tableEpochsRetained {
		return
	}
	cutoff := t.epoch - tableEpochsRetained

	evicted := 0
	for key, tag := range t.tags {
		if tag.lastSeen < cutoff {
			delete(t.tags, key)
			evicted++
		}
	}

	if evicted > 0 && t.onEvict != nil {
		t.onEvict(evicted)
	}
}
