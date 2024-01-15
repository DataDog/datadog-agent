// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package tags

import (
	"fmt"
	"math/bits"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/util"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
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

	// tags contains the cached tags in this entry.
	tags []string
}

// SizeInBytes returns the size of the Entry in bytes.
func (e *Entry) SizeInBytes() int {
	return util.SizeOfStringSlice(e.tags) + 8
}

// DataSizeInBytes returns the size of the Entry data in bytes.
func (e *Entry) DataSizeInBytes() int {
	return util.DataSizeOfStringSlice(e.tags)
}

var _ util.HasSizeInBytes = (*Entry)(nil)

// Tags returns the strings stored in the Entry. The slice may be
// shared with other users and should not be modified. Users can keep
// the slice after the entry was removed from the store; it is not
// recycled or otherwise modified by the store.
func (e *Entry) Tags() []string {
	return e.tags
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
			tags: tagsBuffer.Copy(),
		}
	}

	entry := tc.tagsByKey[key]
	if entry != nil {
		// Can happen concurrently with Release().
		entry.refs.Inc()
		tc.telemetry.hits.Inc()
	} else {
		entry = &Entry{
			tags: tagsBuffer.Copy(),
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
		//nolint:revive // TODO(AML) Fix revive linter
		new := make(map[ckey.TagsKey]*Entry, len(tc.tagsByKey))
		for k, v := range tc.tagsByKey {
			new[k] = v
		}
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
		tlmTagsetRefsCnt.Set(float64(s.refsFreq[i]), t.name, fmt.Sprintf("%d", i+1))
	}
	for i := 3; i < 8; i++ {
		tlmTagsetRefsCnt.Set(float64(s.refsFreq[i]), t.name, fmt.Sprintf("%d", 1<<(i-1)))
	}

	tlmTagsetMinTags.Set(float64(s.minSize), t.name)
	tlmTagsetMaxTags.Set(float64(s.maxSize), t.name)
	tlmTagsetSumTags.Set(float64(s.sumSize), t.name)
	tlmTagsetSumTagBytes.Set(float64(s.sumSizeBytes), t.name, util.BytesKindStruct)
	tlmTagsetSumTagBytes.Set(float64(s.sumDataSizeBytes), t.name, util.BytesKindData)
}

func newCounter(name string, help string, tags ...string) telemetry.Counter {
	return telemetry.NewCounter("aggregator_tags_store", name,
		append([]string{"cache_instance_name"}, tags...), help)
}

func newGauge(name string, help string, tags ...string) telemetry.Gauge {
	return telemetry.NewGauge("aggregator_tags_store", name,
		append([]string{"cache_instance_name"}, tags...), help)
}

var (
	tlmHits              = newCounter("hits_total", "number of times cache already contained the tags")
	tlmMiss              = newCounter("miss_total", "number of times cache did not contain the tags")
	tlmEntries           = newGauge("entries", "number of entries in the tags cache")
	tlmMaxEntries        = newGauge("max_entries", "maximum number of entries since last shrink")
	tlmTagsetMinTags     = newGauge("tagset_min_tags", "minimum number of tags in a tagset")
	tlmTagsetMaxTags     = newGauge("tagset_max_tags", "maximum number of tags in a tagset")
	tlmTagsetSumTags     = newGauge("tagset_sum_tags", "total number of tags stored in all tagsets by the cache")
	tlmTagsetRefsCnt     = newGauge("tagset_refs_count", "distribution of usage count of tagsets in the cache", "ge")
	tlmTagsetSumTagBytes = newGauge("tagset_sum_tags_bytes", "total number of bytes stored in all tagsets by the cache", util.BytesKindTelemetryKey)
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

	n := len(e.tags)
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
