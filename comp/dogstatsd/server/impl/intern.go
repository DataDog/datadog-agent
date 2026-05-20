// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverimpl

import (
	"fmt"
)

// stringInterner is a bounded string dictionary used by the DogStatsD parser.
//
// DogStatsD parses metric names and tags out of packet buffers. Those buffers
// are reused, so parsed names/tags that cross the parser boundary must become
// heap strings. The useful optimization is to avoid making a new heap string
// for values that are already known to be common.
//
// The previous implementation used a single map and cleared the whole map when
// it reached maxSize. That is cheap, but bad for mixed-cardinality workloads: a
// wave of one-off tags evicts every hot tag/name at once and the parser then
// reallocates the hot dictionary again. This implementation uses a small SLRU
// shape instead:
//
//   - recent/probationary entries receive first sightings;
//   - a hit in recent promotes the value to protected;
//   - protected entries survive churn in recent;
//   - both segments use ring eviction to keep memory bounded.
//
// This mirrors the v3 payload-builder lesson: stable strings should become
// dictionary entries, while high-cardinality churn should not reset the whole
// dictionary. Once Go exposes a runtime string interning API, this should be
// revisited. Ref: https://github.com/golang/go/issues/62483
//
// The maps use string keys and values so that lookups with string(key) can use
// the compiler's no-allocation []byte->string map lookup optimization. Keep the
// conversion directly inside the map index expression; assigning it to a local
// string first would allocate on every lookup.
type stringInterner struct {
	recent    map[string]string
	protected map[string]string

	recentRing    []string
	protectedRing []string
	recentNext    int
	protectedNext int
	recentMax     int
	protectedMax  int
	maxSize       int
	id            string

	telemetry *stringInternerInstanceTelemetry
}

func newStringInterner(maxSize int, internerID int, siTelemetry *stringInternerTelemetry) *stringInterner {
	id := fmt.Sprintf("interner_%d", internerID)
	if maxSize < 0 {
		maxSize = 0
	}

	recentMax := 0
	protectedMax := 0
	if maxSize > 0 {
		recentMax = maxSize / 4
		if recentMax < 1 {
			recentMax = 1
		}
		protectedMax = maxSize - recentMax
	}

	return &stringInterner{
		recent:        make(map[string]string),
		protected:     make(map[string]string),
		recentRing:    make([]string, recentMax),
		protectedRing: make([]string, protectedMax),
		recentMax:     recentMax,
		protectedMax:  protectedMax,
		maxSize:       maxSize,
		id:            id,
		telemetry:     siTelemetry.PrepareForID(id),
	}
}

// LoadOrStore returns a stable string for key. Hits return the string already
// retained by the interner and do not allocate. Misses must allocate because the
// input byte slice belongs to a reusable packet buffer.
func (i *stringInterner) LoadOrStore(key []byte) string {
	if i.maxSize <= 0 {
		s := string(key)
		i.telemetry.Miss(len(s))
		return s
	}

	// Keep string(key) inside the map lookup expression: this is the special form
	// the compiler can use without allocating a temporary string on hits.
	if s, found := i.protected[string(key)]; found {
		i.telemetry.Hit()
		return s
	}
	if s, found := i.recent[string(key)]; found {
		i.telemetry.Hit()
		i.promote(s)
		return s
	}

	s := string(key)
	i.telemetry.Miss(len(s))
	i.insertRecent(s)
	return s
}

func (i *stringInterner) insertRecent(s string) {
	if i.recentMax <= 0 {
		i.insertProtected(s)
		return
	}
	if _, found := i.recent[s]; found {
		return
	}
	i.evictRecentSlot()
	i.recentRing[i.recentNext] = s
	i.recentNext = (i.recentNext + 1) % len(i.recentRing)
	i.recent[s] = s
}

func (i *stringInterner) promote(s string) {
	if i.protectedMax <= 0 {
		return
	}
	if _, found := i.protected[s]; found {
		delete(i.recent, s)
		return
	}
	if _, found := i.recent[s]; found {
		delete(i.recent, s)
	}
	i.insertProtected(s)
}

func (i *stringInterner) insertProtected(s string) {
	if i.protectedMax <= 0 {
		return
	}
	if _, found := i.protected[s]; found {
		return
	}
	i.evictProtectedSlot()
	i.protectedRing[i.protectedNext] = s
	i.protectedNext = (i.protectedNext + 1) % len(i.protectedRing)
	i.protected[s] = s
}

func (i *stringInterner) evictRecentSlot() {
	if len(i.recentRing) == 0 {
		return
	}
	evicted := i.recentRing[i.recentNext]
	if evicted == "" {
		return
	}
	if _, found := i.recent[evicted]; found {
		delete(i.recent, evicted)
		i.telemetry.Evict(len(evicted))
	}
}

func (i *stringInterner) evictProtectedSlot() {
	if len(i.protectedRing) == 0 {
		return
	}
	evicted := i.protectedRing[i.protectedNext]
	if evicted == "" {
		return
	}
	if _, found := i.protected[evicted]; found {
		delete(i.protected, evicted)
		i.telemetry.Evict(len(evicted))
	}
}

func (i *stringInterner) len() int {
	return len(i.recent) + len(i.protected)
}
