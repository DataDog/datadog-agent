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
	"sync"
	"time"

	gocache "github.com/patrickmn/go-cache"
	"github.com/twmb/murmur3"
)

// Pool for reusing ASCII85 encoding buffers to reduce allocations
var ascii85BufPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, ascii85.MaxEncodedLen(16))
	},
}

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

// OPTIMIZED: Uses buffer pooling to avoid allocations
func (m *keyHashCache) computeKey(s string) keyHashCacheKey {
	h1, h2 := murmur3.StringSum128(s)
	var bytes [16]byte
	binary.LittleEndian.PutUint64(bytes[0:], h1)
	binary.LittleEndian.PutUint64(bytes[8:], h2)

	// Get buffer from pool instead of allocating
	buf := ascii85BufPool.Get().([]byte)
	defer ascii85BufPool.Put(buf)

	n := ascii85.Encode(buf, bytes[:])

	// Copy result since we're returning the buffer to pool
	result := make([]byte, n)
	copy(result, buf[:n])
	return keyHashCacheKey(result)
}
