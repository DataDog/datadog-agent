// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && linux

package containerd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHiddenBytesCachePersistsAndCopies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	cache := newHiddenBytesCache(path)
	img := hiddenBytesTestImage("cached")
	results := hiddenBytesTestResult(img)
	require.NoError(t, cache.put(img.ID, results))
	results[0].Bytes = 999
	loaded := newHiddenBytesCache(path)
	require.NoError(t, loaded.load())
	got, ok := loaded.get(img.ID)
	require.True(t, ok)
	assert.Equal(t, uint64(10), got[0].Bytes)
	assert.Zero(t, got[1].Bytes)
	got[0].Bytes = 999
	again, ok := loaded.get(img.ID)
	require.True(t, ok)
	assert.Equal(t, uint64(10), again[0].Bytes)
	stat, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), stat.Mode().Perm())
	files, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	assert.Len(t, files, 1, "atomic-write temporary file must be removed")
}

func TestHiddenBytesCacheLRUEviction(t *testing.T) {
	cache := newHiddenBytesCache("")
	results := hiddenBytesTestResult(hiddenBytesTestImage("fixture"))
	for i := 0; i < hiddenBytesCacheEntries; i++ {
		require.NoError(t, cache.put(digest.FromString(strconv.Itoa(i)).String(), results))
	}
	_, ok := cache.get(digest.FromString("0").String())
	require.True(t, ok)
	require.NoError(t, cache.put(digest.FromString("overflow").String(), results))
	assert.Len(t, cache.entries, hiddenBytesCacheEntries)
	_, ok = cache.get(digest.FromString("1").String())
	assert.False(t, ok)
	_, ok = cache.get(digest.FromString("0").String())
	assert.True(t, ok)
}

func TestHiddenBytesCacheByteBound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	cache := newHiddenBytesCache(path)
	results := make([]hiddenLayerResult, hiddenBytesCacheMaxLayers)
	for i := range results {
		results[i] = hiddenLayerResult{DiffID: digest.FromString(strconv.Itoa(i)).String(), Bytes: 42}
	}
	for i := 0; i < 8; i++ {
		require.NoError(t, cache.put(digest.FromString(strconv.Itoa(i)).String(), results))
	}
	stat, err := os.Stat(path)
	require.NoError(t, err)
	assert.LessOrEqual(t, stat.Size(), int64(hiddenBytesCacheMaxBytes))
	assert.Less(t, len(cache.entries), 8)
	loaded := newHiddenBytesCache(path)
	require.NoError(t, loaded.load())
	assert.Equal(t, cache.entries, loaded.entries)
}

func TestHiddenBytesCacheRejectsInvalidData(t *testing.T) {
	img := hiddenBytesTestImage("invalid")
	entry := hiddenBytesCacheEntry{ImageID: img.ID, Layers: hiddenBytesTestResult(img)}
	invalidVersion, err := json.Marshal(hiddenBytesCacheFile{Version: hiddenBytesAlgorithmVersion - 1, Entries: []hiddenBytesCacheEntry{entry}})
	require.NoError(t, err)
	duplicate, err := json.Marshal(hiddenBytesCacheFile{Version: hiddenBytesAlgorithmVersion, Entries: []hiddenBytesCacheEntry{entry, entry}})
	require.NoError(t, err)
	for name, data := range map[string][]byte{
		"truncated":          []byte(`{"version":1`),
		"oversized":          []byte(strings.Repeat(" ", hiddenBytesCacheMaxBytes+1)),
		"previous-algorithm": invalidVersion,
		"duplicate-image":    duplicate,
		"invalid-digest":     []byte(`{"version":` + strconv.Itoa(hiddenBytesAlgorithmVersion) + `,"entries":[{"image_id":"bad","layers":[{"diff_id":"bad","bytes":0}]}]}`),
		"negative-bytes":     []byte(`{"version":` + strconv.Itoa(hiddenBytesAlgorithmVersion) + `,"entries":[{"image_id":"bad","layers":[{"diff_id":"bad","bytes":-1}]}]}`),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cache.json")
			require.NoError(t, os.WriteFile(path, data, 0600))
			cache := newHiddenBytesCache(path)
			require.Error(t, cache.load())
			assert.Empty(t, cache.entries)
		})
	}
}

func TestHiddenBytesCacheWriteFailureStillCachesInMemory(t *testing.T) {
	path := t.TempDir() // Renaming a file over an existing directory must fail.
	cache := newHiddenBytesCache(path)
	img := hiddenBytesTestImage("disk-failure")
	require.Error(t, cache.put(img.ID, hiddenBytesTestResult(img)))
	results, ok := cache.get(img.ID)
	require.True(t, ok)
	assert.Equal(t, hiddenBytesTestResult(img), results)
}
