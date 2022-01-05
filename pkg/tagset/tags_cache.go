// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import "errors"

// tagsCache caches Tags instances using purpose-specific cache keys.
//
// Note that tagsCache instances are not threadsafe
type tagsCache struct {
	// number of inserts between rotations
	insertsPerRotation int

	// number of inserts remaining before next rotation
	untilRotate int

	// tagests contains the constitutent tagset maps. This is a slice of length
	// cacheCount. The first map is the newest, into which new values will be
	// inserted.
	maps []map[uint64]*Tags
}

func newTagsCache(insertsPerRotation, cacheCount int) (tagsCache, error) {
	if cacheCount < 0 {
		return tagsCache{}, errors.New("cacheCount must be greater than zero")
	}
	if insertsPerRotation < 0 {
		return tagsCache{}, errors.New("insertsPerRotation must be greater than zero")
	}
	maps := make([]map[uint64]*Tags, cacheCount)
	for i := range maps {
		maps[i] = make(map[uint64]*Tags)
	}
	return tagsCache{
		insertsPerRotation: insertsPerRotation,
		untilRotate:        insertsPerRotation,
		maps:               maps,
	}, nil
}

// getCachedTags gets an element from the cache, calling miss() to generate the
// element if not found.
func (tc *tagsCache) getCachedTags(key uint64, miss func() *Tags) *Tags {
	v, ok := tc.search(key)
	if !ok {
		v = miss()
		tc.insert(key, v)
	}
	return v
}

// getCachedTagsErr is like getCachedTags, but works for miss() functions that can
// return an error. Errors are not cached.
func (tc *tagsCache) getCachedTagsErr(key uint64, miss func() (*Tags, error)) (*Tags, error) {
	v, ok := tc.search(key)
	if !ok {
		var err error
		v, err = miss()
		if err != nil {
			return nil, err
		}
		tc.insert(key, v)
	}
	return v, nil
}

// search searches for a key in maps older than the first. If found, the key
// is copied to the first map and returned.
func (tc *tagsCache) search(key uint64) (*Tags, bool) {

	v, ok := tc.maps[0][key]
	if ok {
		return v, true
	}

	cacheCount := len(tc.maps)
	for i := 1; i < cacheCount; i++ {
		v, ok = tc.maps[i][key]
		if ok {
			// "recache" this entry in the first map so that it's faster to
			// find next time
			tc.insert(key, v)
			return v, true
		}
	}

	return nil, false
}

// insert inserts a key into the first map. It also performs rotation, if
// necessary.
func (tc *tagsCache) insert(key uint64, val *Tags) {
	tc.maps[0][key] = val
	tc.untilRotate--

	if tc.untilRotate > 0 {
		return
	}

	tc.untilRotate = tc.insertsPerRotation
	tc.rotate()
}

// rotate rotates the cache.
func (tc *tagsCache) rotate() {
	cacheCount := len(tc.maps)

	// try to allocate a new map of the size to which the last map
	// grew before being discarded.
	lastLen := len(tc.maps[cacheCount-1])

	// move all caches forward in the tagests array
	copy(tc.maps[1:cacheCount], tc.maps[:cacheCount-1])
	// and initialize a new first map
	tc.maps[0] = make(map[uint64]*Tags, lastLen)
}
