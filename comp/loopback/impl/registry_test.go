// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package loopbackimpl

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyntheticKeyDeterministic(t *testing.T) {
	k1 := syntheticKey("foo.bar", []string{"env:prod", "region:us"})
	k2 := syntheticKey("foo.bar", []string{"env:prod", "region:us"})
	assert.Equal(t, k1, k2)
	assert.NotZero(t, k1)
}

func TestSyntheticKeyTagSortIndependent(t *testing.T) {
	k1 := syntheticKey("foo", sortedTagsCopy([]string{"b", "a"}))
	k2 := syntheticKey("foo", sortedTagsCopy([]string{"a", "b"}))
	assert.Equal(t, k1, k2)
}

func TestRegisterDedup(t *testing.T) {
	reg := newContextRegistry()
	k1 := reg.register("foo.bar", []string{"env:prod", "region:us"})
	k2 := reg.register("foo.bar", []string{"region:us", "env:prod"}) // different order
	assert.Equal(t, k1, k2)
}

func TestRegisterWithKeyConcurrent(t *testing.T) {
	reg := newContextRegistry()
	const key = uint64(0xabcdef)
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reg.registerWithKey(key, "foo", []string{"a"})
		}()
	}
	wg.Wait()
	keys := reg.lookupKeys("foo", nil)
	require.Len(t, keys, 1)
	assert.Equal(t, key, keys[0])
}

func TestLookupKeysNilTagsMatchAll(t *testing.T) {
	reg := newContextRegistry()
	reg.registerWithKey(1, "m", []string{"env:prod"})
	reg.registerWithKey(2, "m", []string{"env:staging"})

	keys := reg.lookupKeys("m", nil)
	assert.Len(t, keys, 2)
}

func TestLookupKeysTagFilter(t *testing.T) {
	reg := newContextRegistry()
	reg.registerWithKey(1, "m", []string{"env:prod", "region:us"})
	reg.registerWithKey(2, "m", []string{"env:staging"})

	keys := reg.lookupKeys("m", []string{"env:prod"})
	require.Len(t, keys, 1)
	assert.Equal(t, uint64(1), keys[0])
}

func TestLookupKeysUnknownName(t *testing.T) {
	reg := newContextRegistry()
	assert.Nil(t, reg.lookupKeys("nonexistent", nil))
}
