// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics

import (
	"encoding/ascii85"
	"encoding/binary"
	"sort"
	"sync"
	"time"
	"unsafe"

	gocache "github.com/patrickmn/go-cache"
	"github.com/twmb/murmur3"
)

// keyHashCache is a wrapper around the go-cache library.
// It uses a hash function to compute the key from the string
// to be memory efficient.
// Should be used when the key length is large.
type keyHashCache struct {
	cache *gocache.Cache
}

func newKeyHashCache(cache *gocache.Cache) keyHashCache {
	return keyHashCache{
		cache: cache,
	}
}

// keyHashCacheKey is the key type for the keyHashCache.
// This is a new type to avoid confusion with the string type.
type keyHashCacheKey string

func (m *keyHashCache) get(s keyHashCacheKey) (interface{}, bool) {
	return m.cache.Get(string(s))
}

func (m *keyHashCache) set(s keyHashCacheKey, v interface{}, expiration time.Duration) {
	m.cache.Set(string(s), v, expiration)
}

// ascii85EncodedLen128 is the exact output length of ascii85.Encode for a
// 128-bit (16-byte) input. ascii85.MaxEncodedLen(16) == ceil(16/4)*5 == 20.
const ascii85EncodedLen128 = 20

// dimensionSep is the single-byte separator written between each dimension field.
const dimensionSep = byte(0)

// hasherState holds a reusable murmur3 Hash128 and the sort buffer.
// Pooling avoids the heap allocations that murmur3.New128() and slice growth make per call.
type hasherState struct {
	h    murmur3.Hash128
	dims []string
}

var hasherPool = sync.Pool{
	New: func() interface{} {
		return &hasherState{
			h:    murmur3.New128(),
			dims: make([]string, 0, 16),
		}
	},
}

func (m *keyHashCache) computeKey(s string) keyHashCacheKey {
	h1, h2 := murmur3.StringSum128(s)
	var raw [16]byte
	binary.LittleEndian.PutUint64(raw[0:], h1)
	binary.LittleEndian.PutUint64(raw[8:], h2)

	// Use a stack-allocated array to avoid a heap allocation for every cache lookup.
	var buf [ascii85EncodedLen128]byte
	n := ascii85.Encode(buf[:], raw[:])
	return keyHashCacheKey(buf[:n])
}

// computeKeyFromDimensions computes the cache key directly from a Dimensions
// value, bypassing the intermediate string allocation that Dimensions.String()
// would produce.  It replicates the same sort-then-join semantics so that the
// resulting key is identical to computeKey(dimensions.String()).
func (m *keyHashCache) computeKeyFromDimensions(d *Dimensions) keyHashCacheKey {
	hs := hasherPool.Get().(*hasherState)
	hs.dims = hs.dims[:0]
	hs.dims = append(hs.dims, d.tags...)
	hs.dims = append(hs.dims, "name:"+d.name, "host:"+d.host, "originID:"+d.originID)
	sort.Strings(hs.dims)

	// Feed each sorted dimension directly into the pooled streaming hasher,
	// using unsafe to avoid the []byte allocation from string→[]byte conversion.
	hs.h.Reset()
	sep := [1]byte{dimensionSep}
	for _, dim := range hs.dims {
		if len(dim) > 0 {
			_, _ = hs.h.Write(unsafe.Slice(unsafe.StringData(dim), len(dim)))
		}
		_, _ = hs.h.Write(sep[:])
	}
	h1, h2 := hs.h.Sum128()

	hasherPool.Put(hs)

	var raw [16]byte
	binary.LittleEndian.PutUint64(raw[0:], h1)
	binary.LittleEndian.PutUint64(raw[8:], h2)

	var buf [ascii85EncodedLen128]byte
	n := ascii85.Encode(buf[:], raw[:])
	return keyHashCacheKey(buf[:n])
}
