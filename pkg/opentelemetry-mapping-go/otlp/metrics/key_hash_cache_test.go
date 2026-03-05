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
	"fmt"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/require"
)

func TestKeyHashCache(t *testing.T) {
	cache := cache.New(5*time.Minute, 10*time.Minute)
	keyHashCache := newKeyHashCache(cache)

	key := keyHashCache.computeKey("test-key")
	_, found := keyHashCache.get(key)
	require.False(t, found)

	keyHashCache.set(key, "test-value", 50*time.Minute)

	value, found := keyHashCache.get(key)
	require.True(t, found)
	require.Equal(t, "test-value", value)

	_, found = keyHashCache.get("Another key")
	require.False(t, found)
}

func TestKeyHashCache_ComputeKey(t *testing.T) {
	keyHashCache := newKeyHashCache(cache.New(5*time.Minute, 10*time.Minute))
	keys := make(map[keyHashCacheKey]struct{})
	for i := 0; i < 100; i++ {
		key := keyHashCache.computeKey(fmt.Sprintf("test-key-%d", i))
		keys[key] = struct{}{}
	}
	// Make sure we have 100 unique keys
	require.Equal(t, 100, len(keys))
}

// TestComputeKeyFromDimensions verifies that computeKeyFromDimensions produces
// the same cache key as computeKey(dimensions.String()).
func TestComputeKeyFromDimensions(t *testing.T) {
	khc := newKeyHashCache(cache.New(5*time.Minute, 10*time.Minute))

	cases := []Dimensions{
		{name: "metric.name", host: "host-one"},
		{name: "metric.name", host: "host-one", tags: []string{"key1:val1", "key2:val2"}},
		{name: "metric.name", host: "host-two", tags: []string{"key2:val2", "key1:val1"}, originID: "orig"},
		{name: "a.metric.name", tags: []string{"zzz:last", "aaa:first"}},
		// Empty fields
		{},
	}

	for _, d := range cases {
		d := d
		fromString := khc.computeKey(d.String())
		fromDims := khc.computeKeyFromDimensions(&d)
		require.Equal(t, fromString, fromDims,
			"key mismatch for dims %+v", d)
	}
}

// TestComputeKeyFromDimensions_Uniqueness verifies that different Dimensions produce different keys.
func TestComputeKeyFromDimensions_Uniqueness(t *testing.T) {
	khc := newKeyHashCache(cache.New(5*time.Minute, 10*time.Minute))

	dims := []Dimensions{
		{name: "m1", host: "h1"},
		{name: "m2", host: "h1"},
		{name: "m1", host: "h2"},
		{name: "m1", host: "h1", tags: []string{"k:v"}},
		{name: "m1", host: "h1", originID: "o1"},
	}
	keys := make(map[keyHashCacheKey]struct{})
	for _, d := range dims {
		d := d
		keys[khc.computeKeyFromDimensions(&d)] = struct{}{}
	}
	require.Equal(t, len(dims), len(keys), "expected all keys to be unique")
}
