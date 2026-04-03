// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && ncm

package store

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

// insertTestMetadata directly writes a ConfigMetadata entry into the metadata bucket,
// giving tests control over timestamps and other fields that StoreConfig sets automatically.
func insertTestMetadata(t *testing.T, cs *ConfigStore, meta ConfigMetadata) {
	t.Helper()
	err := cs.update(func(tx *bbolt.Tx) error {
		data, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte(metadataBucket)).Put([]byte(meta.ConfigUUID), data)
	})
	require.NoError(t, err)
}

func TestBuildEvictionIndex(t *testing.T) {
	t.Run("empty store returns empty structures", func(t *testing.T) {
		cs := newTestConfigStore(t)
		countMap, entries, err := cs.buildEvictionIndex()
		require.NoError(t, err)
		assert.Empty(t, countMap)
		assert.Empty(t, entries)
	})

	t.Run("counts configs per device correctly", func(t *testing.T) {
		cs := newTestConfigStore(t)
		insertTestMetadata(t, cs, ConfigMetadata{ConfigUUID: "uuid-1", DeviceID: "device:10.0.0.1", LastAccessedAt: 100})
		insertTestMetadata(t, cs, ConfigMetadata{ConfigUUID: "uuid-2", DeviceID: "device:10.0.0.1", LastAccessedAt: 200})
		insertTestMetadata(t, cs, ConfigMetadata{ConfigUUID: "uuid-3", DeviceID: "device:10.0.0.2", LastAccessedAt: 300})

		countMap, entries, err := cs.buildEvictionIndex()
		require.NoError(t, err)
		assert.Equal(t, 2, countMap["device:10.0.0.1"])
		assert.Equal(t, 1, countMap["device:10.0.0.2"])
		assert.Len(t, entries, 3)
	})

	t.Run("entries are sorted ascending by LastAccessedAt (oldest first)", func(t *testing.T) {
		cs := newTestConfigStore(t)
		// Insert out of order intentionally
		insertTestMetadata(t, cs, ConfigMetadata{ConfigUUID: "uuid-newest", DeviceID: "device:10.0.0.1", LastAccessedAt: 300})
		insertTestMetadata(t, cs, ConfigMetadata{ConfigUUID: "uuid-oldest", DeviceID: "device:10.0.0.1", LastAccessedAt: 100})
		insertTestMetadata(t, cs, ConfigMetadata{ConfigUUID: "uuid-middle", DeviceID: "device:10.0.0.1", LastAccessedAt: 200})

		_, entries, err := cs.buildEvictionIndex()
		require.NoError(t, err)
		require.Len(t, entries, 3)
		assert.Equal(t, "uuid-oldest", entries[0].ConfigUUID)
		assert.Equal(t, "uuid-middle", entries[1].ConfigUUID)
		assert.Equal(t, "uuid-newest", entries[2].ConfigUUID)
	})
}

func TestGetGlobalLRUCandidate(t *testing.T) {
	t.Run("returns empty string when no device exceeds K", func(t *testing.T) {
		countMap := map[string]int{"device:10.0.0.1": 2}
		entries := []*ConfigMetadata{
			{ConfigUUID: "uuid-1", DeviceID: "device:10.0.0.1", LastAccessedAt: 100},
			{ConfigUUID: "uuid-2", DeviceID: "device:10.0.0.1", LastAccessedAt: 200},
		}
		// K=2: device has exactly 2 configs, none are evictable
		candidate, remaining := getGlobalLRUCandidate(countMap, entries, 2)
		assert.Empty(t, candidate)
		assert.Len(t, remaining, 2)
	})

	t.Run("returns oldest evictable config UUID", func(t *testing.T) {
		countMap := map[string]int{"device:10.0.0.1": 3}
		entries := []*ConfigMetadata{
			{ConfigUUID: "uuid-oldest", DeviceID: "device:10.0.0.1", LastAccessedAt: 100},
			{ConfigUUID: "uuid-middle", DeviceID: "device:10.0.0.1", LastAccessedAt: 200},
			{ConfigUUID: "uuid-newest", DeviceID: "device:10.0.0.1", LastAccessedAt: 300},
		}
		// K=2: device has 3, oldest is the LRU candidate
		candidate, remaining := getGlobalLRUCandidate(countMap, entries, 2)
		assert.Equal(t, "uuid-oldest", candidate)
		assert.Equal(t, []*ConfigMetadata{
			{ConfigUUID: "uuid-middle", DeviceID: "device:10.0.0.1", LastAccessedAt: 200},
			{ConfigUUID: "uuid-newest", DeviceID: "device:10.0.0.1", LastAccessedAt: 300},
		}, remaining)
	})

	t.Run("skips pinned configs", func(t *testing.T) {
		countMap := map[string]int{"device:10.0.0.1": 3}
		entries := []*ConfigMetadata{
			{ConfigUUID: "uuid-pinned", DeviceID: "device:10.0.0.1", LastAccessedAt: 100, IsPinned: true},
			{ConfigUUID: "uuid-evictable", DeviceID: "device:10.0.0.1", LastAccessedAt: 200},
			{ConfigUUID: "uuid-newest", DeviceID: "device:10.0.0.1", LastAccessedAt: 300},
		}
		candidate, _ := getGlobalLRUCandidate(countMap, entries, 2)
		assert.Equal(t, "uuid-evictable", candidate)
	})

	t.Run("returns globally oldest evictable config across multiple devices", func(t *testing.T) {
		countMap := map[string]int{
			"device:10.0.0.1": 3,
			"device:10.0.0.2": 3,
		}
		entries := []*ConfigMetadata{
			{ConfigUUID: "uuid-d2-oldest", DeviceID: "device:10.0.0.2", LastAccessedAt: 50},
			{ConfigUUID: "uuid-d1-oldest", DeviceID: "device:10.0.0.1", LastAccessedAt: 100},
			{ConfigUUID: "uuid-d1-mid", DeviceID: "device:10.0.0.1", LastAccessedAt: 200},
			{ConfigUUID: "uuid-d2-mid", DeviceID: "device:10.0.0.2", LastAccessedAt: 250},
			{ConfigUUID: "uuid-d1-new", DeviceID: "device:10.0.0.1", LastAccessedAt: 300},
			{ConfigUUID: "uuid-d2-new", DeviceID: "device:10.0.0.2", LastAccessedAt: 350},
		}
		// device:10.0.0.2's oldest (ts=50) is globally older than device:10.0.0.1's oldest (ts=100)
		candidate, _ := getGlobalLRUCandidate(countMap, entries, 2)
		assert.Equal(t, "uuid-d2-oldest", candidate)
	})
}

