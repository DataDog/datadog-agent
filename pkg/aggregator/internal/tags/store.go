// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package tags

import (
	"maps"
	"math/bits"
	"strconv"
	"sync"
	"unsafe"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	sizeutil "github.com/DataDog/datadog-agent/pkg/util/size"
)

// Entry is used to keep track of tag slices shared by the contexts.
type Entry struct {
	// refs is the refcount of this entity.  If this value is zero, then the
	// entity may be reclaimed in Shrink().
	//
	// This value must be first in the struct to ensure proper alignment.  It
	// is not used as a pointer to avoid doubling the number of allocations
	// required per Entry.
	refs atomic.Uint64

	// handles holds this entry's tags as interned handles. This is the
	// representation contexts keep: 8 bytes per tag instead of a 16 byte string
	// header, and tags shared between contexts share one copy of the string.
	handles []tagset.Handle

	// strings is the resolved form of handles, materialized on first use by
	// Tags() and cached from then on. Entries are shared between contexts and
	// outlive many flushes, so each unique tag set is resolved once rather than
	// once per flush.
	strings     []string
	stringsOnce sync.Once
}

// SizeInBytes returns the size of the Entry in bytes.
func (e *Entry) SizeInBytes() int {
	size := int(handleSliceSize) + len(e.handles)*int(handleSize) + 8
	if e.strings != nil {
		size += sizeutil.SizeOfStringSlice(e.strings)
	}
	return size
}

// DataSizeInBytes returns the size of the Entry data in bytes.
func (e *Entry) DataSizeInBytes() int {
	dataSize := 0
	for _, h := range e.handles {
		dataSize += len(h.Value())
	}
	return dataSize
}

var (
	handleSize      = unsafe.Sizeof(tagset.Handle{})
	handleSliceSize = unsafe.Sizeof([]tagset.Handle{})
)

var _ sizeutil.HasSizeInBytes = (*Entry)(nil)

// Tags returns the strings stored in the Entry, resolving the interned handles on
// first call and caching the result. The slice may be shared with other users and
// should not be modified. Users can keep the slice after the entry was removed
// from the store; it is not recycled or otherwise modified by the store.
//
// This is where a context's tags become plain strings again, on their way into a
// serie and then the serializer.
func (e *Entry) Tags() []string {
	e.stringsOnce.Do(func() {
		e.strings = tagset.HandleValues(e.handles)
	})
	return e.strings
}

// Handles returns the interned tags stored in the Entry. The slice must not be
// modified.
func (e *Entry) Handles() []tagset.Handle {
	return e.handles
}

// Release decrements internal reference counter, potentially marking
// the entry as unused.
//
// Can be called concurrently with other store operations.
func (e *Entry) Release() {
	e.refs.Dec()
}

// Store is a reference counted container of tags slices, to be shared
// between contexts.
//
// Store is generally not thread-safe, except Release may be called
// concurrently with other methods.
type Store struct {
	tagsByKey map[ckey.TagsKey]*Entry
	cap       int
	enabled   bool
	telemetry storeTelemetry
}

// NewStore returns new empty Store.
func NewStore(enabled bool, name string) *Store {
	return &Store{
		tagsByKey: map[ckey.TagsKey]*Entry{},
		enabled:   enabled,
		telemetry: newStoreTelemetry(name),
	}
}

// Insert returns an Entry that corresponds to the key. If the key is
// not in the cache, a new entry is stored in the Store with the tags
// retrieved from the tagsBuffer. Insert increments reference count
// for the returned entry; callers should call Entry.Release() when
// the returned pointer is no longer in use.
//
// Store is generally not thread-safe, except Release may be called
// concurrently with other methods.
func (tc *Store) Insert(key ckey.TagsKey, tagsBuffer *tagset.HashingTagsAccumulator) *Entry {
	if !tc.enabled {
		return &Entry{
			handles: tagsBuffer.CopyHandles(),
		}
	}

	entry := tc.tagsByKey[key]
	if entry != nil {
		// Can happen concurrently with Release().
		entry.refs.Inc()
		tc.telemetry.hits.Inc()
	} else {
		entry = &Entry{
			handles: tagsBuffer.CopyHandles(),
		}
		entry.refs.Inc()
		tc.tagsByKey[key] = entry
		tc.cap++
		tc.telemetry.miss.Inc()
	}

	return entry
}

