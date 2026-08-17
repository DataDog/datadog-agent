// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"sync"
	"sync/atomic"
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

	// lastSeen is the Table epoch in which this tag last arrived, and is what
	// makes the table self-sizing: see Table.
	//
	// It is atomic because it is written on the lookup path, which runs under the
	// table's read lock and therefore concurrently across dogstatsd workers.
	// value and hash need no such protection: they are set before the tag is
	// published into the table and never change.
	lastSeen atomic.Uint32
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

// newTag builds a tag that arrived in the given epoch.
func newTag(s string, epoch uint32) *Tag {
	tag := &Tag{value: s, hash: murmur3.StringSum64(s)}
	tag.lastSeen.Store(epoch)
	return tag
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
// epoch and writing every time dirties one cache line per tag per sample — and
// with the table shared between workers, that line would bounce between cores.
// The tag has just been read, so the compare is free; skipping the write is not.
func (t *Tag) touch(epoch uint32) {
	if t.lastSeen.Load() != epoch {
		t.lastSeen.Store(epoch)
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
// A Table is safe for concurrent use and is meant to be shared by all dogstatsd
// workers, so that a tag is stored once per agent rather than once per worker.
//
// Lookups take the read lock, so workers do not serialize against each other on
// the common case of a tag that has been seen before. Only interning a new tag,
// and the periodic sweep, take the write lock.
type Table struct {
	mu   sync.RWMutex
	tags map[string]*Tag

	// epoch and opsUntilCheck are read and updated on the lookup path, under the
	// read lock, so they are atomic. lastEpochStart is only touched under the
	// write lock.
	epoch          atomic.Uint32
	opsUntilCheck  atomic.Int64
	lastEpochStart time.Time

	// onEvict, if set, is called with the number of tags dropped by a sweep. It
	// must be set before the table is shared with any worker.
	onEvict func(evicted int)
}

// NewTable returns an empty Table. sizeHint pre-allocates room for that many
// tags; it is only a hint, the table grows and shrinks with the workload.
func NewTable(sizeHint int) *Table {
	t := &Table{
		tags:           make(map[string]*Tag, sizeHint),
		lastEpochStart: time.Now(),
	}
	t.epoch.Store(1)
	t.opsUntilCheck.Store(tableCheckInterval)
	return t
}

// SetEvictionCallback registers a callback invoked after each sweep that evicted
// at least one tag. It must be called before the table is shared with any
// worker, and the callback runs without the table lock held.
func (t *Table) SetEvictionCallback(onEvict func(evicted int)) {
	t.onEvict = onEvict
}

// LoadOrStore returns the interned tag for key, interning it if this is the
// first time the table has seen it. found reports whether it was already known.
func (t *Table) LoadOrStore(key []byte) (InternedTag, bool) {
	// The map lookup with string(key) does not allocate a string: the compiler
	// recognizes the pattern and looks up the bytes directly.
	// See https://github.com/golang/go/commit/f5f5a8b6209f84961687d993b93ea0d397f5d5bf
	t.mu.RLock()
	tag, found := t.tags[string(key)]
	t.mu.RUnlock()

	if found {
		return t.seen(tag), true
	}
	return t.storeBytes(key)
}

// LoadOrStoreString is LoadOrStore for a key the caller already holds as a string.
func (t *Table) LoadOrStoreString(key string) (InternedTag, bool) {
	t.mu.RLock()
	tag, found := t.tags[key]
	t.mu.RUnlock()

	if found {
		return t.seen(tag), true
	}
	return t.storeString(key)
}

// seen records an arrival of an already-interned tag. It holds no lock: lastSeen
// is atomic and the epoch check is too.
func (t *Table) seen(tag *Tag) InternedTag {
	tag.touch(t.epoch.Load())
	t.maybeAdvanceEpoch()
	return tag
}

func (t *Table) storeBytes(key []byte) (InternedTag, bool) {
	t.mu.Lock()
	// Another worker may have interned this tag between the read and write locks.
	if tag, found := t.tags[string(key)]; found {
		t.mu.Unlock()
		return t.seen(tag), true
	}
	tag := t.insertLocked(string(key))
	t.mu.Unlock()

	t.maybeAdvanceEpoch()
	return tag, false
}

func (t *Table) storeString(key string) (InternedTag, bool) {
	t.mu.Lock()
	if tag, found := t.tags[key]; found {
		t.mu.Unlock()
		return t.seen(tag), true
	}
	tag := t.insertLocked(key)
	t.mu.Unlock()

	t.maybeAdvanceEpoch()
	return tag, false
}

// insertLocked interns key. The write lock must be held.
func (t *Table) insertLocked(key string) *Tag {
	tag := newTag(key, t.epoch.Load())
	t.tags[key] = tag
	return tag
}

// Len returns the number of tags currently interned.
func (t *Table) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.tags)
}

// maybeAdvanceEpoch advances the epoch and sweeps once the epoch interval has
// elapsed. The clock is only read every tableCheckInterval lookups, so this is a
// single atomic decrement in the common case.
func (t *Table) maybeAdvanceEpoch() {
	if t.opsUntilCheck.Add(-1) > 0 {
		return
	}

	t.mu.Lock()
	// Concurrent lookups can drive the counter past zero together; whichever one
	// gets the lock first does the work and resets the counter for the rest.
	var evicted int
	if t.opsUntilCheck.Load() <= 0 {
		t.opsUntilCheck.Store(tableCheckInterval)
		if now := time.Now(); now.Sub(t.lastEpochStart) >= tableEpochInterval {
			t.lastEpochStart = now
			evicted = t.advanceEpochLocked()
		}
	}
	t.mu.Unlock()

	t.reportEvicted(evicted)
}

// advanceEpoch starts a new epoch and sweeps the tags that have now gone unseen
// for long enough, returning how many were dropped.
func (t *Table) advanceEpoch() int {
	t.mu.Lock()
	evicted := t.advanceEpochLocked()
	t.mu.Unlock()

	t.reportEvicted(evicted)
	return evicted
}

// advanceEpochLocked starts a new epoch and sweeps. The write lock must be held.
func (t *Table) advanceEpochLocked() int {
	epoch := t.epoch.Add(1)
	if epoch <= tableEpochsRetained {
		return 0
	}
	cutoff := epoch - tableEpochsRetained

	evicted := 0
	for key, tag := range t.tags {
		if tag.lastSeen.Load() < cutoff {
			delete(t.tags, key)
			evicted++
		}
	}
	return evicted
}

// reportEvicted runs the eviction callback, without the table lock held so that
// telemetry cannot stall lookups.
func (t *Table) reportEvicted(evicted int) {
	if evicted > 0 && t.onEvict != nil {
		t.onEvict(evicted)
	}
}
