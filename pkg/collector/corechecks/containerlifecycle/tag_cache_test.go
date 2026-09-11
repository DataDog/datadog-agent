// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"testing"
	"time"

	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/stretchr/testify/assert"
)

func entityID(id string) taggertypes.EntityID {
	return taggertypes.NewEntityID(taggertypes.ContainerID, id)
}

func TestTagCacheHit(t *testing.T) {
	cache := newTagCache(time.Minute)

	cache.put(entityID("cont1"), []string{"kube_namespace:default"})

	tags, found := cache.get(entityID("cont1"))
	assert.True(t, found)
	assert.Equal(t, []string{"kube_namespace:default"}, tags)
}

func TestTagCacheExpired(t *testing.T) {
	now := time.Now()
	cache := newTagCache(30 * time.Second)
	cache.now = func() time.Time { return now }

	cache.put(entityID("cont1"), []string{"kube_namespace:default"})

	// Just before expiry the entry is still valid.
	now = now.Add(29 * time.Second)
	_, found := cache.get(entityID("cont1"))
	assert.True(t, found)

	// After the TTL the entry is expired.
	now = now.Add(2 * time.Second)
	_, found = cache.get(entityID("cont1"))
	assert.False(t, found)
}

func TestTagCacheEmptyTags(t *testing.T) {
	cache := newTagCache(time.Minute)

	cache.put(entityID("cont1"), nil)

	tags, found := cache.get(entityID("cont1"))
	assert.True(t, found)
	assert.Empty(t, tags)
}

func TestTagCacheDisabled(t *testing.T) {
	cache := newTagCache(0)

	cache.put(entityID("cont1"), []string{"kube_namespace:default"})

	_, found := cache.get(entityID("cont1"))
	assert.False(t, found)
	assert.Empty(t, cache.entries)
}

func TestTagCacheSweep(t *testing.T) {
	now := time.Now()
	cache := newTagCache(30 * time.Second)
	cache.now = func() time.Time { return now }

	cache.put(entityID("cont1"), []string{"kube_namespace:default"})
	now = now.Add(time.Minute)
	cache.put(entityID("cont2"), []string{"kube_namespace:kube-system"})

	assert.Len(t, cache.entries, 1)
	_, found := cache.get(entityID("cont1"))
	assert.False(t, found)
	_, found = cache.get(entityID("cont2"))
	assert.True(t, found)
}