// Shrink will try to release memory if cache usage drops low enough.
//
// Store is generally not thread-safe, except Release may be called
// concurrently with other methods.
func (tc *Store) Shrink() {
	stats := entryStats{}
	for key, entry := range tc.tagsByKey {
		if refs := entry.refs.Load(); refs > 0 {
			stats.visit(entry, refs)
		} else {
			delete(tc.tagsByKey, key)
		}
	}

	if len(tc.tagsByKey) < tc.cap/2 {
		new := make(map[ckey.TagsKey]*Entry, len(tc.tagsByKey))
		maps.Copy(new, tc.tagsByKey)
		tc.cap = len(new)
		tc.tagsByKey = new
	}

	tc.updateTelemetry(&stats)
}

func (tc *Store) updateTelemetry(s *entryStats) {
	t := &tc.telemetry

	tlmMaxEntries.Set(float64(tc.cap), t.name)
	tlmEntries.Set(float64(len(tc.tagsByKey)), t.name)

	for i := 0; i < 3; i++ {
		tlmTagsetRefsCnt.Set(float64(s.refsFreq[i]), t.name, strconv.Itoa(i+1))
	}
	for i := 3; i < 8; i++ {
		tlmTagsetRefsCnt.Set(float64(s.refsFreq[i]), t.name, strconv.Itoa(1<<(i-1)))
	}

	tlmTagsetMinTags.Set(float64(s.minSize), t.name)
	tlmTagsetMaxTags.Set(float64(s.maxSize), t.name)
	tlmTagsetSumTags.Set(float64(s.sumSize), t.name)
	tlmTagsetSumTagBytes.Set(float64(s.sumSizeBytes), t.name, BytesKindStruct)
	tlmTagsetSumTagBytes.Set(float64(s.sumDataSizeBytes), t.name, BytesKindData)
}

func newCounter(name string, help string, tags ...string) telemetry.Counter {
	return telemetryimpl.GetCompatComponent().NewCounter("aggregator_tags_store", name,
		append([]string{"cache_instance_name"}, tags...), help)
}

func newGauge(name string, help string, tags ...string) telemetry.Gauge {
	return telemetryimpl.GetCompatComponent().NewGauge("aggregator_tags_store", name,
		append([]string{"cache_instance_name"}, tags...), help)
}

var (
	// BytesKindTelemetryKey is the tag key used to identify the kind of telemetry value.
	BytesKindTelemetryKey = "bytes_kind"
	// BytesKindStruct is the tag value used to mark bytes as struct.
	BytesKindStruct = "struct"
	// BytesKindData is the tag value used to mark bytes as data. Those are likely to be interned strings.
	BytesKindData = "data"
)

var (
	tlmHits              = newCounter("hits_total", "number of times cache already contained the tags")
	tlmMiss              = newCounter("miss_total", "number of times cache did not contain the tags")
	tlmEntries           = newGauge("entries", "number of entries in the tags cache")
	tlmMaxEntries        = newGauge("max_entries", "maximum number of entries since last shrink")
	tlmTagsetMinTags     = newGauge("tagset_min_tags", "minimum number of tags in a tagset")
	tlmTagsetMaxTags     = newGauge("tagset_max_tags", "maximum number of tags in a tagset")
	tlmTagsetSumTags     = newGauge("tagset_sum_tags", "total number of tags stored in all tagsets by the cache")
	tlmTagsetRefsCnt     = newGauge("tagset_refs_count", "distribution of usage count of tagsets in the cache", "ge")
	tlmTagsetSumTagBytes = newGauge("tagset_sum_tags_bytes", "total number of bytes stored in all tagsets by the cache", BytesKindTelemetryKey)
)

type storeTelemetry struct {
	hits telemetry.SimpleCounter
	miss telemetry.SimpleCounter
	name string
}

func newStoreTelemetry(name string) storeTelemetry {
	return storeTelemetry{
		hits: tlmHits.WithValues(name),
		miss: tlmMiss.WithValues(name),
		name: name,
	}
}

type entryStats struct {
	refsFreq         [8]uint64
	minSize          int
	maxSize          int
	sumSize          int
	sumSizeBytes     int
	sumDataSizeBytes int
	count            int
}

func (s *entryStats) visit(e *Entry, r uint64) {
	if r < 4 {
		s.refsFreq[r-1]++
	} else if r < 64 {
		s.refsFreq[bits.Len64(r)]++ // Len(4) = 3, Len(63) = 6
	} else {
		s.refsFreq[7]++
	}

	n := len(e.handles)
	if n < s.minSize || s.count == 0 {
		s.minSize = n
	}
	if n > s.maxSize {
		s.maxSize = n
	}
	s.sumSize += n
	s.sumSizeBytes += e.SizeInBytes()
	s.sumDataSizeBytes += e.DataSizeInBytes()
	s.count++
}
