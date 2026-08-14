// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverimpl

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// stringInterner hands out tagset.InternedTag values for strings read off the
// wire, so that a tag or metric name seen many times is stored once.
//
// Canonicalization itself is done by the standard library `unique` package: it
// owns the single copy of each string and keeps it alive exactly as long as some
// handle still refers to it. That removes the two problems the hand-rolled
// interner had:
//
//   - a reset dropped the canonical strings, so the same tag arriving after a
//     reset allocated a fresh copy and the agent held several copies of the same
//     tag at once. Handles issued before a reset stay valid and stay canonical.
//   - entries were retained forever (up to maxSize) even if no sample referenced
//     them anymore. `unique` reclaims a string once the last handle goes away.
//
// What is left here is a per-worker lookaside cache. It exists for two reasons:
// looking up by []byte without allocating a string (which unique.Make cannot do),
// and memoizing the tag hash so a given tag is hashed once rather than once per
// sample carrying it. maxSize now only bounds this cache, not the strings.
type stringInterner struct {
	cache   map[string]tagset.InternedTag
	maxSize int
	id      string

	telemetry *stringInternerInstanceTelemetry
}

func newStringInterner(maxSize int, internerID int, siTelemetry *stringInternerTelemetry) *stringInterner {
	id := fmt.Sprintf("interner_%d", internerID)
	i := &stringInterner{
		cache:     make(map[string]tagset.InternedTag),
		id:        id,
		maxSize:   maxSize,
		telemetry: siTelemetry.PrepareForID(id),
	}

	return i
}

// LoadOrStore always returns a handle for the given key, interning it if this is
// the first time the worker sees it.
func (i *stringInterner) LoadOrStore(key []byte) tagset.InternedTag {
	// here is the string interner trick: the map lookup using
	// string(key) doesn't actually allocate a string, but is
	// returning the string value -> no new heap allocation
	// for this string.
	// See https://github.com/golang/go/commit/f5f5a8b6209f84961687d993b93ea0d397f5d5bf
	if t, found := i.cache[string(key)]; found {
		i.telemetry.Hit()
		return t
	}

	if len(i.cache) >= i.maxSize {
		i.telemetry.Reset(len(i.cache))

		i.cache = make(map[string]tagset.InternedTag)
	}

	t := tagset.InternBytes(key)
	// Key the cache on the canonical string so the cache does not retain a second
	// copy of it.
	i.cache[t.Value()] = t

	i.telemetry.Miss(len(key))

	return t
}

// LoadOrStoreString is LoadOrStore for a key the caller already holds as a string.
func (i *stringInterner) LoadOrStoreString(key string) tagset.InternedTag {
	if t, found := i.cache[key]; found {
		i.telemetry.Hit()
		return t
	}

	if len(i.cache) >= i.maxSize {
		i.telemetry.Reset(len(i.cache))

		i.cache = make(map[string]tagset.InternedTag)
	}

	t := tagset.Intern(key)
	i.cache[t.Value()] = t

	i.telemetry.Miss(len(key))

	return t
}
