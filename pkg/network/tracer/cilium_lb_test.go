// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package tracer

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeBackendsIterator struct {
	ids []uint32
	err error
	i   int
}

func (f *fakeBackendsIterator) Next(k *backend4KeyV3, v *backend4ValueV3) bool {
	if f.i >= len(f.ids) {
		return false
	}
	id := f.ids[f.i]
	f.i++
	*k = backend4KeyV3{ID: id}
	*v = backend4ValueV3{Address: [4]byte{10, 0, 0, byte(id)}, Port: htons(8080)}
	return true
}

func (f *fakeBackendsIterator) Err() error {
	return f.err
}

func backendIDs(cache map[uint32]backend) []uint32 {
	ids := make([]uint32, 0, len(cache))
	for id := range cache {
		ids = append(ids, id)
	}
	return ids
}

func TestRefreshBackendsEvictsRemovedBackendsAfterTTL(t *testing.T) {
	cache := map[uint32]backend{}
	start := time.Now()

	refreshBackends(cache, &fakeBackendsIterator{ids: []uint32{1, 2, 3}}, start)
	require.ElementsMatch(t, []uint32{1, 2, 3}, backendIDs(cache))
	assert.Equal(t, uint16(8080), cache[1].port)

	// backend 1 is removed and backend 4 is added; 1 is kept within the TTL
	refreshBackends(cache, &fakeBackendsIterator{ids: []uint32{2, 3, 4}}, start.Add(time.Minute))
	assert.ElementsMatch(t, []uint32{1, 2, 3, 4}, backendIDs(cache))

	// once the TTL has passed since 1 was last seen, it's evicted
	refreshBackends(cache, &fakeBackendsIterator{ids: []uint32{2, 3, 4}}, start.Add(removedBackendTTL+time.Second))
	assert.ElementsMatch(t, []uint32{2, 3, 4}, backendIDs(cache))
}

func TestRefreshBackendsSkipsEvictionOnIterationError(t *testing.T) {
	cache := map[uint32]backend{}
	start := time.Now()

	refreshBackends(cache, &fakeBackendsIterator{ids: []uint32{1, 2}}, start)

	// a failed walk past the TTL doesn't evict backends it didn't get to
	refreshBackends(cache, &fakeBackendsIterator{ids: []uint32{2}, err: errors.New("iteration failed")}, start.Add(removedBackendTTL+time.Second))
	assert.ElementsMatch(t, []uint32{1, 2}, backendIDs(cache))
}

func TestRefreshBackendsKeepsRemovedBackendWhileLookedUp(t *testing.T) {
	cache := map[uint32]backend{}
	start := time.Now()

	refreshBackends(cache, &fakeBackendsIterator{ids: []uint32{1, 2}}, start)
	refreshBackends(cache, &fakeBackendsIterator{ids: []uint32{2}}, start.Add(time.Minute))

	// a live connection still points at removed backend 1
	b, ok := touchBackend(cache, 1, start.Add(9*time.Minute))
	require.True(t, ok)
	assert.Equal(t, uint16(8080), b.port)

	refreshBackends(cache, &fakeBackendsIterator{ids: []uint32{2}}, start.Add(removedBackendTTL+time.Second))
	assert.ElementsMatch(t, []uint32{1, 2}, backendIDs(cache))

	// once lookups stop, it's evicted a TTL after the last one
	refreshBackends(cache, &fakeBackendsIterator{ids: []uint32{2}}, start.Add(9*time.Minute+removedBackendTTL+time.Second))
	assert.ElementsMatch(t, []uint32{2}, backendIDs(cache))
}