func TestGetEvictableExceedingMax(t *testing.T) {
	t.Run("returns empty when no device exceeds N", func(t *testing.T) {
		countMap := map[string]int{"device:10.0.0.1": 3}
		entries := []*ConfigMetadata{
			{ConfigUUID: "uuid-1", DeviceID: "device:10.0.0.1", LastAccessedAt: 100},
			{ConfigUUID: "uuid-2", DeviceID: "device:10.0.0.1", LastAccessedAt: 200},
			{ConfigUUID: "uuid-3", DeviceID: "device:10.0.0.1", LastAccessedAt: 300},
		}
		// N=3: device has exactly 3 total, not over cap
		evictable, remaining := getEvictableExceedingMax(countMap, entries, 3)
		assert.Empty(t, evictable)
		assert.Len(t, remaining, 3)
	})

	t.Run("returns configs until device is within N", func(t *testing.T) {
		countMap := map[string]int{"device:10.0.0.1": 3}
		entries := []*ConfigMetadata{
			{ConfigUUID: "uuid-1", DeviceID: "device:10.0.0.1", LastAccessedAt: 100},
			{ConfigUUID: "uuid-2", DeviceID: "device:10.0.0.1", LastAccessedAt: 200},
			{ConfigUUID: "uuid-3", DeviceID: "device:10.0.0.1", LastAccessedAt: 300},
		}
		evictable, remaining := getEvictableExceedingMax(countMap, entries, 1)
		assert.Equal(t, []string{"uuid-1", "uuid-2"}, evictable)
		assert.Equal(t, []*ConfigMetadata{
			{ConfigUUID: "uuid-3", DeviceID: "device:10.0.0.1", LastAccessedAt: 300},
		}, remaining)
	})

	t.Run("returns configs until all devices are within N", func(t *testing.T) {
		countMap := map[string]int{"device:10.0.0.1": 3, "device:10.0.0.2": 3}
		entries := []*ConfigMetadata{
			{ConfigUUID: "uuid-1", DeviceID: "device:10.0.0.1", LastAccessedAt: 100},
			{ConfigUUID: "uuid-2", DeviceID: "device:10.0.0.1", LastAccessedAt: 200},
			{ConfigUUID: "uuid-3", DeviceID: "device:10.0.0.1", LastAccessedAt: 300},
			{ConfigUUID: "uuid-4", DeviceID: "device:10.0.0.2", LastAccessedAt: 400},
			{ConfigUUID: "uuid-5", DeviceID: "device:10.0.0.2", LastAccessedAt: 500},
			{ConfigUUID: "uuid-6", DeviceID: "device:10.0.0.2", LastAccessedAt: 600},
		}
		evictable, _ := getEvictableExceedingMax(countMap, entries, 1)
		assert.Equal(t, []string{"uuid-1", "uuid-2", "uuid-4", "uuid-5"}, evictable)
	})
}
