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

func TestKeyHashCache_ComputeKeyDeterministic(t *testing.T) {
	// Verify that computeKey produces the same result for the same input across
	// repeated calls. This guards against the stack-allocated buffer retaining
	// state between invocations.
	khc := newKeyHashCache(cache.New(5*time.Minute, 10*time.Minute))
	input := "host:my-host\x00service:backend\x00name:my.metric\x00"
	key1 := khc.computeKey(input)
	key2 := khc.computeKey(input)
	require.Equal(t, key1, key2)
}
